package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

type mockSyncerForPairing struct {
	syncErr   error
	blocklist *core.BlocklistData
}

func (m *mockSyncerForPairing) Sync(ctx context.Context) (*core.BlocklistData, error) {
	return m.blocklist, m.syncErr
}

func (m *mockSyncerForPairing) GetCachedBlocklist() (*core.BlocklistData, error) {
	return m.blocklist, nil
}

type mockCredStoreForPairing struct {
	token string
}

func (m *mockCredStoreForPairing) SaveToken(token string) error {
	m.token = token
	return nil
}

func (m *mockCredStoreForPairing) GetToken() (string, error) {
	return m.token, nil
}

func (m *mockCredStoreForPairing) ClearToken() error {
	m.token = ""
	return nil
}

func TestPairingStateManager_LifecycleTransitions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-pairing-state-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "pairing_state.json")
	mgr := NewPairingStateManager(stateFile)

	// 1. Initial state must be UNINSTALLED
	initSt, err := mgr.GetState()
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if initSt.Status != StatusUninstalled {
		t.Fatalf("Expected UNINSTALLED, got %s", initSt.Status)
	}

	testOwnerSID := "S-1-5-21-929185929-963631574-767379863-1001"

	// 2. UNINSTALLED -> INSTALLED_UNPAIRED
	st1, err := mgr.Transition(StatusInstalledUnpaired, testOwnerSID)
	if err != nil {
		t.Fatalf("Transition to INSTALLED_UNPAIRED failed: %v", err)
	}
	if st1.Status != StatusInstalledUnpaired || st1.OwnerSID != testOwnerSID || st1.Generation != 1 {
		t.Fatalf("Invalid state st1: %+v", st1)
	}

	// 3. INSTALLED_UNPAIRED -> PAIRING_PENDING
	st2, err := mgr.Transition(StatusPairingPending, testOwnerSID)
	if err != nil {
		t.Fatalf("Transition to PAIRING_PENDING failed: %v", err)
	}
	if st2.Status != StatusPairingPending || st2.Generation != 2 {
		t.Fatalf("Invalid state st2: %+v", st2)
	}

	// 4. Invalid transition directly to PAIRED must fail
	_, err = mgr.Transition(StatusPaired, testOwnerSID)
	if err == nil {
		t.Fatalf("Expected error for illegal transition PAIRING_PENDING -> PAIRED, got nil")
	}

	// 5. PAIRING_PENDING -> PAIRING_COMMIT_PENDING
	st3, err := mgr.Transition(StatusPairingCommitPending, testOwnerSID)
	if err != nil {
		t.Fatalf("Transition to PAIRING_COMMIT_PENDING failed: %v", err)
	}
	if st3.Status != StatusPairingCommitPending {
		t.Fatalf("Expected PAIRING_COMMIT_PENDING, got %s", st3.Status)
	}

	// 6. PAIRING_COMMIT_PENDING -> PAIRED
	st4, err := mgr.Transition(StatusPaired, testOwnerSID)
	if err != nil {
		t.Fatalf("Transition to PAIRED failed: %v", err)
	}
	if st4.Status != StatusPaired || st4.PairedAt == "" || st4.LastValidatedAt == "" {
		t.Fatalf("Invalid PAIRED state: %+v", st4)
	}

	// 7. Prevent changing OwnerSID during active lifecycle
	diffSID := "S-1-5-21-000000000-000000000-000000000-9999"
	_, err = mgr.Transition(StatusRepairRequired, diffSID)
	if err == nil {
		t.Fatalf("Expected error when attempting to silently change OwnerSID, got nil")
	}

	// 8. PAIRED -> AUTHENTICATION_REQUIRED
	stAuth, err := mgr.Transition(StatusAuthRequired, testOwnerSID)
	if err != nil {
		t.Fatalf("Transition to AUTHENTICATION_REQUIRED failed: %v", err)
	}
	if stAuth.Status != StatusAuthRequired {
		t.Fatalf("Expected AUTHENTICATION_REQUIRED, got %s", stAuth.Status)
	}

	// 9. AUTHENTICATION_REQUIRED -> PAIRING_PENDING
	stReauth, err := mgr.Transition(StatusPairingPending, testOwnerSID)
	if err != nil {
		t.Fatalf("Transition to PAIRING_PENDING failed: %v", err)
	}
	if stReauth.Status != StatusPairingPending {
		t.Fatalf("Expected PAIRING_PENDING, got %s", stReauth.Status)
	}

	// 10. PAIRING_PENDING -> REPAIR_REQUIRED
	st5, err := mgr.Transition(StatusRepairRequired, testOwnerSID)
	if err != nil {
		t.Fatalf("Transition to REPAIR_REQUIRED failed: %v", err)
	}
	if st5.Status != StatusRepairRequired {
		t.Fatalf("Expected REPAIR_REQUIRED, got %s", st5.Status)
	}
}

