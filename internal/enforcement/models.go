package enforcement

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidUUID             = errors.New("invalid device id: must be a valid UUID")
	ErrInvalidClientType       = errors.New("invalid client type: must be android, extension, or desktop_agent")
	ErrInvalidPlatform         = errors.New("invalid platform: must be android, linux, windows, or macos")
	ErrInvalidBrowser          = errors.New("invalid browser: must be chrome, edge, brave, firefox, or omitted")
	ErrInvalidSentAt           = errors.New("invalid sentAt: timestamp cannot be empty, unparseable, or in future")
	ErrInvalidIssues           = errors.New("invalid issue: contains unrecognized issue code")
	ErrDeviceNotFound          = errors.New("device not found")
	ErrDeviceRevoked           = errors.New("device has been revoked by account owner")
	ErrCredentialExpired       = errors.New("device credential has expired")
	ErrUnauthorizedDevice      = errors.New("unauthorized device access")
	ErrDeviceOwnershipMismatch = errors.New("device belongs to a different account")
	ErrGoalNotFound            = errors.New("goal not found")
	ErrUnauthorizedGoal        = errors.New("unauthorized access to goal enforcement status")
)

const (
	DeviceStatusPaired       = "paired"
	DeviceStatusRevoked      = "revoked"
	DeviceStatusAuthRequired = "auth_required"
)

// Authoritative Health States for devices and clients
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

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var validClientTypes = map[string]bool{
	"android":       true,
	"extension":     true,
	"desktop-agent": true,
	"desktop_agent": true,
}

var validPlatforms = map[string]bool{
	"android": true,
	"linux":   true,
	"windows": true,
	"macos":   true,
}

var validBrowsers = map[string]bool{
	"":        true,
	"chrome":  true,
	"edge":    true,
	"brave":   true,
	"firefox": true,
}

var validIssues = map[string]bool{
	"accessibility_disabled": true,
	"policy_missing":         true,
	"incognito_allowed":      true,
	"dns_drift":              true,
	"service_degraded":       true,
}

// ApplicationTarget defines an explicitly typed application restriction.
type ApplicationTarget struct {
	Kind       string `json:"kind"`       // "desktop_app" | "android_package"
	Identifier string `json:"identifier"` // e.g. "discord", "com.whatsapp"
}

// TargetItem represents a single goal's active enforcement targets.
type TargetItem struct {
	GoalID             string              `json:"goalId"`
	GoalTitle          string              `json:"goalTitle"`
	Status             string              `json:"status"`
	Apps               []string            `json:"apps"` // preserved for backward-compatibility with existing goals/Android clients
	Domains            []string            `json:"domains"`
	ApplicationTargets []ApplicationTarget `json:"applicationTargets,omitempty"`

	// unexported field for ordering
	createdAt string `json:"-"`
}

// BlocklistResponse represents the complete payload for GET /enforcement/blocklist.
type BlocklistResponse struct {
	UserID      string       `json:"userId"`
	Version     string       `json:"version"`
	GeneratedAt string       `json:"generatedAt"`
	Targets     []TargetItem `json:"targets"`
}

// FlexString accepts both JSON strings and JSON numbers/primitives and normalizes to string.
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = FlexString(s)
		return nil
	}
	*f = FlexString(string(b))
	return nil
}

func (f FlexString) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(f))
}

func (f FlexString) String() string {
	return string(f)
}

// HeartbeatRequest payload for POST /enforcement/heartbeat
type HeartbeatRequest struct {
	DeviceID         string     `json:"deviceId"`
	ClientType       string     `json:"clientType"`
	Platform         string     `json:"platform"`
	Browser          string     `json:"browser,omitempty"`
	ClientVersion    string     `json:"clientVersion"`
	Enforcing        bool       `json:"enforcing"`
	BlocklistVersion FlexString `json:"blocklistVersion"`
	Issues           []string   `json:"issues"`
	SentAt           string     `json:"sentAt"`
}

func (r *HeartbeatRequest) Validate() error {
	r.DeviceID = strings.TrimSpace(r.DeviceID)
	if !uuidRegex.MatchString(r.DeviceID) {
		return ErrInvalidUUID
	}

	r.ClientType = strings.ToLower(strings.TrimSpace(r.ClientType))
	if r.ClientType == "desktop-agent" {
		r.ClientType = "desktop_agent"
	}
	if !validClientTypes[r.ClientType] {
		return ErrInvalidClientType
	}

	r.Platform = strings.ToLower(strings.TrimSpace(r.Platform))
	if !validPlatforms[r.Platform] {
		return ErrInvalidPlatform
	}

	r.Browser = strings.ToLower(strings.TrimSpace(r.Browser))
	if !validBrowsers[r.Browser] {
		return ErrInvalidBrowser
	}

	r.ClientVersion = strings.TrimSpace(r.ClientVersion)
	if r.ClientVersion == "" || len(r.ClientVersion) > 64 {
		return errors.New("clientVersion must be non-empty and <= 64 characters")
	}

	if len(string(r.BlocklistVersion)) > 128 {
		return errors.New("blocklistVersion must be <= 128 characters")
	}

	if strings.TrimSpace(r.SentAt) == "" {
		return ErrInvalidSentAt
	}
	t, err := time.Parse(time.RFC3339, r.SentAt)
	if err != nil {
		return ErrInvalidSentAt
	}
	if t.After(time.Now().Add(5 * time.Minute)) {
		return ErrInvalidSentAt
	}

	cleanIssues := make([]string, 0, len(r.Issues))
	seen := make(map[string]bool)
	for _, issue := range r.Issues {
		iss := strings.ToLower(strings.TrimSpace(issue))
		if !validIssues[iss] {
			return ErrInvalidIssues
		}
		if !seen[iss] {
			seen[iss] = true
			cleanIssues = append(cleanIssues, iss)
		}
	}
	r.Issues = cleanIssues

	return nil
}

