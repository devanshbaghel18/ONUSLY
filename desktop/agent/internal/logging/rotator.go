package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

// Standard Error Categories for structured operational diagnostics
const (
	ErrCategoryNone       = "none"
	ErrCategoryAuth       = "auth"
	ErrCategoryNetwork    = "network"
	ErrCategoryPermission = "permission"
	ErrCategoryDriver     = "driver"
	ErrCategoryDisk       = "disk"
	ErrCategoryTimeout    = "timeout"
	ErrCategorySystem     = "system"
)

var (
	jwtRegex    = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+`)
	bearerRegex = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9_.\-]+`)
	secretRegex = regexp.MustCompile(`(?i)(token|secret|password|private_key)\s*[:=]\s*["']?([^"'\s]+)["']?`)
)

// Redact cleans sensitive secrets, tokens, and credentials from log strings.
func Redact(msg string) string {
	msg = jwtRegex.ReplaceAllString(msg, "[REDACTED_JWT]")
	msg = bearerRegex.ReplaceAllString(msg, "Bearer [REDACTED]")
	msg = secretRegex.ReplaceAllString(msg, "$1: [REDACTED]")
	return msg
}

// LogEntry represents an authoritative structured log line.
type LogEntry struct {
	Timestamp     string `json:"timestamp"`
	Severity      string `json:"severity"`       // INFO, WARN, ERROR, FATAL
	Component     string `json:"component"`      // daemon, dns, apps, sync, ipc, update
	Op            string `json:"op"`             // operation being performed
	CorrelationID string `json:"correlation_id"` // trace or transaction ID
	ErrorCategory string `json:"error_category"` // none, auth, network, permission, etc.
	Version       string `json:"version"`
	Platform      string `json:"platform"`
	Message       string `json:"message"`
}

// Config configures rotating file log output.
type Config struct {
	FilePath      string
	MaxSize       int64 // Maximum bytes per log file before rotation
	MaxBackups    int   // Maximum retained historical backups
	RedactSecrets bool
	StdoutMirror  bool
}

// DefaultConfig provides conservative production defaults: 10MB file cap, 3 backups.
func DefaultConfig(filePath string) Config {
	return Config{
		FilePath:      filePath,
		MaxSize:       10 * 1024 * 1024, // 10 Megabytes
		MaxBackups:    3,
		RedactSecrets: true,
		StdoutMirror:  false,
	}
}

// Logger provides bounded, rotating structured logging with fail-open disk exhaustion protection.
type Logger struct {
	mu           sync.Mutex
	cfg          Config
	file         *os.File
	currentSize  int64
	writeErrors  atomic.Uint64
	stdoutWriter io.Writer
}

// NewLogger creates and initializes a rotating structured logger.
func NewLogger(cfg Config) (*Logger, error) {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 10 * 1024 * 1024
	}
	if cfg.MaxBackups <= 0 {
		cfg.MaxBackups = 3
	}

	dir := filepath.Dir(cfg.FilePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	l := &Logger{
		cfg:          cfg,
		stdoutWriter: os.Stdout,
	}

	if err := l.openCurrentFile(); err != nil {
		return nil, err
	}

	return l, nil
}

func (l *Logger) openCurrentFile() error {
	fi, err := os.Stat(l.cfg.FilePath)
	if err == nil {
		l.currentSize = fi.Size()
	} else if os.IsNotExist(err) {
		l.currentSize = 0
	} else {
		return err
	}

	f, err := os.OpenFile(l.cfg.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	l.file = f
	return nil
}

// Rotate closes the current file, shifts backup indices, and creates a fresh active log.
func (l *Logger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.rotateLocked()
}

func (l *Logger) rotateLocked() error {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}

	// Shift existing backups: .2 -> .3, .1 -> .2, active -> .1
	for i := l.cfg.MaxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.cfg.FilePath, i)
		newPath := fmt.Sprintf("%s.%d", l.cfg.FilePath, i+1)
		_ = os.Rename(oldPath, newPath)
	}

	// Rename active to .1
	backup1 := fmt.Sprintf("%s.1", l.cfg.FilePath)
	_ = os.Rename(l.cfg.FilePath, backup1)

	// Reopen empty active log
	return l.openCurrentFile()
}

// WriteEntry outputs a structured log line, rotating if the file cap has been exceeded.
func (l *Logger) WriteEntry(entry LogEntry) error {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if entry.Version == "" {
		entry.Version = core.ReleaseVersion
	}
	if entry.Platform == "" {
		entry.Platform = runtime.GOOS
	}
	if entry.ErrorCategory == "" {
		entry.ErrorCategory = ErrCategoryNone
	}

	if l.cfg.RedactSecrets {
		entry.Message = Redact(entry.Message)
	}

	lineBytes, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	lineBytes = append(lineBytes, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if write would exceed MaxSize
	if l.file != nil && (l.currentSize+int64(len(lineBytes))) > l.cfg.MaxSize {
		_ = l.rotateLocked()
	}

	if l.file != nil {
		n, writeErr := l.file.Write(lineBytes)
		if writeErr != nil {
			// Fail-open: record write error without crashing
			l.writeErrors.Add(1)
		} else {
			l.currentSize += int64(n)
		}
	}

	if l.cfg.StdoutMirror && l.stdoutWriter != nil {
		_, _ = l.stdoutWriter.Write(lineBytes)
	}

	return nil
}

// Info logs an informational message.
func (l *Logger) Info(component, op, correlationID, msg string) {
	_ = l.WriteEntry(LogEntry{
		Severity:      "INFO",
		Component:     component,
		Op:            op,
		CorrelationID: correlationID,
		ErrorCategory: ErrCategoryNone,
		Message:       msg,
	})
}

// Warn logs a warning message.
func (l *Logger) Warn(component, op, correlationID, category, msg string) {
	_ = l.WriteEntry(LogEntry{
		Severity:      "WARN",
		Component:     component,
		Op:            op,
		CorrelationID: correlationID,
		ErrorCategory: category,
		Message:       msg,
	})
}

// Error logs an error event with explicit error categorization.
func (l *Logger) Error(component, op, correlationID, category string, err error, msg string) {
	errMsg := msg
	if err != nil {
		if errMsg != "" {
			errMsg = fmt.Sprintf("%s: %v", msg, err)
		} else {
			errMsg = err.Error()
		}
	}
	_ = l.WriteEntry(LogEntry{
		Severity:      "ERROR",
		Component:     component,
		Op:            op,
		CorrelationID: correlationID,
		ErrorCategory: category,
		Message:       errMsg,
	})
}

// Close closes the underlying log file cleanly.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

// GetWriteErrors returns the count of write errors (e.g. during disk exhaustion).
func (l *Logger) GetWriteErrors() uint64 {
	return l.writeErrors.Load()
}
