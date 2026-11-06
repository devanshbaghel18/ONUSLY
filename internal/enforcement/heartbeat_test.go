package enforcement_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/enforcement"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type mockDeviceRepo struct {
	devices  map[string]enforcement.Device
	sessions map[string]enforcement.PairingSession
}

func newMockDeviceRepo() *mockDeviceRepo {
	return &mockDeviceRepo{
		devices:  make(map[string]enforcement.Device),
		sessions: make(map[string]enforcement.PairingSession),
	}
}

func (m *mockDeviceRepo) UpsertDevice(ctx context.Context, device enforcement.Device) error {
	key := device.UserID + "#" + device.DeviceID
	m.devices[key] = device
	return nil
}

func (m *mockDeviceRepo) ListDevices(ctx context.Context, userID string) ([]enforcement.Device, error) {
	var list []enforcement.Device
	for _, d := range m.devices {
		if d.UserID == userID {
			list = append(list, d)
		}
	}
	return list, nil
}

func (m *mockDeviceRepo) GetDevice(ctx context.Context, userID, deviceID string) (*enforcement.Device, error) {
	key := userID + "#" + deviceID
	d, ok := m.devices[key]
	if !ok {
		return nil, enforcement.ErrDeviceNotFound
	}
	return &d, nil
}

func (m *mockDeviceRepo) SavePairingSession(ctx context.Context, session enforcement.PairingSession) error {
	m.sessions[session.Code] = session
	return nil
}

func (m *mockDeviceRepo) GetPairingSession(ctx context.Context, code string) (*enforcement.PairingSession, error) {
	s, ok := m.sessions[code]
	if !ok {
		return nil, enforcement.ErrDeviceNotFound
	}
	return &s, nil
}

func (m *mockDeviceRepo) ConfirmPairingSession(ctx context.Context, code, userID, token string) error {
	s, ok := m.sessions[code]
	if !ok {
		return enforcement.ErrDeviceNotFound
	}
	s.Confirmed = true
	s.ConfirmedBy = userID
	s.Token = token
	m.sessions[code] = s
	return nil
}

func (m *mockDeviceRepo) ConsumePairingSession(ctx context.Context, code string) (*enforcement.PairingSession, error) {
	s, ok := m.sessions[code]
	if !ok {
		return nil, enforcement.ErrDeviceNotFound
	}
	delete(m.sessions, code)
	return &s, nil
}

type mockNotifSender struct {
	notifications []struct {
		RecipientID string
		EpisodeID   string
		GoalID      string
		Title       string
		Message     string
	}
}

func (m *mockNotifSender) CreateFromEnforcementLapsed(ctx context.Context, recipientID, episodeID, goalID, title, message string) error {
	m.notifications = append(m.notifications, struct {
		RecipientID string
		EpisodeID   string
		GoalID      string
		Title       string
		Message     string
	}{
		RecipientID: recipientID,
		EpisodeID:   episodeID,
		GoalID:      goalID,
		Title:       title,
		Message:     message,
	})
	return nil
}

type mockGoalGetter struct {
	goals map[string]goals.Goal
}

func (m *mockGoalGetter) GetByID(ctx context.Context, ownerID, goalID string) (*goals.Goal, error) {
	g, ok := m.goals[goalID]
	if !ok {
		return nil, enforcement.ErrGoalNotFound
	}
	return &g, nil
}

func (m *mockGoalGetter) List(ctx context.Context, ownerID string) ([]goals.Goal, error) {
	var list []goals.Goal
	for _, g := range m.goals {
		if g.OwnerID == ownerID {
			list = append(list, g)
		}
	}
	return list, nil
}

type mockTokenGen struct{}

func (m *mockTokenGen) GenerateToken(userID, email, handle, name string) (string, error) {
	return "mock-device-jwt-" + userID, nil
}

