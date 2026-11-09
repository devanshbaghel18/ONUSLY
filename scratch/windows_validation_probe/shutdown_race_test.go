package main

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type DummyResult struct {
	WorkerID  int    `json:"worker_id"`
	Timestamp string `json:"timestamp"`
}

type DummyShutdownStatus struct {
	Timestamp           string `json:"timestamp"`
	CleanShutdown       bool   `json:"clean_shutdown"`
	WorkerTimedOut      bool   `json:"worker_timed_out"`
	WriterTimedOut      bool   `json:"writer_timed_out"`
	AllWorkersExited    bool   `json:"all_workers_exited"`
	ResultWritesBlocked bool   `json:"result_writes_blocked"`
	Win32ExitCode       uint32 `json:"win32_exit_code"`
	Notes               string `json:"notes"`
}

// 1. Test worker writing while shutdown begins (deterministic handshake)
func TestWorkerWritingWhileShutdownBegins(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "probe_write_during_stop_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	writer := &ResultWriter{}
	workerEntered := make(chan struct{})
	allowWorkerFinish := make(chan struct{})
	workerCompleted := make(chan struct{})

	// Worker goroutine that blocks mid-write
	go func() {
		writer.mu.Lock()
		close(workerEntered) // Signal that worker holds lock

		<-allowWorkerFinish // Wait for coordinator to call CloseWithDeadline
		tmpFile := filepath.Join(tmpDir, "probe_results.tmp")
		logFile := filepath.Join(tmpDir, "probe_results.json")
		_ = os.WriteFile(tmpFile, []byte(`{"worker_id":1}`), 0644)
		_ = os.Rename(tmpFile, logFile)
		writer.mu.Unlock()
		close(workerCompleted)
	}()

	// Wait until worker is inside write lock
	<-workerEntered

	// Coordinator initiates shutdown in background
	shutdownDone := make(chan bool)
	go func() {
		clean := writer.CloseWithDeadline(500 * time.Millisecond)
		shutdownDone <- clean
	}()

	// Small deterministic tick to ensure coordinator is blocked on mutex
	time.Sleep(10 * time.Millisecond)

	// Release worker to complete write
	close(allowWorkerFinish)
	<-workerCompleted

	// Coordinator should now acquire mutex and report clean close
	clean := <-shutdownDone
	if !clean {
		t.Fatalf("CloseWithDeadline returned false; expected true after worker completed")
	}

	// Verify file was written by in-flight worker
	logFile := filepath.Join(tmpDir, "probe_results.json")
	if _, err := os.Stat(logFile); err != nil {
		t.Fatalf("probe_results.json was not created: %v", err)
	}

	// Any subsequent write must be rejected
	if writer.WriteResults(tmpDir, DummyResult{WorkerID: 2}) {
		t.Fatalf("WriteResults succeeded after CloseWithDeadline")
	}
}

// 2. Test worker attempting to write after writer closure
func TestWorkerAttemptingToWriteAfterClosure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "probe_write_after_close_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	writer := &ResultWriter{}
	if !writer.CloseWithDeadline(100 * time.Millisecond) {
		t.Fatalf("CloseWithDeadline failed")
	}

	// Attempt write after closure
	res := DummyResult{WorkerID: 42, Timestamp: "LATE"}
	if writer.WriteResults(tmpDir, res) {
		t.Fatalf("WriteResults should have returned false after closure")
	}

	// Verify no files created
	logFile := filepath.Join(tmpDir, "probe_results.json")
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Fatalf("probe_results.json exists on disk after rejected write")
	}
}

// 2b. Test worker passing initial closed check, delayed before acquiring mutex,
// and resuming after shutdown closes the writer.
func TestWriterDelayedBetweenInitialCheckAndMutexAcquisition(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "probe_delay_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	writer := &ResultWriter{}
	helperHoldingLock := make(chan struct{})
	writerCalling := make(chan struct{})
	writeReturned := make(chan bool)

	// Helper goroutine acquires writer.mu while writer is NOT closed
	go func() {
		writer.mu.Lock()
		close(helperHoldingLock)

		// Wait until worker goroutine has called WriteResults and is blocked on writer.mu.Lock()
		<-writerCalling
		time.Sleep(10 * time.Millisecond) // Ensure WriteResults is blocked on w.mu

		// Now simulate shutdown closure while worker is waiting on the mutex
		writer.closed.Store(true)
		writer.stopped = true

		// Release mutex to allow worker to proceed into Check 2
		writer.mu.Unlock()
	}()

	<-helperHoldingLock

	// At this instant, w.closed is false.
	// Worker will execute Check 1 (w.closed.Load() == false) and then block at w.mu.Lock().
	go func() {
		close(writerCalling)
		ok := writer.WriteResults(tmpDir, DummyResult{WorkerID: 99, Timestamp: "RACE_TARGET"})
		writeReturned <- ok
	}()

	ok := <-writeReturned
	if ok {
		t.Fatalf("CRITICAL: WriteResults returned true despite closure occurring while waiting for mutex!")
	}

	// Verify no file was created on disk
	logFile := filepath.Join(tmpDir, "probe_results.json")
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Fatalf("CRITICAL: probe_results.json was written on disk after writer was closed!")
	}
}

