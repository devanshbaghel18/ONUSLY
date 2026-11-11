//go:build windows

package dns

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WindowsAdapterManager manages Windows network adapter DNS configuration and fail-open recovery.
type WindowsAdapterManager struct {
	mu           sync.Mutex
	execCommand  func(name string, arg ...string) *exec.Cmd
	mockAdapters []AdapterDNSConfig // For isolated testing without modifying live hardware
}

func NewWindowsAdapterManager() *WindowsAdapterManager {
	return &WindowsAdapterManager{
		execCommand: exec.Command,
	}
}

// SetExecCommand overrides command execution for testing.
func (m *WindowsAdapterManager) SetExecCommand(fn func(name string, arg ...string) *exec.Cmd) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.execCommand = fn
}

// SetMockAdapters sets mock adapter data for testing in sandboxed environments.
func (m *WindowsAdapterManager) SetMockAdapters(adapters []AdapterDNSConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mockAdapters = adapters
}

type psDnsClientAddress struct {
	InterfaceAlias  string   `json:"InterfaceAlias"`
	InterfaceIndex  int      `json:"InterfaceIndex"`
	AddressFamily   int      `json:"AddressFamily"`
	ServerAddresses []string `json:"ServerAddresses"`
}

// DiscoverAdapters inspects active Windows network adapters and their current DNS server settings.
func (m *WindowsAdapterManager) DiscoverAdapters() ([]AdapterDNSConfig, error) {
	m.mu.Lock()
	if len(m.mockAdapters) > 0 {
		defer m.mu.Unlock()
		res := make([]AdapterDNSConfig, len(m.mockAdapters))
		copy(res, m.mockAdapters)
		return res, nil
	}
	execCmd := m.execCommand
	m.mu.Unlock()

	// Query via PowerShell Get-DnsClientServerAddress
	psCmd := `Get-DnsClientServerAddress -AddressFamily IPv4 | Select-Object InterfaceAlias, InterfaceIndex, AddressFamily, ServerAddresses | ConvertTo-Json -Compress`
	cmd := execCmd("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psCmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to query Windows DNS adapters: %w (output: %s)", err, string(out))
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" {
		return []AdapterDNSConfig{}, nil
	}

	// PowerShell ConvertTo-Json outputs single object or array
	var rawList []psDnsClientAddress
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &rawList); err != nil {
			return nil, fmt.Errorf("failed to parse DNS adapter array JSON: %w", err)
		}
	} else {
		var single psDnsClientAddress
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return nil, fmt.Errorf("failed to parse DNS adapter single JSON: %w", err)
		}
		rawList = []psDnsClientAddress{single}
	}

	configs := make([]AdapterDNSConfig, 0, len(rawList))
	for _, item := range rawList {
		aliasLower := strings.ToLower(item.InterfaceAlias)
		// Filter out virtual loopback, pseudo, or container adapters
		if strings.Contains(aliasLower, "loopback") || strings.Contains(aliasLower, "vethernet") {
			continue
		}

		isDhcp := len(item.ServerAddresses) == 0
		servers := item.ServerAddresses
		if servers == nil {
			servers = []string{}
		}

		configs = append(configs, AdapterDNSConfig{
			InterfaceID:     strconv.Itoa(item.InterfaceIndex),
			InterfaceAlias:  item.InterfaceAlias,
			InterfaceIndex:  item.InterfaceIndex,
			AddressFamily:   item.AddressFamily,
			IsDHCP:          isDhcp,
			ServerAddresses: servers,
			IsPhysical:      !strings.Contains(aliasLower, "virtual") && !strings.Contains(aliasLower, "hyper-v"),
			Status:          "Up",
		})
	}

	return configs, nil
}

// BackupDNS records the current DNS settings for all discovered adapters into backupFilePath.
func (m *WindowsAdapterManager) BackupDNS(backupFilePath string) (*DNSBackupState, error) {
	adapters, err := m.DiscoverAdapters()
	if err != nil {
		return nil, err
	}

	state := &DNSBackupState{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Platform:     "windows",
		OriginalDns:  adapters,
		LoopbackIPv4: "127.0.0.1",
		LoopbackIPv6: "::1",
		IsActive:     false,
	}

	dir := filepath.Dir(backupFilePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create DNS backup directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize DNS backup: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", backupFilePath, os.Getpid())
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to write temp DNS backup: %w", err)
	}

	if err := os.Rename(tmpFile, backupFilePath); err != nil {
		_ = os.Remove(tmpFile)
		return nil, fmt.Errorf("failed to commit DNS backup: %w", err)
	}

	return state, nil
}

