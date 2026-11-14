package cache_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/cache"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func TestCache_PersistenceAndIsolation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-cache-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cacheFile := filepath.Join(tmpDir, "blocklist_cache.json")
	c := cache.NewFileBlocklistCache(cacheFile)

	// 1. Initial get on empty cache returns nil
	initial, err := c.Get("user-alpha")
	if err != nil {
		t.Fatalf("unexpected error on empty cache: %v", err)
	}
	if initial != nil {
		t.Fatalf("expected nil for empty cache, got %+v", initial)
	}
	health := c.GetHealth()
	if health.Valid || health.DegradedReason != "no_cache_file_found" {
		t.Errorf("expected no_cache_file_found health, got %+v", health)
	}

	// 2. Save valid blocklist for user-alpha
	dataAlpha := &core.BlocklistData{
		UserID:      "user-alpha",
		Version:     "v1-1111222233334444",
		GeneratedAt: "2026-11-06T00:00:00Z",
		LastSyncAt:  "2026-11-06T00:00:01Z",
		Targets: []core.TargetItem{
			{
				GoalID:    "goal-ig",
				GoalTitle: "Stop Instagram",
				Status:    "active",
				Domains:   []string{"instagram.com"},
			},
		},
	}

	if err := c.Save(dataAlpha); err != nil {
		t.Fatalf("failed to save cache: %v", err)
	}

	// 3. Retrieve with matching user ID -> SUCCESS
	retrieved, err := c.Get("user-alpha")
	if err != nil {
		t.Fatalf("failed to get cache: %v", err)
	}
	if retrieved == nil || len(retrieved.Targets) != 1 {
		t.Fatalf("expected 1 target in cache, got %+v", retrieved)
	}
	if retrieved.Targets[0].Domains[0] != "instagram.com" {
		t.Errorf("expected domain instagram.com, got %s", retrieved.Targets[0].Domains[0])
	}
	if !c.GetHealth().Valid {
		t.Errorf("expected health to be valid")
	}

	// 4. Retrieve with DIFFERENT user ID (Account switching safety) -> Error ErrUserMismatch
	_, err = c.Get("user-beta")
	if err == nil || err != core.ErrUserMismatch {
		t.Fatalf("expected ErrUserMismatch for user-beta, got %v", err)
	}

	// 5. Test corruption recovery with LKG fallback
	// Corrupt the primary cache file
	_ = os.WriteFile(cacheFile, []byte("{ malformed json"), 0600)
	lkgGet, err := c.Get("user-alpha")
	if err != nil {
		t.Fatalf("unexpected error on LKG fallback: %v", err)
	}
	if lkgGet == nil || len(lkgGet.Targets) != 1 {
		t.Fatalf("expected LKG fallback to retrieve 1 target, got %+v", lkgGet)
	}
	lkgHealth := c.GetHealth()
	if !lkgHealth.IsLKG || !lkgHealth.Degraded {
		t.Errorf("expected degraded LKG health, got %+v", lkgHealth)
	}
	// Verify that the corrupt file was quarantined
	if _, err := os.Stat(cacheFile + ".corrupt"); err != nil {
		t.Errorf("expected corrupt file to be quarantined to .corrupt: %v", err)
	}

	// 6. Test corruption without LKG (both corrupt)
	_ = os.WriteFile(cacheFile, []byte("{ bad json 2"), 0600)
	_ = os.Remove(cacheFile + ".lkg")
	corruptGet, err := c.Get("user-alpha")
	if corruptGet != nil {
		t.Fatalf("expected nil when no LKG available, got %+v", corruptGet)
	}
	if !errors.Is(err, cache.ErrCacheCorrupt) {
		t.Fatalf("expected ErrCacheCorrupt, got %v", err)
	}
	noLkgHealth := c.GetHealth()
	if noLkgHealth.Valid || noLkgHealth.DegradedReason != "primary_cache_corrupt_quarantined_no_lkg" {
		t.Errorf("expected primary_cache_corrupt_quarantined_no_lkg, got %+v", noLkgHealth)
	}

	// 7. Test Clear
	if err := c.Clear(); err != nil {
		t.Fatalf("failed to clear cache: %v", err)
	}
	clearedGet, _ := c.Get("user-alpha")
	if clearedGet != nil {
		t.Fatalf("expected nil after clear, got %+v", clearedGet)
	}
}

