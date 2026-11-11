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
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type d8MockRepo struct {
	devices  map[string]enforcement.Device
	sessions map[string]enforcement.PairingSession
}

func newD8MockRepo() *d8MockRepo {
	return &d8MockRepo{
		devices:  make(map[string]enforcement.Device),
		sessions: make(map[string]enforcement.PairingSession),
	}
}

func (m *d8MockRepo) UpsertDevice(ctx context.Context, device enforcement.Device) error {
	key := device.UserID + "#" + device.DeviceID
	m.devices[key] = device
	return nil
}

func (m *d8MockRepo) UpdateHeartbeat(ctx context.Context, device enforcement.Device) error {
	key := device.UserID + "#" + device.DeviceID
	if existing, ok := m.devices[key]; ok && existing.Status == enforcement.DeviceStatusRevoked {
		return enforcement.ErrDeviceRevoked
	}
	m.devices[key] = device
	return nil
}

func (m *d8MockRepo) ListDevices(ctx context.Context, userID string) ([]enforcement.Device, error) {
	var list []enforcement.Device
	for _, d := range m.devices {
		if d.UserID == userID {
			list = append(list, d)
		}
	}
	return list, nil
}

func (m *d8MockRepo) GetDevice(ctx context.Context, userID, deviceID string) (*enforcement.Device, error) {
	key := userID + "#" + deviceID
	d, ok := m.devices[key]
	if !ok {
		return nil, enforcement.ErrDeviceNotFound
	}
	return &d, nil
}

func (m *d8MockRepo) RevokeDevice(ctx context.Context, userID, deviceID, reason string) error {
	key := userID + "#" + deviceID
	d, ok := m.devices[key]
	if !ok {
		return enforcement.ErrDeviceNotFound
	}
	if d.UserID != userID {
		return enforcement.ErrDeviceOwnershipMismatch
	}
	d.Status = enforcement.DeviceStatusRevoked
	d.RevokedAt = time.Now().UTC().Format(time.RFC3339)
	d.RevocationReason = reason
	d.Enforcing = false
	d.CredentialVersion++
	m.devices[key] = d
	return nil
}

func (m *d8MockRepo) SavePairingSession(ctx context.Context, session enforcement.PairingSession) error {
	m.sessions[session.Code] = session
	return nil
}

func (m *d8MockRepo) GetPairingSession(ctx context.Context, code string) (*enforcement.PairingSession, error) {
	s, ok := m.sessions[code]
	if !ok {
		return nil, enforcement.ErrDeviceNotFound
	}
	return &s, nil
}

