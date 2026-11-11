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

func TestWindowsAdapterManager_NeverOverwriteBackupWithLoopback(t *testing.T) {
	tempDir := t.TempDir()
	backupPath := filepath.Join(tempDir, "dns_backup.json")

	mgr := NewWindowsAdapterManager()

	initialAdapters := []AdapterDNSConfig{
		{
			InterfaceID:     "15",
			InterfaceAlias:  "StaticEth",
			InterfaceIndex:  15,
			AddressFamily:   2,
			IsDHCP:          false,
			ServerAddresses: []string{"8.8.8.8", "8.8.4.4"},
			IsPhysical:      true,
			Status:          "Up",
		},
	}
	mgr.SetMockAdapters(initialAdapters)

	// Step 1: Initial Backup
	state1, err := mgr.BackupDNS(backupPath)
	if err != nil {
		t.Fatalf("Initial backup failed: %v", err)
	}
	if len(state1.OriginalDns) != 1 || state1.OriginalDns[0].ServerAddresses[0] != "8.8.8.8" {
		t.Fatalf("Expected static 8.8.8.8 in initial backup, got %v", state1.OriginalDns)
	}

	// Step 2: Loopback applied
	if err := mgr.ApplyLoopbackDNS("127.0.0.1", "::1", backupPath); err != nil {
		t.Fatalf("ApplyLoopbackDNS failed: %v", err)
	}

	// Step 3: Now simulate an accidental second BackupDNS call while loopback is active!
	state2, err := mgr.BackupDNS(backupPath)
	if err != nil {
		t.Fatalf("Second backup failed: %v", err)
	}

	// Verify that state2 STILL holds original static servers 8.8.8.8, NOT loopback 127.0.0.1!
	if len(state2.OriginalDns) != 1 || state2.OriginalDns[0].ServerAddresses[0] != "8.8.8.8" {
		t.Fatalf("CRITICAL SAFETY VIOLATION: Backup was overwritten with loopback! Got: %v", state2.OriginalDns[0].ServerAddresses)
	}
	if state2.OriginalDns[0].IsDHCP {
		t.Fatalf("CRITICAL SAFETY VIOLATION: Static adapter was improperly marked as DHCP!")
	}
}

func TestWindowsAdapterManager_MissingBackupProtection(t *testing.T) {
	tempDir := t.TempDir()
	missingBackupPath := filepath.Join(tempDir, "non_existent_backup.json")

	mgr := NewWindowsAdapterManager()

	// Simulate an adapter currently pointing to loopback
	loopbackAdapters := []AdapterDNSConfig{
		{
			InterfaceID:     "12",
			InterfaceAlias:  "Wi-Fi",
			InterfaceIndex:  12,
			AddressFamily:   2,
			IsDHCP:          false,
			ServerAddresses: []string{"127.0.0.1"},
			IsPhysical:      true,
			Status:          "Up",
		},
	}
	mgr.SetMockAdapters(loopbackAdapters)

	// Attempting to restore with missing backup must NOT silently succeed or wipe to DHCP
	err := mgr.RestoreDNS(missingBackupPath)
	if err == nil {
		t.Fatalf("Expected error when restoring missing backup while loopback is active, got nil")
	}
}

