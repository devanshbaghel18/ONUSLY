//go:build !windows

package apps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LinuxProcessMonitor observes running processes on Linux by traversing /proc.
type LinuxProcessMonitor struct {
	mu           sync.Mutex
	running      bool
	cancelFunc   context.CancelFunc
	eventChan    chan ProcessEvent
	known        map[uint32]int64 // PID -> StartTime
	pollInterval time.Duration
	mockProcs    []ProcessInfo
}

func NewLinuxProcessMonitor(interval time.Duration) *LinuxProcessMonitor {
	if interval <= 0 {
		interval = 1 * time.Second
	}
	return &LinuxProcessMonitor{
		eventChan:    make(chan ProcessEvent, 500),
		known:        make(map[uint32]int64),
		pollInterval: interval,
	}
}

func (m *LinuxProcessMonitor) SetMockProcesses(procs []ProcessInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mockProcs = procs
}

func (m *LinuxProcessMonitor) Events() <-chan ProcessEvent {
	return m.eventChan
}

func (m *LinuxProcessMonitor) Snapshot(ctx context.Context) ([]ProcessInfo, error) {
	m.mu.Lock()
	if len(m.mockProcs) > 0 {
		defer m.mu.Unlock()
		res := make([]ProcessInfo, len(m.mockProcs))
		copy(res, m.mockProcs)
		return res, nil
	}
	m.mu.Unlock()

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc: %w", err)
	}

	var results []ProcessInfo

	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if !entry.IsDir() {
			continue
		}

		pid64, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil {
			continue // Not a numeric PID directory
		}
		pid := uint32(pid64)

		info := inspectLinuxProcess(pid)
		results = append(results, info)
	}

	return results, nil
}

func inspectLinuxProcess(pid uint32) ProcessInfo {
	info := ProcessInfo{
		PID:        pid,
		Confidence: ConfidenceBaseName,
	}

	// 1. Read executable link
	exeLink, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err == nil {
		info.ExePath = exeLink
		info.BaseName = filepath.Base(exeLink)
		info.Confidence = ConfidenceExactPath
	} else {
		// Fall back to comm if exe symlink is unreadable (e.g. kernel worker or permission restricted)
		commData, errComm := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if errComm == nil {
			info.BaseName = strings.TrimSpace(string(commData))
		}
	}

	info.IsProtected = IsProtectedSystemProcess(info.ExePath, info.BaseName)

	// 2. Read stat file for PPID and start time
	statData, errStat := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if errStat == nil {
		fields := strings.Fields(string(statData))
		if len(fields) >= 22 {
			if ppid, errP := strconv.ParseUint(fields[3], 10, 32); errP == nil {
				info.PPID = uint32(ppid)
			}
			if startTicks, errS := strconv.ParseInt(fields[21], 10, 64); errS == nil {
				info.StartTime = startTicks
			}
		}
	}

	return info
}

func (m *LinuxProcessMonitor) Start(parentCtx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(parentCtx)
	m.cancelFunc = cancel
	m.running = true
	m.mu.Unlock()

	initialProcs, err := m.Snapshot(ctx)
	if err == nil {
		m.mu.Lock()
		for _, p := range initialProcs {
			m.known[p.PID] = p.StartTime
		}
		m.mu.Unlock()
	}

	go m.reconciliationLoop(ctx)
	return nil
}

func (m *LinuxProcessMonitor) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return nil
	}
	m.running = false
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	return nil
}

func (m *LinuxProcessMonitor) reconciliationLoop(ctx context.Context) {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentProcs, err := m.Snapshot(ctx)
			if err != nil {
				continue
			}

			currentMap := make(map[uint32]ProcessInfo, len(currentProcs))
			for _, p := range currentProcs {
				currentMap[p.PID] = p
			}

			m.mu.Lock()
			for pid, proc := range currentMap {
				prevStart, exists := m.known[pid]
				if !exists || (proc.StartTime > 0 && prevStart > 0 && prevStart != proc.StartTime) {
					m.known[pid] = proc.StartTime
					m.emitEvent(ProcessEvent{
						Type:      ProcessEventStart,
						Process:   proc,
						Timestamp: time.Now().UTC(),
					})
				}
			}

			for pid, prevStart := range m.known {
				if _, exists := currentMap[pid]; !exists {
					delete(m.known, pid)
					m.emitEvent(ProcessEvent{
						Type: ProcessEventExit,
						Process: ProcessInfo{
							PID:       pid,
							StartTime: prevStart,
						},
						Timestamp: time.Now().UTC(),
					})
				}
			}
			m.mu.Unlock()
		}
	}
}

func (m *LinuxProcessMonitor) emitEvent(event ProcessEvent) {
	select {
	case m.eventChan <- event:
	default:
	}
}