func (m *d8MockRepo) ConfirmPairingSession(ctx context.Context, code, userID, token string) error {
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

func (m *d8MockRepo) ConsumePairingSession(ctx context.Context, code string) (*enforcement.PairingSession, error) {
	s, ok := m.sessions[code]
	if !ok {
		return nil, enforcement.ErrDeviceNotFound
	}
	if !s.Confirmed {
		return nil, enforcement.ErrUnauthorizedDevice
	}
	if time.Now().Unix() > s.ExpiresAt {
		return nil, enforcement.ErrCredentialExpired
	}
	delete(m.sessions, code)
	return &s, nil
}

type d8MockGoalLister struct {
	goals []goals.Goal
}

func (m *d8MockGoalLister) List(ctx context.Context, ownerID string) ([]goals.Goal, error) {
	var list []goals.Goal
	for _, g := range m.goals {
		if g.OwnerID == ownerID {
			list = append(list, g)
		}
	}
	return list, nil
}

func (m *d8MockGoalLister) GetByID(ctx context.Context, ownerID, goalID string) (*goals.Goal, error) {
	for _, g := range m.goals {
		if g.ID == goalID {
			return &g, nil
		}
	}
	return nil, enforcement.ErrGoalNotFound
}

type d8MockTokenGen struct{}

func (m *d8MockTokenGen) GenerateToken(userID, email, handle, name string) (string, error) {
	return "mock-token-user-" + userID, nil
}

func (m *d8MockTokenGen) GenerateDeviceToken(userID, email, handle, name, deviceID string) (string, error) {
	return "mock-token-device-" + userID + "-" + deviceID, nil
}

func TestD8_DeviceRevocation_OwnershipAndIsolation(t *testing.T) {
	repo := newD8MockRepo()
	goalLister := &d8MockGoalLister{}
	svc := enforcement.NewService(goalLister)
	svc.SetDeviceRepository(repo)
	svc.SetTokenGenerator(&d8MockTokenGen{})

	// Setup: User-1 owns Device-A; User-2 owns Device-B
	devA := enforcement.Device{
		DeviceID:        "11111111-1111-1111-1111-111111111111",
		UserID:          "user-1",
		ClientType:      "desktop_agent",
		Platform:        "windows",
		Status:          enforcement.DeviceStatusPaired,
		Enforcing:       true,
		LastHeartbeatAt: time.Now().UTC().Format(time.RFC3339),
	}
	devB := enforcement.Device{
		DeviceID:        "22222222-2222-2222-2222-222222222222",
		UserID:          "user-2",
		ClientType:      "desktop_agent",
		Platform:        "linux",
		Status:          enforcement.DeviceStatusPaired,
		Enforcing:       true,
		LastHeartbeatAt: time.Now().UTC().Format(time.RFC3339),
	}
	_ = repo.UpsertDevice(context.Background(), devA)
	_ = repo.UpsertDevice(context.Background(), devB)

	// 1. Authorized Owner Revocation: User-1 revokes Device-A
	resp, err := svc.RevokeDevice(context.Background(), "user-1", devA.DeviceID, "lost_laptop")
	if err != nil {
		t.Fatalf("RevokeDevice failed for owner: %v", err)
	}
	if resp.Status != enforcement.DeviceStatusRevoked {
		t.Errorf("expected status revoked, got %s", resp.Status)
	}
	if resp.RevokedAt == "" {
		t.Errorf("expected non-empty RevokedAt timestamp")
	}

	// Verify state in repo
	storedA, _ := repo.GetDevice(context.Background(), "user-1", devA.DeviceID)
	if storedA.Status != enforcement.DeviceStatusRevoked || storedA.Enforcing {
		t.Errorf("stored device status expected revoked with enforcing=false: %+v", storedA)
	}
	if storedA.RevocationReason != "lost_laptop" {
		t.Errorf("expected reason lost_laptop, got %s", storedA.RevocationReason)
	}

	// 2. Cross-Account Revocation: User-1 tries to revoke User-2's Device-B -> REJECTED
	_, errCross := svc.RevokeDevice(context.Background(), "user-1", devB.DeviceID, "malicious_attempt")
	if errCross == nil {
		t.Fatalf("expected error when User-1 tries to revoke User-2's device, got nil")
	}

	// Verify User-2's device was NOT altered
	storedB, _ := repo.GetDevice(context.Background(), "user-2", devB.DeviceID)
	if storedB.Status != enforcement.DeviceStatusPaired || !storedB.Enforcing {
		t.Errorf("User-2 device was improperly altered by cross-account attempt!")
	}

	// 3. Repeated / Idempotent Revocation: Revoking already-revoked device is safe
	resp2, errRep := svc.RevokeDevice(context.Background(), "user-1", devA.DeviceID, "repeat_call")
	if errRep != nil {
		t.Fatalf("repeated RevokeDevice should be idempotent, got: %v", errRep)
	}
	if resp2.Status != enforcement.DeviceStatusRevoked {
		t.Errorf("expected status revoked on repeat, got %s", resp2.Status)
	}
}

func TestD8_RevokedDevice_RejectedOnHeartbeatAndBlocklist(t *testing.T) {
	repo := newD8MockRepo()
	goalLister := &d8MockGoalLister{
		goals: []goals.Goal{
			{ID: "g1", OwnerID: "user-test", Status: "active", BlockedDomains: []string{"distraction.com"}},
		},
	}
	svc := enforcement.NewService(goalLister)
	svc.SetDeviceRepository(repo)
	svc.SetTokenGenerator(&d8MockTokenGen{})

	handler := enforcement.NewHandler(svc)

	devID := "33333333-3333-3333-3333-333333333333"
	dev := enforcement.Device{
		DeviceID:   devID,
		UserID:     "user-test",
		ClientType: "desktop_agent",
		Platform:   "windows",
		Status:     enforcement.DeviceStatusPaired,
		Enforcing:  true,
	}
	_ = repo.UpsertDevice(context.Background(), dev)

	// Step 1: Active device heartbeats and gets blocklist successfully
	hbReq := enforcement.HeartbeatRequest{
		DeviceID:         devID,
		ClientType:       "desktop_agent",
		Platform:         "windows",
		ClientVersion:    "1.0.0",
		Enforcing:        true,
		BlocklistVersion: "v1-init",
		SentAt:           time.Now().UTC().Format(time.RFC3339),
	}
	hbBody, _ := json.Marshal(hbReq)

	req1 := httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(hbBody))
	req1 = req1.WithContext(context.WithValue(req1.Context(), middleware.UserIDKey, "user-test"))
	req1 = req1.WithContext(context.WithValue(req1.Context(), middleware.DeviceIDKey, devID))
	w1 := httptest.NewRecorder()
	handler.Heartbeat(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for active device heartbeat, got %d", w1.Code)
	}

	// Step 2: Now revoke the device
	_, err := svc.RevokeDevice(context.Background(), "user-test", devID, "security_incident")
	if err != nil {
		t.Fatalf("RevokeDevice failed: %v", err)
	}

	// Step 3: Revoked device attempts to send heartbeat -> HTTP 403 DEVICE_REVOKED
	req2 := httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewReader(hbBody))
	req2 = req2.WithContext(context.WithValue(req2.Context(), middleware.UserIDKey, "user-test"))
	req2 = req2.WithContext(context.WithValue(req2.Context(), middleware.DeviceIDKey, devID))
	w2 := httptest.NewRecorder()
	handler.Heartbeat(w2, req2)

	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for revoked device heartbeat, got %d", w2.Code)
	}
	var errResp map[string]string
	_ = json.Unmarshal(w2.Body.Bytes(), &errResp)
	if errResp["error"] != "DEVICE_REVOKED" || errResp["code"] != "device_revoked" {
		t.Fatalf("expected structured DEVICE_REVOKED error, got: %+v", errResp)
	}

	// Step 4: Revoked device attempts to query blocklist -> HTTP 403 DEVICE_REVOKED
	req3 := httptest.NewRequest("GET", "/enforcement/blocklist", nil)
	req3 = req3.WithContext(context.WithValue(req3.Context(), middleware.UserIDKey, "user-test"))
	req3 = req3.WithContext(context.WithValue(req3.Context(), middleware.DeviceIDKey, devID))
	w3 := httptest.NewRecorder()
	handler.GetBlocklist(w3, req3)

	if w3.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for revoked device blocklist sync, got %d", w3.Code)
	}
	var errRespSync map[string]string
	_ = json.Unmarshal(w3.Body.Bytes(), &errRespSync)
	if errRespSync["error"] != "DEVICE_REVOKED" {
		t.Fatalf("expected DEVICE_REVOKED on sync, got: %+v", errRespSync)
	}
}

