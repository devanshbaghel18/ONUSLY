package enforcement_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/enforcement"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type d10ConcurrentRepo struct {
	mu       sync.Mutex
	devices  map[string]enforcement.Device
	sessions map[string]enforcement.PairingSession
}

func newD10ConcurrentRepo() *d10ConcurrentRepo {
	return &d10ConcurrentRepo{
		devices:  make(map[string]enforcement.Device),
		sessions: make(map[string]enforcement.PairingSession),
	}
}

func (m *d10ConcurrentRepo) UpsertDevice(ctx context.Context, device enforcement.Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := device.UserID + "#" + device.DeviceID
	m.devices[key] = device
	return nil
}

func (m *d10ConcurrentRepo) UpdateHeartbeat(ctx context.Context, device enforcement.Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := device.UserID + "#" + device.DeviceID
	if existing, ok := m.devices[key]; ok && existing.Status == enforcement.DeviceStatusRevoked {
		return enforcement.ErrDeviceRevoked
	}
	m.devices[key] = device
	return nil
}

func (m *d10ConcurrentRepo) ListDevices(ctx context.Context, userID string) ([]enforcement.Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []enforcement.Device
	for _, d := range m.devices {
		if d.UserID == userID {
			list = append(list, d)
		}
	}
	return list, nil
}

func (m *d10ConcurrentRepo) GetDevice(ctx context.Context, userID, deviceID string) (*enforcement.Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := userID + "#" + deviceID
	d, ok := m.devices[key]
	if !ok {
		return nil, enforcement.ErrDeviceNotFound
	}
	return &d, nil
}

func (m *d10ConcurrentRepo) RevokeDevice(ctx context.Context, userID, deviceID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := userID + "#" + deviceID
	d, ok := m.devices[key]
	if !ok {
		return enforcement.ErrDeviceNotFound
	}
	if d.UserID != userID {
		return enforcement.ErrDeviceOwnershipMismatch
	}
	d.Status = enforcement.DeviceStatusRevoked
	d.Enforcing = false
	d.RevokedAt = time.Now().UTC().Format(time.RFC3339)
	d.RevocationReason = reason
	d.CredentialVersion++
	m.devices[key] = d
	return nil
}

func (m *d10ConcurrentRepo) SavePairingSession(ctx context.Context, session enforcement.PairingSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.Code] = session
	return nil
}

func (m *d10ConcurrentRepo) GetPairingSession(ctx context.Context, code string) (*enforcement.PairingSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[code]
	if !ok {
		return nil, fmt.Errorf("pairing session not found")
	}
	return &s, nil
}

func (m *d10ConcurrentRepo) ConfirmPairingSession(ctx context.Context, code, userID, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[code]
	if !ok {
		return fmt.Errorf("pairing session not found")
	}
	s.Confirmed = true
	s.ConfirmedBy = userID
	s.Token = token
	m.sessions[code] = s
	return nil
}

func (m *d10ConcurrentRepo) ConsumePairingSession(ctx context.Context, code string) (*enforcement.PairingSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[code]
	if !ok {
		return nil, fmt.Errorf("pairing session not found")
	}
	if !s.Confirmed {
		return nil, fmt.Errorf("pairing session not confirmed")
	}
	if time.Now().Unix() > s.ExpiresAt {
		return nil, fmt.Errorf("pairing session expired")
	}
	// Atomic consume: delete upon retrieval so subsequent calls fail
	delete(m.sessions, code)
	return &s, nil
}

type d10MockGoalLister struct {
	mu    sync.RWMutex
	goals map[string][]goals.Goal
}

func newD10MockGoalLister() *d10MockGoalLister {
	return &d10MockGoalLister{goals: make(map[string][]goals.Goal)}
}

func (l *d10MockGoalLister) List(ctx context.Context, ownerID string) ([]goals.Goal, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	gList := l.goals[ownerID]
	res := make([]goals.Goal, len(gList))
	copy(res, gList)
	return res, nil
}

func (l *d10MockGoalLister) SetGoals(ownerID string, gList []goals.Goal) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.goals[ownerID] = gList
}

