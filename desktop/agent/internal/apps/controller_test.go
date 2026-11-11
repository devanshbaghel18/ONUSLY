package apps

import (
	"context"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

// mockEnforcer implements AppEnforcer for test verification.
type mockEnforcer struct {
	enforcedPIDs []uint32
}

func (m *mockEnforcer) Enforce(ctx context.Context, proc ProcessInfo, target TargetApp) (bool, error) {
	m.enforcedPIDs = append(m.enforcedPIDs, proc.PID)
	return true, nil
}

func (m *mockEnforcer) CapabilityTier() string {
	return "Tier 1 - Detection & Response (Mock)"
}

// mockMonitor implements ProcessMonitor for test verification.
type mockMonitor struct {
	procs     []ProcessInfo
	eventChan chan ProcessEvent
}

func newMockMonitor() *mockMonitor {
	return &mockMonitor{
		eventChan: make(chan ProcessEvent, 10),
	}
}

func (m *mockMonitor) Start(ctx context.Context) error {
	return nil
}

func (m *mockMonitor) Stop() error {
	return nil
}

func (m *mockMonitor) Snapshot(ctx context.Context) ([]ProcessInfo, error) {
	res := make([]ProcessInfo, len(m.procs))
	copy(res, m.procs)
	return res, nil
}

func (m *mockMonitor) Events() <-chan ProcessEvent {
	return m.eventChan
}

func TestAppEnforcementController_LifecycleAndEnforcement(t *testing.T) {
	mon := newMockMonitor()
	policyMgr := NewAppPolicyManager()
	enforcer := &mockEnforcer{}

	ctrl := NewAppEnforcementController(mon, policyMgr, enforcer, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("Controller start failed: %v", err)
	}
	defer ctrl.Stop()

	// 1. Configure active blocklist targeting discord
	blocklist := &core.BlocklistData{
		Version: "v1",
		Targets: []core.TargetItem{
			{
				GoalID:    "g-focus",
				GoalTitle: "Stop Distractions",
				Status:    "active",
				Apps:      []string{"discord"},
			},
		},
	}
	ctrl.UpdateBlocklist(blocklist)

	// 2. Simulate launch of Discord.exe
	discordProc := ProcessInfo{
		PID:      1234,
		BaseName: "Discord.exe",
		ExePath:  `C:\Users\test\AppData\Local\Discord\Discord.exe`,
	}
	mon.eventChan <- ProcessEvent{
		Type:      ProcessEventStart,
		Process:   discordProc,
		Timestamp: time.Now().UTC(),
	}

	// Wait up to 1s for controller to enforce
	time.Sleep(150 * time.Millisecond)

	stats := ctrl.GetStats()
	if stats.RestrictedDetected != 1 {
		t.Errorf("Expected 1 restricted process detected, got %d", stats.RestrictedDetected)
	}
	if stats.EnforcedTerminated != 1 {
		t.Errorf("Expected 1 enforced termination, got %d", stats.EnforcedTerminated)
	}

	events := ctrl.GetRecentEvents()
	if len(events) != 1 || events[0].PID != 1234 || events[0].AppID != "discord" {
		t.Errorf("Unexpected recorded event: %+v", events)
	}

	// 3. Goal completed -> dynamic unblock
	blocklist.Targets[0].Status = "completed"
	blocklist.Version = "v2"
	ctrl.UpdateBlocklist(blocklist)

	// Simulate second launch of Discord.exe
	mon.eventChan <- ProcessEvent{
		Type: ProcessEventStart,
		Process: ProcessInfo{
			PID:      5678,
			BaseName: "Discord.exe",
		},
		Timestamp: time.Now().UTC(),
	}

	time.Sleep(150 * time.Millisecond)

	statsAfter := ctrl.GetStats()
	if statsAfter.RestrictedDetected != 1 {
		t.Errorf("Expected RestrictedDetected to remain 1 after completion, got %d", statsAfter.RestrictedDetected)
	}
	if statsAfter.EnforcedTerminated != 1 {
		t.Errorf("Expected EnforcedTerminated to remain 1 after completion, got %d", statsAfter.EnforcedTerminated)
	}
}

