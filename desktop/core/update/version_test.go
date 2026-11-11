package update_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/desktop/core/update"
)

func TestSemVer_ParsingAndComparison(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int // -1 (v1 < v2), 0 (v1 == v2), 1 (v1 > v2)
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.10.0", "1.9.0", 1},
		{"v1.0.0", "1.0.0", 0},

		// Pre-release vs Release (Release has higher precedence!)
		{"1.0.0", "1.0.0-rc1", 1},
		{"1.0.0-rc1", "1.0.0", -1},

		// Pre-release comparisons
		{"1.0.0-rc2", "1.0.0-rc1", 1},
		{"1.0.0-rc1", "1.0.0-rc2", -1},
		{"1.0.0-beta.2", "1.0.0-beta.1", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha", 1},
	}

	for _, tc := range tests {
		cmp, err := update.CompareVersions(tc.v1, tc.v2)
		if err != nil {
			t.Fatalf("CompareVersions(%q, %q) returned unexpected error: %v", tc.v1, tc.v2, err)
		}
		if cmp != tc.expected {
			t.Errorf("CompareVersions(%q, %q): expected %d, got %d", tc.v1, tc.v2, tc.expected, cmp)
		}
	}
}

func TestAntiRollbackManager_Lifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-antirollback-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "anti_rollback.json")
	mgr := update.NewAntiRollbackManager(stateFile)

	// 1. Initial empty state allows any version
	initM := &update.Manifest{
		ReleaseVersion:  "1.0.0-rc1",
		SecurityVersion: 10,
	}
	if err := mgr.CheckUpdate(initM, false); err != nil {
		t.Fatalf("initial update check failed: %v", err)
	}

	// 2. Commit initial version
	if err := mgr.RecordAccepted(initM); err != nil {
		t.Fatalf("failed to record accepted update: %v", err)
	}

	st, _ := mgr.GetState()
	if st.HighestReleaseVersion != "1.0.0-rc1" || st.HighestSecurityVersion != 10 {
		t.Fatalf("unexpected state after initial record: %+v", st)
	}

	// 3. Higher version passes
	newerM := &update.Manifest{
		ReleaseVersion:  "1.0.0",
		SecurityVersion: 11,
	}
	if err := mgr.CheckUpdate(newerM, false); err != nil {
		t.Fatalf("newer update check failed: %v", err)
	}
	if err := mgr.RecordAccepted(newerM); err != nil {
		t.Fatalf("failed to record newer update: %v", err)
	}

	// 4. Downgrade attempt: lower release version rejected
	downgradeM := &update.Manifest{
		ReleaseVersion:  "1.0.0-rc1",
		SecurityVersion: 11,
	}
	if err := mgr.CheckUpdate(downgradeM, false); err == nil {
		t.Fatalf("expected error on SemVer downgrade attempt, got nil")
	}

	// 5. Downgrade attempt: lower security version rejected
	secDowngradeM := &update.Manifest{
		ReleaseVersion:  "1.0.1",
		SecurityVersion: 9, // lower than 11!
	}
	if err := mgr.CheckUpdate(secDowngradeM, false); err == nil {
		t.Fatalf("expected error on security version downgrade attempt, got nil")
	}

	// 6. Explicit allowDowngrade allows recovery
	if err := mgr.CheckUpdate(downgradeM, true); err != nil {
		t.Fatalf("allowDowngrade=true should permit authorized recovery: %v", err)
	}
}

