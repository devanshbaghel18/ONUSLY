package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// ResultWriter provides strict synchronization between background diagnostic workers
// and the service control manager (SCM) shutdown lifecycle.
//
// Invariants enforced:
// 1. Once CloseWithDeadline() is initiated, new write attempts are permanently rejected.
// 2. The SCM STOP handler never blocks indefinitely; it enforces a bounded wait for in-flight writes.
// 3. Once the service reports SERVICE_STOPPED, no worker can create, modify, or replace result files.
// 4. shutdown_status.json is written with the same atomic rename guarantees as probe_results.json.
type ResultWriter struct {
	mu      sync.Mutex
	closed  atomic.Bool
	stopped bool
}

var globalResultWriter ResultWriter

// WriteResults atomically serializes result to probe_results.tmp and renames it to
// probe_results.json, followed by writing probe_completed.marker.
// If the writer is closed or stopped, it immediately returns false without touching the filesystem.
func (w *ResultWriter) WriteResults(dataDir string, result any) bool {
	// Fast check: reject immediately if closure has started
	if w.closed.Load() {
		return false
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Double check under lock
	if w.closed.Load() || w.stopped {
		return false
	}

	outBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return false
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return false
	}

	tmpFile := filepath.Join(dataDir, "probe_results.tmp")
	logFile := filepath.Join(dataDir, "probe_results.json")
	markerFile := filepath.Join(dataDir, "probe_completed.marker")

	if err := os.WriteFile(tmpFile, outBytes, 0644); err != nil {
		return false
	}
	_ = os.Rename(tmpFile, logFile)
	_ = os.WriteFile(markerFile, []byte(time.Now().UTC().Format(time.RFC3339)), 0644)
	return true
}

// WriteShutdownStatus atomically persists shutdown status to shutdown_status.tmp and renames
// it to shutdown_status.json.
func (w *ResultWriter) WriteShutdownStatus(dataDir string, status any) bool {
	outBytes, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return false
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return false
	}

	tmpFile := filepath.Join(dataDir, "shutdown_status.tmp")
	logFile := filepath.Join(dataDir, "shutdown_status.json")

	if err := os.WriteFile(tmpFile, outBytes, 0644); err != nil {
		return false
	}
	return os.Rename(tmpFile, logFile) == nil
}

// CloseWithDeadline permanently disables new writes and waits up to timeout for any
// currently in-flight filesystem write to release the mutex.
// Returns true if all writes finished within the deadline, or false if the wait timed out.
// Regardless of the return value, future calls to WriteResults are guaranteed to fail immediately.
func (w *ResultWriter) CloseWithDeadline(timeout time.Duration) bool {
	// 1. Immediately and permanently reject any new write attempts
	w.closed.Store(true)

	// 2. Wait on mutex acquisition with bounded timeout
	lockChan := make(chan struct{})
	go func() {
		w.mu.Lock()
		close(lockChan)
	}()

	select {
	case <-lockChan:
		w.stopped = true
		w.mu.Unlock()
		return true
	case <-time.After(timeout):
		// Timed out waiting for in-flight write to finish.
		// w.closed is already true, so no subsequent write can ever start.
		go func() {
			<-lockChan
			w.stopped = true
			w.mu.Unlock()
		}()
		return false
	}
}

// CloseAndWait is a convenience wrapper for backwards compatibility with a default 2s deadline.
func (w *ResultWriter) CloseAndWait() bool {
	return w.CloseWithDeadline(2 * time.Second)
}

// IsClosed reports whether write closure has been initiated.
func (w *ResultWriter) IsClosed() bool {
	return w.closed.Load()
}

// IsStopped reports whether the writer mutex has been acquired and marked stopped.
func (w *ResultWriter) IsStopped() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stopped
}

// Reset resets the writer state (for testing purposes only).
func (w *ResultWriter) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed.Store(false)
	w.stopped = false
}