// TestWindowsAdapterManager_DualStack4Combinations validates all 4 dual-stack permutations
// required by ONUSLY D7.2 Phase D:
// a. IPv4 Static, IPv6 DHCP
// b. IPv4 DHCP, IPv6 Static
// c. IPv4 Static, IPv6 Static
// d. IPv4 DHCP, IPv6 DHCP
func TestWindowsAdapterManager_DualStack4Combinations(t *testing.T) {
	tempDir := t.TempDir()
	backupPath := filepath.Join(tempDir, "dns_dualstack_backup.json")

	mgr := NewWindowsAdapterManager()

	initialAdapters := []AdapterDNSConfig{
		// Combination a: IPv4 Static, IPv6 DHCP (Adapter Index 10)
		{
			InterfaceID:     "10",
			InterfaceAlias:  "AdapterA_IPv4",
			InterfaceIndex:  10,
			AddressFamily:   2,
			IsDHCP:          false,
			ServerAddresses: []string{"8.8.8.8", "8.8.4.4"},
			IsPhysical:      true,
			Status:          "Up",
		},
		{
			InterfaceID:     "10",
			InterfaceAlias:  "AdapterA_IPv6",
			InterfaceIndex:  10,
			AddressFamily:   23,
			IsDHCP:          true,
			ServerAddresses: []string{},
			IsPhysical:      true,
			Status:          "Up",
		},
		// Combination b: IPv4 DHCP, IPv6 Static (Adapter Index 20)
		{
			InterfaceID:     "20",
			InterfaceAlias:  "AdapterB_IPv4",
			InterfaceIndex:  20,
			AddressFamily:   2,
			IsDHCP:          true,
			ServerAddresses: []string{},
			IsPhysical:      true,
			Status:          "Up",
		},
		{
			InterfaceID:     "20",
			InterfaceAlias:  "AdapterB_IPv6",
			InterfaceIndex:  20,
			AddressFamily:   23,
			IsDHCP:          false,
			ServerAddresses: []string{"2001:4860:4860::8888", "2001:4860:4860::8844"},
			IsPhysical:      true,
			Status:          "Up",
		},
		// Combination c: IPv4 Static, IPv6 Static (Adapter Index 30)
		{
			InterfaceID:     "30",
			InterfaceAlias:  "AdapterC_IPv4",
			InterfaceIndex:  30,
			AddressFamily:   2,
			IsDHCP:          false,
			ServerAddresses: []string{"1.1.1.1"},
			IsPhysical:      true,
			Status:          "Up",
		},
		{
			InterfaceID:     "30",
			InterfaceAlias:  "AdapterC_IPv6",
			InterfaceIndex:  30,
			AddressFamily:   23,
			IsDHCP:          false,
			ServerAddresses: []string{"2606:4700:4700::1111"},
			IsPhysical:      true,
			Status:          "Up",
		},
		// Combination d: IPv4 DHCP, IPv6 DHCP (Adapter Index 40)
		{
			InterfaceID:     "40",
			InterfaceAlias:  "AdapterD_IPv4",
			InterfaceIndex:  40,
			AddressFamily:   2,
			IsDHCP:          true,
			ServerAddresses: []string{},
			IsPhysical:      true,
			Status:          "Up",
		},
		{
			InterfaceID:     "40",
			InterfaceAlias:  "AdapterD_IPv6",
			InterfaceIndex:  40,
			AddressFamily:   23,
			IsDHCP:          true,
			ServerAddresses: []string{},
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
	if len(backupState.OriginalDns) != 8 {
		t.Fatalf("Expected 8 original adapter entries (4 adapters x 2 families), got %d", len(backupState.OriginalDns))
	}

	// Step 2: Apply Loopback DNS (IPv4 -> 127.0.0.1, IPv6 -> ::1)
	if err := mgr.ApplyLoopbackDNS("127.0.0.1", "::1", backupPath); err != nil {
		t.Fatalf("ApplyLoopbackDNS failed: %v", err)
	}

	// Verify loopback applied per address family
	current, _ := mgr.DiscoverAdapters()
	for _, a := range current {
		if a.AddressFamily == 2 {
			if len(a.ServerAddresses) != 1 || a.ServerAddresses[0] != "127.0.0.1" {
				t.Fatalf("Adapter %s IPv4 expected 127.0.0.1, got %v", a.InterfaceAlias, a.ServerAddresses)
			}
		} else if a.AddressFamily == 23 {
			if len(a.ServerAddresses) != 1 || a.ServerAddresses[0] != "::1" {
				t.Fatalf("Adapter %s IPv6 expected ::1, got %v", a.InterfaceAlias, a.ServerAddresses)
			}
		}
	}

	// Step 3: Restore DNS
	if err := mgr.RestoreDNS(backupPath); err != nil {
		t.Fatalf("RestoreDNS failed: %v", err)
	}

	restored, _ := mgr.DiscoverAdapters()
	if len(restored) != 8 {
		t.Fatalf("Expected 8 restored adapter entries, got %d", len(restored))
	}

	// Verify Combination a restored correctly
	if restored[0].IsDHCP || len(restored[0].ServerAddresses) != 2 || restored[0].ServerAddresses[0] != "8.8.8.8" {
		t.Errorf("Combination a IPv4 not restored to static 8.8.8.8: %v", restored[0])
	}
	if !restored[1].IsDHCP || len(restored[1].ServerAddresses) != 0 {
		t.Errorf("Combination a IPv6 not restored to DHCP: %v", restored[1])
	}

	// Verify Combination b restored correctly
	if !restored[2].IsDHCP || len(restored[2].ServerAddresses) != 0 {
		t.Errorf("Combination b IPv4 not restored to DHCP: %v", restored[2])
	}
	if restored[3].IsDHCP || len(restored[3].ServerAddresses) != 2 || restored[3].ServerAddresses[0] != "2001:4860:4860::8888" {
		t.Errorf("Combination b IPv6 not restored to static 2001:4860:4860::8888: %v", restored[3])
	}

	// Verify Combination c restored correctly
	if restored[4].IsDHCP || len(restored[4].ServerAddresses) != 1 || restored[4].ServerAddresses[0] != "1.1.1.1" {
		t.Errorf("Combination c IPv4 not restored to static 1.1.1.1: %v", restored[4])
	}
	if restored[5].IsDHCP || len(restored[5].ServerAddresses) != 1 || restored[5].ServerAddresses[0] != "2606:4700:4700::1111" {
		t.Errorf("Combination c IPv6 not restored to static 2606:4700:4700::1111: %v", restored[5])
	}

	// Verify Combination d restored correctly
	if !restored[6].IsDHCP || len(restored[6].ServerAddresses) != 0 {
		t.Errorf("Combination d IPv4 not restored to DHCP: %v", restored[6])
	}
	if !restored[7].IsDHCP || len(restored[7].ServerAddresses) != 0 {
		t.Errorf("Combination d IPv6 not restored to DHCP: %v", restored[7])
	}
}
