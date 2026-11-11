package apps

import (
	"context"
	"time"
)

// ProcessEventType indicates whether a process was launched or exited.
type ProcessEventType string

const (
	ProcessEventStart ProcessEventType = "start"
	ProcessEventExit  ProcessEventType = "exit"
)

// ProcessEvent conveys an observed process lifecycle change.
type ProcessEvent struct {
	Type      ProcessEventType `json:"type"`
	Process   ProcessInfo      `json:"process"`
	Timestamp time.Time        `json:"timestamp"`
}

// ProcessMonitor abstracts platform-specific process observation and lifecycle tracking.
type ProcessMonitor interface {
	Start(ctx context.Context) error
	Stop() error
	Snapshot(ctx context.Context) ([]ProcessInfo, error)
	Events() <-chan ProcessEvent
}

