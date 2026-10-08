package enforcement_test

import (
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

type mockGoalLister struct {
	goalsByUser map[string][]goals.Goal
}

func (m *mockGoalLister) List(ctx context.Context, ownerID string) ([]goals.Goal, error) {
	return m.goalsByUser[ownerID], nil
}

func TestEnforcement_FilteringAndOrdering(t *testing.T) {
	mockLister := &mockGoalLister{
		goalsByUser: map[string][]goals.Goal{
			"user-1": {
				{
					ID:             "g-completed",
					Title:          "Completed Goal",
					Status:         "completed",
					CreatedAt:      "2026-10-01T10:00:00Z",
					BlockedApps:    []string{"com.app.completed"},
					BlockedDomains: []string{"completed.com"},
				},
				{
					ID:          "g-no-targets",
					Title:       "No Targets Goal",
					Status:      "active",
					CreatedAt:   "2026-10-01T11:00:00Z",
					BlockedApps: []string{},
				},
				{
					ID:             "g-2",
					Title:          "Second Goal",
					Status:         "proof_submitted",
					CreatedAt:      "2026-10-01T13:00:00Z",
					BlockedApps:    []string{"com.twitter.android", "com.instagram.android"},
					BlockedDomains: []string{"twitter.com", "instagram.com"},
				},
				{
					ID:             "g-1",
					Title:          "First Goal",
					Status:         "active",
					CreatedAt:      "2026-10-01T12:00:00Z",
					BlockedApps:    []string{"com.tiktok.android"},
					BlockedDomains: []string{"tiktok.com"},
				},
			},
		},
	}

	service := enforcement.NewService(mockLister)
	handler := enforcement.NewHandler(service)

	req := httptest.NewRequest("GET", "/enforcement/blocklist?userId=user-hacker", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-1"))
	rec := httptest.NewRecorder()

	handler.GetBlocklist(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("expected ETag header to be set")
	}
	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "private, no-cache" {
		t.Errorf("expected Cache-Control 'private, no-cache', got %q", cacheControl)
	}

	var res enforcement.BlocklistResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.UserID != "user-1" {
		t.Errorf("expected userId 'user-1', got %q", res.UserID)
	}

	// Should only include g-1 and g-2 (g-completed and g-no-targets excluded)
	if len(res.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(res.Targets))
	}

	// Ordered by createdAt: g-1 (12:00) before g-2 (13:00)
	if res.Targets[0].GoalID != "g-1" {
		t.Errorf("expected first target to be g-1, got %q", res.Targets[0].GoalID)
	}
	if res.Targets[1].GoalID != "g-2" {
		t.Errorf("expected second target to be g-2, got %q", res.Targets[1].GoalID)
	}

	// g-2 apps and domains sorted ascending
	g2Apps := res.Targets[1].Apps
	if len(g2Apps) != 2 || g2Apps[0] != "com.instagram.android" || g2Apps[1] != "com.twitter.android" {
		t.Errorf("expected apps sorted ascending, got %v", g2Apps)
	}
	g2Domains := res.Targets[1].Domains
	if len(g2Domains) != 2 || g2Domains[0] != "instagram.com" || g2Domains[1] != "twitter.com" {
		t.Errorf("expected domains sorted ascending, got %v", g2Domains)
	}
}

func TestEnforcement_ETagAnd304NotModified(t *testing.T) {
	mockLister := &mockGoalLister{
		goalsByUser: map[string][]goals.Goal{
			"user-1": {
				{
					ID:             "g-1",
					Title:          "First Goal",
					Status:         "active",
					CreatedAt:      "2026-10-01T12:00:00Z",
					BlockedApps:    []string{"com.instagram.android"},
					BlockedDomains: []string{"instagram.com"},
				},
			},
		},
	}

	service := enforcement.NewService(mockLister)
	handler := enforcement.NewHandler(service)

	// First request: 200 OK with ETag
	req1 := httptest.NewRequest("GET", "/enforcement/blocklist", nil)
	req1 = req1.WithContext(context.WithValue(req1.Context(), middleware.UserIDKey, "user-1"))
	rec1 := httptest.NewRecorder()

	handler.GetBlocklist(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec1.Code)
	}
	etag := rec1.Header().Get("ETag")

	// Second request with If-None-Match matching ETag: 304 Not Modified and NO body
	req2 := httptest.NewRequest("GET", "/enforcement/blocklist", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), middleware.UserIDKey, "user-1"))
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()

	handler.GetBlocklist(rec2, req2)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("expected 304 Not Modified, got %d", rec2.Code)
	}
	if rec2.Body.Len() != 0 {
		t.Fatalf("expected 304 response to have NO body, got %d bytes (%s)", rec2.Body.Len(), rec2.Body.String())
	}
	if rec2.Header().Get("ETag") != etag {
		t.Errorf("expected 304 to include ETag header %q, got %q", etag, rec2.Header().Get("ETag"))
	}
	if rec2.Header().Get("Cache-Control") != "private, no-cache" {
		t.Errorf("expected 304 to include Cache-Control 'private, no-cache'")
	}

	// Also test matching with weak ETag prefix W/
	req3 := httptest.NewRequest("GET", "/enforcement/blocklist", nil)
	req3 = req3.WithContext(context.WithValue(req3.Context(), middleware.UserIDKey, "user-1"))
	req3.Header.Set("If-None-Match", "W/"+etag)
	rec3 := httptest.NewRecorder()

	handler.GetBlocklist(rec3, req3)
	if rec3.Code != http.StatusNotModified {
		t.Fatalf("expected 304 for weak ETag match, got %d", rec3.Code)
	}

	// Mismatched ETag: 200 OK
	req4 := httptest.NewRequest("GET", "/enforcement/blocklist", nil)
	req4 = req4.WithContext(context.WithValue(req4.Context(), middleware.UserIDKey, "user-1"))
	req4.Header.Set("If-None-Match", `"v1-differenthash"`)
	rec4 := httptest.NewRecorder()

	handler.GetBlocklist(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for mismatched ETag, got %d", rec4.Code)
	}
}