func TestHeartbeat_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     enforcement.HeartbeatRequest
		wantErr error
	}{
		{
			name: "valid request",
			req: enforcement.HeartbeatRequest{
				DeviceID:         "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
				ClientType:       "desktop-agent",
				Platform:         "linux",
				Browser:          "chrome",
				ClientVersion:    "1.0.0",
				Enforcing:        true,
				BlocklistVersion: "v1-abcdef0123456789",
				Issues:           []string{"policy_missing"},
				SentAt:           time.Now().UTC().Format(time.RFC3339),
			},
			wantErr: nil,
		},
		{
			name: "invalid device UUID",
			req: enforcement.HeartbeatRequest{
				DeviceID:      "not-a-uuid",
				ClientType:    "desktop-agent",
				Platform:      "linux",
				ClientVersion: "1.0.0",
				SentAt:        time.Now().UTC().Format(time.RFC3339),
			},
			wantErr: enforcement.ErrInvalidUUID,
		},
		{
			name: "invalid client type",
			req: enforcement.HeartbeatRequest{
				DeviceID:      "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
				ClientType:    "smart-tv",
				Platform:      "linux",
				ClientVersion: "1.0.0",
				SentAt:        time.Now().UTC().Format(time.RFC3339),
			},
			wantErr: enforcement.ErrInvalidClientType,
		},
		{
			name: "invalid platform",
			req: enforcement.HeartbeatRequest{
				DeviceID:      "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
				ClientType:    "desktop-agent",
				Platform:      "solaris",
				ClientVersion: "1.0.0",
				SentAt:        time.Now().UTC().Format(time.RFC3339),
			},
			wantErr: enforcement.ErrInvalidPlatform,
		},
		{
			name: "invalid browser",
			req: enforcement.HeartbeatRequest{
				DeviceID:      "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
				ClientType:    "extension",
				Platform:      "linux",
				Browser:       "opera",
				ClientVersion: "1.0.0",
				SentAt:        time.Now().UTC().Format(time.RFC3339),
			},
			wantErr: enforcement.ErrInvalidBrowser,
		},
		{
			name: "invalid issue enum",
			req: enforcement.HeartbeatRequest{
				DeviceID:      "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
				ClientType:    "extension",
				Platform:      "linux",
				Browser:       "chrome",
				ClientVersion: "1.0.0",
				Issues:        []string{"fake_issue"},
				SentAt:        time.Now().UTC().Format(time.RFC3339),
			},
			wantErr: enforcement.ErrInvalidIssues,
		},
		{
			name: "future sentAt timestamp rejected",
			req: enforcement.HeartbeatRequest{
				DeviceID:      "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
				ClientType:    "desktop-agent",
				Platform:      "linux",
				ClientVersion: "1.0.0",
				SentAt:        time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
			},
			wantErr: enforcement.ErrInvalidSentAt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == nil && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if tt.wantErr != nil && (err == nil || err != tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestHeartbeat_API_AndLapseDeduplication(t *testing.T) {
	mockRepo := newMockDeviceRepo()
	mockNotif := &mockNotifSender{}
	mockGoals := &mockGoalGetter{
		goals: map[string]goals.Goal{
			"goal-1": {
				ID:         "goal-1",
				OwnerID:    "user-alpha",
				ApproverID: "approver-beta",
				Title:      "No Social Media",
				Status:     "active",
			},
		},
	}

	service := enforcement.NewService(mockGoals)
	service.SetGoalGetter(mockGoals)
	service.SetDeviceRepository(mockRepo)
	service.SetNotificationSender(mockNotif)
	handler := enforcement.NewHandler(service)

	deviceID := "11111111-2222-3333-4444-555555555555"

	// 1. Send initial healthy heartbeat
	healthyPayload := enforcement.HeartbeatRequest{
		DeviceID:         deviceID,
		ClientType:       "desktop-agent",
		Platform:         "linux",
		Browser:          "chrome",
		ClientVersion:    "1.0.0",
		Enforcing:        true,
		BlocklistVersion: "v1-0000000000000000",
		Issues:           []string{},
		SentAt:           time.Now().UTC().Format(time.RFC3339),
	}

	body, _ := json.Marshal(healthyPayload)
	req := httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-alpha"))
	rec := httptest.NewRecorder()

	handler.Heartbeat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify no notifications sent for healthy device
	if len(mockNotif.notifications) != 0 {
		t.Fatalf("expected 0 notifications for healthy device, got %d", len(mockNotif.notifications))
	}

	// 2. Report an issue (policy_missing) -> Lapse detected!
	lapsedPayload := healthyPayload
	lapsedPayload.Enforcing = false
	lapsedPayload.Issues = []string{"policy_missing"}
	lapsedPayload.SentAt = time.Now().UTC().Format(time.RFC3339)

	body, _ = json.Marshal(lapsedPayload)
	req = httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-alpha"))
	rec = httptest.NewRecorder()

	handler.Heartbeat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// Expect 2 notifications: 1 for owner (user-alpha) + 1 for approver (approver-beta)
	if len(mockNotif.notifications) != 2 {
		t.Fatalf("expected 2 lapse notifications, got %d", len(mockNotif.notifications))
	}

	// 3. Send second heartbeat while still lapsed (Deduplication test)
	// Must NOT duplicate lapse notifications for the same ongoing episode!
	body, _ = json.Marshal(lapsedPayload)
	req = httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-alpha"))
	rec = httptest.NewRecorder()

	handler.Heartbeat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	if len(mockNotif.notifications) != 2 {
		t.Fatalf("expected notifications to remain 2 (deduplicated), got %d", len(mockNotif.notifications))
	}

	// 4. Recover health
	body, _ = json.Marshal(healthyPayload)
	req = httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-alpha"))
	rec = httptest.NewRecorder()

	handler.Heartbeat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	dev, err := mockRepo.GetDevice(context.Background(), "user-alpha", deviceID)
	if err != nil {
		t.Fatalf("failed to retrieve device: %v", err)
	}
	if dev.LapseEpisodeID != "" {
		t.Errorf("expected LapseEpisodeID to be cleared after recovery, got %q", dev.LapseEpisodeID)
	}
}

func TestGetDevices_And_GoalEnforcementStatus(t *testing.T) {
	mockRepo := newMockDeviceRepo()
	mockGoals := &mockGoalGetter{
		goals: map[string]goals.Goal{
			"goal-focus": {
				ID:         "goal-focus",
				OwnerID:    "owner-123",
				ApproverID: "approver-456",
				Title:      "Focus Time",
				Status:     "active",
			},
		},
	}

	service := enforcement.NewService(mockGoals)
	service.SetGoalGetter(mockGoals)
	service.SetDeviceRepository(mockRepo)
	handler := enforcement.NewHandler(service)

	// Save one healthy device and one lapsed device
	_ = mockRepo.UpsertDevice(context.Background(), enforcement.Device{
		DeviceID:         "11111111-1111-1111-1111-111111111111",
		UserID:           "owner-123",
		ClientType:       "desktop-agent",
		Platform:         "linux",
		Browser:          "chrome",
		ClientVersion:    "1.0.0",
		Enforcing:        true,
		BlocklistVersion: "v1-123",
		Issues:           []string{},
		LastHeartbeatAt:  time.Now().UTC().Format(time.RFC3339),
	})
	_ = mockRepo.UpsertDevice(context.Background(), enforcement.Device{
		DeviceID:         "22222222-2222-2222-2222-222222222222",
		UserID:           "owner-123",
		ClientType:       "android",
		Platform:         "android",
		ClientVersion:    "1.0.0",
		Enforcing:        false,
		BlocklistVersion: "v1-123",
		Issues:           []string{"accessibility_disabled"},
		LastHeartbeatAt:  time.Now().UTC().Format(time.RFC3339),
	})

	// 1. GET /enforcement/devices (Owner only)
	req := httptest.NewRequest("GET", "/enforcement/devices", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "owner-123"))
	rec := httptest.NewRecorder()

	handler.GetDevices(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var devResp enforcement.DeviceCoverageResponse
	_ = json.NewDecoder(rec.Body).Decode(&devResp)
	if devResp.TotalCount != 2 {
		t.Errorf("expected 2 devices, got %d", devResp.TotalCount)
	}
	if devResp.Enforcing {
		t.Errorf("expected Enforcing to be false because one device is non-enforcing")
	}

	// 2. GET /goals/{id}/enforcement-status by Owner (Receives device enumeration)
	req = httptest.NewRequest("GET", "/goals/goal-focus/enforcement-status", nil)
	req.SetPathValue("id", "goal-focus")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "owner-123"))
	rec = httptest.NewRecorder()

	handler.GetGoalEnforcementStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var ownerStatus enforcement.GoalEnforcementStatusResponse
	_ = json.NewDecoder(rec.Body).Decode(&ownerStatus)
	if ownerStatus.AggregateStatus != "lapsed" {
		t.Errorf("expected aggregate status 'lapsed', got %q", ownerStatus.AggregateStatus)
	}
	if len(ownerStatus.Devices) != 2 {
		t.Errorf("expected owner to receive 2 devices, got %d", len(ownerStatus.Devices))
	}

	// 3. GET /goals/{id}/enforcement-status by Approver (Privacy preservation: NO device list)
	req = httptest.NewRequest("GET", "/goals/goal-focus/enforcement-status", nil)
	req.SetPathValue("id", "goal-focus")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "approver-456"))
	rec = httptest.NewRecorder()

	handler.GetGoalEnforcementStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var approverStatus enforcement.GoalEnforcementStatusResponse
	_ = json.NewDecoder(rec.Body).Decode(&approverStatus)
	if approverStatus.AggregateStatus != "lapsed" {
		t.Errorf("expected approver aggregate status 'lapsed', got %q", approverStatus.AggregateStatus)
	}
	if len(approverStatus.Devices) != 0 {
		t.Errorf("PRIVACY VIOLATION: approver must NOT receive device records, got %d devices", len(approverStatus.Devices))
	}

	// 4. GET /goals/{id}/enforcement-status by Unauthorized Third-Party -> 403 Forbidden
	req = httptest.NewRequest("GET", "/goals/goal-focus/enforcement-status", nil)
	req.SetPathValue("id", "goal-focus")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "unauthorized-stranger"))
	rec = httptest.NewRecorder()

	handler.GetGoalEnforcementStatus(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for third party, got %d", rec.Code)
	}
}