func TestCache_SeamlessLegacyMigration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-migration-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create legacy blocklist_cache.json
	legacyPath := filepath.Join(tmpDir, "blocklist_cache.json")
	legacyData := &core.BlocklistData{
		UserID:      "migrated-user",
		Version:     "v1-migrated-etag-9999",
		GeneratedAt: "2026-11-11T00:00:00Z",
		LastSyncAt:  "2026-11-11T00:00:01Z",
		Targets: []core.TargetItem{
			{
				GoalID:    "goal-migrated",
				GoalTitle: "Migrated Goal",
				Status:    "active",
				Domains:   []string{"example.com"},
			},
		},
	}
	legacyCache := cache.NewFileBlocklistCache(legacyPath)
	if err := legacyCache.Save(legacyData); err != nil {
		t.Fatalf("failed to save legacy cache: %v", err)
	}

	// Now configure a new cache pointing to policy_cache.json in the same dir
	newPath := filepath.Join(tmpDir, "policy_cache.json")
	newCache := cache.NewFileBlocklistCache(newPath)

	// Fetch from new cache: should automatically migrate legacy cache!
	migrated, err := newCache.Get("migrated-user")
	if err != nil {
		t.Fatalf("failed to get from migrated cache: %v", err)
	}
	if migrated == nil || len(migrated.Targets) != 1 || migrated.Targets[0].Domains[0] != "example.com" {
		t.Fatalf("expected 1 migrated target, got %+v", migrated)
	}
	if !newCache.GetHealth().Migrated {
		t.Errorf("expected health.Migrated to be true, got %+v", newCache.GetHealth())
	}
	// Verify that policy_cache.json was written
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected policy_cache.json to exist after migration: %v", err)
	}
}

