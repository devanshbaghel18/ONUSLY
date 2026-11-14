//go:build windows

package main

import (
	"os"
	"path/filepath"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
	"github.com/devanshbaghel18/ONUSLY/desktop/policy"
)

func newPlatformPolicyManager(configDir string) (core.PolicyManager, error) {
	selfExe, err := os.Executable()
	var nativeHostBin string
	if err == nil {
		selfDir := filepath.Dir(selfExe)
		candidate := filepath.Join(selfDir, "onusly-native-host.exe")
		if _, errStat := os.Stat(candidate); errStat == nil {
			nativeHostBin = candidate
		}
	}
	if nativeHostBin == "" {
		programFiles := os.Getenv("ProgramFiles")
		if programFiles == "" {
			programFiles = `C:\Program Files`
		}
		nativeHostBin = filepath.Join(programFiles, "Onusly", "onusly-native-host.exe")
	}

	manifestDir := filepath.Join(configDir, "manifests")
	backupDir := filepath.Join(configDir, "policy-backups", "backup_state.json")
	updateURL := "https://api.onusly.com/downloads/updates.xml"

	reg := policy.NewRealWindowsRegistry()
	mgr := policy.NewWindowsPolicyManager(nativeHostBin, manifestDir, backupDir, updateURL, reg)
	return mgr, nil
}