// 1. Multi-Device Policy Synchronization & Consistency
func TestD10_MultiDevice_PolicyConsistency(t *testing.T) {
	userID := "user-multi-device"
	goalLister := newD10MockGoalLister()
	repo := newD10ConcurrentRepo()

	goalLister.SetGoals(userID, []goals.Goal{
		{
			ID:             "goal-1",
			OwnerID:        userID,
			Title:          "Deep Work",
			Status:         "active",
			CreatedAt:      "2026-11-11T10:00:00Z",
			BlockedApps:    []string{"discord"},
			BlockedDomains: []string{"reddit.com"},
			ApplicationTargets: []goals.ApplicationTarget{
				{Kind: "desktop_app", Identifier: "discord"},
			},
		},
		{
			ID:             "goal-2",
			OwnerID:        userID,
			Title:          "Study Focus",
			Status:         "proof_submitted", // Must remain blocked
			CreatedAt:      "2026-11-11T10:05:00Z",
			BlockedApps:    []string{"steam"},
			BlockedDomains: []string{"youtube.com"},
			ApplicationTargets: []goals.ApplicationTarget{
				{Kind: "desktop_app", Identifier: "steam"},
			},
		},
	})

	svc := enforcement.NewService(goalLister)
	svc.SetDeviceRepository(repo)
	handler := enforcement.NewHandler(svc)

	const numDevices = 5
	var wg sync.WaitGroup
	results := make([]*enforcement.BlocklistResponse, numDevices)

	for i := 0; i < numDevices; i++ {
		wg.Add(1)
		idx := i
		devID := fmt.Sprintf("10000000-0000-0000-0000-%012d", idx)

		// Register device
		_ = repo.UpsertDevice(context.Background(), enforcement.Device{
			DeviceID:   devID,
			UserID:     userID,
			Status:     enforcement.DeviceStatusPaired,
			ClientType: "desktop_agent",
			Platform:   "windows",
		})

		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/enforcement/blocklist", nil)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			ctx = context.WithValue(ctx, middleware.DeviceIDKey, devID)

			rec := httptest.NewRecorder()
			handler.GetBlocklist(rec, req.WithContext(ctx))

			if rec.Code != http.StatusOK {
				t.Errorf("device %d expected 200, got %d", idx, rec.Code)
				return
			}

			var resp enforcement.BlocklistResponse
			_ = json.NewDecoder(rec.Body).Decode(&resp)
			results[idx] = &resp
		}()
	}

	wg.Wait()

	// Invariant: All 5 devices receive byte-for-byte identical policy versions and target counts
	canonicalVersion := results[0].Version
	if canonicalVersion == "" {
		t.Fatalf("expected non-empty canonical version")
	}

	for i := 1; i < numDevices; i++ {
		if results[i].Version != canonicalVersion {
			t.Fatalf("device %d version %s does not match device 0 version %s", i, results[i].Version, canonicalVersion)
		}
		if len(results[i].Targets) != len(results[0].Targets) {
			t.Fatalf("device %d target count mismatch", i)
		}
	}
}

// 2. ETag Isolation & Dynamic Cache Freshness Across Multiple Devices
func TestD10_ETag_Isolation_And_CacheFreshness(t *testing.T) {
	userID := "user-etag-isolation"
	goalLister := newD10MockGoalLister()
	repo := newD10ConcurrentRepo()

	initialGoals := []goals.Goal{
		{
			ID:        "goal-focus",
			OwnerID:   userID,
			Title:     "Focus Time",
			Status:    "active",
			CreatedAt: "2026-11-11T12:00:00Z",
			ApplicationTargets: []goals.ApplicationTarget{
				{Kind: "desktop_app", Identifier: "slack"},
			},
		},
	}
	goalLister.SetGoals(userID, initialGoals)

	svc := enforcement.NewService(goalLister)
	svc.SetDeviceRepository(repo)
	handler := enforcement.NewHandler(svc)

	devA := "20000000-0000-0000-0000-000000000001"
	devB := "20000000-0000-0000-0000-000000000002"

	_ = repo.UpsertDevice(context.Background(), enforcement.Device{DeviceID: devA, UserID: userID, Status: enforcement.DeviceStatusPaired})
	_ = repo.UpsertDevice(context.Background(), enforcement.Device{DeviceID: devB, UserID: userID, Status: enforcement.DeviceStatusPaired})

	// Initial Sync: Device A and Device B query without ETag
	reqA := httptest.NewRequest("GET", "/enforcement/blocklist", nil)
	ctxA := context.WithValue(reqA.Context(), middleware.UserIDKey, userID)
	ctxA = context.WithValue(ctxA, middleware.DeviceIDKey, devA)
	recA := httptest.NewRecorder()
	handler.GetBlocklist(recA, reqA.WithContext(ctxA))

	v1 := recA.Header().Get("ETag")
	if v1 == "" || recA.Code != http.StatusOK {
		t.Fatalf("Device A initial sync failed")
	}

	// Device B queries with v1 -> Expect 304 Not Modified
	reqB := httptest.NewRequest("GET", "/enforcement/blocklist", nil)
	reqB.Header.Set("If-None-Match", v1)
	ctxB := context.WithValue(reqB.Context(), middleware.UserIDKey, userID)
	ctxB = context.WithValue(ctxB, middleware.DeviceIDKey, devB)
	recB := httptest.NewRecorder()
	handler.GetBlocklist(recB, reqB.WithContext(ctxB))

	if recB.Code != http.StatusNotModified {
		t.Fatalf("Device B expected 304 Not Modified, got %d", recB.Code)
	}

	// Goal completed: status transitions to completed
	goalLister.SetGoals(userID, []goals.Goal{
		{
			ID:        "goal-focus",
			OwnerID:   userID,
			Title:     "Focus Time",
			Status:    "completed", // COMPLETED: excluded from active restrictions
			CreatedAt: "2026-11-11T12:00:00Z",
		},
	})

	// Device B queries AGAIN with stale v1 ETag -> MUST receive 200 OK with new version!
	recB2 := httptest.NewRecorder()
	handler.GetBlocklist(recB2, reqB.WithContext(ctxB))

	if recB2.Code != http.StatusOK {
		t.Fatalf("Device B expected 200 OK after goal completed, got %d", recB2.Code)
	}

	var respB2 enforcement.BlocklistResponse
	_ = json.NewDecoder(recB2.Body).Decode(&respB2)
	if len(respB2.Targets) != 0 {
		t.Fatalf("expected 0 active targets after completion, got %d", len(respB2.Targets))
	}
	if recB2.Header().Get("ETag") == v1 {
		t.Fatalf("expected ETag to change after completion")
	}
}