func TestPairingStateManager_RecoveryInterruptedCommit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-recovery-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "pairing_state.json")
	mgr := NewPairingStateManager(stateFile)

	testOwnerSID := "S-1-5-21-12345"
	_, _ = mgr.Transition(StatusInstalledUnpaired, testOwnerSID)
	_, _ = mgr.Transition(StatusPairingPending, testOwnerSID)
	_, _ = mgr.Transition(StatusPairingCommitPending, testOwnerSID)

	credStore := &mockCredStoreForPairing{token: "valid-jwt"}
	syncer := &mockSyncerForPairing{
		blocklist: &core.BlocklistData{
			UserID:  "test-user",
			Version: "v1",
		},
	}

	// Recovery with valid token and healthy backend -> promotes to PAIRED
	recovered, err := mgr.Recover(context.Background(), credStore, syncer)
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}
	if recovered.Status != StatusPaired {
		t.Fatalf("Expected recovered state PAIRED, got %s", recovered.Status)
	}

	// Recovery with missing token -> transitions to REPAIR_REQUIRED
	credStore.token = ""
	recoveredAfterLoss, err := mgr.Recover(context.Background(), credStore, syncer)
	if err != nil {
		t.Fatalf("Recover after loss failed: %v", err)
	}
	if recoveredAfterLoss.Status != StatusRepairRequired {
		t.Fatalf("Expected REPAIR_REQUIRED when token missing, got %s", recoveredAfterLoss.Status)
	}
}

func TestPairingStateManager_RevocationAndRepair(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-revocation-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "pairing_state.json")
	mgr := NewPairingStateManager(stateFile)

	testOwnerSID := "S-1-5-21-998877"

	// 1. Establish initial PAIRED state
	_, _ = mgr.Transition(StatusInstalledUnpaired, testOwnerSID)
	_, _ = mgr.Transition(StatusPairingPending, testOwnerSID)
	_, _ = mgr.Transition(StatusPairingCommitPending, testOwnerSID)
	pairedSt, err := mgr.Transition(StatusPaired, testOwnerSID)
	if err != nil || pairedSt.Status != StatusPaired {
		t.Fatalf("Failed to establish PAIRED state: %v", err)
	}

	// 2. Transition PAIRED -> REVOKED
	revSt, err := mgr.Transition(StatusRevoked, testOwnerSID)
	if err != nil {
		t.Fatalf("Transition to REVOKED failed: %v", err)
	}
	if revSt.Status != StatusRevoked {
		t.Fatalf("Expected status REVOKED, got: %s", revSt.Status)
	}

	// 3. Verify Recover() leaves REVOKED intact (does not promote or change)
	credStore := &mockCredStoreForPairing{token: "stale-jwt"}
	syncer := &mockSyncerForPairing{}
	recSt, err := mgr.Recover(context.Background(), credStore, syncer)
	if err != nil {
		t.Fatalf("Recover() failed on revoked state: %v", err)
	}
	if recSt.Status != StatusRevoked {
		t.Fatalf("Recover() on REVOKED must remain REVOKED, got: %s", recSt.Status)
	}

	// 4. Re-pairing: REVOKED -> PAIRING_PENDING
	pendingSt, err := mgr.Transition(StatusPairingPending, testOwnerSID)
	if err != nil {
		t.Fatalf("Transition from REVOKED to PAIRING_PENDING failed: %v", err)
	}
	if pendingSt.Status != StatusPairingPending {
		t.Fatalf("Expected PAIRING_PENDING, got: %s", pendingSt.Status)
	}

	// 5. Complete re-pairing cycle -> PAIRING_COMMIT_PENDING -> PAIRED
	commitSt, err := mgr.Transition(StatusPairingCommitPending, testOwnerSID)
	if err != nil || commitSt.Status != StatusPairingCommitPending {
		t.Fatalf("Transition to PAIRING_COMMIT_PENDING failed: %v", err)
	}

	repairedSt, err := mgr.Transition(StatusPaired, testOwnerSID)
	if err != nil || repairedSt.Status != StatusPaired {
		t.Fatalf("Transition to PAIRED failed: %v", err)
	}
	if repairedSt.OwnerSID != testOwnerSID {
		t.Errorf("OwnerSID must be preserved across re-pairing: expected %s, got %s", testOwnerSID, repairedSt.OwnerSID)
	}
}

