package goals_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

	// 4. Attempt to add a target (add com.reddit.frontpage) -> MUST SUCCEED (add-only)
	{
		patchBody := `{"blockedApps":["com.instagram.android", "com.twitter.android", "com.reddit.frontpage"]}`
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

	// 5. Attempt to change accountability to "none" or "community" -> MUST FAIL WITH 400 accountability_required
	{
		patchBody := `{"approvalType":"none"}`
		req := httptest.NewRequest("PATCH", targetURL, strings.NewReader(patchBody))
		req.SetPathValue("id", g.ID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Update(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request when changing accountability to none, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "accountability_required") {
			t.Fatalf("expected body to contain 'accountability_required', got %s", rec.Body.String())
		}
	}
	{
		patchBody := `{"approvalType":"community"}`
		req := httptest.NewRequest("PATCH", targetURL, strings.NewReader(patchBody))
		req.SetPathValue("id", g.ID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Update(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request when changing accountability to community, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "accountability_required") {
			t.Fatalf("expected body to contain 'accountability_required', got %s", rec.Body.String())
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

// TestGoal_CreateTargetedGoalRequiresFriendAccountability verifies that creating a targeted
// goal requires friend approval and rejects none, community, or missing approver with 400 accountability_required.
func TestGoal_CreateTargetedGoalRequiresFriendAccountability(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	repo := goals.NewRepository(db, tableName)
	service := goals.NewService(repo, nil)
	handler := goals.NewHandler(service)

	ownerID := "test-owner-" + uuid.NewString()

	// 1. Missing approvalType entirely on targeted goal -> 400 accountability_required
	{
		body := `{"title":"Targeted Goal","blockedApps":["com.instagram.android"]}`
		req := httptest.NewRequest("POST", "/goals", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Create(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "accountability_required") {
			t.Errorf("expected 'accountability_required', got %q", rec.Body.String())
		}
	}

	// 2. approvalType: "none" on targeted goal -> 400 accountability_required
	{
		body := `{"title":"Targeted Goal","blockedApps":["com.instagram.android"],"approvalType":"none"}`
		req := httptest.NewRequest("POST", "/goals", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Create(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "accountability_required") {
			t.Errorf("expected 'accountability_required', got %q", rec.Body.String())
		}
	}

	// 3. approvalType: "community" on targeted goal -> 400 accountability_required
	{
		body := `{"title":"Targeted Goal","blockedDomains":["instagram.com"],"approvalType":"community"}`
		req := httptest.NewRequest("POST", "/goals", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Create(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "accountability_required") {
			t.Errorf("expected 'accountability_required', got %q", rec.Body.String())
		}
	}

	// 4. approvalType: "friend" with valid approverEmail -> 201 Created
	{
		body := `{"title":"Targeted Goal","blockedApps":["com.instagram.android"],"approvalType":"friend","approverEmail":"friend@example.com"}`
		req := httptest.NewRequest("POST", "/goals", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
		rec := httptest.NewRecorder()

		handler.Create(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	}
}

// TestGoal_ConcurrentTargetAndCommunityRaceCondition verifies that concurrent PATCH
// requests attempting to add targets while another switches approval to community
// cannot bypass accountability rules. The DynamoDB conditional checks guarantee that
// targets and community/none can never coexist.
func TestGoal_ConcurrentTargetAndCommunityRaceCondition(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	repo := goals.NewRepository(db, tableName)
	service := goals.NewService(repo, nil)
	handler := goals.NewHandler(service)

	for iter := 0; iter < 3; iter++ {
		ownerID := "test-race-owner-" + uuid.NewString()
		friendType := "friend"
		friendEmail := "partner@example.com"

		// 1. Create an active goal without targets and with friend approval
		g, err := service.CreateWithInput(context.Background(), ownerID, goals.CreateGoalInput{
			Title:         fmt.Sprintf("Race Test Goal %d", iter),
			Description:   "Testing concurrent race-safety between targets and community",
			ApprovalType:  &friendType,
			ApproverEmail: &friendEmail,
		})
		if err != nil {
			t.Fatalf("failed to create race test goal: %v", err)
		}
		cleanupGoal(t, db, tableName, ownerID, g.ID)

		targetURL := fmt.Sprintf("/goals/%s", g.ID)

		var wg sync.WaitGroup
		wg.Add(2)

		var codeTargets, codeCommunity int
		var bodyTargets, bodyCommunity string

		startGate := make(chan struct{})

		// Goroutine 1: Adds targets
		go func() {
			defer wg.Done()
			<-startGate
			patchBody := `{"blockedApps":["com.instagram.android"]}`
			req := httptest.NewRequest("PATCH", targetURL, strings.NewReader(patchBody))
			req.SetPathValue("id", g.ID)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
			rec := httptest.NewRecorder()

			handler.Update(rec, req)
			codeTargets = rec.Code
			bodyTargets = rec.Body.String()
		}()

		// Goroutine 2: Switches approval to community
		go func() {
			defer wg.Done()
			<-startGate
			patchBody := `{"approvalType":"community"}`
			req := httptest.NewRequest("PATCH", targetURL, strings.NewReader(patchBody))
			req.SetPathValue("id", g.ID)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
			rec := httptest.NewRecorder()

			handler.Update(rec, req)
			codeCommunity = rec.Code
			bodyCommunity = rec.Body.String()
		}()

		// Release both goroutines simultaneously
		close(startGate)
		wg.Wait()

		// Fetch the final persisted state from DynamoDB
		finalGoal, err := repo.GetByID(context.Background(), ownerID, g.ID)
		if err != nil {
			t.Fatalf("failed to fetch final goal state: %v", err)
		}

		hasTargets := len(finalGoal.BlockedApps) > 0 || len(finalGoal.BlockedDomains) > 0
		isCommunity := strings.EqualFold(finalGoal.ApprovalType, "community")

		// CRITICAL INVARIANT: A goal must NEVER have targets and community approval simultaneously
		if hasTargets && isCommunity {
			t.Fatalf("RACE CONDITION BYPASS DETECTED: Goal has targets (%v) and community approval! (codeTargets=%d body=%s, codeCommunity=%d body=%s)",
				finalGoal.BlockedApps, codeTargets, bodyTargets, codeCommunity, bodyCommunity)
		}

		// At least one of the conflicting operations should have failed if both raced
		if codeTargets == http.StatusOK && codeCommunity == http.StatusOK {
			t.Fatalf("Both concurrent updates succeeded with 200 OK! State: apps=%v, approvalType=%s",
				finalGoal.BlockedApps, finalGoal.ApprovalType)
		}

		t.Logf("Iteration %d passed: final approvalType=%s, targets=%v (codeTargets=%d, codeCommunity=%d)",
			iter, finalGoal.ApprovalType, finalGoal.BlockedApps, codeTargets, codeCommunity)
	}
}

// TestGoal_DeleteWhileProofSubmittedBlocked ensures that a goal owner cannot delete a goal
// while it is in the "proof_submitted" state waiting for an approver decision.
func TestGoal_DeleteWhileProofSubmittedBlocked(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	repo := goals.NewRepository(db, tableName)
	service := goals.NewService(repo, nil)
	handler := goals.NewHandler(service)

	ownerID := "test-owner-delete-guard-" + uuid.NewString()[:8]

	g, err := service.CreateWithInput(context.Background(), ownerID, goals.CreateGoalInput{
		Title:       "Goal under review",
		Description: "Should not be deletable during review",
	})
	if err != nil {
		t.Fatalf("failed to create goal: %v", err)
	}
	t.Cleanup(func() {
		// Clean up DynamoDB entry after test
		_ = repo.Delete(context.Background(), ownerID, g.ID)
	})

	// Transition to proof_submitted
	_, err = repo.UpdateStatus(context.Background(), ownerID, g.ID, "proof_submitted")
	if err != nil {
		t.Fatalf("failed to transition to proof_submitted: %v", err)
	}

	// Attempt DELETE via HTTP handler
	req := httptest.NewRequest("DELETE", "/goals/"+g.ID, nil)
	req.SetPathValue("id", g.ID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when deleting goal in review, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Verify goal is still present in database
	persisted, err := service.GetByID(context.Background(), ownerID, g.ID)
	if err != nil || persisted == nil {
		t.Fatalf("goal was deleted while under review!")
	}
	if persisted.Status != "proof_submitted" {
		t.Fatalf("expected status proof_submitted, got %s", persisted.Status)
	}

	// Transition back to active -> delete should now succeed for an untargeted goal
	_, err = repo.UpdateStatus(context.Background(), ownerID, g.ID, "active")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	rec2 := httptest.NewRecorder()
	handler.Delete(rec2, req)
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content when deleting active untargeted goal, got %d", rec2.Code)
	}
}
