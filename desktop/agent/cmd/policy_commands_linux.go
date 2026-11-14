//go:build !windows

package main

import (
	"path/filepath"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
	"github.com/devanshbaghel18/ONUSLY/desktop/policy"
)

func newPlatformPolicyManager(configDir string) (core.PolicyManager, error) {
	nativeHostBin := "/usr/local/bin/onusly-native-host"
	backupDir := filepath.Join(configDir, "policy-backups")
	mgr := policy.NewLinuxPolicyManager(nativeHostBin, backupDir, false)
	return mgr, nil
}

