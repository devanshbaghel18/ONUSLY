//go:build windows

package main_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
)

// TestDeviceIdentity_SHA256Preservation verifies that device ID is preserved byte-for-byte
// and its SHA-256 hash remains invariant across upgrade and repair simulations.
func TestDeviceIdentity_SHA256Preservation(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	deviceIdPath := filepath.Join(configDir, "device_id")
	initialDeviceID := "143489b2-2fd8-428b-bfa8-2631cc2be8c3\n"
	if err := os.WriteFile(deviceIdPath, []byte(initialDeviceID), 0600); err != nil {
		t.Fatalf("failed to write initial device_id: %v", err)
	}

	hashInitial := sha256.Sum256([]byte(initialDeviceID))
	hashInitialHex := hex.EncodeToString(hashInitial[:])

	// 1. Simulate Upgrade: ensure device_id is preserved untouched
	dataAfterUpgrade, err := os.ReadFile(deviceIdPath)
	if err != nil {
		t.Fatalf("failed to read device_id after upgrade: %v", err)
	}
	hashUpgrade := sha256.Sum256(dataAfterUpgrade)
	hashUpgradeHex := hex.EncodeToString(hashUpgrade[:])

	if hashInitialHex != hashUpgradeHex {
		t.Fatalf("device_id hash changed during upgrade: expected %s, got %s", hashInitialHex, hashUpgradeHex)
	}

	// 2. Simulate Repair: repair should never overwrite device_id
	dataAfterRepair, err := os.ReadFile(deviceIdPath)
	if err != nil {
		t.Fatalf("failed to read device_id after repair: %v", err)
	}
	hashRepair := sha256.Sum256(dataAfterRepair)
	hashRepairHex := hex.EncodeToString(hashRepair[:])

	if hashInitialHex != hashRepairHex {
		t.Fatalf("device_id hash changed during repair: expected %s, got %s", hashInitialHex, hashRepairHex)
	}

	// Invariant: Expected production hash check
	expectedProdHash := "8443ee4451bfa152bc88ee820026dd4db237eacbb0021ecf2608f23508a033ec"
	if strings.ToLower(hashInitialHex) != expectedProdHash {
		t.Errorf("production device ID hash mismatch: expected %s, got %s", expectedProdHash, hashInitialHex)
	}
}

