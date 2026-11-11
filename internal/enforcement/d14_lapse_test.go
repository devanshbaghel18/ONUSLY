package enforcement_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/enforcement"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/metrics"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type d14MockNotifSender struct {
	lapsedNotifications []struct {
		RecipientID string
		EpisodeID   string
		GoalID      string
		Title       string
		Message     string
	}
	recoveredNotifications []struct {
		RecipientID string
		EpisodeID   string
		GoalID      string
		Title       string
		Message     string
	}
}

func (m *d14MockNotifSender) CreateFromEnforcementLapsed(ctx context.Context, recipientID, episodeID, goalID, title, message string) error {
	m.lapsedNotifications = append(m.lapsedNotifications, struct {
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

func (m *d14MockNotifSender) CreateFromEnforcementRecovered(ctx context.Context, recipientID, episodeID, goalID, title, message string) error {
	m.recoveredNotifications = append(m.recoveredNotifications, struct {
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

type d14MockGoalRepo struct {
	goals []goals.Goal
}

func (m *d14MockGoalRepo) List(ctx context.Context, ownerID string) ([]goals.Goal, error) {
	var userGoals []goals.Goal
	for _, g := range m.goals {
		if g.OwnerID == ownerID {
			userGoals = append(userGoals, g)
		}
	}
	return userGoals, nil
}

type d14MockDeviceRepo struct {
	devices map[string]enforcement.Device
}

func newD14MockDeviceRepo() *d14MockDeviceRepo {
	return &d14MockDeviceRepo{
		devices: make(map[string]enforcement.Device),
	}
}

func (r *d14MockDeviceRepo) GetDevice(ctx context.Context, userID, deviceID string) (*enforcement.Device, error) {
	key := userID + "#" + deviceID
	dev, ok := r.devices[key]
	if !ok {
		return nil, enforcement.ErrDeviceNotFound
	}
	return &dev, nil
}

func (r *d14MockDeviceRepo) UpdateHeartbeat(ctx context.Context, dev enforcement.Device) error {
	key := dev.UserID + "#" + dev.DeviceID
	r.devices[key] = dev
	return nil
}

func (r *d14MockDeviceRepo) RevokeDevice(ctx context.Context, userID, deviceID, reason string) error {
	return nil
}

func (r *d14MockDeviceRepo) ListDevices(ctx context.Context, userID string) ([]enforcement.Device, error) {
	var list []enforcement.Device
	for _, d := range r.devices {
		if d.UserID == userID {
			list = append(list, d)
		}
	}
	return list, nil
}

func (r *d14MockDeviceRepo) SavePairingSession(ctx context.Context, session enforcement.PairingSession) error {
	return nil
}

func (r *d14MockDeviceRepo) GetPairingSession(ctx context.Context, code string) (*enforcement.PairingSession, error) {
	return nil, nil
}

func (r *d14MockDeviceRepo) ConfirmPairingSession(ctx context.Context, code, confirmedBy, token string) error {
	return nil
}

func (r *d14MockDeviceRepo) ConsumePairingSession(ctx context.Context, code string) (*enforcement.PairingSession, error) {
	return nil, nil
}

func (r *d14MockDeviceRepo) UpsertDevice(ctx context.Context, dev enforcement.Device) error {
	key := dev.UserID + "#" + dev.DeviceID
	r.devices[key] = dev
	return nil
}

func TestD14_LapseDeduplicationAndRecoveryNotification(t *testing.T) {
	mockRepo := newD14MockDeviceRepo()
	mockNotif := &d14MockNotifSender{}
	mockGoals := &d14MockGoalRepo{
		goals: []goals.Goal{
			{
				ID:         "goal-sre-1",
				OwnerID:    "user-sre",
				ApproverID: "approver-sre",
				Title:      "Reliability Engineering Goal",
				Status:     "active",
			},
		},
	}

	service := enforcement.NewService(mockGoals)
	service.SetDeviceRepository(mockRepo)
	service.SetNotificationSender(mockNotif)
	handler := enforcement.NewHandler(service)

	deviceID := "22222222-3333-4444-5555-666666666666"

	// 1. Initial Healthy Heartbeat
	healthyPayload := map[string]interface{}{
		"deviceId":         deviceID,
		"clientType":       "desktop_agent",
		"platform":         "windows",
		"clientVersion":    "1.0.0-rc1",
		"enforcing":        true,
		"blocklistVersion": "v1-abcdef0123456789",
		"issues":           []string{},
		"sentAt":           time.Now().UTC().Format(time.RFC3339),
	}

	body, _ := json.Marshal(healthyPayload)
	req := httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-sre"))
	rec := httptest.NewRecorder()

	handler.Heartbeat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	dev, err := mockRepo.GetDevice(context.Background(), "user-sre", deviceID)
	if err != nil {
		t.Fatalf("failed to retrieve device: %v", err)
	}
	if dev.HealthStatus != enforcement.HealthStateHealthy {
		t.Errorf("expected healthStatus HEALTHY, got %q", dev.HealthStatus)
	}
	if dev.LapseEpisodeID != "" {
		t.Errorf("expected empty LapseEpisodeID for healthy device, got %q", dev.LapseEpisodeID)
	}
	if len(mockNotif.lapsedNotifications) != 0 {
		t.Errorf("expected 0 lapse notifications, got %d", len(mockNotif.lapsedNotifications))
	}

	// 2. Report Lapse (dns_drift)
	lapsedPayload1 := map[string]interface{}{
		"deviceId":         deviceID,
		"clientType":       "desktop_agent",
		"platform":         "windows",
		"clientVersion":    "1.0.0-rc1",
		"enforcing":        false,
		"blocklistVersion": "v1-abcdef0123456789",
		"issues":           []string{"dns_drift"},
		"sentAt":           time.Now().UTC().Format(time.RFC3339),
	}

	body, _ = json.Marshal(lapsedPayload1)
	req = httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-sre"))
	rec = httptest.NewRecorder()

	handler.Heartbeat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	dev, _ = mockRepo.GetDevice(context.Background(), "user-sre", deviceID)
	if dev.HealthStatus != enforcement.HealthStateDNSUnavailable {
		t.Errorf("expected healthStatus DNS_UNAVAILABLE, got %q", dev.HealthStatus)
	}
	episode1 := dev.LapseEpisodeID
	if episode1 == "" {
		t.Fatalf("expected non-empty LapseEpisodeID on lapse")
	}

	// Must notify 1 owner + 1 approver = 2 notifications
	if len(mockNotif.lapsedNotifications) != 2 {
		t.Fatalf("expected 2 lapse notifications (owner + approver), got %d", len(mockNotif.lapsedNotifications))
	}

	// 3. Second Heartbeat still in lapse (Deduplication Check)
	body, _ = json.Marshal(lapsedPayload1)
	req = httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-sre"))
	rec = httptest.NewRecorder()

	handler.Heartbeat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	dev, _ = mockRepo.GetDevice(context.Background(), "user-sre", deviceID)
	if dev.LapseEpisodeID != episode1 {
		t.Errorf("expected episode ID %q to be preserved, got %q", episode1, dev.LapseEpisodeID)
	}
	// INVARIANT: Notification count must NOT increase (deduplication!)
	if len(mockNotif.lapsedNotifications) != 2 {
		t.Fatalf("expected lapse notifications to remain 2 (deduplicated), got %d", len(mockNotif.lapsedNotifications))
	}

	// 4. Third Heartbeat with different issue (policy_missing) still during lapse
	lapsedPayload2 := map[string]interface{}{
		"deviceId":         deviceID,
		"clientType":       "desktop_agent",
		"platform":         "windows",
		"clientVersion":    "1.0.0-rc1",
		"enforcing":        false,
		"blocklistVersion": "v1-abcdef0123456789",
		"issues":           []string{"policy_missing"},
		"sentAt":           time.Now().UTC().Format(time.RFC3339),
	}
	body, _ = json.Marshal(lapsedPayload2)
	req = httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-sre"))
	rec = httptest.NewRecorder()

	handler.Heartbeat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	if len(mockNotif.lapsedNotifications) != 2 {
		t.Fatalf("expected notifications to remain 2 (deduplicated), got %d", len(mockNotif.lapsedNotifications))
	}

	// 5. Recovery Heartbeat: enforcement resumed!
	body, _ = json.Marshal(healthyPayload)
	req = httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-sre"))
	rec = httptest.NewRecorder()

	handler.Heartbeat(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	dev, _ = mockRepo.GetDevice(context.Background(), "user-sre", deviceID)
	if dev.LapseEpisodeID != "" {
		t.Errorf("expected LapseEpisodeID to be cleared after recovery, got %q", dev.LapseEpisodeID)
	}
	if dev.HealthStatus != enforcement.HealthStateHealthy {
		t.Errorf("expected HealthStatus HEALTHY after recovery, got %q", dev.HealthStatus)
	}

	// INVARIANT: Clean recovery notifications emitted!
	if len(mockNotif.recoveredNotifications) != 2 {
		t.Fatalf("expected 2 recovery notifications (owner + approver), got %d", len(mockNotif.recoveredNotifications))
	}

	ownerRecovery := mockNotif.recoveredNotifications[0]
	if ownerRecovery.RecipientID != "user-sre" {
		t.Errorf("expected owner recipient 'user-sre', got %q", ownerRecovery.RecipientID)
	}
	approverRecovery := mockNotif.recoveredNotifications[1]
	if approverRecovery.RecipientID != "approver-sre" {
		t.Errorf("expected approver recipient 'approver-sre', got %q", approverRecovery.RecipientID)
	}

	// Verify Prometheus text exposition includes the lapse episode metrics
	promText := metrics.DefaultRegistry.PrometheusFormat()
	if !bytes.Contains([]byte(promText), []byte("onusly_lapse_episodes_started_total")) {
		t.Errorf("expected prometheus metrics to include onusly_lapse_episodes_started_total")
	}
	if !bytes.Contains([]byte(promText), []byte("onusly_lapse_episodes_resolved_total")) {
		t.Errorf("expected prometheus metrics to include onusly_lapse_episodes_resolved_total")
	}
}