func TestD8_PairingSession_SecurityInvariants(t *testing.T) {
	repo := newD8MockRepo()
	svc := enforcement.NewService(&d8MockGoalLister{})
	svc.SetDeviceRepository(repo)
	svc.SetTokenGenerator(&d8MockTokenGen{})

	// 1. Initiate pairing
	session, err := svc.InitiatePairing(context.Background(), "desktop_agent", "windows", "44444444-4444-4444-4444-444444444444")
	if err != nil {
		t.Fatalf("InitiatePairing failed: %v", err)
	}
	if len(session.Code) != 6 {
		t.Fatalf("expected 6-char pairing code, got %s", session.Code)
	}

	// 2. Confirm pairing by User-Alpha
	errConfirm := svc.ConfirmPairing(context.Background(), session.Code, "user-alpha", "alpha@example.com", "alpha", "User Alpha")
	if errConfirm != nil {
		t.Fatalf("ConfirmPairing failed: %v", errConfirm)
	}

	// 3. Confirming AGAIN is rejected
	errConfirm2 := svc.ConfirmPairing(context.Background(), session.Code, "user-beta", "beta@example.com", "beta", "User Beta")
	if errConfirm2 == nil {
		t.Fatalf("expected error when confirming already confirmed pairing session, got nil")
	}

	// 4. Exchange pairing token
	token, errExchange := svc.ExchangePairingToken(context.Background(), session.Code)
	if errExchange != nil {
		t.Fatalf("ExchangePairingToken failed: %v", errExchange)
	}
	if token != "mock-token-device-user-alpha-44444444-4444-4444-4444-444444444444" {
		t.Fatalf("unexpected token issued: %s", token)
	}

	// 5. Replay attack: exchanging the code a SECOND time must fail (single-use token)
	_, errReplay := svc.ExchangePairingToken(context.Background(), session.Code)
	if errReplay == nil {
		t.Fatalf("CRITICAL SECURITY VIOLATION: pairing code was replayed successfully! Must be single-use.")
	}

	// 6. Expired session rejection
	expSession, _ := svc.InitiatePairing(context.Background(), "desktop_agent", "linux", "55555555-5555-5555-5555-555555555555")
	expSession.ExpiresAt = time.Now().Add(-1 * time.Hour).Unix()
	_ = repo.SavePairingSession(context.Background(), *expSession)

	errExpConfirm := svc.ConfirmPairing(context.Background(), expSession.Code, "user-gamma", "gamma@test.com", "gamma", "Gamma")
	if errExpConfirm == nil {
		t.Fatalf("expected error confirming expired pairing session, got nil")
	}
}

