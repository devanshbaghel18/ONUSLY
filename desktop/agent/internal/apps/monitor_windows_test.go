//go:build windows

package apps

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestWindowsProcessMonitor_LiveSnapshot(t *testing.T) {
	mon := NewWindowsProcessMonitor(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	procs, err := mon.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Live snapshot failed: %v", err)
	}

	if len(procs) == 0 {
		t.Fatalf("Expected non-zero processes from live snapshot")
	}

	t.Logf("Observed %d live processes on Windows host", len(procs))

	// Verify our own process is present in the snapshot
	myPid := uint32(os.Getpid())
	foundSelf := false
	for _, p := range procs {
		if p.PID == myPid {
			foundSelf = true
			if p.StartTime == 0 {
				t.Errorf("Expected non-zero creation time for self process")
			}
			t.Logf("Successfully inspected own process: PID=%d, ExePath=%s, StartTime=%d", p.PID, p.ExePath, p.StartTime)
			break
		}
	}

	if !foundSelf {
		t.Errorf("Own PID %d not found in snapshot results", myPid)
	}
}

func TestWindowsProcessMonitor_MockLifecycleEvents(t *testing.T) {
	mon := NewWindowsProcessMonitor(50 * time.Millisecond)

	initialMock := []ProcessInfo{
		{PID: 100, BaseName: "notepad.exe", StartTime: 1000},
	}
	mon.SetMockProcesses(initialMock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mon.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer mon.Stop()

	// 1. Add new process -> expect Start event
	updatedMock := []ProcessInfo{
		{PID: 100, BaseName: "notepad.exe", StartTime: 1000},
		{PID: 200, BaseName: "game.exe", StartTime: 2000},
	}
	mon.SetMockProcesses(updatedMock)

	select {
	case ev := <-mon.Events():
		if ev.Type != ProcessEventStart || ev.Process.PID != 200 {
			t.Errorf("Expected start event for PID 200, got %+v", ev)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("Timed out waiting for start event")
	}

	// 2. Remove process -> expect Exit event
	exitMock := []ProcessInfo{
		{PID: 100, BaseName: "notepad.exe", StartTime: 1000},
	}
	mon.SetMockProcesses(exitMock)

	select {
	case ev := <-mon.Events():
		if ev.Type != ProcessEventExit || ev.Process.PID != 200 {
			t.Errorf("Expected exit event for PID 200, got %+v", ev)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("Timed out waiting for exit event")
	}
}