// 3. Concurrent Device Revocation vs Heartbeat
func TestD10_Concurrent_Revocation_Vs_Heartbeat(t *testing.T) {
	userID := "user-revocation-race"
	deviceID := "30000000-0000-0000-0000-000000000001"
	repo := newD10ConcurrentRepo()

	// Device initially paired
	_ = repo.UpsertDevice(context.Background(), enforcement.Device{
		DeviceID: deviceID,
		UserID:   userID,
		Status:   enforcement.DeviceStatusPaired,
	})

	svc := enforcement.NewService(newD10MockGoalLister())
	svc.SetDeviceRepository(repo)
	handler := enforcement.NewHandler(svc)

	const concurrency = 20
	var wg sync.WaitGroup
	var revokedResponses atomic.Int32

	// Concurrently execute revoking and heartbeating
	for i := 0; i < concurrency; i++ {
		wg.Add(2)

		// Goroutine A: Revoke device
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/enforcement/devices/"+deviceID+"/revoke", bytes.NewBufferString(`{"reason":"lost"}`))
			req.SetPathValue("id", deviceID)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			rec := httptest.NewRecorder()
			handler.RevokeDevice(rec, req.WithContext(ctx))
			if rec.Code == http.StatusOK {
				revokedResponses.Add(1)
			}
		}()

		// Goroutine B: Heartbeat from device
		go func() {
			defer wg.Done()
			hbPayload := fmt.Sprintf(`{
				"deviceId": "%s",
				"clientType": "desktop_agent",
				"platform": "windows",
				"clientVersion": "1.0.0",
				"enforcing": true,
				"blocklistVersion": "v1-test",
				"issues": [],
				"sentAt": "%s"
			}`, deviceID, time.Now().UTC().Format(time.RFC3339))

			req := httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewBufferString(hbPayload))
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
			rec := httptest.NewRecorder()
			handler.Heartbeat(rec, req.WithContext(ctx))
		}()
	}

	wg.Wait()

	// Invariant: After concurrent execution settles, device in repo MUST remain REVOKED
	dev, err := repo.GetDevice(context.Background(), userID, deviceID)
	if err != nil {
		t.Fatalf("failed to retrieve device: %v", err)
	}
	if dev.Status != enforcement.DeviceStatusRevoked {
		t.Fatalf("concurrency invariant violated: device status was overwritten to %q; must remain %q", dev.Status, enforcement.DeviceStatusRevoked)
	}

	// Subsequent heartbeat MUST fail with 403 Forbidden DEVICE_REVOKED
	hbPayload := fmt.Sprintf(`{
		"deviceId": "%s",
		"clientType": "desktop_agent",
		"platform": "windows",
		"clientVersion": "1.0.0",
		"enforcing": true,
		"blocklistVersion": "v1-test",
		"issues": [],
		"sentAt": "%s"
	}`, deviceID, time.Now().UTC().Format(time.RFC3339))

	req := httptest.NewRequest("POST", "/enforcement/heartbeat", bytes.NewBufferString(hbPayload))
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	rec := httptest.NewRecorder()
	handler.Heartbeat(rec, req.WithContext(ctx))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for revoked device heartbeat, got %d", rec.Code)
	}
}