func TestD8_RePairingRevokedDevice_PreservesDeviceID(t *testing.T) {
	repo := newD8MockRepo()
	goalLister := &d8MockGoalLister{
		goals: []goals.Goal{
			{ID: "g2", OwnerID: "user-repair", Status: "active", BlockedDomains: []string{"focus.com"}},
		},
	}
	svc := enforcement.NewService(goalLister)
	svc.SetDeviceRepository(repo)
	svc.SetTokenGenerator(&d8MockTokenGen{})

	canonicalDeviceID := "66666666-6666-6666-6666-666666666666"

	// 1. Device was previously registered and then revoked
	dev := enforcement.Device{
		DeviceID:          canonicalDeviceID,
		UserID:            "user-repair",
		ClientType:        "desktop_agent",
		Platform:          "windows",
		Status:            enforcement.DeviceStatusRevoked,
		RevokedAt:         "2026-11-11T12:00:00Z",
		RevocationReason:  "security_audit",
		CredentialVersion: 1,
		Enforcing:         false,
	}
	_ = repo.UpsertDevice(context.Background(), dev)

	// Verify it is currently blocked
	_, errHb := svc.ProcessHeartbeat(context.Background(), "user-repair", enforcement.HeartbeatRequest{
		DeviceID:         canonicalDeviceID,
		ClientType:       "desktop_agent",
		Platform:         "windows",
		ClientVersion:    "1.0.0",
		Enforcing:        true,
		BlocklistVersion: "v1",
		SentAt:           time.Now().UTC().Format(time.RFC3339),
	})
	if errHb == nil {
		t.Fatalf("revoked device must be rejected before re-pairing")
	}

	// 2. Initiate Re-Pairing using the SAME canonical device ID
	repairSession, err := svc.InitiatePairing(context.Background(), "desktop_agent", "windows", canonicalDeviceID)
	if err != nil {
		t.Fatalf("InitiatePairing failed: %v", err)
	}

	// 3. User confirms pairing on dashboard
	errConfirm := svc.ConfirmPairing(context.Background(), repairSession.Code, "user-repair", "repair@example.com", "repair", "Repair User")
	if errConfirm != nil {
		t.Fatalf("ConfirmPairing failed: %v", errConfirm)
	}

	// 4. Agent exchanges token
	token, errEx := svc.ExchangePairingToken(context.Background(), repairSession.Code)
	if errEx != nil {
		t.Fatalf("ExchangePairingToken failed: %v", errEx)
	}
	if token == "" {
		t.Fatalf("expected non-empty token")
	}

	// 5. Verify device in repo is now PAIRED, revokedAt cleared, and credentialVersion incremented
	restoredDev, _ := repo.GetDevice(context.Background(), "user-repair", canonicalDeviceID)
	if restoredDev.Status != enforcement.DeviceStatusPaired {
		t.Errorf("expected restored device status paired, got %s", restoredDev.Status)
	}
	if restoredDev.RevokedAt != "" {
		t.Errorf("expected RevokedAt to be cleared upon re-pairing, got %s", restoredDev.RevokedAt)
	}
	if restoredDev.CredentialVersion <= 1 {
		t.Errorf("expected CredentialVersion to be incremented upon re-pairing, got %d", restoredDev.CredentialVersion)
	}

	// 6. Device heartbeats and syncs again successfully
	hbResp, errAfter := svc.ProcessHeartbeat(context.Background(), "user-repair", enforcement.HeartbeatRequest{
		DeviceID:         canonicalDeviceID,
		ClientType:       "desktop_agent",
		Platform:         "windows",
		ClientVersion:    "1.0.0",
		Enforcing:        true,
		BlocklistVersion: "v1-restored",
		SentAt:           time.Now().UTC().Format(time.RFC3339),
	})
	if errAfter != nil {
		t.Fatalf("heartbeat should succeed after re-pairing: %v", errAfter)
	}
	if !hbResp.Enforcing {
		t.Errorf("expected enforcing to be true after re-pairing")
	}
}

