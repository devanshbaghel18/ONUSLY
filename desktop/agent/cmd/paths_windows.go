//go:build windows

package main

import (
	"os"
	"path/filepath"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/dns"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
)

func getDefaultConfigDir() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "Onusly", "config")
}

func getDefaultDataDir() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "Onusly", "data")
}

func getDefaultIPCPath() string {
	return `\\.\pipe\OnuslyAgentIPC`
}

func newPlatformCredentialStore(configDir string) core.CredentialStore {
	secureDir := os.Getenv("ONUSLY_SECURE_DIR")
	if secureDir == "" {
		if configDir != "" {
			secureDir = filepath.Join(filepath.Dir(configDir), "secure")
		} else {
			programData := os.Getenv("ProgramData")
			if programData == "" {
				programData = `C:\ProgramData`
			}
			secureDir = filepath.Join(programData, "Onusly", "secure")
		}
	}
	return auth.NewDPAPICredentialStore(filepath.Join(secureDir, "token.dat"))
}

func getPlatformName() string {
	return "windows"
}

func newPlatformAdapterManager() dns.AdapterManager {
	return dns.NewWindowsAdapterManager()
}

func resolvePairingOwner() (ownerSID string, ownerUID uint32, err error) {
	sess, err := win.GetActiveConsoleSession()
	if err == nil && sess != nil && sess.UserSID != "" {
		return sess.UserSID, 0, nil
	}
	return "", 0, err
}
