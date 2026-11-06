package cache_test

import (
	"os"
	"path/filepath"
	"testing"

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

	// 4. Retrieve with DIFFERENT user ID (Account switching safety) -> Error ErrUserMismatch
	_, err = c.Get("user-beta")
	if err == nil || err != core.ErrUserMismatch {
		t.Fatalf("expected ErrUserMismatch for user-beta, got %v", err)
	}

	// 5. Test corruption recovery
	_ = os.WriteFile(cacheFile, []byte("{ malformed json"), 0600)
	corruptGet, err := c.Get("user-alpha")
	if corruptGet != nil {
		t.Fatalf("expected nil on corrupt cache, got %+v", corruptGet)
	}

	// 6. Test Clear
	if err := c.Clear(); err != nil {
		t.Fatalf("failed to clear cache: %v", err)
	}
	clearedGet, _ := c.Get("user-alpha")
	if clearedGet != nil {
		t.Fatalf("expected nil after clear, got %+v", clearedGet)
	}
}
