package proof_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
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

func setupTestGoal(t *testing.T, db *dynamodb.Client, tableName, status string) (ownerID, approverID, goalID string) {
	t.Helper()
	ownerID = "test-owner-" + uuid.NewString()
	approverID = "test-approver-" + uuid.NewString()
	goalID = "test-goal-" + uuid.NewString()

	ctx := context.Background()

	goal := goals.Goal{
		PK:          "USER#" + ownerID,
		SK:          "GOAL#" + goalID,
		ID:          goalID,
		OwnerID:     ownerID,
		ApproverID:  approverID,
		Status:      status,
		Title:       "Proof Test Goal",
		Description: "Testing DynamoDB conditional proof submission",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	goalItem, err := attributevalue.MarshalMap(goal)
	if err != nil {
		t.Fatalf("failed to marshal test goal: %v", err)
	}
	_, err = db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      goalItem,
	})
	if err != nil {
		t.Fatalf("failed to put test goal: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		_, _ = db.DeleteItem(cleanupCtx, &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
				"SK": &types.AttributeValueMemberS{Value: "GOAL#" + goalID},
			},
		})

		out, err := db.Query(cleanupCtx, &dynamodb.QueryInput{
			TableName:              aws.String(tableName),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
				":sk": &types.AttributeValueMemberS{Value: "PROOF#" + goalID + "#"},
			},
		})
		if err == nil {
			for _, item := range out.Items {
				if skVal, ok := item["SK"].(*types.AttributeValueMemberS); ok {
					_, _ = db.DeleteItem(cleanupCtx, &dynamodb.DeleteItemInput{
						TableName: aws.String(tableName),
						Key: map[string]types.AttributeValue{
							"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
							"SK": &types.AttributeValueMemberS{Value: skVal.Value},
						},
					})
				}
			}
		}
	})

	return ownerID, approverID, goalID
}

// TestProofSubmission_ConcurrencyRace simulates TWO simultaneous proof submissions
// arriving at the exact same instant for an "active" goal.
// Expected result: exactly 1 submission succeeds, exactly 1 fails with conflict,
// the goal transitions to "proof_submitted", and exactly 1 proof item exists in DynamoDB.
func TestProofSubmission_ConcurrencyRace(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, _, goalID := setupTestGoal(t, db, tableName, "active")

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)

	type submitResult struct {
		proof *proof.Proof
		err   error
	}

	startBarrier := make(chan struct{})
	results := make(chan submitResult, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	for i := 0; i < 2; i++ {
		go func(workerID int) {
			defer wg.Done()
			<-startBarrier

			p, err := proofService.Submit(
				context.Background(),
				ownerID,
				goalID,
				"text",
				fmt.Sprintf("Evidence submitted concurrently by worker %d", workerID),
				"",
				"",
			)
			results <- submitResult{proof: p, err: err}
		}(i)
	}

	close(startBarrier)
	wg.Wait()
	close(results)

	var successCount int
	var conflictCount int
	var otherErrors []error

	for res := range results {
		if res.err == nil {
			successCount++
			if res.proof == nil {
				t.Errorf("expected non-nil proof on success")
			}
		} else if errors.Is(res.err, proof.ErrProofAlreadySubmitted) || errors.Is(res.err, proof.ErrGoalNotActive) {
			conflictCount++
		} else {
			otherErrors = append(otherErrors, res.err)
		}
	}

	if len(otherErrors) > 0 {
		t.Fatalf("unexpected non-conflict errors: %v", otherErrors)
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 submission to succeed, got %d", successCount)
	}
	if conflictCount != 1 {
		t.Errorf("expected exactly 1 submission to fail with conflict, got %d", conflictCount)
	}

	finalGoal, err := goalService.GetByID(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to fetch final goal: %v", err)
	}
	if finalGoal.Status != "proof_submitted" {
		t.Errorf("expected final goal status 'proof_submitted', got %q", finalGoal.Status)
	}

	proofs, err := proofService.ListByGoal(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to list proofs: %v", err)
	}
	if len(proofs) != 1 {
		t.Errorf("expected exactly 1 proof in DynamoDB, found %d", len(proofs))
	}
}

// TestProofSubmission_DuplicateWhenAlreadySubmitted verifies that attempting to submit
// proof on a goal that is already "proof_submitted" fails deterministically with ErrProofAlreadySubmitted.
func TestProofSubmission_DuplicateWhenAlreadySubmitted(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, _, goalID := setupTestGoal(t, db, tableName, "active")

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)

	// First submission: succeeds
	firstProof, err := proofService.Submit(
		context.Background(),
		ownerID,
		goalID,
		"text",
		"Legitimate first proof",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("first submission failed: %v", err)
	}
	if firstProof == nil {
		t.Fatalf("expected non-nil first proof")
	}

	// Second submission: must fail with ErrProofAlreadySubmitted
	secondProof, err := proofService.Submit(
		context.Background(),
		ownerID,
		goalID,
		"text",
		"Duplicate submission attempt",
		"",
		"",
	)
	if err == nil {
		t.Fatalf("expected second submission to fail, but it succeeded: %+v", secondProof)
	}
	if !errors.Is(err, proof.ErrProofAlreadySubmitted) {
		t.Errorf("expected ErrProofAlreadySubmitted, got %v", err)
	}

	// Verify only 1 proof item exists
	proofs, err := proofService.ListByGoal(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to list proofs: %v", err)
	}
	if len(proofs) != 1 {
		t.Errorf("expected exactly 1 proof in DynamoDB, found %d", len(proofs))
	}
}

