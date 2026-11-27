//go:build windows

package main

import "testing"

func TestWindowsDefaultPipeMatchesAgentIPC(t *testing.T) {
	if got := getDefaultIPCPath(); got != `\\.\pipe\OnuslyAgentIPC` {
		t.Fatalf("expected default pipe path \\\\.\\pipe\\OnuslyAgentIPC, got %q", got)
	}
	if FallbackWindowsIPCPath != `\\.\pipe\OnuslyDesktopPipe` {
		t.Fatalf("expected fallback pipe path \\\\.\\pipe\\OnuslyDesktopPipe, got %q", FallbackWindowsIPCPath)
	}
}
