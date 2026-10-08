package goals_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

// TestTargetMutationSecurity_TargetLocking verifies that:
// 1. Once a goal has targets, targets are add-only. Removing targets returns 409 targets_locked.
// 2. Active goal with targets cannot be deleted -> returns 409 goal_locked.
// 3. Goal with targets cannot have accountability changed to none -> returns 409.
// 4. Completed goal with targets CAN be deleted.
func TestTargetMutationSecurity_TargetLocking(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	repo := goals.NewRepository(db, tableName)
	service := goals.NewService(repo, nil)
	handler := goals.NewHandler(service)

	ownerID := "test-owner-" + uuid.NewString()

	// 1. Create goal with initial targets
	apps := []string{"com.instagram.android", "com.twitter.android"}
	domains := []string{"instagram.com", "x.com"}
	g, err := service.Create(context.Background(), ownerID, "Locked Target Goal", "Must lock targets", apps, domains)
	if err != nil {
		t.Fatalf("failed to create goal with targets: %v", err)
	}
	cleanupGoal(t, db, tableName, ownerID, g.ID)

	// Verify targets stored correctly
	if len(g.BlockedApps) != 2 || len(g.BlockedDomains) != 2 {
		t.Fatalf("expected 2 apps and 2 domains, got apps=%v domains=%v", g.BlockedApps, g.BlockedDomains)
	}

	targetURL := fmt.Sprintf("/goals/%s", g.ID)

	// 2. Attempt to remove a target (removing com.twitter.android) via PATCH -> MUST FAIL WITH 409 targets_locked
	{
		patchBody := `{"blockedApps":["com.instagram.android"]}`
		req := httptest.NewRequest("PATCH", targetURL, strings.NewReader(patchBody))
		req.SetPathValue("id", g.ID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Update(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict when removing target, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "targets_locked") {
			t.Errorf("expected response to contain 'targets_locked', got %q", rec.Body.String())
		}
	}

	// 3. Attempt to empty targets via PATCH -> MUST FAIL WITH 409 targets_locked
	{
		patchBody := `{"blockedApps":[]}`
		req := httptest.NewRequest("PATCH", targetURL, strings.NewReader(patchBody))
		req.SetPathValue("id", g.ID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Update(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict when setting empty targets, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "targets_locked") {
			t.Errorf("expected response to contain 'targets_locked', got %q", rec.Body.String())
		}
	}

	// 4. Attempt to add a target (add com.tiktok.android) -> MUST SUCCEED (add-only)
	{
		patchBody := `{"blockedApps":["com.instagram.android", "com.twitter.android", "com.zhiliaoapp.musically"]}`
		req := httptest.NewRequest("PATCH", targetURL, strings.NewReader(patchBody))
		req.SetPathValue("id", g.ID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Update(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK when adding target, got %d (body: %s)", rec.Code, rec.Body.String())
		}

		// Verify that all 3 apps are present
		updated, err := service.GetByID(context.Background(), ownerID, g.ID)
		if err != nil {
			t.Fatalf("failed to get goal: %v", err)
		}
		if len(updated.BlockedApps) != 3 {
			t.Errorf("expected 3 apps, got %d (%v)", len(updated.BlockedApps), updated.BlockedApps)
		}
	}

	// 5. Attempt to change accountability to "none" -> MUST FAIL WITH 409
	{
		patchBody := `{"approvalType":"none"}`
		req := httptest.NewRequest("PATCH", targetURL, strings.NewReader(patchBody))
		req.SetPathValue("id", g.ID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Update(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict when changing accountability to none, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	}

	// 6. Attempt to DELETE goal while active and targets exist -> MUST FAIL WITH 409 goal_locked
	{
		req := httptest.NewRequest("DELETE", targetURL, nil)
		req.SetPathValue("id", g.ID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Delete(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict when deleting active locked goal, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "goal_locked") {
			t.Errorf("expected body to contain 'goal_locked', got %q", rec.Body.String())
		}

		// Verify goal still exists
		stillThere, err := service.GetByID(context.Background(), ownerID, g.ID)
		if err != nil || stillThere == nil {
			t.Fatalf("expected goal to still exist after failed delete")
		}
	}

	// 7. Transition goal to "completed" via UpdateStatus -> DELETE should now SUCCEED
	{
		_, err := repo.UpdateStatus(context.Background(), ownerID, g.ID, "completed")
		if err != nil {
			t.Fatalf("failed to update status to completed: %v", err)
		}

		req := httptest.NewRequest("DELETE", targetURL, nil)
		req.SetPathValue("id", g.ID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Delete(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 No Content when deleting completed goal, got %d (body: %s)", rec.Code, rec.Body.String())
		}

		// Verify goal is gone
		deletedGoal, err := service.GetByID(context.Background(), ownerID, g.ID)
		if err == nil && deletedGoal != nil {
			t.Fatalf("expected goal to be deleted")
		}
	}
}

// TestGoal_CreateWithInvalidTargetsReturns400 verifies that invalid package names
// or domain names return 400 Bad Request and identify the offending value.
func TestGoal_CreateWithInvalidTargetsReturns400(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	repo := goals.NewRepository(db, tableName)
	service := goals.NewService(repo, nil)
	handler := goals.NewHandler(service)

	ownerID := "test-owner-" + uuid.NewString()

	// Invalid app package
	{
		body := `{"title":"Invalid App Goal","blockedApps":["invalid-package-name"]}`
		req := httptest.NewRequest("POST", "/goals", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Create(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "invalid-package-name") {
			t.Errorf("expected error message to identify bad app value, got %q", rec.Body.String())
		}
	}

	// Invalid domain
	{
		body := `{"title":"Invalid Domain Goal","blockedDomains":["192.168.1.1"]}`
		req := httptest.NewRequest("POST", "/goals", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Create(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "192.168.1.1") {
			t.Errorf("expected error message to identify bad domain value, got %q", rec.Body.String())
		}
	}
}
