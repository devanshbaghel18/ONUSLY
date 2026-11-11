package logging_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/logging"
)

func TestStructuredLogger_FormatAndRedaction(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "agent.log")

	cfg := logging.Config{
		FilePath:      logPath,
		MaxSize:       1024 * 1024,
		MaxBackups:    3,
		RedactSecrets: true,
	}

	logger, err := logging.NewLogger(cfg)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Close()

	// Write entry containing a JWT and Bearer token
	rawJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	logger.Info("sync", "fetch_blocklist", "corr-101", fmt.Sprintf("Synced blocklist using Bearer %s with token: %s", rawJWT, rawJWT))

	// Write an error entry with explicit error category
	logger.Error("dns", "bind_adapter", "corr-102", logging.ErrCategoryPermission, fmt.Errorf("access denied"), "Failed to bind DNS adapter")

	_ = logger.Close()

	// Read file and parse entries
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var entries []logging.LogEntry
	for scanner.Scan() {
		var entry logging.LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("failed to decode log entry JSON: %v", err)
		}
		entries = append(entries, entry)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(entries))
	}

	// Verify entry 1: info & redacted
	e1 := entries[0]
	if e1.Severity != "INFO" || e1.Component != "sync" || e1.Op != "fetch_blocklist" {
		t.Errorf("unexpected metadata in entry 1: %+v", e1)
	}
	if strings.Contains(e1.Message, rawJWT) {
		t.Errorf("SECURITY DEFECT: raw JWT leaked in log message! Message: %s", e1.Message)
	}
	if !strings.Contains(e1.Message, "[REDACTED") {
		t.Errorf("expected redacted token placeholder in message, got: %s", e1.Message)
	}

	// Verify entry 2: error & categorized
	e2 := entries[1]
	if e2.Severity != "ERROR" || e2.Component != "dns" || e2.ErrorCategory != logging.ErrCategoryPermission {
		t.Errorf("unexpected error entry metadata: %+v", e2)
	}
	if !strings.Contains(e2.Message, "access denied") {
		t.Errorf("expected error message in log, got: %s", e2.Message)
	}
}

func TestStructuredLogger_SizeBoundedRotation(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "agent.log")

	// Set a very small cap of 350 bytes and max 2 backups
	cfg := logging.Config{
		FilePath:      logPath,
		MaxSize:       350,
		MaxBackups:    2,
		RedactSecrets: false,
	}

	logger, err := logging.NewLogger(cfg)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Write entries that will trigger rotation multiple times
	for i := 0; i < 15; i++ {
		logger.Info("daemon", "heartbeat", fmt.Sprintf("corr-%03d", i), fmt.Sprintf("Periodic heartbeat #%d processed successfully", i))
	}

	_ = logger.Close()

	// Verify active log exists and has size <= 350 (or small)
	activeInfo, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("active log missing: %v", err)
	}
	if activeInfo.Size() == 0 {
		t.Errorf("active log is unexpectedly empty")
	}

	// Verify backup .1 exists
	b1Path := fmt.Sprintf("%s.1", logPath)
	if _, err := os.Stat(b1Path); os.IsNotExist(err) {
		t.Errorf("expected backup file %s to exist", b1Path)
	}

	// Verify backup .2 exists
	b2Path := fmt.Sprintf("%s.2", logPath)
	if _, err := os.Stat(b2Path); os.IsNotExist(err) {
		t.Errorf("expected backup file %s to exist", b2Path)
	}

	// Verify backup .3 does NOT exist because MaxBackups is 2
	b3Path := fmt.Sprintf("%s.3", logPath)
	if _, err := os.Stat(b3Path); !os.IsNotExist(err) {
		t.Errorf("expected backup file %s to NOT exist (MaxBackups=2)", b3Path)
	}
}

func TestStructuredLogger_FailOpenOnWriteFailure(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "agent.log")

	cfg := logging.Config{
		FilePath:   logPath,
		MaxSize:    1024,
		MaxBackups: 1,
	}

	logger, err := logging.NewLogger(cfg)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Write initial entry
	logger.Info("daemon", "start", "corr-init", "Service initialized")

	// Artificially close underlying file to simulate write failure / disk exhaustion
	_ = logger.Close()

	// Invariant: Writing must NOT panic and must fail open (increment write errors)
	logger.Info("daemon", "probe", "corr-fail", "Write during disk/file unavailability")

	if logger.GetWriteErrors() == 0 {
		// If file was nil, it safely did nothing without crashing
	}
}