func TestD8_ZeroCredentialExposure_DashboardAPIs(t *testing.T) {
	repo := newD8MockRepo()
	goalLister := &d8MockGoalLister{
		goals: []goals.Goal{
			{ID: "g3", OwnerID: "user-audit", ApproverID: "partner-audit", Status: "active"},
		},
	}
	svc := enforcement.NewService(goalLister)
	svc.SetDeviceRepository(repo)
	svc.SetTokenGenerator(&d8MockTokenGen{})

	dev := enforcement.Device{
		DeviceID:         "77777777-7777-7777-7777-777777777777",
		UserID:           "user-audit",
		ClientType:       "desktop_agent",
		Platform:         "windows",
		Status:           enforcement.DeviceStatusPaired,
		Enforcing:        true,
		BlocklistVersion: "v1-audit",
		LastHeartbeatAt:  time.Now().UTC().Format(time.RFC3339),
	}
	_ = repo.UpsertDevice(context.Background(), dev)

	// 1. Query GetDevices (owner view)
	devResp, err := svc.GetDevices(context.Background(), "user-audit")
	if err != nil {
		t.Fatalf("GetDevices failed: %v", err)
	}
	bytesDevs, _ := json.Marshal(devResp)
	jsonStr := string(bytesDevs)
	if bytes.Contains(bytesDevs, []byte("Token")) || bytes.Contains(bytesDevs, []byte("token")) || bytes.Contains(bytesDevs, []byte("jwt")) || bytes.Contains(bytesDevs, []byte("secret")) {
		t.Fatalf("CRITICAL SECURITY VIOLATION: GetDevices response leaks tokens/secrets: %s", jsonStr)
	}

	// 2. Query GoalEnforcementStatus (approver view)
	partnerResp, err := svc.GetGoalEnforcementStatus(context.Background(), "partner-audit", "partner@test.com", "partner", "g3")
	if err != nil {
		t.Fatalf("GetGoalEnforcementStatus failed: %v", err)
	}
	// Invariant: Approver must NOT see concrete device objects or full device IDs
	if len(partnerResp.Devices) > 0 {
		t.Fatalf("CRITICAL PRIVACY VIOLATION: Approver can see concrete device list!")
	}
	if partnerResp.DevicesCount != 1 || partnerResp.EnforcingCount != 1 {
		t.Errorf("expected aggregate counts to be 1, got %+v", partnerResp)
	}
}
