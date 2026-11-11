package apps

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

// AppEnforcementController coordinates process monitoring, policy evaluation, and platform enforcement.
type AppEnforcementController struct {
	monitor    ProcessMonitor
	policyMgr  *AppPolicyManager
	enforcer   AppEnforcer
	inventory  InventoryScanner
	mu         sync.RWMutex
	running    bool
	cancelFunc context.CancelFunc
	stats      AppEnforcementStats
	eventLog         []EnforcementEvent
	maxEvents        int
	recentlyEnforced map[uint32]time.Time

	// Atomic counters for lock-free reporting
	totalMonitored atomic.Uint64
	totalDetected  atomic.Uint64
	totalEnforced  atomic.Uint64
	totalFailed    atomic.Uint64
}

// NewAppEnforcementController initializes a new application enforcement subsystem.
func NewAppEnforcementController(
	monitor ProcessMonitor,
	policyMgr *AppPolicyManager,
	enforcer AppEnforcer,
	inventory InventoryScanner,
) *AppEnforcementController {
	if policyMgr == nil {
		policyMgr = NewAppPolicyManager()
	}
	return &AppEnforcementController{
		monitor:          monitor,
		policyMgr:        policyMgr,
		enforcer:         enforcer,
		inventory:        inventory,
		maxEvents:        100,
		recentlyEnforced: make(map[uint32]time.Time),
	}
}

// Start activates the process monitor and begins real-time enforcement and reconciliation.
func (c *AppEnforcementController) Start(parentCtx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(parentCtx)
	c.cancelFunc = cancel
	c.running = true
	c.mu.Unlock()

	if err := c.monitor.Start(ctx); err != nil {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
		cancel()
		return fmt.Errorf("failed to start process monitor: %w", err)
	}

	// 1. Initial reconciliation
	go c.reconcileOnce(ctx)

	// 2. Event listener loop
	go c.eventLoop(ctx)

	// 3. Periodic full reconciliation loop
	go c.reconciliationLoop(ctx)

	return nil
}

// Stop cleanly terminates the process monitor and enforcement loops.
func (c *AppEnforcementController) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return nil
	}
	c.running = false
	if c.cancelFunc != nil {
		c.cancelFunc()
	}
	_ = c.monitor.Stop()
	return nil
}

// IsRunning reports whether the controller is active.
func (c *AppEnforcementController) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// UpdateBlocklist updates the authoritative policy snapshot and triggers immediate reconciliation.
func (c *AppEnforcementController) UpdateBlocklist(data *core.BlocklistData) {
	c.policyMgr.UpdateFromBlocklist(data)
	c.mu.RLock()
	running := c.running
	c.mu.RUnlock()

	if running {
		go c.reconcileOnce(context.Background())
	}
}

// GetPolicyManager returns the policy manager.
func (c *AppEnforcementController) GetPolicyManager() *AppPolicyManager {
	return c.policyMgr
}

// GetInventoryScanner returns the inventory scanner.
func (c *AppEnforcementController) GetInventoryScanner() InventoryScanner {
	return c.inventory
}

// Stats returns current aggregate enforcement metrics (alias for GetStats).
func (c *AppEnforcementController) Stats() AppEnforcementStats {
	return c.GetStats()
}

// GetStats returns current aggregate enforcement metrics.
func (c *AppEnforcementController) GetStats() AppEnforcementStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return AppEnforcementStats{
		MonitoredProcesses: c.totalMonitored.Load(),
		RestrictedDetected: c.totalDetected.Load(),
		EnforcedTerminated: c.totalEnforced.Load(),
		EnforcementFailed:  c.totalFailed.Load(),
		LastReconciled:     c.stats.LastReconciled,
	}
}

// GetRecentEvents returns a copy of recently recorded enforcement events.
func (c *AppEnforcementController) GetRecentEvents() []EnforcementEvent {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]EnforcementEvent, len(c.eventLog))
	copy(res, c.eventLog)
	return res
}

func (c *AppEnforcementController) eventLoop(ctx context.Context) {
	events := c.monitor.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Type == ProcessEventStart {
				c.handleProcessStart(ctx, ev.Process)
			}
		}
	}
}

func (c *AppEnforcementController) handleProcessStart(ctx context.Context, proc ProcessInfo) {
	target, state, conf, reason := c.policyMgr.EvaluateProcess(proc)
	if state != StateRestricted {
		return
	}

	c.mu.Lock()
	if lastEnforced, exists := c.recentlyEnforced[proc.PID]; exists && time.Since(lastEnforced) < 5*time.Second {
		c.mu.Unlock()
		return
	}
	c.recentlyEnforced[proc.PID] = time.Now()
	if len(c.recentlyEnforced) > 200 {
		cutoff := time.Now().Add(-10 * time.Second)
		for pid, t := range c.recentlyEnforced {
			if t.Before(cutoff) {
				delete(c.recentlyEnforced, pid)
			}
		}
	}
	c.mu.Unlock()

	c.totalDetected.Add(1)
	log.Printf("[App Enforcement] Restricted process detected: %s (PID: %d, Goal: %s, Confidence: %s)",
		proc.BaseName, proc.PID, target.GoalTitle, conf)

	tier := c.enforcer.CapabilityTier()
	success, err := c.enforcer.Enforce(ctx, proc, target)
	nowStr := time.Now().UTC().Format(time.RFC3339)

	event := EnforcementEvent{
		Timestamp:   nowStr,
		PID:         proc.PID,
		ProcessName: proc.BaseName,
		ExePath:     proc.ExePath,
		AppID:       target.AppID,
		GoalID:      target.GoalID,
		Action:      "terminated",
		Success:     success,
		Reason:      reason,
		Tier:        tier,
	}

	if success {
		c.totalEnforced.Add(1)
		log.Printf("[App Enforcement] Successfully enforced restriction on PID %d (%s)", proc.PID, proc.BaseName)
	} else {
		c.totalFailed.Add(1)
		event.Action = "failed"
		event.Reason = fmt.Sprintf("enforcement failed: %v", err)
		log.Printf("[App Enforcement] Notice: enforcement on PID %d failed: %v", proc.PID, err)
	}

	c.recordEvent(event)
}

func (c *AppEnforcementController) reconciliationLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.reconcileOnce(ctx)
		}
	}
}

func (c *AppEnforcementController) reconcileOnce(ctx context.Context) {
	snapCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	procs, err := c.monitor.Snapshot(snapCtx)
	cancel()
	if err != nil {
		return
	}

	c.totalMonitored.Store(uint64(len(procs)))

	for _, proc := range procs {
		_, state, _, _ := c.policyMgr.EvaluateProcess(proc)
		if state == StateRestricted {
			c.handleProcessStart(ctx, proc)
		}
	}

	c.mu.Lock()
	c.stats.LastReconciled = time.Now().UTC()
	c.mu.Unlock()
}

func (c *AppEnforcementController) recordEvent(ev EnforcementEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.eventLog) >= c.maxEvents {
		c.eventLog = c.eventLog[1:]
	}
	c.eventLog = append(c.eventLog, ev)
}