// TestProofSubmission_CompletedGoalRejected verifies that submitting proof on an already
// "completed" goal is rejected with ErrGoalNotActive and does not reset the status.
func TestProofSubmission_CompletedGoalRejected(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, _, goalID := setupTestGoal(t, db, tableName, "completed")

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)

	_, err := proofService.Submit(
		context.Background(),
		ownerID,
		goalID,
		"text",
		"Attempting to submit proof on completed goal",
		"",
		"",
	)
	if err == nil {
		t.Fatalf("expected submission on completed goal to fail, but succeeded")
	}
	if !errors.Is(err, proof.ErrGoalNotActive) {
		t.Errorf("expected ErrGoalNotActive, got %v", err)
	}

	// Verify goal is still completed
	g, err := goalService.GetByID(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to fetch goal: %v", err)
	}
	if g.Status != "completed" {
		t.Errorf("expected goal status to remain 'completed', got %q", g.Status)
	}

	// Verify zero proofs exist
	proofs, err := proofService.ListByGoal(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to list proofs: %v", err)
	}
	if len(proofs) != 0 {
		t.Errorf("expected 0 proofs for completed goal, found %d", len(proofs))
	}
}

// TestProofSubmission_Validation verifies that malformed proof submissions are rejected.
func TestProofSubmission_Validation(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, _, goalID := setupTestGoal(t, db, tableName, "active")

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)

	ctx := context.Background()

	// 1. Text proof with empty explanation
	_, err := proofService.Submit(ctx, ownerID, goalID, "text", "   ", "", "")
	if !errors.Is(err, proof.ErrInvalidProof) {
		t.Errorf("expected ErrInvalidProof for empty text, got %v", err)
	}

	// 2. Link proof with empty URL
	_, err = proofService.Submit(ctx, ownerID, goalID, "link", "", "   ", "")
	if !errors.Is(err, proof.ErrInvalidProof) {
		t.Errorf("expected ErrInvalidProof for empty link, got %v", err)
	}

	// 3. Photo proof with empty URL
	_, err = proofService.Submit(ctx, ownerID, goalID, "photo", "", "", "   ")
	if !errors.Is(err, proof.ErrInvalidProof) {
		t.Errorf("expected ErrInvalidProof for empty photo URL, got %v", err)
	}

	// 4. Invalid proof type
	_, err = proofService.Submit(ctx, ownerID, goalID, "audio", "voice note", "", "")
	if !errors.Is(err, proof.ErrInvalidProofType) {
		t.Errorf("expected ErrInvalidProofType, got %v", err)
	}

	// 5. Non-existent goal
	_, err = proofService.Submit(ctx, ownerID, "non-existent-goal-id", "text", "valid text", "", "")
	if !errors.Is(err, proof.ErrGoalNotFound) {
		t.Errorf("expected ErrGoalNotFound, got %v", err)
	}
}

// TestProofHandler_HTTPStatusCodes verifies that the HTTP handler returns 201 Created on success,
// 409 Conflict for duplicate/completed submissions, 404 for non-existent goals, and 400 for bad input.
func TestProofHandler_HTTPStatusCodes(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, _, goalID := setupTestGoal(t, db, tableName, "active")

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	proofHandler := proof.NewHandler(proofService)

	targetURL := fmt.Sprintf("/goals/%s/proofs", goalID)

	// 1. Invalid proof body -> 400 Bad Request
	reqBody := `{"proofType":"text","textExplanation":""}`
	req := httptest.NewRequest("POST", targetURL, strings.NewReader(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
	rec := httptest.NewRecorder()

	proofHandler.Submit(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", rec.Code)
	}

	// 2. Legitimate submission -> 201 Created
	reqBody = `{"proofType":"text","textExplanation":"Valid proof explanation"}`
	req = httptest.NewRequest("POST", targetURL, strings.NewReader(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
	rec = httptest.NewRecorder()

	proofHandler.Submit(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// 3. Duplicate submission on same goal -> 409 Conflict
	reqBody = `{"proofType":"text","textExplanation":"Duplicate proof"}`
	req = httptest.NewRequest("POST", targetURL, strings.NewReader(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
	rec = httptest.NewRecorder()

	proofHandler.Submit(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict for duplicate submission, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "proof has already been submitted") {
		t.Errorf("expected body to mention already submitted, got %s", rec.Body.String())
	}

	// 4. Non-existent goal -> 404 Not Found
	nonExistentURL := "/goals/non-existent-goal/proofs"
	reqBody = `{"proofType":"text","textExplanation":"Proof for missing goal"}`
	req = httptest.NewRequest("POST", nonExistentURL, strings.NewReader(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
	rec = httptest.NewRecorder()

	proofHandler.Submit(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for non-existent goal, got %d", rec.Code)
	}
}
