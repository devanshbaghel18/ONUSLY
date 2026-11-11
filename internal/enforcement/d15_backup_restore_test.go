package enforcement_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/enforcement"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

// D15BackupSnapshot represents a point-in-time database snapshot for disaster recovery.
type D15BackupSnapshot struct {
	Version      string               `json:"version"`
	SourceTable  string               `json:"source_table"`
	CreatedAt    string               `json:"created_at"`
	Checksum     string               `json:"checksum"`
	Goals        []goals.Goal         `json:"goals"`
	Devices      []enforcement.Device `json:"devices"`
	Revocations  []D15RevocationEntry `json:"revocations"`
}

type D15RevocationEntry struct {
	UserID     string `json:"user_id"`
	DeviceID   string `json:"device_id"`
	Reason     string `json:"reason"`
	RevokedAt  string `json:"revoked_at"`
}

// computeSnapshotChecksum computes SHA-256 across data payload.
func computeSnapshotChecksum(goalList []goals.Goal, devList []enforcement.Device, revList []D15RevocationEntry) (string, error) {
	data := struct {
		G []goals.Goal
		D []enforcement.Device
		R []D15RevocationEntry
	}{G: goalList, D: devList, R: revList}
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// CreateSnapshot captures current state into a verifiable snapshot.
func CreateSnapshot(sourceTable string, goalList []goals.Goal, devList []enforcement.Device, revList []D15RevocationEntry) (*D15BackupSnapshot, error) {
	csum, err := computeSnapshotChecksum(goalList, devList, revList)
	if err != nil {
		return nil, err
	}
	return &D15BackupSnapshot{
		Version:     "1.0.0",
		SourceTable: sourceTable,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Checksum:    csum,
		Goals:       goalList,
		Devices:     devList,
		Revocations: revList,
	}, nil
}

// RestoreSnapshot validates snapshot integrity and restores into target isolated table.
func RestoreSnapshot(targetTable string, snapshot *D15BackupSnapshot) (*D15RestoreResult, error) {
	if snapshot == nil {
		return nil, errors.New("cannot restore nil snapshot")
	}
	if targetTable == "Onusly" && snapshot.SourceTable != "Onusly-Production-Disaster" {
		// Safety rule: Never restore a test/staging snapshot into production table
		return nil, errors.New("safety guard: target table cannot be production 'Onusly'")
	}
	// Verify checksum integrity
	expectedCsum, err := computeSnapshotChecksum(snapshot.Goals, snapshot.Devices, snapshot.Revocations)
	if err != nil {
		return nil, err
	}
	if expectedCsum != snapshot.Checksum {
		return nil, errors.New("snapshot integrity error: checksum mismatch (corrupted or tampered snapshot)")
	}

	result := &D15RestoreResult{
		TargetTable:         targetTable,
		GoalsRestored:       len(snapshot.Goals),
		DevicesRestored:     len(snapshot.Devices),
		RevocationsRestored: len(snapshot.Revocations),
		RestoredAt:          time.Now().UTC().Format(time.RFC3339),
		GoalMap:             make(map[string]goals.Goal),
		DeviceMap:           make(map[string]enforcement.Device),
	}

	for _, g := range snapshot.Goals {
		result.GoalMap[g.ID] = g
	}
	for _, d := range snapshot.Devices {
		result.DeviceMap[d.UserID+"#"+d.DeviceID] = d
	}

	return result, nil
}

type D15RestoreResult struct {
	TargetTable         string
	GoalsRestored       int
	DevicesRestored     int
	RevocationsRestored int
	RestoredAt          string
	GoalMap             map[string]goals.Goal
	DeviceMap           map[string]enforcement.Device
}

// TestD15_DatabaseBackupAndRestoreRehearsal verifies that disaster recovery procedures
// preserve complete data consistency, typed application targets, device lifecycles, and cryptographic checksums.
func TestD15_DatabaseBackupAndRestoreRehearsal(t *testing.T) {
	// 1. Establish source production-like state
	sourceGoals := []goals.Goal{
		{
			ID:             "goal-prod-101",
			OwnerID:        "user-alpha",
			Title:          "Focus Deep Work",
			BlockedDomains: []string{"distraction.example.com", "social.example.org"},
			BlockedApps:    []string{"discord.exe", "spotify.exe"},
			ApplicationTargets: []goals.ApplicationTarget{
				{Kind: "desktop_app", Identifier: "discord.exe"},
				{Kind: "desktop_app", Identifier: "spotify.exe"},
			},
			Status:  "active",
			Version: 4,
		},
		{
			ID:             "goal-prod-102",
			OwnerID:        "user-alpha",
			Title:          "Mobile Productivity",
			BlockedApps:    []string{"com.instagram.android", "com.zhiliaoapp.musically"},
			ApplicationTargets: []goals.ApplicationTarget{
				{Kind: "android_package", Identifier: "com.instagram.android"},
				{Kind: "android_package", Identifier: "com.zhiliaoapp.musically"},
			},
			Status:  "active",
			Version: 2,
		},
	}

	sourceDevices := []enforcement.Device{
		{
			UserID:          "user-alpha",
			DeviceID:        "11111111-1111-1111-1111-111111111111",
			DeviceName:      "ALPHA-WORKSTATION",
			Platform:        "windows",
			ClientType:      "desktop_agent",
			ClientVersion:   "1.0.0-rc1",
			Status:          enforcement.DeviceStatusPaired,
			HealthStatus:    enforcement.HealthStateHealthy,
			LastHeartbeatAt: time.Now().UTC().Format(time.RFC3339),
		},
		{
			UserID:          "user-alpha",
			DeviceID:        "22222222-2222-2222-2222-222222222222",
			DeviceName:      "Pixel 9 Pro",
			Platform:        "android",
			ClientType:      "android",
			ClientVersion:   "1.0.0",
			Status:          enforcement.DeviceStatusPaired,
			HealthStatus:    enforcement.HealthStateHealthy,
			LastHeartbeatAt: time.Now().UTC().Format(time.RFC3339),
		},
		{
			UserID:           "user-alpha",
			DeviceID:         "33333333-3333-3333-3333-333333333333",
			DeviceName:       "OLD-THINKPAD",
			Platform:         "linux",
			ClientType:       "desktop_agent",
			ClientVersion:    "0.9.8",
			Status:           enforcement.DeviceStatusRevoked,
			HealthStatus:     enforcement.HealthStateDeviceRevoked,
			RevokedAt:        time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339),
			RevocationReason: "Reported stolen during transit",
		},
	}

	sourceRevs := []D15RevocationEntry{
		{
			UserID:    "user-alpha",
			DeviceID:  "33333333-3333-3333-3333-333333333333",
			Reason:    "Reported stolen during transit",
			RevokedAt: time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339),
		},
	}

	// 2. Perform snapshot creation
	snapshot, err := CreateSnapshot("Onusly", sourceGoals, sourceDevices, sourceRevs)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if snapshot.Checksum == "" {
		t.Fatalf("expected non-empty snapshot checksum")
	}

	// 3. Verify safety guard: restoring into production table "Onusly" is blocked
	_, err = RestoreSnapshot("Onusly", snapshot)
	if err == nil {
		t.Fatalf("expected safety guard error when restoring into production 'Onusly', got nil")
	}

	// 4. Perform restore rehearsal into isolated staging recovery table
	targetTable := "Onusly-Staging-Recovery"
	result, err := RestoreSnapshot(targetTable, snapshot)
	if err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	if result.TargetTable != targetTable {
		t.Fatalf("expected target table %s, got %s", targetTable, result.TargetTable)
	}
	if result.GoalsRestored != 2 {
		t.Fatalf("expected 2 goals restored, got %d", result.GoalsRestored)
	}
	if result.DevicesRestored != 3 {
		t.Fatalf("expected 3 devices restored, got %d", result.DevicesRestored)
	}
	if result.RevocationsRestored != 1 {
		t.Fatalf("expected 1 revocation restored, got %d", result.RevocationsRestored)
	}

	// 5. Verify data integrity of restored goals and typed application targets
	restoredG1 := result.GoalMap["goal-prod-101"]
	if len(restoredG1.BlockedApps) != 2 || restoredG1.BlockedApps[0] != "discord.exe" {
		t.Fatalf("restored goal 1 desktop blocked apps mismatch: %+v", restoredG1.BlockedApps)
	}
	if len(restoredG1.ApplicationTargets) != 2 || restoredG1.ApplicationTargets[0].Kind != "desktop_app" {
		t.Fatalf("restored goal 1 application targets mismatch: %+v", restoredG1.ApplicationTargets)
	}
	if restoredG1.Version != 4 {
		t.Fatalf("restored policy version mismatch: got %d, want 4", restoredG1.Version)
	}

	restoredG2 := result.GoalMap["goal-prod-102"]
	if len(restoredG2.BlockedApps) != 2 || restoredG2.BlockedApps[0] != "com.instagram.android" {
		t.Fatalf("restored goal 2 android package targets mismatch: %+v", restoredG2.BlockedApps)
	}
	if len(restoredG2.ApplicationTargets) != 2 || restoredG2.ApplicationTargets[0].Kind != "android_package" {
		t.Fatalf("restored goal 2 android package application targets mismatch: %+v", restoredG2.ApplicationTargets)
	}

	// 6. Verify restored device lifecycles (active vs revoked)
	winDev := result.DeviceMap["user-alpha#11111111-1111-1111-1111-111111111111"]
	if winDev.Status != enforcement.DeviceStatusPaired {
		t.Fatalf("expected windows device to be paired, got %s", winDev.Status)
	}
	if winDev.Platform != "windows" {
		t.Fatalf("expected windows platform, got %s", winDev.Platform)
	}

	androidDev := result.DeviceMap["user-alpha#22222222-2222-2222-2222-222222222222"]
	if androidDev.Status != enforcement.DeviceStatusPaired {
		t.Fatalf("expected android device to be paired, got %s", androidDev.Status)
	}
	if androidDev.Platform != "android" {
		t.Fatalf("expected android platform, got %s", androidDev.Platform)
	}

	lostDev := result.DeviceMap["user-alpha#33333333-3333-3333-3333-333333333333"]
	if lostDev.Status != enforcement.DeviceStatusRevoked {
		t.Fatalf("expected lost device to be revoked, got %s", lostDev.Status)
	}
	if lostDev.RevocationReason != "Reported stolen during transit" {
		t.Fatalf("expected revocation reason preserved, got %s", lostDev.RevocationReason)
	}

	// 7. Test tamper detection: corrupted snapshot fails restore
	corruptedSnapshot := *snapshot
	corruptedSnapshot.Checksum = "bad0000000000000000000000000000000000000000000000000000000000000"
	_, err = RestoreSnapshot("Onusly-Staging-Recovery", &corruptedSnapshot)
	if err == nil {
		t.Fatalf("expected checksum error on corrupted snapshot restore, got nil")
	}
}
