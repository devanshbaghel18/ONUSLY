package metrics_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/metrics"
)

func TestD14_Metrics_PrometheusExposition(t *testing.T) {
	reg := metrics.NewRegistry()

	// 1. Record various metrics
	reg.RecordHTTPRequest("GET", "/health", 200, 1500*time.Microsecond)
	reg.RecordHTTPRequest("POST", "/goals/12345678-abcd-1234-abcd-123456789012", 400, 3*time.Millisecond)
	reg.RecordAuthFailure("expired_token")
	reg.RecordAuthFailure("invalid_signature")
	reg.RecordAuthzFailure("goal")
	reg.RecordLapseEpisode("started")
	reg.RecordLapseEpisode("resolved")
	reg.RecordPairingOutcome("attempt", "")
	reg.RecordPairingOutcome("success", "")
	reg.RecordPairingOutcome("failure", "code_expired")
	reg.RecordNotificationDelivery("proof_submitted", true)
	reg.RecordNotificationDelivery("enforcement_lapsed", false)
	reg.RecordUpdateVerification("windows", true, "")
	reg.RecordUpdateVerification("linux", false, "signature_invalid")
	reg.RecordDBOperation("GetItem", "Devices", true, 2*time.Millisecond)
	reg.RecordDBOperation("PutItem", "Goals", false, 5*time.Millisecond)
	reg.RecordPolicyVersionChange()
	reg.RecordSyncOutcome("fresh_policy")
	reg.RecordHeartbeat("windows", "desktop_agent", "healthy")

	// 2. Validate Prometheus text exposition
	prom := reg.PrometheusFormat()

	expectedSubstrings := []string{
		"# HELP onusly_api_requests_total",
		"# TYPE onusly_api_requests_total counter",
		`onusly_api_requests_total{method="GET",path="/health",status="OK"} 1`,
		`onusly_api_failures_total{method="POST",path="/goals/:id",status="Bad Request"} 1`,
		"# HELP onusly_auth_failures_total",
		`onusly_auth_failures_total{reason="expired_token"} 1`,
		`onusly_auth_failures_total{reason="invalid_signature"} 1`,
		`onusly_authz_failures_total{resource="goal"} 1`,
		"onusly_lapse_episodes_started_total 1",
		"onusly_lapse_episodes_resolved_total 1",
		"onusly_pairing_attempts_total 1",
		"onusly_pairing_success_total 1",
		`onusly_pairing_failures_total{reason="code_expired"} 1`,
		`onusly_notifications_total{type="proof_submitted"} 1`,
		`onusly_update_verifications_total{platform="windows"} 1`,
		`onusly_db_operations_total{op="GetItem",table="Devices",status="success"} 1`,
		`onusly_sync_outcomes_total{outcome="fresh_policy"} 1`,
		`onusly_heartbeats_total{platform="windows",client_type="desktop_agent",status="healthy"} 1`,
		"onusly_policy_version_changes_total 1",
		"# TYPE onusly_api_latency_ms gauge",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(prom, sub) {
			t.Errorf("Prometheus output missing expected string %q\nFull output:\n%s", sub, prom)
		}
	}

	// 3. Validate JSON snapshot
	snap := reg.Snapshot()
	if snap["privacy_guarantee"] == nil {
		t.Errorf("expected privacy guarantee in snapshot")
	}
	if snap["lapse_episodes_started_total"] != uint64(1) {
		t.Errorf("expected lapse_episodes_started_total=1, got %v", snap["lapse_episodes_started_total"])
	}
	if snap["pairing_success_total"] != uint64(1) {
		t.Errorf("expected pairing_success_total=1, got %v", snap["pairing_success_total"])
	}
}

func TestD14_Metrics_ZeroPIIPathSanitization(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"/health", "/health"},
		{"/auth/google", "/auth/google"},
		{"/goals/c2a291f0-0fa4-41d6-84d9-813f4be8a8c1", "/goals/:id"},
		{"/enforcement/devices/143489b2-2fd8-428b-bfa8-2631cc2be8c3/revoke", "/enforcement/devices/:id/revoke"},
		{"/users/user-1234567890/profile", "/users/:id/profile"},
		{"/proofs/proof-abcdef123456", "/proofs/:id"},
	}

	for _, tc := range testCases {
		sanitized := metrics.SanitizePath(tc.input)
		if sanitized != tc.expected {
			t.Errorf("SanitizePath(%q) = %q; expected %q", tc.input, sanitized, tc.expected)
		}
	}
}

func TestD14_Metrics_HandlerFormatsAndAccess(t *testing.T) {
	reg := metrics.NewRegistry()
	reg.RecordHTTPRequest("GET", "/health", 200, time.Millisecond)
	handler := reg.Handler()

	// 1. JSON Request (default)
	reqJSON := httptest.NewRequest("GET", "/metrics", nil)
	reqJSON.Header.Set("Accept", "application/json")
	recJSON := httptest.NewRecorder()
	handler(recJSON, reqJSON)

	if recJSON.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for JSON metrics, got %d", recJSON.Code)
	}
	if !strings.Contains(recJSON.Header().Get("Content-Type"), "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", recJSON.Header().Get("Content-Type"))
	}
	var snap map[string]interface{}
	if err := json.Unmarshal(recJSON.Body.Bytes(), &snap); err != nil {
		t.Fatalf("failed to parse JSON metrics response: %v", err)
	}

	// 2. Prometheus Format via query param
	reqProm := httptest.NewRequest("GET", "/metrics?format=prometheus", nil)
	recProm := httptest.NewRecorder()
	handler(recProm, reqProm)

	if recProm.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Prometheus metrics, got %d", recProm.Code)
	}
	if !strings.Contains(recProm.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("expected Content-Type text/plain, got %q", recProm.Header().Get("Content-Type"))
	}
	if !strings.Contains(recProm.Body.String(), "# HELP onusly_api_requests_total") {
		t.Errorf("expected Prometheus body with help header, got %s", recProm.Body.String())
	}

	// 3. Prometheus Format via Accept header
	reqAccept := httptest.NewRequest("GET", "/metrics", nil)
	reqAccept.Header.Set("Accept", "text/plain; version=0.0.4")
	recAccept := httptest.NewRecorder()
	handler(recAccept, reqAccept)

	if recAccept.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recAccept.Code)
	}
	if !strings.Contains(recAccept.Body.String(), "# TYPE onusly_api_requests_total counter") {
		t.Errorf("expected Prometheus body from Accept header")
	}
}

func TestD14_Metrics_ConcurrencySafety(t *testing.T) {
	reg := metrics.NewRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			reg.RecordHTTPRequest("GET", "/health", 200, time.Millisecond)
			reg.RecordDBOperation("GetItem", "Devices", true, time.Millisecond)
			reg.RecordAuthFailure("invalid_token")
			reg.RecordLapseEpisode("started")
			reg.RecordLapseEpisode("resolved")
			reg.RecordHeartbeat("windows", "desktop_agent", "healthy")
			_ = reg.Snapshot()
			_ = reg.PrometheusFormat()
		}(i)
	}

	wg.Wait()

	snap := reg.Snapshot()
	if snap["lapse_episodes_started_total"] != uint64(50) {
		t.Errorf("expected 50 started lapse episodes, got %v", snap["lapse_episodes_started_total"])
	}
	if snap["lapse_episodes_resolved_total"] != uint64(50) {
		t.Errorf("expected 50 resolved lapse episodes, got %v", snap["lapse_episodes_resolved_total"])
	}
}

