//go:build windows

package apps_test

import (
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/apps"
)

func newTestEnforcer() apps.AppEnforcer {
	return apps.NewWindowsAppEnforcer()
}

func newTestMonitor() apps.ProcessMonitor {
	return apps.NewWindowsProcessMonitor(1 * time.Second)
}

