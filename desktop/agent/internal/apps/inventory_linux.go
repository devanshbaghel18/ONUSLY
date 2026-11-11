//go:build !windows

package apps

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LinuxInventoryScanner discovers installed applications on Linux using XDG .desktop files.
type LinuxInventoryScanner struct {
	mu           sync.Mutex
	cached       []ApplicationIdentity
	lastScan     time.Time
	cacheTTL     time.Duration
	mockApps     []ApplicationIdentity
}

func NewLinuxInventoryScanner(cacheTTL time.Duration) *LinuxInventoryScanner {
	if cacheTTL <= 0 {
		cacheTTL = 30 * time.Minute
	}
	return &LinuxInventoryScanner{
		cacheTTL: cacheTTL,
	}
}

func (s *LinuxInventoryScanner) SetMockApplications(apps []ApplicationIdentity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mockApps = apps
}

func (s *LinuxInventoryScanner) ScanApplications(ctx context.Context) ([]ApplicationIdentity, error) {
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

	dirs := []string{
		"/usr/share/applications",
		"/usr/local/share/applications",
		"/var/lib/flatpak/exports/share/applications",
		"/var/lib/snapd/desktop/applications",
	}

	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "applications"))
	}

	seen := make(map[string]bool)
	var discovered []ApplicationIdentity

	for _, dir := range dirs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".desktop") {
				continue
			}

			filePath := filepath.Join(dir, entry.Name())
			identity, errParse := parseDesktopFile(filePath)
			if errParse != nil || identity == nil {
				continue
			}

			if seen[identity.AppID] {
				continue
			}

			seen[identity.AppID] = true
			discovered = append(discovered, *identity)
		}
	}

	s.mu.Lock()
	s.cached = discovered
	s.lastScan = time.Now()
	s.mu.Unlock()

	return discovered, nil
}

func parseDesktopFile(path string) (*ApplicationIdentity, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var name, exec string
	var noDisplay bool

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "Name":
			if name == "" {
				name = val
			}
		case "Exec":
			if exec == "" {
				exec = val
			}
		case "NoDisplay":
			if strings.EqualFold(val, "true") {
				noDisplay = true
			}
		}
	}

	if noDisplay || name == "" {
		return nil, nil
	}

	cleanExec := strings.Fields(exec)
	var binPath, baseName string
	if len(cleanExec) > 0 {
		binPath = cleanExec[0]
		baseName = filepath.Base(binPath)
	}

	appID := NormalizeAppID(name)
	if appID == "" {
		appID = NormalizeAppID(strings.TrimSuffix(filepath.Base(path), ".desktop"))
	}

	var exeBases []string
	if baseName != "" {
		exeBases = append(exeBases, baseName)
	}
	if catalogExes, ok := knownAppExeCatalog[appID]; ok {
		exeBases = append(exeBases, catalogExes...)
	}

	return &ApplicationIdentity{
		AppID:        appID,
		DisplayName:  name,
		Platform:     "linux",
		ExePath:      binPath,
		ExeBaseNames: exeBases,
		IsSystem:     false,
	}, nil
}