func TestDevicePairing_Lifecycle(t *testing.T) {
	mockRepo := newMockDeviceRepo()
	mockGoals := &mockGoalGetter{}
	service := enforcement.NewService(mockGoals)
	service.SetDeviceRepository(mockRepo)
	service.SetTokenGenerator(&mockTokenGen{})
	handler := enforcement.NewHandler(service)

	// 1. Agent requests pairing code
	reqBody, _ := json.Marshal(map[string]string{
		"clientType": "desktop-agent",
		"platform":   "linux",
		"deviceId":   "99999999-9999-9999-9999-999999999999",
	})
	req := httptest.NewRequest("POST", "/enforcement/pair/request", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	handler.RequestPairing(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var pairResp struct {
		Code      string `json:"code"`
		ExpiresAt int64  `json:"expiresAt"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&pairResp)
	if len(pairResp.Code) != 6 {
		t.Fatalf("expected 6-character pairing code, got %q", pairResp.Code)
	}

	// 2. User confirms pairing code on Dashboard
	confirmBody, _ := json.Marshal(map[string]string{"code": pairResp.Code})
	req = httptest.NewRequest("POST", "/enforcement/pair/confirm", bytes.NewReader(confirmBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-desktop-owner"))
	rec = httptest.NewRecorder()

	handler.ConfirmPairing(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Agent exchanges pairing code for JWT
	exchangeBody, _ := json.Marshal(map[string]string{"code": pairResp.Code})
	req = httptest.NewRequest("POST", "/enforcement/pair/exchange", bytes.NewReader(exchangeBody))
	rec = httptest.NewRecorder()

	handler.ExchangePairing(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var tokenResp struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&tokenResp)
	if tokenResp.Token != "mock-device-jwt-user-desktop-owner" {
		t.Fatalf("unexpected token received: %q", tokenResp.Token)
	}

	// 4. Repeated exchange must fail (Single-use security check)
	req = httptest.NewRequest("POST", "/enforcement/pair/exchange", bytes.NewReader(exchangeBody))
	rec = httptest.NewRecorder()
	handler.ExchangePairing(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized on duplicate token exchange, got %d", rec.Code)
	}
}

func TestHeartbeat_InactivityAndGoalInvariants(t *testing.T) {
	mockRepo := newMockDeviceRepo()
	mockGoals := &mockGoalGetter{
		goals: map[string]goals.Goal{
			"goal-active": {
				ID:         "goal-active",
				OwnerID:    "user-x",
				ApproverID: "approver-y",
				Title:      "Goal Must Never Change",
				Status:     "active",
			},
			"goal-submitted": {
				ID:         "goal-submitted",
				OwnerID:    "user-x",
				ApproverID: "approver-y",
				Title:      "Proof Submitted Goal",
				Status:     "proof_submitted",
			},
		},
	}

	service := enforcement.NewService(mockGoals)
	service.SetGoalGetter(mockGoals)
	service.SetDeviceRepository(mockRepo)
	handler := enforcement.NewHandler(service)

	// Subtest 1: 24-hour inactivity handling vs shorter offline intervals
	t.Run("TwentyFourHour_Inactivity_And_Shorter_Offline_Windows", func(t *testing.T) {
		// Device A: 2 hours offline (shorter interval) -> NOT lapsed, approver not notified
		_ = mockRepo.UpsertDevice(context.Background(), enforcement.Device{
			DeviceID:        "dev-recent",
			UserID:          "user-x",
			ClientType:      "desktop-agent",
			Platform:        "linux",
			Enforcing:       true,
			LastHeartbeatAt: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		})

		resp, err := service.GetGoalEnforcementStatus(context.Background(), "approver-y", "", "", "goal-active")
		if err != nil {
			t.Fatalf("failed to get status: %v", err)
		}
		if resp.AggregateStatus != "enforcing" || resp.HasIssues {
			t.Errorf("expected 2-hour offline device to remain 'enforcing' without issues, got %+v", resp)
		}

		// Device B: 25 hours offline (exceeds 24h silence window) -> lapsed!
		_ = mockRepo.UpsertDevice(context.Background(), enforcement.Device{
			DeviceID:        "dev-stale",
			UserID:          "user-x",
			ClientType:      "android",
			Platform:        "android",
			Enforcing:       true,
			LastHeartbeatAt: time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339),
		})

		resp, err = service.GetGoalEnforcementStatus(context.Background(), "approver-y", "", "", "goal-active")
		if err != nil {
			t.Fatalf("failed to get status: %v", err)
		}
		if resp.AggregateStatus != "lapsed" || !resp.HasIssues {
			t.Errorf("expected 25-hour silent device to trigger 'lapsed' aggregate status, got %+v", resp)
		}
	})

	// Subtest 2: Heartbeats NEVER change goal status
	t.Run("Heartbeats_Never_Change_Goal_State", func(t *testing.T) {
		gActive, _ := mockGoals.GetByID(context.Background(), "user-x", "goal-active")
		if gActive.Status != "active" {
			t.Fatalf("pre-condition failed: goal-active status is %s", gActive.Status)
		}

		gSubmitted, _ := mockGoals.GetByID(context.Background(), "user-x", "goal-submitted")
		if gSubmitted.Status != "proof_submitted" {
			t.Fatalf("pre-condition failed: goal-submitted status is %s", gSubmitted.Status)
		}

		// Send multiple heartbeats (both healthy and lapsed)
		for i := 0; i < 5; i++ {
			reqBody, _ := json.Marshal(enforcement.HeartbeatRequest{
				DeviceID:         "11111111-2222-3333-4444-555555555555",
				ClientType:       "desktop-agent",
				Platform:         "linux",
				ClientVersion:    "1.0.0",
				Enforcing:        i%2 == 0,
				BlocklistVersion: "v1-test",
				Issues:           []string{},
				SentAt:           time.Now().UTC().Format(time.RFC3339),
			})
			req := httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(reqBody))
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-x"))
			rec := httptest.NewRecorder()
			handler.Heartbeat(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("heartbeat returned %d", rec.Code)
			}
		}

		// Verify goal statuses remain strictly intact
		gActivePost, _ := mockGoals.GetByID(context.Background(), "user-x", "goal-active")
		if gActivePost.Status != "active" {
			t.Errorf("INVARIANT VIOLATION: goal-active status changed to %q", gActivePost.Status)
		}

		gSubmittedPost, _ := mockGoals.GetByID(context.Background(), "user-x", "goal-submitted")
		if gSubmittedPost.Status != "proof_submitted" {
			t.Errorf("INVARIANT VIOLATION: goal-submitted status changed to %q", gSubmittedPost.Status)
		}
	})

	// Subtest 3: Device record TTL expiration (30 days)
	t.Run("Device_Record_TTL_Expiration", func(t *testing.T) {
		dev, err := service.ProcessHeartbeat(context.Background(), "user-x", enforcement.HeartbeatRequest{
			DeviceID:         "33333333-4444-5555-6666-777777777777",
			ClientType:       "desktop-agent",
			Platform:         "linux",
			ClientVersion:    "1.0.0",
			Enforcing:        true,
			BlocklistVersion: "v1-ttl",
			Issues:           []string{},
			SentAt:           time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			t.Fatalf("failed to process heartbeat: %v", err)
		}

		expectedExpiry := time.Now().Add(30 * 24 * time.Hour).Unix()
		diff := dev.ExpiresAt - expectedExpiry
		if diff < -10 || diff > 10 {
			t.Errorf("expected 30-day TTL ~%d, got %d", expectedExpiry, dev.ExpiresAt)
		}
	})

	// Subtest 4: Device ownership isolation
	t.Run("Device_Ownership_Isolation", func(t *testing.T) {
		// User Y querying devices must NOT see User X's devices
		devicesY, err := service.GetDevices(context.Background(), "user-y")
		if err != nil {
			t.Fatalf("failed to get devices for user-y: %v", err)
		}
		if devicesY.TotalCount != 0 {
			t.Errorf("ACCOUNT ISOLATION VIOLATION: User Y received %d devices belonging to other users", devicesY.TotalCount)
		}
	})
}

// TestHeartbeat_ContractReconciliation rigorously enforces the canonical schema defined in PRD Appendix B.
// It verifies:
// 1. ClientType: canonical desktop_agent vs legacy desktop-agent normalization, and rejection of invalid types.
// 2. Platform: strict validation of android, linux, windows, macos, and explicit rejection of unexpected chrome_mv3.
// 3. BlocklistVersion: flexible deserialization of both numeric integers and strings.
// 4. Browser: validation of chrome, edge, brave, firefox, or omitted.
func TestHeartbeat_ContractReconciliation(t *testing.T) {
	mockRepo := newMockDeviceRepo()
	service := enforcement.NewService(nil)
	service.SetDeviceRepository(mockRepo)
	handler := enforcement.NewHandler(service)

	t.Run("ClientType_Canonical_desktop_agent_Accepted", func(t *testing.T) {
		req := enforcement.HeartbeatRequest{
			DeviceID:         "11111111-1111-1111-1111-111111111111",
			ClientType:       "desktop_agent",
			Platform:         "linux",
			ClientVersion:    "1.0.0",
			Enforcing:        true,
			BlocklistVersion: "1",
			SentAt:           time.Now().UTC().Format(time.RFC3339),
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected desktop_agent to be accepted, got: %v", err)
		}
		if req.ClientType != "desktop_agent" {
			t.Fatalf("expected canonical ClientType 'desktop_agent', got: %q", req.ClientType)
		}
	})

	t.Run("ClientType_Legacy_desktop-agent_NormalizedTo_desktop_agent", func(t *testing.T) {
		req := enforcement.HeartbeatRequest{
			DeviceID:         "11111111-1111-1111-1111-111111111111",
			ClientType:       "desktop-agent",
			Platform:         "linux",
			ClientVersion:    "1.0.0",
			Enforcing:        true,
			BlocklistVersion: "1",
			SentAt:           time.Now().UTC().Format(time.RFC3339),
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected desktop-agent to be accepted, got: %v", err)
		}
		if req.ClientType != "desktop_agent" {
			t.Fatalf("expected normalized ClientType 'desktop_agent', got: %q", req.ClientType)
		}
	})

	t.Run("ClientType_InvalidTypes_Rejected", func(t *testing.T) {
		invalidTypes := []string{"ios", "desktop", "agent", "service", "unknown", ""}
		for _, ct := range invalidTypes {
			req := enforcement.HeartbeatRequest{
				DeviceID:      "11111111-1111-1111-1111-111111111111",
				ClientType:    ct,
				Platform:      "linux",
				ClientVersion: "1.0.0",
				SentAt:        time.Now().UTC().Format(time.RFC3339),
			}
			err := req.Validate()
			if !errors.Is(err, enforcement.ErrInvalidClientType) {
				t.Errorf("expected ErrInvalidClientType for %q, got: %v", ct, err)
			}
		}
	})

	t.Run("Platform_AllowedPlatforms_Accepted", func(t *testing.T) {
		validPlatforms := []string{"android", "linux", "windows", "macos"}
		for _, p := range validPlatforms {
			req := enforcement.HeartbeatRequest{
				DeviceID:      "11111111-1111-1111-1111-111111111111",
				ClientType:    "desktop_agent",
				Platform:      p,
				ClientVersion: "1.0.0",
				SentAt:        time.Now().UTC().Format(time.RFC3339),
			}
			if err := req.Validate(); err != nil {
				t.Errorf("expected platform %q to be valid, got: %v", p, err)
			}
		}
	})

	t.Run("Platform_Unexpected_chrome_mv3_Strictly_Rejected", func(t *testing.T) {
		req := enforcement.HeartbeatRequest{
			DeviceID:      "11111111-1111-1111-1111-111111111111",
			ClientType:    "extension",
			Platform:      "chrome_mv3", // INVALID: extension runtime is not an OS platform
			ClientVersion: "1.0.0",
			SentAt:        time.Now().UTC().Format(time.RFC3339),
		}
		err := req.Validate()
		if !errors.Is(err, enforcement.ErrInvalidPlatform) {
			t.Fatalf("expected ErrInvalidPlatform for chrome_mv3, got: %v", err)
		}
	})

	t.Run("BlocklistVersion_Numeric_Integer_Deserialization", func(t *testing.T) {
		// Client sends JSON integer: {"blocklistVersion": 42}
		rawJSON := []byte(`{
			"deviceId": "22222222-2222-2222-2222-222222222222",
			"clientType": "desktop_agent",
			"platform": "linux",
			"clientVersion": "1.0.0",
			"enforcing": true,
			"blocklistVersion": 42,
			"issues": [],
			"sentAt": "` + time.Now().UTC().Format(time.RFC3339) + `"
		}`)

		var req enforcement.HeartbeatRequest
		if err := json.Unmarshal(rawJSON, &req); err != nil {
			t.Fatalf("failed to unmarshal JSON with numeric blocklistVersion: %v", err)
		}
		if req.BlocklistVersion.String() != "42" {
			t.Fatalf("expected BlocklistVersion '42', got: %q", req.BlocklistVersion.String())
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected validation success for numeric blocklistVersion, got: %v", err)
		}

		// Also verify end-to-end HTTP processing
		httpReq := httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(rawJSON))
		httpReq = httpReq.WithContext(context.WithValue(httpReq.Context(), middleware.UserIDKey, "user-reconcile"))
		rec := httptest.NewRecorder()
		handler.Heartbeat(rec, httpReq)
		if rec.Code != http.StatusOK {
			t.Fatalf("HTTP handler rejected numeric blocklistVersion: code=%d body=%s", rec.Code, rec.Body.String())
		}

		// Check stored device model
		dev, err := mockRepo.GetDevice(context.Background(), "user-reconcile", "22222222-2222-2222-2222-222222222222")
		if err != nil || dev == nil {
			t.Fatalf("device not found in repo: %v", err)
		}
		if dev.BlocklistVersion != "42" {
			t.Fatalf("expected stored BlocklistVersion '42', got: %q", dev.BlocklistVersion)
		}
	})

	t.Run("BlocklistVersion_String_Deserialization", func(t *testing.T) {
		rawJSON := []byte(`{
			"deviceId": "33333333-3333-3333-3333-333333333333",
			"clientType": "extension",
			"platform": "linux",
			"browser": "chrome",
			"clientVersion": "1.0.0",
			"enforcing": true,
			"blocklistVersion": "v1-abcdef0123456789",
			"issues": [],
			"sentAt": "` + time.Now().UTC().Format(time.RFC3339) + `"
		}`)

		var req enforcement.HeartbeatRequest
		if err := json.Unmarshal(rawJSON, &req); err != nil {
			t.Fatalf("failed to unmarshal JSON with string blocklistVersion: %v", err)
		}
		if req.BlocklistVersion.String() != "v1-abcdef0123456789" {
			t.Fatalf("expected BlocklistVersion 'v1-abcdef0123456789', got: %q", req.BlocklistVersion.String())
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected validation success for string blocklistVersion, got: %v", err)
		}
	})

	t.Run("Browser_AllowedBrowsers_And_Rejection", func(t *testing.T) {
		validBrowsers := []string{"chrome", "edge", "brave", "firefox", ""}
		for _, b := range validBrowsers {
			req := enforcement.HeartbeatRequest{
				DeviceID:      "11111111-1111-1111-1111-111111111111",
				ClientType:    "extension",
				Platform:      "linux",
				Browser:       b,
				ClientVersion: "1.0.0",
				SentAt:        time.Now().UTC().Format(time.RFC3339),
			}
			if err := req.Validate(); err != nil {
				t.Errorf("expected browser %q to be valid, got: %v", b, err)
			}
		}

		invalidBrowsers := []string{"safari", "opera", "vivaldi", "tor", "arc"}
		for _, b := range invalidBrowsers {
			req := enforcement.HeartbeatRequest{
				DeviceID:      "11111111-1111-1111-1111-111111111111",
				ClientType:    "extension",
				Platform:      "linux",
				Browser:       b,
				ClientVersion: "1.0.0",
				SentAt:        time.Now().UTC().Format(time.RFC3339),
			}
			err := req.Validate()
			if !errors.Is(err, enforcement.ErrInvalidBrowser) {
				t.Errorf("expected ErrInvalidBrowser for %q, got: %v", b, err)
			}
		}
	})
}
