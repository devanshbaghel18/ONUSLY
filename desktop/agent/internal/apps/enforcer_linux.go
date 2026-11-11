//go:build !windows

package apps

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
)

// LinuxAppEnforcer enforces application restrictions on Linux using process signals.
type LinuxAppEnforcer struct {
	mu           sync.Mutex
	mockEnforced map[uint32]bool
	useMock      bool
}

func NewLinuxAppEnforcer() *LinuxAppEnforcer {
	return &LinuxAppEnforcer{
		mockEnforced: make(map[uint32]bool),
	}
}

func (e *LinuxAppEnforcer) SetMockMode(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.useMock = enabled
}

func (e *LinuxAppEnforcer) CapabilityTier() string {
	return "Tier 1 - Detection & Response"
}

func (e *LinuxAppEnforcer) Enforce(ctx context.Context, proc ProcessInfo, target TargetApp) (bool, error) {
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

	// 1. Send SIGTERM
	errTerm := syscall.Kill(int(proc.PID), syscall.SIGTERM)
	if errTerm != nil {
		if errTerm == syscall.ESRCH {
			return true, nil // Already exited
		}
		return false, fmt.Errorf("failed to send SIGTERM to PID %d: %w", proc.PID, errTerm)
	}

	// 2. Brief grace period
	time.Sleep(50 * time.Millisecond)

	// Check if process still exists
	if errCheck := syscall.Kill(int(proc.PID), 0); errCheck == nil {
		// Still running, send SIGKILL
		_ = syscall.Kill(int(proc.PID), syscall.SIGKILL)
	}

	return true, nil
}

