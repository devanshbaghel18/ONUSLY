package goals_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/shared"
)

func getTestDynamoClient(t *testing.T) (*dynamodb.Client, string) {
	t.Helper()
	tableName := "Onusly"
	db := shared.NewDynamoClient()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		t.Skipf("DynamoDB table %q is not accessible (%v). Skipping integration test.", tableName, err)
	}

	return db, tableName
}

func cleanupGoal(t *testing.T, db *dynamodb.Client, tableName, ownerID, goalID string) {
	t.Helper()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, _ = db.DeleteItem(cleanupCtx, &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
				"SK": &types.AttributeValueMemberS{Value: "GOAL#" + goalID},
			},
		})
	})
}

// TestGoal_CRUD_OwnerIsolation tests that users can only access, list, and delete their own goals.
func TestGoal_CRUD_OwnerIsolation(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	repo := goals.NewRepository(db, tableName)
	service := goals.NewService(repo, nil)

	user1ID := "test-user1-" + uuid.NewString()
	user2ID := "test-user2-" + uuid.NewString()

	// User 1 creates Goal
	g1, err := service.Create(context.Background(), user1ID, "User 1 Goal", "Only User 1 can see", nil, nil)
	if err != nil {
		t.Fatalf("failed to create goal: %v", err)
	}
	cleanupGoal(t, db, tableName, user1ID, g1.ID)

	// User 1 can Get
	fetched, err := service.GetByID(context.Background(), user1ID, g1.ID)
	if err != nil {
		t.Fatalf("User 1 failed to get own goal: %v", err)
	}
	if fetched.ID != g1.ID {
		t.Errorf("expected ID %s, got %s", g1.ID, fetched.ID)
	}

	// User 2 CANNOT Get User 1's goal
	_, err = service.GetByID(context.Background(), user2ID, g1.ID)
	if err == nil {
		t.Errorf("User 2 was able to fetch User 1's goal (isolation violation)")
	}

	// User 1 List contains Goal
	list1, err := service.List(context.Background(), user1ID)
	if err != nil {
		t.Fatalf("User 1 failed to list goals: %v", err)
	}
	var foundInList1 bool
	for _, item := range list1 {
		if item.ID == g1.ID {
			foundInList1 = true
			break
		}
	}
	if !foundInList1 {
		t.Errorf("User 1's goal missing from list")
	}

	// User 2 List does NOT contain Goal
	list2, err := service.List(context.Background(), user2ID)
	if err != nil {
		t.Fatalf("User 2 failed to list goals: %v", err)
	}
	for _, item := range list2 {
		if item.ID == g1.ID {
			t.Errorf("User 1's goal leaked in User 2's list")
		}
	}

	// User 2 Delete does NOT delete User 1's goal
	_ = service.Delete(context.Background(), user2ID, g1.ID)
	stillThere, err := service.GetByID(context.Background(), user1ID, g1.ID)
	if err != nil || stillThere == nil {
		t.Errorf("User 2 was able to delete User 1's goal")
	}

	// User 1 Delete succeeds
	err = service.Delete(context.Background(), user1ID, g1.ID)
	if err != nil {
		t.Fatalf("User 1 failed to delete own goal: %v", err)
	}
	_, err = service.GetByID(context.Background(), user1ID, g1.ID)
	if err == nil {
		t.Errorf("expected goal to be deleted")
	}
}