// TestCache_ReconciliationMatrix8Scenarios rigorously validates the complete 8-scenario
// policy-cache durability and migration matrix required by ONUSLY D7.2 Phase D.
func TestCache_ReconciliationMatrix8Scenarios(t *testing.T) {
	// Scenario 1: Valid primary + valid LKG (primary newer) -> primary loaded
	t.Run("Scenario1_ValidPrimary_ValidLKG_PrimaryNewer", func(t *testing.T) {
		tmpDir := t.TempDir()
		primaryPath := filepath.Join(tmpDir, "policy_cache.json")
		lkgPath := primaryPath + ".lkg"

		primaryData := &core.BlocklistData{
			UserID:      "user-1",
			Version:     "v2-20261111-2000",
			GeneratedAt: "2026-11-11T20:00:00Z",
			LastSyncAt:  "2026-11-11T20:00:01Z",
			Targets:     []core.TargetItem{{GoalID: "g1", Status: "active", Domains: []string{"primary.com"}}},
		}
		lkgData := &core.BlocklistData{
			UserID:      "user-1",
			Version:     "v1-20261111-1000",
			GeneratedAt: "2026-11-11T10:00:00Z",
			LastSyncAt:  "2026-11-11T10:00:01Z",
			Targets:     []core.TargetItem{{GoalID: "g1", Status: "active", Domains: []string{"lkg.com"}}},
		}

		c := cache.NewFileBlocklistCache(primaryPath)
		_ = c.Save(primaryData)
		// Manually overwrite LKG with older data
		lkgBytes, _ := json.Marshal(lkgData)
		_ = os.WriteFile(lkgPath, lkgBytes, 0600)

		res, err := c.Get("user-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Targets[0].Domains[0] != "primary.com" {
			t.Fatalf("expected primary to be loaded, got %s", res.Targets[0].Domains[0])
		}
		if c.GetHealth().Degraded || !c.GetHealth().Valid {
			t.Errorf("expected valid, non-degraded health")
		}
	})

	// Scenario 2: Corrupt primary + valid LKG -> LKG loaded, primary healed
	t.Run("Scenario2_CorruptPrimary_ValidLKG_HealsPrimary", func(t *testing.T) {
		tmpDir := t.TempDir()
		primaryPath := filepath.Join(tmpDir, "policy_cache.json")
		lkgPath := primaryPath + ".lkg"

		lkgData := &core.BlocklistData{
			UserID:      "user-1",
			Version:     "v1-lkg-saved",
			GeneratedAt: "2026-11-11T12:00:00Z",
			LastSyncAt:  "2026-11-11T12:00:01Z",
			Targets:     []core.TargetItem{{GoalID: "g2", Status: "active", Domains: []string{"healed.com"}}},
		}
		lkgBytes, _ := json.Marshal(lkgData)
		_ = os.WriteFile(lkgPath, lkgBytes, 0600)
		_ = os.WriteFile(primaryPath, []byte("<<< CORRUPT BINARY JUNK >>>"), 0600)

		c := cache.NewFileBlocklistCache(primaryPath)
		res, err := c.Get("user-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Targets[0].Domains[0] != "healed.com" {
			t.Fatalf("expected LKG target, got %s", res.Targets[0].Domains[0])
		}
		if !c.GetHealth().IsLKG || !c.GetHealth().Degraded {
			t.Errorf("expected degraded LKG health, got %+v", c.GetHealth())
		}
		// Verify primary was healed on disk
		healedBytes, err := os.ReadFile(primaryPath)
		if err != nil || len(healedBytes) == 0 {
			t.Fatalf("expected primary to be healed on disk: %v", err)
		}
		// Verify corrupt file quarantined
		if _, err := os.Stat(primaryPath + ".corrupt"); err != nil {
			t.Fatalf("expected .corrupt quarantine file to exist: %v", err)
		}
	})

	// Scenario 3: Valid primary + corrupt LKG -> primary loaded, LKG updated/healed
	t.Run("Scenario3_ValidPrimary_CorruptLKG_HealsLKG", func(t *testing.T) {
		tmpDir := t.TempDir()
		primaryPath := filepath.Join(tmpDir, "policy_cache.json")
		lkgPath := primaryPath + ".lkg"

		primaryData := &core.BlocklistData{
			UserID:      "user-1",
			Version:     "v1-valid-primary",
			GeneratedAt: "2026-11-11T15:00:00Z",
			LastSyncAt:  "2026-11-11T15:00:01Z",
			Targets:     []core.TargetItem{{GoalID: "g3", Status: "active", Domains: []string{"lkgheal.com"}}},
		}
		c := cache.NewFileBlocklistCache(primaryPath)
		_ = c.Save(primaryData)

		// Intentionally corrupt LKG
		_ = os.WriteFile(lkgPath, []byte("### CORRUPT LKG ###"), 0600)

		res, err := c.Get("user-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Targets[0].Domains[0] != "lkgheal.com" {
			t.Fatalf("expected primary target, got %s", res.Targets[0].Domains[0])
		}
		// Verify LKG was healed with primary data
		lkgBytes, err := os.ReadFile(lkgPath)
		if err != nil {
			t.Fatalf("expected LKG to exist and be healed: %v", err)
		}
		var healedLKG core.BlocklistData
		if err := json.Unmarshal(lkgBytes, &healedLKG); err != nil || healedLKG.Version != "v1-valid-primary" {
			t.Fatalf("expected LKG to be valid JSON with primary version, got err: %v", err)
		}
	})

	// Scenario 4: Missing primary + valid legacy blocklist -> migrated to primary + LKG
	t.Run("Scenario4_MissingPrimary_ValidLegacy_Migrated", func(t *testing.T) {
		tmpDir := t.TempDir()
		primaryPath := filepath.Join(tmpDir, "policy_cache.json")
		legacyPath := filepath.Join(tmpDir, "blocklist_cache.json")

		legacyData := &core.BlocklistData{
			UserID:      "user-legacy",
			Version:     "v1-legacy-migrated",
			GeneratedAt: "2026-11-11T16:00:00Z",
			LastSyncAt:  "2026-11-11T16:00:01Z",
			Targets:     []core.TargetItem{{GoalID: "g4", Status: "active", Domains: []string{"migrated4.com"}}},
		}
		legacyBytes, _ := json.Marshal(legacyData)
		_ = os.WriteFile(legacyPath, legacyBytes, 0600)

		c := cache.NewFileBlocklistCache(primaryPath)
		res, err := c.Get("user-legacy")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Targets[0].Domains[0] != "migrated4.com" {
			t.Fatalf("expected migrated target, got %s", res.Targets[0].Domains[0])
		}
		if !c.GetHealth().Migrated {
			t.Errorf("expected health.Migrated to be true")
		}
		// Verify primary and LKG exist
		if _, err := os.Stat(primaryPath); err != nil {
			t.Fatalf("expected primary to exist: %v", err)
		}
		if _, err := os.Stat(primaryPath + ".lkg"); err != nil {
			t.Fatalf("expected LKG to exist: %v", err)
		}
		// Verify legacy was archived
		if _, err := os.Stat(legacyPath + ".archived"); err != nil {
			t.Fatalf("expected legacy to be archived: %v", err)
		}
	})

	// Scenario 5: Primary exists (newer) + legacy exists (older) -> primary retained, legacy archived
	t.Run("Scenario5_PrimaryNewer_LegacyOlder_PrimaryRetained", func(t *testing.T) {
		tmpDir := t.TempDir()
		primaryPath := filepath.Join(tmpDir, "policy_cache.json")
		legacyPath := filepath.Join(tmpDir, "blocklist_cache.json")

		primaryData := &core.BlocklistData{
			UserID:      "user-5",
			Version:     "v2-newer",
			GeneratedAt: "2026-11-11T18:00:00Z",
			LastSyncAt:  "2026-11-11T18:00:01Z",
			Targets:     []core.TargetItem{{GoalID: "g5", Status: "active", Domains: []string{"newer-primary.com"}}},
		}
		legacyData := &core.BlocklistData{
			UserID:      "user-5",
			Version:     "v1-older",
			GeneratedAt: "2026-11-11T12:00:00Z",
			LastSyncAt:  "2026-11-11T12:00:01Z",
			Targets:     []core.TargetItem{{GoalID: "g5", Status: "active", Domains: []string{"older-legacy.com"}}},
		}

		c := cache.NewFileBlocklistCache(primaryPath)
		_ = c.Save(primaryData)
		legacyBytes, _ := json.Marshal(legacyData)
		_ = os.WriteFile(legacyPath, legacyBytes, 0600)

		res, err := c.Get("user-5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Targets[0].Domains[0] != "newer-primary.com" {
			t.Fatalf("expected newer primary to be retained, got %s", res.Targets[0].Domains[0])
		}
		if _, err := os.Stat(legacyPath + ".archived"); err != nil {
			t.Fatalf("expected older legacy to be archived: %v", err)
		}
	})

	// Scenario 6: Primary exists (older) + legacy exists (newer) -> newer legacy migrated
	t.Run("Scenario6_PrimaryOlder_LegacyNewer_LegacyMigrated", func(t *testing.T) {
		tmpDir := t.TempDir()
		primaryPath := filepath.Join(tmpDir, "policy_cache.json")
		legacyPath := filepath.Join(tmpDir, "blocklist_cache.json")

		primaryData := &core.BlocklistData{
			UserID:      "user-6",
			Version:     "v1-older",
			GeneratedAt: "2026-11-11T10:00:00Z",
			LastSyncAt:  "2026-11-11T10:00:01Z",
			Targets:     []core.TargetItem{{GoalID: "g6", Status: "active", Domains: []string{"older-primary.com"}}},
		}
		legacyData := &core.BlocklistData{
			UserID:      "user-6",
			Version:     "v2-newer",
			GeneratedAt: "2026-11-11T20:00:00Z",
			LastSyncAt:  "2026-11-11T20:00:01Z",
			Targets:     []core.TargetItem{{GoalID: "g6", Status: "active", Domains: []string{"newer-legacy.com"}}},
		}

		c := cache.NewFileBlocklistCache(primaryPath)
		_ = c.Save(primaryData)
		legacyBytes, _ := json.Marshal(legacyData)
		_ = os.WriteFile(legacyPath, legacyBytes, 0600)

		res, err := c.Get("user-6")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Targets[0].Domains[0] != "newer-legacy.com" {
			t.Fatalf("expected newer legacy to be migrated, got %s", res.Targets[0].Domains[0])
		}
		if !c.GetHealth().Migrated {
			t.Errorf("expected health.Migrated to be true")
		}
		if _, err := os.Stat(legacyPath + ".archived"); err != nil {
			t.Fatalf("expected legacy to be archived after migration: %v", err)
		}
	})

	// Scenario 7: Schema version mismatch (different version tags) -> deterministic selection
	t.Run("Scenario7_SchemaVersionMismatch_DeterministicSelection", func(t *testing.T) {
		tmpDir := t.TempDir()
		primaryPath := filepath.Join(tmpDir, "policy_cache.json")
		legacyPath := filepath.Join(tmpDir, "blocklist_cache.json")

		primaryData := &core.BlocklistData{
			UserID:      "user-7",
			Version:     "v1-schema",
			GeneratedAt: "2026-11-11T11:00:00Z",
			LastSyncAt:  "2026-11-11T11:00:00Z",
			Targets:     []core.TargetItem{{GoalID: "g7", Status: "active", Domains: []string{"v1-tag.com"}}},
		}
		legacyData := &core.BlocklistData{
			UserID:      "user-7",
			Version:     "v2-schema",
			GeneratedAt: "2026-11-11T14:00:00Z",
			LastSyncAt:  "2026-11-11T14:00:00Z",
			Targets:     []core.TargetItem{{GoalID: "g7", Status: "active", Domains: []string{"v2-tag.com"}}},
		}

		c := cache.NewFileBlocklistCache(primaryPath)
		_ = c.Save(primaryData)
		legacyBytes, _ := json.Marshal(legacyData)
		_ = os.WriteFile(legacyPath, legacyBytes, 0600)

		res, err := c.Get("user-7")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// v2 is newer by timestamp and version
		if res.Targets[0].Domains[0] != "v2-tag.com" {
			t.Fatalf("expected deterministic newer schema v2-tag.com, got %s", res.Targets[0].Domains[0])
		}
	})

	// Scenario 8: Corrupt primary + corrupt LKG + corrupt legacy -> safe empty state, degraded health
	t.Run("Scenario8_AllCorrupt_SafeEmptyState_DegradedHealth", func(t *testing.T) {
		tmpDir := t.TempDir()
		primaryPath := filepath.Join(tmpDir, "policy_cache.json")
		lkgPath := primaryPath + ".lkg"
		legacyPath := filepath.Join(tmpDir, "blocklist_cache.json")

		_ = os.WriteFile(primaryPath, []byte("{ corrupted primary"), 0600)
		_ = os.WriteFile(lkgPath, []byte("{ corrupted lkg"), 0600)
		_ = os.WriteFile(legacyPath, []byte("{ corrupted legacy"), 0600)

		c := cache.NewFileBlocklistCache(primaryPath)
		res, err := c.Get("user-8")
		if res != nil {
			t.Fatalf("expected nil data when all are corrupt, got %+v", res)
		}
		if !errors.Is(err, cache.ErrCacheCorrupt) {
			t.Fatalf("expected ErrCacheCorrupt, got %v", err)
		}
		health := c.GetHealth()
		if health.Valid || !health.Degraded {
			t.Errorf("expected invalid, degraded health, got %+v", health)
		}
		// Verify corrupt files were quarantined
		if _, err := os.Stat(primaryPath + ".corrupt"); err != nil {
			t.Errorf("expected primary .corrupt to exist: %v", err)
		}
		if _, err := os.Stat(lkgPath + ".corrupt"); err != nil {
			t.Errorf("expected lkg .corrupt to exist: %v", err)
		}
		if _, err := os.Stat(legacyPath + ".corrupt"); err != nil {
			t.Errorf("expected legacy .corrupt to exist: %v", err)
		}
	})
}

