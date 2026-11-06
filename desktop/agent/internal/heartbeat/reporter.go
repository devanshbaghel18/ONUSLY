package heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

type Reporter struct {
	apiBaseURL    string
	credStore     core.CredentialStore
	identityStore core.DeviceIdentityStore
	syncer        core.BlocklistSyncer
	platform      string
	version       string
	httpClient    *http.Client
}

func NewReporter(
	apiBaseURL string,
	credStore core.CredentialStore,
	identityStore core.DeviceIdentityStore,
	syncer core.BlocklistSyncer,
	platform string,
	version string,
	httpClient *http.Client,
) *Reporter {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if version == "" {
		version = "1.0.0"
	}
	return &Reporter{
		apiBaseURL:    apiBaseURL,
		credStore:     credStore,
		identityStore: identityStore,
		syncer:        syncer,
		platform:      platform,
		version:       version,
		httpClient:    httpClient,
	}
}

type heartbeatPayload struct {
	DeviceID         string   `json:"deviceId"`
	ClientType       string   `json:"clientType"`
	Platform         string   `json:"platform"`
	Browser          string   `json:"browser,omitempty"`
	ClientVersion    string   `json:"clientVersion"`
	Enforcing        bool     `json:"enforcing"`
	BlocklistVersion string   `json:"blocklistVersion"`
	Issues           []string `json:"issues"`
	SentAt           string   `json:"sentAt"`
}

func (r *Reporter) SendHeartbeat(ctx context.Context, issues []string) error {
	token, err := r.credStore.GetToken()
	if err != nil || token == "" {
		return core.ErrUnauthenticated
	}

	deviceID, err := r.identityStore.GetDeviceID()
	if err != nil {
		return fmt.Errorf("failed to retrieve device id: %w", err)
	}

	blocklistVersion := ""
	enforcing := true
	if r.syncer != nil {
		cached, _ := r.syncer.GetCachedBlocklist()
		if cached != nil {
			blocklistVersion = cached.Version
			enforcing = len(cached.Targets) > 0
		}
	}

	if len(issues) > 0 {
		enforcing = false
	}

	cleanIssues := issues
	if cleanIssues == nil {
		cleanIssues = []string{}
	}

	payload := heartbeatPayload{
		DeviceID:         deviceID,
		ClientType:       "desktop_agent",
		Platform:         r.platform,
		ClientVersion:    r.version,
		Enforcing:        enforcing,
		BlocklistVersion: blocklistVersion,
		Issues:           cleanIssues,
		SentAt:           time.Now().UTC().Format(time.RFC3339),
	}

	url := fmt.Sprintf("%s/enforcement/heartbeat", strings.TrimRight(r.apiBaseURL, "/"))
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("heartbeat dispatch error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat returned status %d", resp.StatusCode)
	}

	return nil
}
