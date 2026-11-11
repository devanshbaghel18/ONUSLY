//go:build windows

package apps

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WindowsProcessMonitor observes running processes and lifecycle events using Win32 Toolhelp APIs.
type WindowsProcessMonitor struct {
	mu           sync.Mutex
	running      bool
	cancelFunc   context.CancelFunc
	eventChan    chan ProcessEvent
	known        map[uint32]int64 // PID -> StartTime
	pollInterval time.Duration
	mockProcs    []ProcessInfo
}

// NewWindowsProcessMonitor creates a process monitor with the given polling interval.
func NewWindowsProcessMonitor(interval time.Duration) *WindowsProcessMonitor {
	if interval <= 0 {
		interval = 1 * time.Second
	}
	return &WindowsProcessMonitor{
		eventChan:    make(chan ProcessEvent, 500),
		known:        make(map[uint32]int64),
		pollInterval: interval,
	}
}

// SetMockProcesses sets mock process list for testing.
func (m *WindowsProcessMonitor) SetMockProcesses(procs []ProcessInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mockProcs = procs
}

// Events returns the process event channel.
func (m *WindowsProcessMonitor) Events() <-chan ProcessEvent {
	return m.eventChan
}

// Snapshot takes a point-in-time inventory of all currently running processes.
func (m *WindowsProcessMonitor) Snapshot(ctx context.Context) ([]ProcessInfo, error) {
	m.mu.Lock()
	if len(m.mockProcs) > 0 {
		defer m.mu.Unlock()
		res := make([]ProcessInfo, len(m.mockProcs))
		copy(res, m.mockProcs)
		return res, nil
	}
	m.mu.Unlock()

	hSnapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(hSnapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := windows.Process32First(hSnapshot, &entry); err != nil {
		return nil, err
	}

	var results []ProcessInfo

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		pid := entry.ProcessID
		ppid := entry.ParentProcessID
		baseName := windows.UTF16ToString(entry.ExeFile[:])

		if pid > 0 {
			info := inspectWindowsProcess(pid, ppid, baseName)
			results = append(results, info)
		}

		if err := windows.Process32Next(hSnapshot, &entry); err != nil {
			break
		}
	}

	return results, nil
}

// inspectWindowsProcess gathers detailed process metadata with graceful handling of access restrictions.
func inspectWindowsProcess(pid, ppid uint32, baseName string) ProcessInfo {
	info := ProcessInfo{
		PID:         pid,
		PPID:        ppid,
		BaseName:    baseName,
		Confidence:  ConfidenceBaseName,
		IsProtected: IsProtectedSystemProcess("", baseName),
	}

	// Request limited query information (accessible to LocalService without SeDebugPrivilege)
	hProcess, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		// Access denied or process exited during inspection: record with base metadata
		return info
	}
	defer windows.CloseHandle(hProcess)

	// 1. Query full executable path
	var pathBuf [windows.MAX_PATH * 2]uint16
	pathSize := uint32(len(pathBuf))
	if err := windows.QueryFullProcessImageName(hProcess, 0, &pathBuf[0], &pathSize); err == nil {
		info.ExePath = windows.UTF16ToString(pathBuf[:pathSize])
		info.Confidence = ConfidenceExactPath
		info.IsProtected = IsProtectedSystemProcess(info.ExePath, baseName)
	}

	// 2. Query process creation time (for PID reuse detection)
	var creationTime, exitTime, kernelTime, userTime windows.Filetime
	if err := windows.GetProcessTimes(hProcess, &creationTime, &exitTime, &kernelTime, &userTime); err == nil {
		info.StartTime = creationTime.Nanoseconds()
	}

	// 3. Query terminal session ID
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(pid, &sessionID); err == nil {
		info.SessionID = sessionID
	}

	return info
}

// Start begins periodic process monitoring and event generation.
func (m *WindowsProcessMonitor) Start(parentCtx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(parentCtx)
	m.cancelFunc = cancel
	m.running = true
	m.mu.Unlock()

	// Initial snapshot population
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

// Stop cleanly terminates the process monitor.
func (m *WindowsProcessMonitor) Stop() error {
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

func (m *WindowsProcessMonitor) reconciliationLoop(ctx context.Context) {
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
			// Detect starts and PID reuse
			for pid, proc := range currentMap {
				prevStart, exists := m.known[pid]
				if !exists || (proc.StartTime > 0 && prevStart > 0 && prevStart != proc.StartTime) {
					// New process or reused PID
					m.known[pid] = proc.StartTime
					m.emitEvent(ProcessEvent{
						Type:      ProcessEventStart,
						Process:   proc,
						Timestamp: time.Now().UTC(),
					})
				}
			}

			// Detect exits
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

func (m *WindowsProcessMonitor) emitEvent(event ProcessEvent) {
	select {
	case m.eventChan <- event:
	default:
		// Channel buffer full: drop event rather than deadlocking reconciliation
	}
}