// Device represents a registered user device in DynamoDB
// PK = USER#<sub
// SK = DEVICE#<deviceId>
type Device struct {
	PK                string   `dynamodbav:"PK" json:"-"`
	SK                string   `dynamodbav:"SK" json:"-"`
	DeviceID          string   `dynamodbav:"DeviceId" json:"deviceId"`
	UserID            string   `dynamodbav:"UserId" json:"userId"`
	ClientType        string   `dynamodbav:"ClientType" json:"clientType"`
	Platform          string   `dynamodbav:"Platform" json:"platform"`
	Browser           string   `dynamodbav:"Browser,omitempty" json:"browser,omitempty"`
	ClientVersion     string   `dynamodbav:"ClientVersion" json:"clientVersion"`
	Enforcing         bool     `dynamodbav:"Enforcing" json:"enforcing"`
	BlocklistVersion  string   `dynamodbav:"BlocklistVersion" json:"blocklistVersion"`
	Issues            []string `dynamodbav:"Issues" json:"issues"`
	Status            string   `dynamodbav:"Status,omitempty" json:"status"` // "paired" | "revoked" | "auth_required"
	HealthStatus      string   `dynamodbav:"HealthStatus,omitempty" json:"healthStatus,omitempty"`
	RevokedAt         string   `dynamodbav:"RevokedAt,omitempty" json:"revokedAt,omitempty"`
	RevocationReason  string   `dynamodbav:"RevocationReason,omitempty" json:"revocationReason,omitempty"`
	CredentialVersion int64    `dynamodbav:"CredentialVersion,omitempty" json:"credentialVersion,omitempty"`
	DeviceName        string   `dynamodbav:"DeviceName,omitempty" json:"deviceName,omitempty"`
	LastHeartbeatAt   string   `dynamodbav:"LastHeartbeatAt" json:"lastHeartbeatAt"`
	LapseEpisodeID    string   `dynamodbav:"LapseEpisodeID,omitempty" json:"lapseEpisodeId,omitempty"`
	CreatedAt         string   `dynamodbav:"CreatedAt" json:"createdAt"`
	UpdatedAt         string   `dynamodbav:"UpdatedAt" json:"updatedAt"`
	ExpiresAt         int64    `dynamodbav:"ExpiresAt" json:"expiresAt"` // Unix timestamp (30 days TTL)
}

// RevokeDeviceRequest represents payload for POST /enforcement/devices/{id}/revoke
type RevokeDeviceRequest struct {
	Reason string `json:"reason,omitempty"`
}

// RevokeDeviceResponse returns the confirmation of revocation
type RevokeDeviceResponse struct {
	DeviceID  string `json:"deviceId"`
	Status    string `json:"status"`
	RevokedAt string `json:"revokedAt"`
}

// DeviceCoverageResponse returns all registered devices for the authenticated owner
type DeviceCoverageResponse struct {
	UserID     string   `json:"userId"`
	TotalCount int      `json:"totalCount"`
	Enforcing  bool     `json:"enforcing"`
	Devices    []Device `json:"devices"`
}

// GoalEnforcementStatusResponse represents the response for GET /goals/{id}/enforcement-status
type GoalEnforcementStatusResponse struct {
	GoalID          string   `json:"goalId"`
	AggregateStatus string   `json:"aggregateStatus"` // "enforcing" | "lapsed" | "no_devices"
	DevicesCount    int      `json:"devicesCount"`
	EnforcingCount  int      `json:"enforcingCount"`
	HasIssues       bool     `json:"hasIssues"`
	LastSyncAt      string   `json:"lastSyncAt,omitempty"`
	Devices         []Device `json:"devices,omitempty"` // Included for owner; omitted for approver
}

// PairingSession stores a temporary short-lived code for linking Desktop Agent to an account.
// PK = PAIR#<code>
// SK = SESSION
type PairingSession struct {
	PK          string `dynamodbav:"PK" json:"-"`
	SK          string `dynamodbav:"SK" json:"-"`
	Code        string `dynamodbav:"Code" json:"code"`
	DeviceID    string `dynamodbav:"DeviceId" json:"deviceId"`
	Platform    string `dynamodbav:"Platform" json:"platform"`
	ClientType  string `dynamodbav:"ClientType" json:"clientType"`
	Confirmed   bool   `dynamodbav:"Confirmed" json:"confirmed"`
	ConfirmedBy string `dynamodbav:"ConfirmedBy,omitempty" json:"confirmedBy,omitempty"`
	Token       string `dynamodbav:"Token,omitempty" json:"token,omitempty"`
	CreatedAt   string `dynamodbav:"CreatedAt" json:"createdAt"`
	ExpiresAt   int64  `dynamodbav:"ExpiresAt" json:"expiresAt"`
}
