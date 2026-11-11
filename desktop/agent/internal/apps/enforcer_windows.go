//go:build windows

package apps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

// WindowsAppEnforcer enforces application restrictions by detecting and terminating unauthorized processes.
type WindowsAppEnforcer struct {
	mu           sync.Mutex
	mockEnforced map[uint32]bool
	useMock      bool
}

// NewWindowsAppEnforcer creates a Windows enforcer.
func NewWindowsAppEnforcer() *WindowsAppEnforcer {
	return &WindowsAppEnforcer{
		mockEnforced: make(map[uint32]bool),
	}
}

// SetMockMode enables mock enforcement for unit testing.
func (e *WindowsAppEnforcer) SetMockMode(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.useMock = enabled
}

// CapabilityTier reports the supported enforcement tier on Windows.
func (e *WindowsAppEnforcer) CapabilityTier() string {
	return "Tier 1 - Detection & Response"
}

// Enforce restricts a process by terminating it after verifying identity and safety invariants.
func (e *WindowsAppEnforcer) Enforce(ctx context.Context, proc ProcessInfo, target TargetApp) (bool, error) {
	// 1. Strict immunity invariant
	if proc.PID <= 4 || int(proc.PID) == os.Getpid() || IsProtectedSystemProcess(proc.ExePath, proc.BaseName) {
		return false, fmt.Errorf("refusing to enforce against protected system process %s (PID: %d)", proc.BaseName, proc.PID)
	}

	e.mu.Lock()
	if e.useMock {
		e.mockEnforced[proc.PID] = true
		e.mu.Unlock()
		return true, nil
	}
	e.mu.Unlock()

	// 2. Open process handle with terminate permissions
	desiredAccess := uint32(windows.PROCESS_TERMINATE | windows.PROCESS_QUERY_LIMITED_INFORMATION)
	hProcess, err := windows.OpenProcess(desiredAccess, false, proc.PID)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			// Process already exited
			return true, nil
		}
		return false, fmt.Errorf("failed to open process %d for termination: %w", proc.PID, err)
	}
	defer windows.CloseHandle(hProcess)

	// 3. TOCTOU PID reuse check: Verify creation time matches before terminating
	if proc.StartTime > 0 {
		var creationTime, exitTime, kernelTime, userTime windows.Filetime
		if errTimes := windows.GetProcessTimes(hProcess, &creationTime, &exitTime, &kernelTime, &userTime); errTimes == nil {
			currentStart := creationTime.Nanoseconds()
			if currentStart > 0 && currentStart != proc.StartTime {
				return false, fmt.Errorf("PID %d was reused by new process (start time mismatch)", proc.PID)
			}
		}
	}

	// 4. Terminate process with exit code 1
	if err := windows.TerminateProcess(hProcess, 1); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			var exitCode uint32
			if errExit := windows.GetExitCodeProcess(hProcess, &exitCode); errExit == nil && exitCode != 259 {
				// Process has already exited or completed termination
				return true, nil
			}
		}
		return false, fmt.Errorf("failed to terminate process %d (%s): %w", proc.PID, proc.BaseName, err)
	}

	return true, nil
}