func TestCache_ClockRollbackDetection(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "policy_cache.json")
	c := cache.NewFileBlocklistCache(cacheFile)

	// Case 1: Normal LastSyncAt (10 minutes in the past) -> No ClockTampered
	normalSync := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	dataNormal := &core.BlocklistData{
		UserID:      "user-clock-1",
		Version:     "v1-normal",
		GeneratedAt: normalSync,
		LastSyncAt:  normalSync,
		Targets: []core.TargetItem{
			{
				GoalID:    "g-1",
				GoalTitle: "No Tamper",
				Status:    "active",
				Domains:   []string{"distraction.com"},
			},
		},
	}
	if err := c.Save(dataNormal); err != nil {
		t.Fatalf("failed to save cache: %v", err)
	}

	res, err := c.Get("user-clock-1")
	if err != nil || res == nil {
		t.Fatalf("failed to get cache: %v", err)
	}
	health := c.GetHealth()
	if health.ClockTampered {
		t.Errorf("expected ClockTampered=false for normal sync time, got true")
	}

	// Case 2: LastSyncAt is 2 hours in the future (simulating user turned system clock back 2 hours)
	futureSync := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	dataFuture := &core.BlocklistData{
		UserID:      "user-clock-1",
		Version:     "v2-tampered",
		GeneratedAt: futureSync,
		LastSyncAt:  futureSync,
		Targets: []core.TargetItem{
			{
				GoalID:    "g-1",
				GoalTitle: "Tampered Clock",
				Status:    "active",
				Domains:   []string{"distraction.com"},
			},
		},
	}
	if err := c.Save(dataFuture); err != nil {
		t.Fatalf("failed to save future cache: %v", err)
	}

	resTampered, err := c.Get("user-clock-1")
	if err != nil || resTampered == nil {
		t.Fatalf("failed to get tampered cache: %v", err)
	}
	healthTampered := c.GetHealth()
	if !healthTampered.ClockTampered {
		t.Errorf("expected ClockTampered=true when system clock is rolled back, got false")
	}
	if !healthTampered.Degraded || healthTampered.DegradedReason != "system_clock_rollback_detected" {
		t.Errorf("expected degraded reason 'system_clock_rollback_detected', got %+v", healthTampered)
	}
}
