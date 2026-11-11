//go:build !windows

package apps

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLinuxInventoryScanner_Mock(t *testing.T) {
	mockData := []ApplicationIdentity{
		{
			AppID:        "discord",
			DisplayName:  "Discord",
			Platform:     "linux",
			ExeBaseNames: []string{"discord"},
		},
		{
			AppID:        "steam",
			DisplayName:  "Steam",
			Platform:     "linux",
			ExeBaseNames: []string{"steam"},
		},
	}

	scanner := NewLinuxInventoryScanner(10 * time.Minute)
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

func TestLinuxDesktopFileParsing(t *testing.T) {
	tempDir := t.TempDir()
	desktopFile := filepath.Join(tempDir, "spotify.desktop")

	content := `[Desktop Entry]
Name=Spotify
GenericName=Music Player
Comment=Spotify streaming music client
Icon=spotify-client
Exec=spotify %U
TryExec=spotify
Terminal=false
Type=Application
Categories=Audio;Music;Player;AudioVideo;
MimeType=x-scheme-handler/spotify;
`
	if err := os.WriteFile(desktopFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	identity, err := parseDesktopFile(desktopFile)
	if err != nil {
		t.Fatalf("parseDesktopFile failed: %v", err)
	}
	if identity == nil {
		t.Fatalf("Expected valid identity, got nil")
	}
	if identity.AppID != "spotify" {
		t.Errorf("Expected AppID spotify, got %s", identity.AppID)
	}
	if identity.DisplayName != "Spotify" {
		t.Errorf("Expected DisplayName Spotify, got %s", identity.DisplayName)
	}
	if len(identity.ExeBaseNames) == 0 || identity.ExeBaseNames[0] != "spotify" {
		t.Errorf("Expected ExeBaseNames to contain spotify, got %v", identity.ExeBaseNames)
	}
}

