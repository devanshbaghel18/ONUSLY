package core

import (
	"context"
	"errors"
)

// Canonical release version metadata.
const (
	// ReleaseVersion is the authoritative single source of versioning for the ONUSLY release candidate.
	ReleaseVersion = "1.0.0-rc1"
	// SecurityVersion is the monotonically increasing anti-rollback counter.
	SecurityVersion uint64 = 1
)

var (
	// BuildCommit and BuildDate can be stamped via ldflags (-X) during CI release builds.
	BuildCommit = "2c26d96"
	BuildDate   = "2026-11-11"
)

var (
	ErrUnauthenticated   = errors.New("agent is not authenticated with an ONUSLY account")
	ErrDeviceRevoked     = errors.New("device authorization has been revoked by the account owner")
	ErrCredentialExpired = errors.New("device authentication credential has expired")
	ErrInvalidRequest    = errors.New("invalid request payload")
	ErrUnauthorized      = errors.New("unauthorized IPC request")
	ErrServiceOffline    = errors.New("backend API is unreachable")
	ErrUserMismatch      = errors.New("blocklist user ID does not match active authenticated session")
)

// ApplicationTarget defines an explicitly typed enforcement target.
type ApplicationTarget struct {
	Kind       string `json:"kind"`       // "desktop_app" | "android_package"
	Identifier string `json:"identifier"` // e.g. "discord", "com.whatsapp"
}

// TargetItem represents a single goal's active enforcement targets.
type TargetItem struct {
	GoalID             string              `json:"goalId"`
	GoalTitle          string              `json:"goalTitle"`
	Status             string              `json:"status"`
	Apps               []string            `json:"apps"`
	Domains            []string            `json:"domains"`
	ApplicationTargets []ApplicationTarget `json:"applicationTargets,omitempty"`
}

// BlocklistData represents the authoritative blocklist payload cached by the agent.
type BlocklistData struct {
	UserID      string       `json:"userId"`
	Version     string       `json:"version"`
	GeneratedAt string       `json:"generatedAt"`
	LastSyncAt  string       `json:"lastSyncAt"`
	Targets     []TargetItem `json:"targets"`
}

// Authoritative Health States for ONUSLY clients and components.
const (
	HealthStateHealthy                = "HEALTHY"
	HealthStateDegraded               = "DEGRADED"
	HealthStateOffline                = "OFFLINE"
	HealthStateAuthenticationRequired = "AUTHENTICATION_REQUIRED"
	HealthStateDeviceRevoked          = "DEVICE_REVOKED"
	HealthStatePolicyStale            = "POLICY_STALE"
	HealthStatePolicyMissing          = "POLICY_MISSING"
	HealthStateDNSUnavailable         = "DNS_UNAVAILABLE"
	HealthStateBrowserDegraded        = "BROWSER_INTEGRATION_DEGRADED"
	HealthStateAppEnforcementDegraded = "APP_ENFORCEMENT_DEGRADED"
)

// Component Health States
const (
	ComponentHealthHealthy      = "HEALTHY"
	ComponentHealthDegraded     = "DEGRADED"
	ComponentHealthUnavailable  = "UNAVAILABLE"
	ComponentHealthStale        = "STALE"
	ComponentHealthFailing      = "FAILING"
	ComponentHealthDisconnected = "DISCONNECTED"
)

