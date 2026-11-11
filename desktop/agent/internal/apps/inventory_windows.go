//go:build windows

package apps

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"
)

// WindowsInventoryScanner discovers installed applications on Windows using registry keys and shortcuts.
type WindowsInventoryScanner struct {
	mu           sync.Mutex
	cached       []ApplicationIdentity
	lastScan     time.Time
	cacheTTL     time.Duration
	mockApps     []ApplicationIdentity
}

// NewWindowsInventoryScanner creates a scanner with the specified cache TTL.
func NewWindowsInventoryScanner(cacheTTL time.Duration) *WindowsInventoryScanner {
	if cacheTTL <= 0 {
		cacheTTL = 30 * time.Minute
	}
	return &WindowsInventoryScanner{
		cacheTTL: cacheTTL,
	}
}

// SetMockApplications overrides discovered applications for testing.
func (s *WindowsInventoryScanner) SetMockApplications(apps []ApplicationIdentity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mockApps = apps
}

// ScanApplications scans Windows registry uninstall locations for installed software.
func (s *WindowsInventoryScanner) ScanApplications(ctx context.Context) ([]ApplicationIdentity, error) {
	s.mu.Lock()
	if len(s.mockApps) > 0 {
		defer s.mu.Unlock()
		res := make([]ApplicationIdentity, len(s.mockApps))
		copy(res, s.mockApps)
		return res, nil
	}

	if len(s.cached) > 0 && time.Since(s.lastScan) < s.cacheTTL {
		defer s.mu.Unlock()
		res := make([]ApplicationIdentity, len(s.cached))
		copy(res, s.cached)
		return res, nil
	}
	s.mu.Unlock()

	targets := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	}

	seen := make(map[string]bool)
	var discovered []ApplicationIdentity

	for _, t := range targets {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		k, err := registry.OpenKey(t.root, t.path, registry.READ)
		if err != nil {
			continue
		}

		subkeys, err := k.ReadSubKeyNames(-1)
		k.Close()
		if err != nil {
			continue
		}

		for _, sub := range subkeys {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			subKeyPath := t.path + `\` + sub
			sk, err := registry.OpenKey(t.root, subKeyPath, registry.READ)
			if err != nil {
				continue
			}

			dispName, _, _ := sk.GetStringValue("DisplayName")
			dispName = strings.TrimSpace(dispName)
			if dispName == "" {
				sk.Close()
				continue
			}

			// Skip system updates and hidden components
			sysComp, _, _ := sk.GetIntegerValue("SystemComponent")
			parentKey, _, _ := sk.GetStringValue("ParentKeyName")
			releaseType, _, _ := sk.GetStringValue("ReleaseType")
			if sysComp == 1 || parentKey != "" || strings.EqualFold(releaseType, "Update") {
				sk.Close()
				continue
			}

			appID := NormalizeAppID(dispName)
			if appID == "" || seen[appID] {
				sk.Close()
				continue
			}

			publisher, _, _ := sk.GetStringValue("Publisher")
			installLoc, _, _ := sk.GetStringValue("InstallLocation")
			dispIcon, _, _ := sk.GetStringValue("DisplayIcon")
			sk.Close()

			var exeBases []string
			var primaryExe string

			// Extract executable from DisplayIcon if pointing to an .exe
			if dispIcon != "" {
				iconParts := strings.Split(dispIcon, ",")
				cleanIcon := strings.Trim(strings.TrimSpace(iconParts[0]), `"`)
				if strings.EqualFold(filepath.Ext(cleanIcon), ".exe") {
					primaryExe = cleanIcon
					exeBases = append(exeBases, filepath.Base(cleanIcon))
				}
			}

			// Add common catalog binaries for well-known software
			if catalogExes, ok := knownAppExeCatalog[appID]; ok {
				exeBases = append(exeBases, catalogExes...)
			}

			identity := ApplicationIdentity{
				AppID:        appID,
				DisplayName:  dispName,
				Platform:     "windows",
				ExePath:      primaryExe,
				ExeBaseNames: exeBases,
				InstallDir:   installLoc,
				Publisher:    publisher,
				IsSystem:     false,
			}

			seen[appID] = true
			discovered = append(discovered, identity)
		}
	}

	s.mu.Lock()
	s.cached = discovered
	s.lastScan = time.Now()
	s.mu.Unlock()

	return discovered, nil
}