func TestEnforcement_VersionRules(t *testing.T) {
	// Base target goal
	baseGoal := goals.Goal{
		ID:             "g-1",
		Title:          "Study Time",
		Description:    "Chapter 1 reading",
		ApprovalType:   "friend",
		ApproverEmail:  "friend@example.com",
		Status:         "active",
		CreatedAt:      "2026-10-01T12:00:00Z",
		BlockedApps:    []string{"com.instagram.android"},
		BlockedDomains: []string{"instagram.com"},
	}

	mockLister := &mockGoalLister{
		goalsByUser: map[string][]goals.Goal{
			"user-1": {baseGoal},
			"user-2": {},
		},
	}
	service := enforcement.NewService(mockLister)

	baseRes, _ := service.GetBlocklist(context.Background(), "user-1")
	baseVersion := baseRes.Version

	if !testing.Short() {
		// Rule 1: Version MUST NOT change with passage of time
		time.Sleep(10 * time.Millisecond)
		timeRes, _ := service.GetBlocklist(context.Background(), "user-1")
		if timeRes.Version != baseVersion {
			t.Errorf("version changed over time: %q vs %q", timeRes.Version, baseVersion)
		}
	}

	// Rule 2: Version MUST NOT change when description changes
	mockLister.goalsByUser["user-1"] = []goals.Goal{
		{
			ID:             baseGoal.ID,
			Title:          baseGoal.Title,
			Description:    "Completely different description",
			ApprovalType:   baseGoal.ApprovalType,
			ApproverEmail:  baseGoal.ApproverEmail,
			Status:         baseGoal.Status,
			CreatedAt:      baseGoal.CreatedAt,
			BlockedApps:    baseGoal.BlockedApps,
			BlockedDomains: baseGoal.BlockedDomains,
		},
	}
	descRes, _ := service.GetBlocklist(context.Background(), "user-1")
	if descRes.Version != baseVersion {
		t.Errorf("version changed when description changed: %q vs %q", descRes.Version, baseVersion)
	}

	// Rule 3: Version MUST NOT change when accountability-only settings change
	mockLister.goalsByUser["user-1"] = []goals.Goal{
		{
			ID:             baseGoal.ID,
			Title:          baseGoal.Title,
			Description:    baseGoal.Description,
			ApprovalType:   "community",
			ApproverEmail:  "",
			Status:         baseGoal.Status,
			CreatedAt:      baseGoal.CreatedAt,
			BlockedApps:    baseGoal.BlockedApps,
			BlockedDomains: baseGoal.BlockedDomains,
		},
	}
	accRes, _ := service.GetBlocklist(context.Background(), "user-1")
	if accRes.Version != baseVersion {
		t.Errorf("version changed when accountability changed: %q vs %q", accRes.Version, baseVersion)
	}

	// Rule 4: Version MUST NOT change when goals with NO targets are added
	mockLister.goalsByUser["user-1"] = []goals.Goal{
		baseGoal,
		{
			ID:          "g-untargeted",
			Title:       "Untargeted Goal",
			Status:      "active",
			CreatedAt:   "2026-10-01T13:00:00Z",
			BlockedApps: []string{},
		},
	}
	noTargetRes, _ := service.GetBlocklist(context.Background(), "user-1")
	if noTargetRes.Version != baseVersion {
		t.Errorf("version changed when untargeted goal added: %q vs %q", noTargetRes.Version, baseVersion)
	}

	// Rule 5: Version MUST NOT change for other users' activity
	mockLister.goalsByUser["user-2"] = []goals.Goal{
		{
			ID:             "g-user2",
			Title:          "User 2 Goal",
			Status:         "active",
			CreatedAt:      "2026-10-01T14:00:00Z",
			BlockedApps:    []string{"com.facebook.katana"},
			BlockedDomains: []string{"facebook.com"},
		},
	}
	user2ActRes, _ := service.GetBlocklist(context.Background(), "user-1")
	if user2ActRes.Version != baseVersion {
		t.Errorf("version changed due to another user's activity: %q vs %q", user2ActRes.Version, baseVersion)
	}

	// --- NOW RULES WHERE VERSION MUST CHANGE ---

	// Rule 6: Version MUST change when a new target goal is created
	mockLister.goalsByUser["user-1"] = []goals.Goal{
		baseGoal,
		{
			ID:             "g-2",
			Title:          "New Target Goal",
			Status:         "active",
			CreatedAt:      "2026-10-01T14:00:00Z",
			BlockedApps:    []string{"com.twitter.android"},
			BlockedDomains: []string{"twitter.com"},
		},
	}
	newGoalRes, _ := service.GetBlocklist(context.Background(), "user-1")
	if newGoalRes.Version == baseVersion {
		t.Errorf("version did not change when new target goal added")
	}

	// Rule 7: Version MUST change when targets are added
	mockLister.goalsByUser["user-1"] = []goals.Goal{
		{
			ID:             baseGoal.ID,
			Title:          baseGoal.Title,
			Status:         baseGoal.Status,
			CreatedAt:      baseGoal.CreatedAt,
			BlockedApps:    []string{"com.instagram.android", "com.twitter.android"},
			BlockedDomains: baseGoal.BlockedDomains,
		},
	}
	targetsAddedRes, _ := service.GetBlocklist(context.Background(), "user-1")
	if targetsAddedRes.Version == baseVersion {
		t.Errorf("version did not change when targets were added")
	}

	// Rule 8: Version MUST change when goal title changes
	mockLister.goalsByUser["user-1"] = []goals.Goal{
		{
			ID:             baseGoal.ID,
			Title:          "Changed Title",
			Status:         baseGoal.Status,
			CreatedAt:      baseGoal.CreatedAt,
			BlockedApps:    baseGoal.BlockedApps,
			BlockedDomains: baseGoal.BlockedDomains,
		},
	}
	titleRes, _ := service.GetBlocklist(context.Background(), "user-1")
	if titleRes.Version == baseVersion {
		t.Errorf("version did not change when goal title changed")
	}

	// Rule 9: Version MUST change when active -> proof_submitted
	mockLister.goalsByUser["user-1"] = []goals.Goal{
		{
			ID:             baseGoal.ID,
			Title:          baseGoal.Title,
			Status:         "proof_submitted",
			CreatedAt:      baseGoal.CreatedAt,
			BlockedApps:    baseGoal.BlockedApps,
			BlockedDomains: baseGoal.BlockedDomains,
		},
	}
	submittedRes, _ := service.GetBlocklist(context.Background(), "user-1")
	if submittedRes.Version == baseVersion {
		t.Errorf("version did not change when status transitioned to proof_submitted")
	}

	// Rule 10: Version MUST change when proof_submitted -> completed
	mockLister.goalsByUser["user-1"] = []goals.Goal{
		{
			ID:             baseGoal.ID,
			Title:          baseGoal.Title,
			Status:         "completed",
			CreatedAt:      baseGoal.CreatedAt,
			BlockedApps:    baseGoal.BlockedApps,
			BlockedDomains: baseGoal.BlockedDomains,
		},
	}
	completedRes, _ := service.GetBlocklist(context.Background(), "user-1")
	if completedRes.Version == baseVersion || completedRes.Version == submittedRes.Version {
		t.Errorf("version did not change when status transitioned to completed")
	}

	// Rule 11: Version MUST change when proof_submitted -> active (after rejection)
	// (submittedRes.Version != baseVersion verified above; transitioning back to active restores baseVersion)
	mockLister.goalsByUser["user-1"] = []goals.Goal{baseGoal}
	rejectedRes, _ := service.GetBlocklist(context.Background(), "user-1")
	if rejectedRes.Version != baseVersion {
		t.Errorf("version did not restore to active version after rejection: %q vs %q", rejectedRes.Version, baseVersion)
	}
	if rejectedRes.Version == submittedRes.Version {
		t.Errorf("version remained at proof_submitted version after rejection")
	}
}

func TestEnforcement_Unauthorized(t *testing.T) {
	mockLister := &mockGoalLister{}
	service := enforcement.NewService(mockLister)
	handler := enforcement.NewHandler(service)

	req := httptest.NewRequest("GET", "/enforcement/blocklist", nil)
	// No UserID in context
	rec := httptest.NewRecorder()

	handler.GetBlocklist(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
	}
}