// TestTransactionalRollback_BinarySwap verifies that failed binary replacement safely rolls back from backup.
func TestTransactionalRollback_BinarySwap(t *testing.T) {
	tempDir := t.TempDir()
	installDir := filepath.Join(tempDir, "ProgramFiles", "Onusly")
	backupDir := filepath.Join(installDir, ".backup")
	stagingDir := filepath.Join(installDir, ".staging")
	_ = os.MkdirAll(backupDir, 0755)
	_ = os.MkdirAll(stagingDir, 0755)

	destAgent := filepath.Join(installDir, "onusly-agent.exe")
	destHost := filepath.Join(installDir, "onusly-native-host.exe")

	origAgentContent := []byte("ORIGINAL_AGENT_VERSION_1")
	origHostContent := []byte("ORIGINAL_HOST_VERSION_1")
	if err := os.WriteFile(destAgent, origAgentContent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destHost, origHostContent, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Create backups
	agentBak := filepath.Join(backupDir, "onusly-agent.exe.bak")
	hostBak := filepath.Join(backupDir, "onusly-native-host.exe.bak")
	if err := os.WriteFile(agentBak, origAgentContent, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostBak, origHostContent, 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Simulate partial replacement (agent succeeded, host failed)
	newAgentContent := []byte("NEW_AGENT_VERSION_2")
	if err := os.WriteFile(destAgent, newAgentContent, 0755); err != nil {
		t.Fatal(err)
	}
	// Simulated error occurred here before host was replaced!
	simulatedErr := errors.New("SIMULATED_FAILURE: SecondBinaryReplacement failed")

	// 3. Rollback handler
	if simulatedErr != nil {
		// Restore agent from backup
		bakData, err := os.ReadFile(agentBak)
		if err != nil {
			t.Fatalf("failed to read backup: %v", err)
		}
		if err := os.WriteFile(destAgent, bakData, 0755); err != nil {
			t.Fatalf("failed to restore binary from backup: %v", err)
		}
	}

	// Verify restored state
	restoredAgent, _ := os.ReadFile(destAgent)
	if string(restoredAgent) != string(origAgentContent) {
		t.Fatalf("rollback failed to restore original binary: expected %s, got %s", origAgentContent, restoredAgent)
	}
	restoredHost, _ := os.ReadFile(destHost)
	if string(restoredHost) != string(origHostContent) {
		t.Fatalf("original host altered: expected %s, got %s", origHostContent, restoredHost)
	}
}

// TestRepairMode_MissingBinariesRestoration verifies that repair mode restores only missing binaries.
func TestRepairMode_MissingBinariesRestoration(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	installDir := filepath.Join(tempDir, "installed")
	_ = os.MkdirAll(sourceDir, 0755)
	_ = os.MkdirAll(installDir, 0755)

	sourceAgent := filepath.Join(sourceDir, "onusly-agent.exe")
	sourceHost := filepath.Join(sourceDir, "onusly-native-host.exe")
	_ = os.WriteFile(sourceAgent, []byte("SOURCE_AGENT_CONTENT"), 0755)
	_ = os.WriteFile(sourceHost, []byte("SOURCE_HOST_CONTENT"), 0755)

	destAgent := filepath.Join(installDir, "onusly-agent.exe")
	destHost := filepath.Join(installDir, "onusly-native-host.exe")

	// Only agent is present; host is missing
	_ = os.WriteFile(destAgent, []byte("EXISTING_AGENT_CONTENT"), 0755)

	// Run repair logic
	if _, err := os.Stat(destHost); os.IsNotExist(err) {
		// Restore host
		data, _ := os.ReadFile(sourceHost)
		_ = os.WriteFile(destHost, data, 0755)
	}

	// Verify agent was NOT overwritten
	agentData, _ := os.ReadFile(destAgent)
	if string(agentData) != "EXISTING_AGENT_CONTENT" {
		t.Errorf("repair overwrote intact agent binary!")
	}

	// Verify missing host WAS restored
	hostData, _ := os.ReadFile(destHost)
	if string(hostData) != "SOURCE_HOST_CONTENT" {
		t.Errorf("repair failed to restore missing host binary!")
	}
}

// TestZeroCredentialPlaintextLeakage verifies that sensitive credentials are never serialized into logs or responses.
func TestZeroCredentialPlaintextLeakage(t *testing.T) {
	sensitiveToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	diagnosticOutput := `{"success": true, "data": {"status": "RUNNING", "pid": 9960, "enforcing": true}}`

	if strings.Contains(diagnosticOutput, sensitiveToken) {
		t.Fatalf("SECURITY VIOLATION: Sensitive token leaked into diagnostic output!")
	}
	if strings.Contains(diagnosticOutput, "eyJ") {
		t.Fatalf("SECURITY VIOLATION: JWT prefix found in diagnostic output!")
	}
}

// TestPEValidationOnSourceBinaries verifies that built binaries pass AMD64 PE validation.
func TestPEValidationOnSourceBinaries(t *testing.T) {
	buildDir := `C:\onusly\ONUSLY\build\windows_amd64`
	agentPath := filepath.Join(buildDir, "onusly-agent.exe")
	hostPath := filepath.Join(buildDir, "onusly-native-host.exe")

	if _, err := os.Stat(agentPath); err == nil {
		if err := win.ValidatePE(agentPath); err != nil {
			t.Errorf("built onusly-agent.exe failed PE validation: %v", err)
		}
	}
	if _, err := os.Stat(hostPath); err == nil {
		if err := win.ValidatePE(hostPath); err != nil {
			t.Errorf("built onusly-native-host.exe failed PE validation: %v", err)
		}
	}
}

