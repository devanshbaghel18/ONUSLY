//go:build !windows

package apps_test

import (
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/apps"
)

func newTestEnforcer() apps.AppEnforcer {
	return apps.NewLinuxAppEnforcer()
}

func newTestMonitor() apps.ProcessMonitor {
	return apps.NewLinuxProcessMonitor(1 * time.Second)
}

