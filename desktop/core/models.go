package core

import (
	"context"
	"errors"
)

var (
	ErrUnauthenticated = errors.New("agent is not authenticated with an ONUSLY account")
	ErrInvalidRequest  = errors.New("invalid request payload")
	ErrUnauthorized    = errors.New("unauthorized IPC request")
	ErrServiceOffline  = errors.New("backend API is unreachable")
	ErrUserMismatch    = errors.New("blocklist user ID does not match active authenticated session")
)

// TargetItem represents a single goal's active enforcement targets.
type TargetItem struct {
	GoalID    string   `json:"goalId"`
	GoalTitle string   `json:"goalTitle"`
	Status    string   `json:"status"`
	Apps      []string `json:"apps"`
	Domains   []string `json:"domains"`
}

// BlocklistData represents the authoritative blocklist payload cached by the agent.
type BlocklistData struct {
	UserID      string       `json:"userId"`
	Version     string       `json:"version"`
	GeneratedAt string       `json:"generatedAt"`
	LastSyncAt  string       `json:"lastSyncAt"`
	Targets     []TargetItem `json:"targets"`
}

// AgentStatus reflects the current runtime health and state of the desktop agent.
type AgentStatus struct {
	Connected        bool     `json:"connected"`
	UserID           string   `json:"userId,omitempty"`
	DeviceID         string   `json:"deviceId"`
	Platform         string   `json:"platform"`
	Version          string   `json:"version"`
	LastSyncAt       string   `json:"lastSyncAt,omitempty"`
	BlocklistVersion string   `json:"blocklistVersion,omitempty"`
	TargetsCount     int      `json:"targetsCount"`
	Enforcing        bool     `json:"enforcing"`
	Issues           []string `json:"issues"`
}

// IPCRequest represents a message from the browser native host to the agent.
type IPCRequest struct {
	Action string `json:"action"` // "get-blocklist" | "get-status" | "ping"
}

// IPCResponse represents the agent's reply to the browser native host.
type IPCResponse struct {
	Success   bool           `json:"success"`
	Error     string         `json:"error,omitempty"`
	Blocklist *BlocklistData `json:"blocklist,omitempty"`
	Status    *AgentStatus   `json:"status,omitempty"`
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