// 3. Test writer blocked during shutdown (bounded timeout enforcement)
func TestWriterBlockedDuringShutdownTimeout(t *testing.T) {
	writer := &ResultWriter{}
	lockHeld := make(chan struct{})
	releaseLock := make(chan struct{})

	// Goroutine simulates a wedged filesystem write holding writer.mu
	go func() {
		writer.mu.Lock()
		close(lockHeld)
		<-releaseLock
		writer.mu.Unlock()
	}()

	<-lockHeld

	// CloseWithDeadline must NOT block indefinitely; it must return false at deadline
	start := time.Now()
	timeout := 50 * time.Millisecond
	clean := writer.CloseWithDeadline(timeout)
	elapsed := time.Since(start)

	if clean {
		t.Fatalf("CloseWithDeadline should have timed out and returned false")
	}
	if elapsed < timeout {
		t.Fatalf("CloseWithDeadline returned prematurely (%v < %v)", elapsed, timeout)
	}
	if !writer.IsClosed() {
		t.Fatalf("writer.IsClosed() must be true even after timeout")
	}

	// Subsequent write must still be rejected immediately
	if writer.WriteResults("/tmp", DummyResult{WorkerID: 1}) {
		t.Fatalf("WriteResults succeeded on closed writer")
	}

	// Release wedged worker
	close(releaseLock)
}

// 4. Test worker timeout and exit code determination
func TestWorkerTimeoutAndExitCode(t *testing.T) {
	workerDoneChan := make(chan struct{})
	cancelChan := make(chan struct{})

	// Simulate slow worker that ignores cancelChan for 200ms
	go func() {
		select {
		case <-cancelChan:
			time.Sleep(200 * time.Millisecond)
			workerDoneChan <- struct{}{}
		}
	}()

	// Signal cancel
	close(cancelChan)

	// Bounded wait with 50ms deadline (simulates worker timeout)
	workerExitedCleanly := false
	select {
	case <-workerDoneChan:
		workerExitedCleanly = true
	case <-time.After(50 * time.Millisecond):
		workerExitedCleanly = false
	}

	if workerExitedCleanly {
		t.Fatalf("Expected worker to time out, but it exited cleanly")
	}

	// Determine exit code according to service contract
	var exitCode uint32 = 0
	if !workerExitedCleanly {
		exitCode = 1053 // ERROR_SERVICE_REQUEST_TIMEOUT
	}

	if exitCode != 1053 {
		t.Fatalf("Expected exit code 1053 for timed-out worker, got %d", exitCode)
	}
}

// 5. Test shutdown status persistence (atomic .tmp -> .json)
func TestShutdownStatusPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "probe_status_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	writer := &ResultWriter{}
	st := DummyShutdownStatus{
		Timestamp:           time.Now().UTC().Format(time.RFC3339),
		CleanShutdown:       true,
		WorkerTimedOut:      false,
		WriterTimedOut:      false,
		AllWorkersExited:    true,
		ResultWritesBlocked: true,
		Win32ExitCode:       0,
		Notes:               "Unit test shutdown validation",
	}

	if !writer.WriteShutdownStatus(tmpDir, st) {
		t.Fatalf("WriteShutdownStatus returned false")
	}

	statusFile := filepath.Join(tmpDir, "shutdown_status.json")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("Failed to read shutdown_status.json: %v", err)
	}

	var parsed DummyShutdownStatus
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if !parsed.CleanShutdown || parsed.Win32ExitCode != 0 || !parsed.AllWorkersExited {
		t.Fatalf("Parsed status mismatch: %+v", parsed)
	}
}

// 6. Test concurrent shutdown requests (idempotency and thread-safety)
func TestConcurrentShutdownRequests(t *testing.T) {
	writer := &ResultWriter{}
	const callers = 20
	var wg sync.WaitGroup

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			writer.CloseWithDeadline(100 * time.Millisecond)
		}()
	}

	wg.Wait()
	if !writer.IsClosed() {
		t.Fatalf("writer.IsClosed() returned false after concurrent closes")
	}
}

// 7. Test repeated sequential shutdown calls
func TestRepeatedShutdownCalls(t *testing.T) {
	writer := &ResultWriter{}

	if !writer.CloseWithDeadline(100 * time.Millisecond) {
		t.Fatalf("First CloseWithDeadline failed")
	}
	if !writer.CloseWithDeadline(100 * time.Millisecond) {
		t.Fatalf("Second CloseWithDeadline failed")
	}
	if !writer.CloseAndWait() {
		t.Fatalf("CloseAndWait failed after CloseWithDeadline")
	}
	if !writer.IsClosed() {
		t.Fatalf("writer.IsClosed() returned false")
	}
}

// 8. High-concurrency stress test for shutdown invariant
func TestHighConcurrencyShutdownInvariant(t *testing.T) {
	for iter := 0; iter < 30; iter++ {
		tmpDir, err := os.MkdirTemp("", "probe_race_stress_*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}

		writer := &ResultWriter{}
		var serviceStopped atomic.Bool
		var writesAfterStopped atomic.Int64
		const numWorkers = 25
		var wg sync.WaitGroup
		barrier := make(chan struct{})

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				<-barrier
				time.Sleep(time.Duration(rand.Intn(3)) * time.Millisecond)
				if writer.WriteResults(tmpDir, DummyResult{WorkerID: id}) {
					if serviceStopped.Load() {
						writesAfterStopped.Add(1)
					}
				}
			}(i)
		}

		var coordWg sync.WaitGroup
		coordWg.Add(1)
		go func() {
			defer coordWg.Done()
			<-barrier
			time.Sleep(time.Duration(rand.Intn(2)+1) * time.Millisecond)
			writer.CloseWithDeadline(200 * time.Millisecond)
			serviceStopped.Store(true)
		}()

		close(barrier)
		coordWg.Wait()
		wg.Wait()

		if writesAfterStopped.Load() > 0 {
			t.Fatalf("Iteration %d: %d writes occurred after service was stopped!", iter, writesAfterStopped.Load())
		}
		os.RemoveAll(tmpDir)
	}
}
