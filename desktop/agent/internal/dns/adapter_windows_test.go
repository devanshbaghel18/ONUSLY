//go:build windows

package dns

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsAdapterManager_MockLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	backupPath := filepath.Join(tempDir, "dns_backup.json")

	mgr := NewWindowsAdapterManager()

	initialAdapters := []AdapterDNSConfig{
		{
			InterfaceID:     "10",
			InterfaceAlias:  "Ethernet0",
			InterfaceIndex:  10,
			AddressFamily:   2,
			IsDHCP:          true,
			ServerAddresses: []string{},
			IsPhysical:      true,
			Status:          "Up",
		},
		{
			InterfaceID:     "12",
			InterfaceAlias:  "Wi-Fi",
			InterfaceIndex:  12,
			AddressFamily:   2,
			IsDHCP:          false,
			ServerAddresses: []string{"1.1.1.1", "1.0.0.1"},
			IsPhysical:      true,
			Status:          "Up",
		},
	}

	mgr.SetMockAdapters(initialAdapters)

	// Step 1: Backup
	backupState, err := mgr.BackupDNS(backupPath)
	if err != nil {
		t.Fatalf("BackupDNS failed: %v", err)
	}
	if len(backupState.OriginalDns) != 2 {
		t.Fatalf("Expected 2 backed up adapters, got %d", len(backupState.OriginalDns))
	}
	if backupState.IsActive {
		t.Errorf("Expected IsActive to be false initially")
	}

	// Step 2: Apply Loopback DNS
	if err := mgr.ApplyLoopbackDNS("127.0.0.1", "::1", backupPath); err != nil {
		t.Fatalf("ApplyLoopbackDNS failed: %v", err)
	}

	active, err := mgr.IsRedirectionActive(backupPath)
	if err != nil {
		t.Fatalf("IsRedirectionActive failed: %v", err)
	}
	if !active {
		t.Errorf("Expected redirection to be active")
	}

	// Verify mock adapters have loopback
	currentAdapters, _ := mgr.DiscoverAdapters()
	for _, a := range currentAdapters {
		if len(a.ServerAddresses) != 1 || a.ServerAddresses[0] != "127.0.0.1" {
			t.Errorf("Adapter %s did not have loopback DNS applied: %v", a.InterfaceAlias, a.ServerAddresses)
		}
	}

	// Step 3: Restore DNS (Fail-Open)
	if err := mgr.RestoreDNS(backupPath); err != nil {
		t.Fatalf("RestoreDNS failed: %v", err)
	}

	activeAfter, err := mgr.IsRedirectionActive(backupPath)
	if err != nil {
		t.Fatalf("IsRedirectionActive after restore failed: %v", err)
	}
	if activeAfter {
		t.Errorf("Expected redirection to be inactive after restore")
	}

	restoredAdapters, _ := mgr.DiscoverAdapters()
	if len(restoredAdapters[0].ServerAddresses) != 0 {
		t.Errorf("Expected Ethernet0 to be restored to empty (DHCP), got %v", restoredAdapters[0].ServerAddresses)
	}
	if len(restoredAdapters[1].ServerAddresses) != 2 || restoredAdapters[1].ServerAddresses[0] != "1.1.1.1" {
		t.Errorf("Expected Wi-Fi to be restored to 1.1.1.1, got %v", restoredAdapters[1].ServerAddresses)
	}

	// Verify backup file exists and is valid
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("Backup file should exist: %v", err)
	}
}