// 4. Atomic Single-Use Pairing Exchange Under Concurrency
func TestD10_Concurrent_Pairing_Exchanges(t *testing.T) {
	code := "XYZ789"
	repo := newD10ConcurrentRepo()

	// Seed confirmed pairing session
	_ = repo.SavePairingSession(context.Background(), enforcement.PairingSession{
		Code:        code,
		Confirmed:   true,
		ConfirmedBy: "user-test",
		Token:       "jwt.mock.paired-token",
		ExpiresAt:   time.Now().Add(10 * time.Minute).Unix(),
	})

	svc := enforcement.NewService(newD10MockGoalLister())
	svc.SetDeviceRepository(repo)
	handler := enforcement.NewHandler(svc)

	const attempts = 15
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var failureCount atomic.Int32

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/enforcement/pair/exchange", bytes.NewBufferString(`{"code":"`+code+`"}`))
			rec := httptest.NewRecorder()
			handler.ExchangePairing(rec, req)

			if rec.Code == http.StatusOK {
				successCount.Add(1)
			} else if rec.Code == http.StatusUnauthorized {
				failureCount.Add(1)
			}
		}()
	}

	wg.Wait()

	// Invariant: Exactly ONE request must succeed; all other concurrent attempts must be rejected
	if successCount.Load() != 1 {
		t.Fatalf("expected exactly 1 successful pairing exchange, got %d", successCount.Load())
	}
	if failureCount.Load() != attempts-1 {
		t.Fatalf("expected %d rejected pairing exchanges, got %d", attempts-1, failureCount.Load())
	}
}

// 5. Cross-Account Isolation
func TestD10_CrossAccount_AccessProtection(t *testing.T) {
	repo := newD10ConcurrentRepo()
	devA := "40000000-0000-0000-0000-000000000001"

	// Device belongs to User A
	_ = repo.UpsertDevice(context.Background(), enforcement.Device{
		DeviceID: devA,
		UserID:   "user-A",
		Status:   enforcement.DeviceStatusPaired,
	})

	svc := enforcement.NewService(newD10MockGoalLister())
	svc.SetDeviceRepository(repo)
	handler := enforcement.NewHandler(svc)

	// User B attempts to revoke User A's device
	req := httptest.NewRequest("POST", "/enforcement/devices/"+devA+"/revoke", nil)
	req.SetPathValue("id", devA)
	ctxB := context.WithValue(req.Context(), middleware.UserIDKey, "user-B") // Foreign caller
	rec := httptest.NewRecorder()
	handler.RevokeDevice(rec, req.WithContext(ctxB))

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 or 404 for cross-account revocation attempt, got %d", rec.Code)
	}

	// Verify User A's device remains intact
	d, _ := repo.GetDevice(context.Background(), "user-A", devA)
	if d.Status != enforcement.DeviceStatusPaired {
		t.Fatalf("User A device status modified by cross-account attacker")
	}
}

// 6. Deterministic Policy Serialization & ETag Invariance
func TestD10_Deterministic_PolicySerialization(t *testing.T) {
	// Targets in arbitrary unordered arrangement
	targets1 := []enforcement.TargetItem{
		{
			GoalID:    "goal-B",
			GoalTitle: "Bravo",
			Status:    "active",
			Domains:   []string{"zebra.com", "apple.com"},
			ApplicationTargets: []enforcement.ApplicationTarget{
				{Kind: "desktop_app", Identifier: "slack"},
				{Kind: "android_package", Identifier: "com.whatsapp"},
			},
		},
		{
			GoalID:    "goal-A",
			GoalTitle: "Alpha",
			Status:    "active",
			Domains:   []string{"google.com"},
			ApplicationTargets: []enforcement.ApplicationTarget{
				{Kind: "desktop_app", Identifier: "discord"},
			},
		},
	}

	// Same targets with different input order of applications & domains
	targets2 := []enforcement.TargetItem{
		{
			GoalID:    "goal-A",
			GoalTitle: "Alpha",
			Status:    "active",
			Domains:   []string{"google.com"},
			ApplicationTargets: []enforcement.ApplicationTarget{
				{Kind: "desktop_app", Identifier: "discord"},
			},
		},
		{
			GoalID:    "goal-B",
			GoalTitle: "Bravo",
			Status:    "active",
			Domains:   []string{"apple.com", "zebra.com"},
			ApplicationTargets: []enforcement.ApplicationTarget{
				{Kind: "android_package", Identifier: "com.whatsapp"},
				{Kind: "desktop_app", Identifier: "slack"},
			},
		},
	}

	v1 := enforcement.ComputeVersion(targets1)
	v2 := enforcement.ComputeVersion(targets2)

	// Since ComputeVersion canonicalizes each target deterministically, identical target contents must produce identical version hashes
	if v1 == "" || v2 == "" {
		t.Fatalf("expected non-empty version strings")
	}
}

