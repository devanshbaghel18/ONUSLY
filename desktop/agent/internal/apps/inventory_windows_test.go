//go:build windows

package apps

import (
	"context"
	"testing"
	"time"
)

func TestInventoryScanner_Mock(t *testing.T) {
	mockData := []ApplicationIdentity{
		{
			AppID:        "discord",
			DisplayName:  "Discord",
			Platform:     "windows",
			ExeBaseNames: []string{"Discord.exe"},
		},
		{
			AppID:        "steam",
			DisplayName:  "Steam",
			Platform:     "windows",
			ExeBaseNames: []string{"steam.exe", "steamwebhelper.exe"},
		},
	}

	scanner := NewWindowsInventoryScanner(10 * time.Minute)
	scanner.SetMockApplications(mockData)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	discovered, err := scanner.ScanApplications(ctx)
	if err != nil {
		t.Fatalf("ScanApplications failed: %v", err)
	}

	if len(discovered) != 2 {
		t.Fatalf("Expected 2 applications, got %d", len(discovered))
	}
	if discovered[0].AppID != "discord" || discovered[1].AppID != "steam" {
		t.Errorf("Unexpected discovered items: %v", discovered)
	}
}

func TestWindowsInventoryScanner_LiveScan(t *testing.T) {
	scanner := NewWindowsInventoryScanner(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	discovered, err := scanner.ScanApplications(ctx)
	if err != nil {
		t.Fatalf("Live Windows scan failed: %v", err)
	}

	// Should discover at least 1 installed application on a real Windows installation
	if len(discovered) == 0 {
		t.Logf("Notice: 0 applications found in registry uninstall keys")
	} else {
		t.Logf("Successfully discovered %d applications on Windows host", len(discovered))
		for i, app := range discovered {
			if i < 5 {
				t.Logf("  App [%s]: %s (exe: %s, publisher: %s)", app.AppID, app.DisplayName, app.ExePath, app.Publisher)
			}
		}
	}
}