// TestGoal_StatusBypassAttemptBlocked verifies that PATCH /goals/{id} cannot bypass
// the state machine or change status directly.
func TestGoal_StatusBypassAttemptBlocked(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	repo := goals.NewRepository(db, tableName)
	service := goals.NewService(repo, nil)
	handler := goals.NewHandler(service)

	ownerID := "test-owner-" + uuid.NewString()
	g, err := service.Create(context.Background(), ownerID, "Status Test Goal", "Testing status bypass", nil, nil)
	if err != nil {
		t.Fatalf("failed to create goal: %v", err)
	}
	cleanupGoal(t, db, tableName, ownerID, g.ID)

	// Attempt PATCH with {"status": "completed"}
	targetURL := fmt.Sprintf("/goals/%s", g.ID)
	reqBody := `{"status":"completed","title":"New Title"}`
	req := httptest.NewRequest("PATCH", targetURL, strings.NewReader(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
	rec := httptest.NewRecorder()

	handler.Update(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	// Verify status is STILL "active"
	updated, err := service.GetByID(context.Background(), ownerID, g.ID)
	if err != nil {
		t.Fatalf("failed to fetch goal: %v", err)
	}
	if updated.Status != "active" {
		t.Errorf("STATUS BYPASS VULNERABILITY: goal status changed to %q via PATCH", updated.Status)
	}
	if updated.Title != "New Title" {
		t.Errorf("expected title to be updated to 'New Title', got %q", updated.Title)
	}
}

// TestGoal_AccountabilityModificationRules verifies that accountability settings
// can be set on active goals, but CANNOT be modified once proof has been submitted.
func TestGoal_AccountabilityModificationRules(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	repo := goals.NewRepository(db, tableName)
	service := goals.NewService(repo, nil)

	ownerID := "test-owner-" + uuid.NewString()
	g, err := service.Create(context.Background(), ownerID, "Accountability Goal", "Testing rules", nil, nil)
	if err != nil {
		t.Fatalf("failed to create goal: %v", err)
	}
	cleanupGoal(t, db, tableName, ownerID, g.ID)

	// 1. Update accountability while active -> succeeds
	comm := "community"
	_, err = service.Update(context.Background(), ownerID, g.ID, goals.UpdateGoalInput{
		ApprovalType: &comm,
	})
	if err != nil {
		t.Fatalf("expected accountability update to succeed on active goal, got: %v", err)
	}

	// 2. Set goal to proof_submitted
	_, err = repo.UpdateStatus(context.Background(), ownerID, g.ID, "proof_submitted")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// 3. Attempt to change accountability while proof_submitted -> MUST FAIL
	noneType := "none"
	_, err = service.Update(context.Background(), ownerID, g.ID, goals.UpdateGoalInput{
		ApprovalType: &noneType,
	})
	if err == nil {
		t.Errorf("expected modifying accountability on proof_submitted goal to fail, but succeeded")
	}

	// 4. Updating title/description while proof_submitted is still allowed
	newDesc := "Updated description while in review"
	updated, err := service.Update(context.Background(), ownerID, g.ID, goals.UpdateGoalInput{
		Description: &newDesc,
	})
	if err != nil {
		t.Fatalf("expected title/description update to succeed, got: %v", err)
	}
	if updated.Description != newDesc {
		t.Errorf("expected description %q, got %q", newDesc, updated.Description)
	}
}

// TestGoal_NonexistentGoal404 verifies that GET and PATCH return 404 for missing goals.
func TestGoal_NonexistentGoal404(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	repo := goals.NewRepository(db, tableName)
	service := goals.NewService(repo, nil)
	handler := goals.NewHandler(service)

	ownerID := "test-owner-" + uuid.NewString()
	missingURL := "/goals/non-existent-id"

	// GET -> 404
	reqGet := httptest.NewRequest("GET", missingURL, nil)
	reqGet = reqGet.WithContext(context.WithValue(reqGet.Context(), middleware.UserIDKey, ownerID))
	recGet := httptest.NewRecorder()
	handler.GetByID(recGet, reqGet)
	if recGet.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing goal GET, got %d", recGet.Code)
	}

	// PATCH -> 404
	reqPatch := httptest.NewRequest("PATCH", missingURL, strings.NewReader(`{"title":"New"}`))
	reqPatch = reqPatch.WithContext(context.WithValue(reqPatch.Context(), middleware.UserIDKey, ownerID))
	recPatch := httptest.NewRecorder()
	handler.Update(recPatch, reqPatch)
	if recPatch.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing goal PATCH, got %d", recPatch.Code)
	}
}
