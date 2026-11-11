//go:build !windows

package apps

import (
	"context"
	"testing"
	"time"
)

func TestLinuxProcessMonitor_MockLifecycleEvents(t *testing.T) {
	mon := NewLinuxProcessMonitor(50 * time.Millisecond)

	initialMock := []ProcessInfo{
		{PID: 100, BaseName: "bash", StartTime: 1000},
	}
	mon.SetMockProcesses(initialMock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mon.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer mon.Stop()

	// 1. Add process -> Start event
	updatedMock := []ProcessInfo{
		{PID: 100, BaseName: "bash", StartTime: 1000},
		{PID: 200, BaseName: "game", StartTime: 2000},
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

	// 2. Remove process -> Exit event
	exitMock := []ProcessInfo{
		{PID: 100, BaseName: "bash", StartTime: 1000},
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

