package metrics_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/metrics"
)

func TestMetricsRegistry_PathSanitization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/goals", "/goals"},
		{"/goals/12345678-1234-1234-1234-123456789abc", "/goals/:id"},
		{"/goals/some-long-opaque-identifier-1234", "/goals/:id"},
		{"/enforcement/devices/00000000-0000-0000-0000-000000000000/revoke", "/enforcement/devices/:id/revoke"},
		{"/", "/"},
	}

	for _, tc := range tests {
		got := metrics.SanitizePath(tc.input)
		if got != tc.expected {
			t.Errorf("SanitizePath(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestMetricsRegistry_RecordingAndSnapshot(t *testing.T) {
	reg := metrics.NewRegistry()

	reg.RecordHTTPRequest("POST", "/goals", 201, 15*time.Millisecond)
	reg.RecordHTTPRequest("POST", "/goals", 400, 5*time.Millisecond)
	reg.RecordDBOperation("PutItem", "Onusly", true, 8*time.Millisecond)
	reg.RecordDBOperation("UpdateItem", "Onusly", false, 12*time.Millisecond)
	reg.RecordPolicyGeneration(3 * time.Millisecond)
	reg.RecordPolicyVersionChange()
	reg.RecordSyncOutcome("200_ok")
	reg.RecordSyncOutcome("304_cached")
	reg.RecordHeartbeat("windows", "desktop_agent", "enforcing")
	reg.RecordIdempotencyHit("/goals")
	reg.RecordIdempotencyConflict("/goals")
	reg.RecordConditionalConflict("goal", "update")

	snap := reg.Snapshot()

	if snap["policy_version_changes"].(uint64) != 1 {
		t.Fatalf("expected policy_version_changes = 1, got %v", snap["policy_version_changes"])
	}

	reqs := snap["api_requests_total"].(map[string]uint64)
	if reqs["POST /goals (Created)"] != 1 {
		t.Fatalf("expected 1 Created request, got %v", reqs["POST /goals (Created)"])
	}
	if reqs["POST /goals (Bad Request)"] != 1 {
		t.Fatalf("expected 1 Bad Request, got %v", reqs["POST /goals (Bad Request)"])
	}

	fails := snap["api_failures_total"].(map[string]uint64)
	if fails["POST /goals (Bad Request)"] != 1 {
		t.Fatalf("expected 1 failure recorded, got %v", fails)
	}

	// Verify HTTP Handler
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var jsonMap map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&jsonMap); err != nil {
		t.Fatalf("failed to parse JSON from /metrics: %v", err)
	}

	if jsonMap["privacy_guarantee"] == nil {
		t.Fatalf("missing privacy guarantee in metrics JSON")
	}
}

