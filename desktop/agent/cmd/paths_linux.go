//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func getDefaultConfigDir() string {
	if os.Geteuid() == 0 {
		return "/etc/onusly"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "onusly")
}

func getDefaultDataDir() string {
	if os.Geteuid() == 0 {
		return "/var/lib/onusly"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "onusly")
}

func getDefaultIPCPath() string {
	if os.Geteuid() == 0 {
		return "/run/onusly/agent.sock"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "onusly", "agent.sock")
}

func newPlatformCredentialStore(configDir string) core.CredentialStore {
	return auth.NewFileCredentialStore(filepath.Join(configDir, "token"))
}

func getPlatformName() string {
	return "linux"
}

func resolvePairingOwner() (ownerSID string, ownerUID uint32, err error) {
	var targetUID uint32
	if sudoUID := os.Getenv("SUDO_UID"); sudoUID != "" {
		if parsed, err := strconv.ParseUint(sudoUID, 10, 32); err == nil {
			targetUID = uint32(parsed)
		}
	} else if os.Geteuid() != 0 {
		targetUID = uint32(os.Geteuid())
	}
	return "", targetUID, nil
}