// ApplyLoopbackDNS backs up current adapter settings and points active adapters to the local loopback DNS.
func (m *WindowsAdapterManager) ApplyLoopbackDNS(loopbackIPv4, loopbackIPv6 string, backupFilePath string) error {
	m.mu.Lock()
	execCmd := m.execCommand
	hasMock := len(m.mockAdapters) > 0
	m.mu.Unlock()

	if loopbackIPv4 == "" {
		loopbackIPv4 = "127.0.0.1"
	}

	// 1. Ensure backup exists before any modification
	state, err := m.BackupDNS(backupFilePath)
	if err != nil {
		return fmt.Errorf("failed to create DNS backup before redirection: %w", err)
	}

	if hasMock {
		// Mock testing mode: update mock adapters without modifying live host network
		m.mu.Lock()
		for i := range m.mockAdapters {
			m.mockAdapters[i].ServerAddresses = []string{loopbackIPv4}
		}
		m.mu.Unlock()
		state.IsActive = true
		stateData, _ := json.MarshalIndent(state, "", "  ")
		_ = os.WriteFile(backupFilePath, stateData, 0600)
		return nil
	}

	// 2. Set loopback DNS on active adapters
	for _, adapter := range state.OriginalDns {
		if !adapter.IsPhysical {
			continue
		}
		psSetCmd := fmt.Sprintf(`Set-DnsClientServerAddress -InterfaceIndex %d -ServerAddresses ("%s")`, adapter.InterfaceIndex, loopbackIPv4)
		cmd := execCmd("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psSetCmd)
		if out, errSet := cmd.CombinedOutput(); errSet != nil {
			// Fail-open: if setting fails on any adapter, restore original configuration immediately
			_ = m.RestoreDNS(backupFilePath)
			return fmt.Errorf("failed to set loopback DNS on adapter %s (index %d): %w (output: %s)", adapter.InterfaceAlias, adapter.InterfaceIndex, errSet, string(out))
		}
	}

	state.IsActive = true
	stateData, _ := json.MarshalIndent(state, "", "  ")
	_ = os.WriteFile(backupFilePath, stateData, 0600)

	// Flush Windows DNS cache
	_ = execCmd("ipconfig", "/flushdns").Run()

	return nil
}

// RestoreDNS restores the pre-existing DNS settings for all adapters from backupFilePath (Fail-Open Recovery).
func (m *WindowsAdapterManager) RestoreDNS(backupFilePath string) error {
	data, err := os.ReadFile(backupFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No backup exists; already clean
		}
		return fmt.Errorf("failed to read DNS backup file: %w", err)
	}

	var state DNSBackupState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("corrupt DNS backup file: %w", err)
	}

	m.mu.Lock()
	execCmd := m.execCommand
	hasMock := len(m.mockAdapters) > 0
	m.mu.Unlock()

	if hasMock {
		m.mu.Lock()
		m.mockAdapters = state.OriginalDns
		m.mu.Unlock()
		state.IsActive = false
		stateData, _ := json.MarshalIndent(state, "", "  ")
		_ = os.WriteFile(backupFilePath, stateData, 0600)
		return nil
	}

	var errs []string
	for _, adapter := range state.OriginalDns {
		var psRestoreCmd string
		if adapter.IsDHCP || len(adapter.ServerAddresses) == 0 {
			psRestoreCmd = fmt.Sprintf(`Set-DnsClientServerAddress -InterfaceIndex %d -ResetServerAddresses`, adapter.InterfaceIndex)
		} else {
			quotedAddrs := make([]string, len(adapter.ServerAddresses))
			for i, a := range adapter.ServerAddresses {
				quotedAddrs[i] = fmt.Sprintf(`"%s"`, a)
			}
			addrListStr := strings.Join(quotedAddrs, ",")
			psRestoreCmd = fmt.Sprintf(`Set-DnsClientServerAddress -InterfaceIndex %d -ServerAddresses (%s)`, adapter.InterfaceIndex, addrListStr)
		}

		cmd := execCmd("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psRestoreCmd)
		if out, errRest := cmd.CombinedOutput(); errRest != nil {
			errs = append(errs, fmt.Sprintf("adapter %s: %v (%s)", adapter.InterfaceAlias, errRest, string(out)))
		}
	}

	state.IsActive = false
	stateData, _ := json.MarshalIndent(state, "", "  ")
	_ = os.WriteFile(backupFilePath, stateData, 0600)

	// Flush Windows DNS cache
	_ = execCmd("ipconfig", "/flushdns").Run()

	if len(errs) > 0 {
		return errors.New("errors during DNS restoration: " + strings.Join(errs, "; "))
	}

	return nil
}

// IsRedirectionActive checks whether loopback DNS redirection is currently active according to the backup state file.
func (m *WindowsAdapterManager) IsRedirectionActive(backupFilePath string) (bool, error) {
	data, err := os.ReadFile(backupFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var state DNSBackupState
	if err := json.Unmarshal(data, &state); err != nil {
		return false, err
	}
	return state.IsActive, nil
}

