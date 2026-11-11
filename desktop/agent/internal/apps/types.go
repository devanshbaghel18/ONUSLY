package apps

import (
	"time"
)

// MatchConfidence reflects how reliably a running process was matched to an application.
type MatchConfidence string

const (
	ConfidencePackageID MatchConfidence = "package_id" // Highest (AppX / Flatpak / Snap package)
	ConfidenceAppID     MatchConfidence = "app_id"     // High (Application User Model ID / Desktop Entry ID)
	ConfidenceExactPath MatchConfidence = "exact_path" // Strong (Exact verified executable path)
	ConfidenceDirBinary MatchConfidence = "dir_binary" // Medium (Known install directory + binary name)
	ConfidenceBaseName  MatchConfidence = "basename"   // Weak (Process executable filename fallback)
	ConfidenceNone      MatchConfidence = "none"
)

// ApplicationIdentity defines the platform-neutral identity of an installed or targeted application.
type ApplicationIdentity struct {
	AppID        string          `json:"appId"`                  // Canonical ID (e.g., "discord", "steam", "spotify")
	DisplayName  string          `json:"displayName"`            // Human-readable title
	Platform     string          `json:"platform"`               // "windows" or "linux"
	PackageName  string          `json:"packageName,omitempty"`  // AppX Package Family Name or Linux package
	AUMID        string          `json:"aumid,omitempty"`        // Application User Model ID (Windows)
	ExePath      string          `json:"exePath,omitempty"`      // Primary executable path if known
	ExeBaseNames []string        `json:"exeBaseNames"`           // Known binary filenames (e.g., ["Discord.exe"])
	InstallDir   string          `json:"installDir,omitempty"`   // Application root installation directory
	Publisher    string          `json:"publisher,omitempty"`    // Publisher or vendor name
	IsSystem     bool            `json:"isSystem"`               // True if essential OS system component (protected)
}

// ProcessInfo represents an observed runtime process on the system.
type ProcessInfo struct {
	PID         uint32               `json:"pid"`
	PPID        uint32               `json:"ppid"`
	ExePath     string               `json:"exePath"`
	BaseName    string               `json:"baseName"`
	StartTime   int64                `json:"startTime"` // Creation timestamp in UnixNano (PID reuse prevention)
	SessionID   uint32               `json:"sessionId"` // Interactive console or terminal session
	Identity    *ApplicationIdentity `json:"identity,omitempty"`
	Confidence  MatchConfidence      `json:"confidence"`
	MatchReason string               `json:"matchReason,omitempty"`
	IsProtected bool                 `json:"isProtected"` // Immunity flag
}

// EnforcementState represents the current enforcement disposition of an application or process.
type EnforcementState string

const (
	StateAllowed               EnforcementState = "allowed"
	StateRestricted            EnforcementState = "restricted"
	StateEnforcementPending    EnforcementState = "enforcement_pending"
	StateEnforced              EnforcementState = "enforced"
	StateEnforcementFailed     EnforcementState = "enforcement_failed"
	StateUnsupported           EnforcementState = "unsupported_enforcement"
	StatePendingPolicySync     EnforcementState = "pending_policy_sync"
	StateUnknownIdentity       EnforcementState = "unknown_identity"
)

// EnforcementEvent records a concrete enforcement action taken on a process.
type EnforcementEvent struct {
	Timestamp   string `json:"timestamp"`
	PID         uint32 `json:"pid"`
	ProcessName string `json:"processName"`
	ExePath     string `json:"exePath"`
	AppID       string `json:"appId"`
	GoalID      string `json:"goalId"`
	Action      string `json:"action"` // "detected", "terminated", "failed", "unrestricted"
	Success     bool   `json:"success"`
	Reason      string `json:"reason"`
	Tier        string `json:"tier"` // Capability tier
}

// TargetApp represents an authoritative application restriction configured in a goal.
type TargetApp struct {
	GoalID      string   `json:"goalId"`
	GoalTitle   string   `json:"goalTitle"`
	Status      string   `json:"status"` // "active", "proof_submitted", "rejected", "completed"
	AppID       string   `json:"appId"`
	Aliases     []string `json:"aliases,omitempty"`
	Restricted  bool     `json:"restricted"` // True if active, proof_submitted, or rejected
}

// AppEnforcementStats tracks runtime aggregate metrics without recording sensitive user activity.
type AppEnforcementStats struct {
	MonitoredProcesses uint64    `json:"monitoredProcesses"`
	RestrictedDetected uint64    `json:"restrictedDetected"`
	EnforcedTerminated uint64    `json:"enforcedTerminated"`
	EnforcementFailed  uint64    `json:"enforcementFailed"`
	LastReconciled     time.Time `json:"lastReconciled"`
}