// AgentStatus reflects the current runtime health and state of the desktop agent.
type AgentStatus struct {
	Connected             bool     `json:"connected"`
	UserID                string   `json:"userId,omitempty"`
	DeviceID              string   `json:"deviceId"`
	Platform              string   `json:"platform"`
	Version               string   `json:"version"`
	LastSyncAt            string   `json:"lastSyncAt,omitempty"`
	BlocklistVersion      string   `json:"blocklistVersion,omitempty"`
	TargetsCount          int      `json:"targetsCount"`
	Enforcing             bool     `json:"enforcing"`
	EnforcementStatus     string   `json:"enforcementStatus,omitempty"` // "HEALTHY" | "ENFORCEMENT_DEGRADED" | "POLICY_MISSING" | "UNPAIRED"
	HealthState           string   `json:"healthState,omitempty"`       // Authoritative aggregate health state
	DNSHealth             string   `json:"dnsHealth,omitempty"`
	AppsHealth            string   `json:"appsHealth,omitempty"`
	BrowserHealth         string   `json:"browserHealth,omitempty"`
	SyncHealth            string   `json:"syncHealth,omitempty"`
	LastSyncOutcome       string   `json:"lastSyncOutcome,omitempty"`
	LastErrorCategory     string   `json:"lastErrorCategory,omitempty"`
	LKGActive             bool     `json:"lkgActive"`
	CacheValid            bool     `json:"cacheValid"`
	Issues                []string `json:"issues"`
	PairingState          string   `json:"pairingState,omitempty"`
	OwnerSID              string   `json:"ownerSid,omitempty"`
	DNSResolverRunning    bool     `json:"dnsResolverRunning"`
	DNSResolverListening  bool     `json:"dnsResolverListening"`
	DNSQueriesBlocked     uint64   `json:"dnsQueriesBlocked"`
	DNSQueriesForwarded   uint64   `json:"dnsQueriesForwarded"`
	AppsControllerRunning bool     `json:"appsControllerRunning"`
	AppsMonitoredCount    uint64   `json:"appsMonitoredCount"`
	AppsRestrictedCount   int      `json:"appsRestrictedCount"`
	AppsEnforcedCount     uint64   `json:"appsEnforcedCount"`
	AppsFailedCount       uint64   `json:"appsFailedCount"`
}

// IPCRequest represents a message from the browser native host or CLI to the agent.
type IPCRequest struct {
	Action string `json:"action"` // "get-blocklist" | "get-status" | "ping" | "pair-request" | "pair-poll" | "pair-cancel" | "diagnostics" | "apps-inventory" | "apps-status"
	Code   string `json:"code,omitempty"`
}

// IPCResponse represents the agent's reply to the browser native host or CLI.
type IPCResponse struct {
	Success     bool              `json:"success"`
	Error       string            `json:"error,omitempty"`
	Blocklist   *BlocklistData    `json:"blocklist,omitempty"`
	Status      *AgentStatus      `json:"status,omitempty"`
	PairCode    string            `json:"pairCode,omitempty"`
	ExpiresAt   int64             `json:"expiresAt,omitempty"`
	Diagnostics map[string]string `json:"diagnostics,omitempty"`
	Apps        []any             `json:"apps,omitempty"`
}

// BrowserType identifies a supported desktop web browser.
type BrowserType string

const (
	BrowserChrome   BrowserType = "chrome"
	BrowserChromium BrowserType = "chromium"
	BrowserBrave    BrowserType = "brave"
	BrowserEdge     BrowserType = "edge"
	BrowserFirefox  BrowserType = "firefox"
)

// BrowserInfo contains detection details for an installed browser.
type BrowserInfo struct {
	Type                  BrowserType `json:"type"`
	BinaryPath            string      `json:"binaryPath"`
	PolicyDir             string      `json:"policyDir"`
	NativeHostDir         string      `json:"nativeHostDir"`
	UserNativeHostDir     string      `json:"userNativeHostDir"`
	SupportsManagedPolicy bool        `json:"supportsManagedPolicy"`
	Installed             bool        `json:"installed"`
}

// PolicyIssue indicates policy drift or configuration problems.
type PolicyIssue struct {
	Browser BrowserType `json:"browser"`
	Issue   string      `json:"issue"`
	Details string      `json:"details"`
}

// Interfaces for clean platform abstraction:

type CredentialStore interface {
	SaveToken(token string) error
	GetToken() (string, error)
	ClearToken() error
}

type DeviceIdentityStore interface {
	GetDeviceID() (string, error)
}

type BlocklistCache interface {
	Get(expectedUserID string) (*BlocklistData, error)
	Save(data *BlocklistData) error
	Clear() error
}

type BlocklistSyncer interface {
	Sync(ctx context.Context) (*BlocklistData, error)
	GetCachedBlocklist() (*BlocklistData, error)
}

type HeartbeatReporter interface {
	SendHeartbeat(ctx context.Context, issues []string) error
}

type PolicyManager interface {
	DetectBrowsers() ([]BrowserInfo, error)
	ApplyPolicies(browsers []BrowserInfo) error
	VerifyPolicies(browsers []BrowserInfo) ([]PolicyIssue, error)
	RemovePolicies(browsers []BrowserInfo) error
}

type ServiceManager interface {
	InstallService() error
	UninstallService() error
	StartService() error
	StopService() error
	IsRunning() (bool, error)
}
