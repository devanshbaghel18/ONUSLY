//go:build !windows

package dns

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LinuxAdapterManager manages Linux resolver configuration across systemd-resolved, NetworkManager, and resolv.conf.
type LinuxAdapterManager struct {
	mu           sync.Mutex
	execCommand  func(name string, arg ...string) *exec.Cmd
	mockAdapters []AdapterDNSConfig
}

func NewLinuxAdapterManager() *LinuxAdapterManager {
	return &LinuxAdapterManager{
		execCommand: exec.Command,
	}
}

func (m *LinuxAdapterManager) SetExecCommand(fn func(name string, arg ...string) *exec.Cmd) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.execCommand = fn
}

func (m *LinuxAdapterManager) SetMockAdapters(adapters []AdapterDNSConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mockAdapters = adapters
}

func (m *LinuxAdapterManager) DiscoverAdapters() ([]AdapterDNSConfig, error) {
	m.mu.Lock()
	if len(m.mockAdapters) > 0 {
		defer m.mu.Unlock()
		res := make([]AdapterDNSConfig, len(m.mockAdapters))
		copy(res, m.mockAdapters)
		return res, nil
	}
	execCmd := m.execCommand
	m.mu.Unlock()

	// 1. Try resolvectl
	cmd := execCmd("resolvectl", "dns")
	if out, err := cmd.Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		var configs []AdapterDNSConfig
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "Global:") {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				linkName := strings.TrimPrefix(parts[0], "Link ")
				linkName = strings.TrimSpace(strings.Split(linkName, " ")[0])
				servers := strings.Fields(parts[1])
				configs = append(configs, AdapterDNSConfig{
					InterfaceID:     linkName,
					InterfaceAlias:  linkName,
					AddressFamily:   2,
					IsDHCP:          len(servers) == 0,
					ServerAddresses: servers,
					IsPhysical:      !strings.HasPrefix(linkName, "lo") && !strings.HasPrefix(linkName, "docker") && !strings.HasPrefix(linkName, "veth"),
					Status:          "Up",
				})
			}
		}
		if len(configs) > 0 {
			return configs, nil
		}
	}

	// 2. Fallback to /etc/resolv.conf inspection
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to read /etc/resolv.conf: %w", err)
	}

	var servers []string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				servers = append(servers, fields[1])
			}
		}
	}

	return []AdapterDNSConfig{
		{
			InterfaceID:     "system-resolv-conf",
			InterfaceAlias:  "resolv.conf",
			AddressFamily:   2,
			IsDHCP:          false,
			ServerAddresses: servers,
			IsPhysical:      true,
			Status:          "Up",
		},
	}, nil
}

func (m *LinuxAdapterManager) BackupDNS(backupFilePath string) (*DNSBackupState, error) {
	adapters, err := m.DiscoverAdapters()
	if err != nil {
		return nil, err
	}

	state := &DNSBackupState{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Platform:     "linux",
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

func (m *LinuxAdapterManager) ApplyLoopbackDNS(loopbackIPv4, loopbackIPv6 string, backupFilePath string) error {
	m.mu.Lock()
	execCmd := m.execCommand
	hasMock := len(m.mockAdapters) > 0
	m.mu.Unlock()

	if loopbackIPv4 == "" {
		loopbackIPv4 = "127.0.0.1"
	}

	state, err := m.BackupDNS(backupFilePath)
	if err != nil {
		return fmt.Errorf("failed to create DNS backup before redirection: %w", err)
	}

	if hasMock {
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

	// Apply link-specific DNS via resolvectl if available
	for _, adapter := range state.OriginalDns {
		if !adapter.IsPhysical || adapter.InterfaceID == "system-resolv-conf" {
			continue
		}
		_ = execCmd("resolvectl", "dns", adapter.InterfaceID, loopbackIPv4).Run()
		_ = execCmd("resolvectl", "domain", adapter.InterfaceID, "~.").Run()
	}

	state.IsActive = true
	stateData, _ := json.MarshalIndent(state, "", "  ")
	_ = os.WriteFile(backupFilePath, stateData, 0600)

	return nil
}

func (m *LinuxAdapterManager) RestoreDNS(backupFilePath string) error {
	data, err := os.ReadFile(backupFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
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
		if !adapter.IsPhysical || adapter.InterfaceID == "system-resolv-conf" {
			continue
		}
		// Revert interface DNS settings to system defaults (DHCP / previous state)
		if errRevert := execCmd("resolvectl", "revert", adapter.InterfaceID).Run(); errRevert != nil {
			errs = append(errs, fmt.Sprintf("revert %s: %v", adapter.InterfaceID, errRevert))
		}
	}

	state.IsActive = false
	stateData, _ := json.MarshalIndent(state, "", "  ")
	_ = os.WriteFile(backupFilePath, stateData, 0600)

	if len(errs) > 0 {
		return errors.New("errors during DNS restoration: " + strings.Join(errs, "; "))
	}

	return nil
}

func (m *LinuxAdapterManager) IsRedirectionActive(backupFilePath string) (bool, error) {
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

