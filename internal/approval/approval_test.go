package approval_test

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

	"github.com/devanshbaghel18/ONUSLY/internal/approval"
	"github.com/devanshbaghel18/ONUSLY/internal/events"
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

func setupCustomTestGoalAndProof(t *testing.T, db *dynamodb.Client, tableName, status, ownerID, approverID string) (goalID, proofID string) {
	t.Helper()
	goalID = "test-goal-" + uuid.NewString()
	proofID = "test-proof-" + uuid.NewString()

	ctx := context.Background()

	goal := goals.Goal{
		PK:          "USER#" + ownerID,
		SK:          "GOAL#" + goalID,
		ID:          goalID,
		OwnerID:     ownerID,
		ApproverID:  approverID,
		Status:      status,
		Title:       "Approval Test Goal",
		Description: "Testing DynamoDB conditional approval atomicity",
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

	p := proof.Proof{
		PK:              "USER#" + ownerID,
		SK:              "PROOF#" + goalID + "#" + proofID,
		ID:              proofID,
		GoalID:          goalID,
		OwnerID:         ownerID,
		ProofType:       "text",
		TextExplanation: "Verified evidence of completion",
		SubmittedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	proofItem, err := attributevalue.MarshalMap(p)
	if err != nil {
		t.Fatalf("failed to marshal test proof: %v", err)
	}
	_, err = db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      proofItem,
	})
	if err != nil {
		t.Fatalf("failed to put test proof: %v", err)
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

		_, _ = db.DeleteItem(cleanupCtx, &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
				"SK": &types.AttributeValueMemberS{Value: "PROOF#" + goalID + "#" + proofID},
			},
		})

		out, err := db.Query(cleanupCtx, &dynamodb.QueryInput{
			TableName:              aws.String(tableName),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
				":sk": &types.AttributeValueMemberS{Value: "APPROVAL#" + goalID + "#"},
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

	return goalID, proofID
}

func setupTestGoalAndProof(t *testing.T, db *dynamodb.Client, tableName string) (ownerID, approverID, goalID, proofID string) {
	t.Helper()
	ownerID = "test-owner-" + uuid.NewString()
	approverID = "test-approver-" + uuid.NewString()
	goalID, proofID = setupCustomTestGoalAndProof(t, db, tableName, "proof_submitted", ownerID, approverID)
	return ownerID, approverID, goalID, proofID
}

// -------------------------------------------------------------
// CONCURRENCY & RACE CONDITION TESTS
// -------------------------------------------------------------

// TestApprovalConcurrency_ApproveVsApprove tests two simultaneous approvals.
func TestApprovalConcurrency_ApproveVsApprove(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, approverID, goalID, proofID := setupTestGoalAndProof(t, db, tableName)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	type decisionResult struct {
		approval *approval.Approval
		err      error
	}

	startBarrier := make(chan struct{})
	results := make(chan decisionResult, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	for i := 0; i < 2; i++ {
		go func(workerID int) {
			defer wg.Done()
			<-startBarrier

			app, err := approvalService.Decide(
				context.Background(),
				approverID,
				ownerID,
				goalID,
				proofID,
				"approved",
				fmt.Sprintf("Approve from worker %d", workerID),
			)
			results <- decisionResult{approval: app, err: err}
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
		} else if errors.Is(res.err, approval.ErrAlreadyDecided) {
			conflictCount++
		} else {
			otherErrors = append(otherErrors, res.err)
		}
	}

	if len(otherErrors) > 0 {
		t.Fatalf("unexpected non-conflict errors: %v", otherErrors)
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 approval to succeed, got %d", successCount)
	}
	if conflictCount != 1 {
		t.Errorf("expected exactly 1 approval to fail with conflict, got %d", conflictCount)
	}

	finalGoal, err := goalService.GetByID(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to fetch final goal: %v", err)
	}
	if finalGoal.Status != "completed" {
		t.Errorf("expected final goal status 'completed', got %q", finalGoal.Status)
	}

	approvals, err := approvalService.ListByGoal(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to list approvals: %v", err)
	}
	if len(approvals) != 1 {
		t.Errorf("expected exactly 1 approval record in DynamoDB, found %d", len(approvals))
	}
	if len(approvals) == 1 && approvals[0].Status != "approved" {
		t.Errorf("expected approval status 'approved', got %q", approvals[0].Status)
	}
}

// TestApprovalConcurrency_ApproveVsReject tests simultaneous approve and reject.
// Exactly one must win, and the goal status must deterministically match the winner.
func TestApprovalConcurrency_ApproveVsReject(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, approverID, goalID, proofID := setupTestGoalAndProof(t, db, tableName)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	type decisionResult struct {
		decisionType string
		approval     *approval.Approval
		err          error
	}

	startBarrier := make(chan struct{})
	results := make(chan decisionResult, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	// Worker 1: approve
	go func() {
		defer wg.Done()
		<-startBarrier
		app, err := approvalService.Decide(
			context.Background(),
			approverID,
			ownerID,
			goalID,
			proofID,
			"approved",
			"Worker approve",
		)
		results <- decisionResult{decisionType: "approved", approval: app, err: err}
	}()

	// Worker 2: reject
	go func() {
		defer wg.Done()
		<-startBarrier
		app, err := approvalService.Decide(
			context.Background(),
			approverID,
			ownerID,
			goalID,
			proofID,
			"rejected",
			"Worker reject",
		)
		results <- decisionResult{decisionType: "rejected", approval: app, err: err}
	}()

	close(startBarrier)
	wg.Wait()
	close(results)

	var successCount int
	var conflictCount int
	var winningDecision string

	for res := range results {
		if res.err == nil {
			successCount++
			winningDecision = res.decisionType
		} else if errors.Is(res.err, approval.ErrAlreadyDecided) {
			conflictCount++
		} else {
			t.Fatalf("unexpected error: %v", res.err)
		}
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 decision to succeed, got %d", successCount)
	}
	if conflictCount != 1 {
		t.Errorf("expected exactly 1 decision to fail with conflict, got %d", conflictCount)
	}

	finalGoal, err := goalService.GetByID(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to fetch final goal: %v", err)
	}

	if winningDecision == "approved" && finalGoal.Status != "completed" {
		t.Errorf("approve won: expected final goal status 'completed', got %q", finalGoal.Status)
	}
	if winningDecision == "rejected" && finalGoal.Status != "active" {
		t.Errorf("reject won: expected final goal status 'active', got %q", finalGoal.Status)
	}

	approvals, err := approvalService.ListByGoal(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to list approvals: %v", err)
	}
	if len(approvals) != 1 {
		t.Errorf("expected exactly 1 approval record, found %d", len(approvals))
	}
	if len(approvals) == 1 && approvals[0].Status != winningDecision {
		t.Errorf("expected approval record status %q, got %q", winningDecision, approvals[0].Status)
	}
}

// TestApprovalConcurrency_RejectVsReject tests two simultaneous rejections.
func TestApprovalConcurrency_RejectVsReject(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, approverID, goalID, proofID := setupTestGoalAndProof(t, db, tableName)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	type decisionResult struct {
		approval *approval.Approval
		err      error
	}

	startBarrier := make(chan struct{})
	results := make(chan decisionResult, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	for i := 0; i < 2; i++ {
		go func(workerID int) {
			defer wg.Done()
			<-startBarrier

			app, err := approvalService.Decide(
				context.Background(),
				approverID,
				ownerID,
				goalID,
				proofID,
				"rejected",
				fmt.Sprintf("Reject from worker %d", workerID),
			)
			results <- decisionResult{approval: app, err: err}
		}(i)
	}

	close(startBarrier)
	wg.Wait()
	close(results)

	var successCount int
	var conflictCount int

	for res := range results {
		if res.err == nil {
			successCount++
		} else if errors.Is(res.err, approval.ErrAlreadyDecided) {
			conflictCount++
		} else {
			t.Fatalf("unexpected error: %v", res.err)
		}
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 rejection to succeed, got %d", successCount)
	}
	if conflictCount != 1 {
		t.Errorf("expected exactly 1 rejection to fail with conflict, got %d", conflictCount)
	}

	finalGoal, err := goalService.GetByID(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to fetch final goal: %v", err)
	}
	if finalGoal.Status != "active" {
		t.Errorf("expected final goal status 'active', got %q", finalGoal.Status)
	}

	approvals, err := approvalService.ListByGoal(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to list approvals: %v", err)
	}
	if len(approvals) != 1 {
		t.Errorf("expected exactly 1 approval record, found %d", len(approvals))
	}
	if len(approvals) == 1 && approvals[0].Status != "rejected" {
		t.Errorf("expected approval status 'rejected', got %q", approvals[0].Status)
	}
}

// -------------------------------------------------------------
// SEQUENTIAL DUPLICATE DECISION TESTS
// -------------------------------------------------------------

// TestApprovalSequential_AllPermutations tests sequential duplicate decisions:
// 1. approve -> approve
// 2. approve -> reject
// 3. reject -> approve
// 4. reject -> reject
func TestApprovalSequential_AllPermutations(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	cases := []struct {
		name       string
		first      string
		second     string
		finalState string
	}{
		{"ApproveThenApprove", "approved", "approved", "completed"},
		{"ApproveThenReject", "approved", "rejected", "completed"},
		{"RejectThenApprove", "rejected", "approved", "active"},
		{"RejectThenReject", "rejected", "rejected", "active"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ownerID, approverID, goalID, proofID := setupTestGoalAndProof(t, db, tableName)

			goalRepo := goals.NewRepository(db, tableName)
			goalService := goals.NewService(goalRepo, nil)
			proofRepo := proof.NewRepository(db, tableName)
			proofService := proof.NewService(proofRepo, goalService)
			approvalRepo := approval.NewRepository(db, tableName)
			approvalService := approval.NewService(approvalRepo, goalService, proofService)

			// First decision must succeed
			firstApp, err := approvalService.Decide(
				context.Background(),
				approverID,
				ownerID,
				goalID,
				proofID,
				tc.first,
				"First decision",
			)
			if err != nil {
				t.Fatalf("first decision failed: %v", err)
			}
			if firstApp.Status != tc.first {
				t.Errorf("expected status %s, got %s", tc.first, firstApp.Status)
			}

			// Second decision must fail with ErrAlreadyDecided
			_, err = approvalService.Decide(
				context.Background(),
				approverID,
				ownerID,
				goalID,
				proofID,
				tc.second,
				"Second duplicate decision",
			)
			if err == nil {
				t.Fatalf("expected second decision to fail with conflict, but succeeded")
			}
			if !errors.Is(err, approval.ErrAlreadyDecided) {
				t.Errorf("expected ErrAlreadyDecided, got: %v", err)
			}

			// Verify final goal status
			g, err := goalService.GetByID(context.Background(), ownerID, goalID)
			if err != nil {
				t.Fatalf("failed to fetch goal: %v", err)
			}
			if g.Status != tc.finalState {
				t.Errorf("expected final status %q, got %q", tc.finalState, g.Status)
			}

			// Verify exactly 1 approval record exists
			apps, err := approvalService.ListByGoal(context.Background(), ownerID, goalID)
			if err != nil {
				t.Fatalf("failed to list approvals: %v", err)
			}
			if len(apps) != 1 {
				t.Errorf("expected exactly 1 approval record, found %d", len(apps))
			}
		})
	}
}

// -------------------------------------------------------------
// AUTHORIZATION & VALIDATION TESTS
// -------------------------------------------------------------

// TestApproval_SelfApprovalForbidden verifies that a goal owner cannot approve their own goal.
func TestApproval_SelfApprovalForbidden(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID := "test-owner-" + uuid.NewString()
	// Goal where ownerID == approverID
	goalID, proofID := setupCustomTestGoalAndProof(t, db, tableName, "proof_submitted", ownerID, ownerID)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	_, err := approvalService.Decide(
		context.Background(),
		ownerID, // Owner attempting approval
		ownerID,
		goalID,
		proofID,
		"approved",
		"Self-approval attempt",
	)
	if err == nil {
		t.Fatalf("expected self-approval to be forbidden, but succeeded")
	}
	if !errors.Is(err, approval.ErrSelfApproval) && !errors.Is(err, approval.ErrUnauthorized) {
		t.Errorf("expected ErrSelfApproval or ErrUnauthorized, got %v", err)
	}

	// Verify goal is still in proof_submitted
	g, err := goalService.GetByID(context.Background(), ownerID, goalID)
	if err != nil {
		t.Fatalf("failed to get goal: %v", err)
	}
	if g.Status != "proof_submitted" {
		t.Errorf("expected goal status 'proof_submitted', got %q", g.Status)
	}
}

// TestApproval_UnauthorizedApprover verifies that an unassigned user cannot approve.
func TestApproval_UnauthorizedApprover(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, _, goalID, proofID := setupTestGoalAndProof(t, db, tableName)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	_, err := approvalService.Decide(
		context.Background(),
		"random-unauthorized-user",
		ownerID,
		goalID,
		proofID,
		"approved",
		"Unauthorized attempt",
	)
	if !errors.Is(err, approval.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

// TestApproval_GoalStatesRejection verifies approval is rejected when goal is active or completed.
func TestApproval_GoalStatesRejection(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	for _, invalidStatus := range []string{"active", "completed"} {
		t.Run("Status_"+invalidStatus, func(t *testing.T) {
			ownerID := "test-owner-" + uuid.NewString()
			approverID := "test-approver-" + uuid.NewString()
			goalID, proofID := setupCustomTestGoalAndProof(t, db, tableName, invalidStatus, ownerID, approverID)

			goalRepo := goals.NewRepository(db, tableName)
			goalService := goals.NewService(goalRepo, nil)
			proofRepo := proof.NewRepository(db, tableName)
			proofService := proof.NewService(proofRepo, goalService)
			approvalRepo := approval.NewRepository(db, tableName)
			approvalService := approval.NewService(approvalRepo, goalService, proofService)

			_, err := approvalService.Decide(
				context.Background(),
				approverID,
				ownerID,
				goalID,
				proofID,
				"approved",
				"Decision on invalid status",
			)
			if err == nil {
				t.Fatalf("expected decision on status %s to fail, but succeeded", invalidStatus)
			}
			if !errors.Is(err, approval.ErrAlreadyDecided) {
				t.Errorf("expected ErrAlreadyDecided, got %v", err)
			}

			// Goal status must remain unchanged
			g, _ := goalService.GetByID(context.Background(), ownerID, goalID)
			if g.Status != invalidStatus {
				t.Errorf("expected goal status to remain %q, got %q", invalidStatus, g.Status)
			}
		})
	}
}

// TestApproval_NonexistentGoalAndProof verifies 404 behavior for nonexistent goal and proof.
func TestApproval_NonexistentGoalAndProof(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, approverID, goalID, proofID := setupTestGoalAndProof(t, db, tableName)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	// 1. Non-existent goal
	_, err := approvalService.Decide(
		context.Background(),
		approverID,
		ownerID,
		"missing-goal-id",
		proofID,
		"approved",
		"Test",
	)
	if !errors.Is(err, approval.ErrGoalNotFound) {
		t.Errorf("expected ErrGoalNotFound, got %v", err)
	}

	// 2. Non-existent proof
	_, err = approvalService.Decide(
		context.Background(),
		approverID,
		ownerID,
		goalID,
		"missing-proof-id",
		"approved",
		"Test",
	)
	if !errors.Is(err, approval.ErrProofNotFound) {
		t.Errorf("expected ErrProofNotFound, got %v", err)
	}
}

// TestApproval_ProofSubstitutionRejection verifies that approving proof from another goal is rejected.
func TestApproval_ProofSubstitutionRejection(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID1, approverID1, goalID1, _ := setupTestGoalAndProof(t, db, tableName)
	ownerID2, _, _, proofID2 := setupTestGoalAndProof(t, db, tableName)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	// Attempt to use proofID from Goal 2 (Owner 2) on Goal 1 (Owner 1)
	_, err := approvalService.Decide(
		context.Background(),
		approverID1,
		ownerID1,
		goalID1,
		proofID2, // Foreign proof
		"approved",
		"Substitution attempt",
	)
	if err == nil {
		t.Fatalf("expected proof substitution to fail, but succeeded")
	}
	if !errors.Is(err, approval.ErrInvalidProof) && !errors.Is(err, approval.ErrProofNotFound) {
		t.Errorf("expected ErrInvalidProof or ErrProofNotFound, got %v", err)
	}

	// Attempt with mismatching ownerID
	_, err = approvalService.Decide(
		context.Background(),
		approverID1,
		ownerID2, // Mismatched owner in request
		goalID1,
		proofID2,
		"approved",
		"Owner mismatch attempt",
	)
	if err == nil {
		t.Fatalf("expected owner mismatch to fail, but succeeded")
	}
}

// TestApproval_InvalidDecisionStatus verifies that non-approved/rejected statuses are rejected.
func TestApproval_InvalidDecisionStatus(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, approverID, goalID, proofID := setupTestGoalAndProof(t, db, tableName)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	for _, invalidStatus := range []string{"maybe", "pending", "accepted", "COMPLETED", ""} {
		_, err := approvalService.Decide(
			context.Background(),
			approverID,
			ownerID,
			goalID,
			proofID,
			invalidStatus,
			"Test invalid status",
		)
		if !errors.Is(err, approval.ErrInvalidDecision) {
			t.Errorf("expected ErrInvalidDecision for %q, got %v", invalidStatus, err)
		}
	}
}

// -------------------------------------------------------------
// HTTP HANDLER STATUS CODE TESTS
// -------------------------------------------------------------

// TestApprovalHandler_HTTPStatusCodes verifies that the HTTP handler properly maps:
// 201 Created on valid decision
// 403 Forbidden for unauthorized approver or self-approval
// 404 Not Found for missing goal or proof
// 409 Conflict for already decided goal
// 400 Bad Request for invalid decision status or invalid proof
func TestApprovalHandler_HTTPStatusCodes(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, approverID, goalID, proofID := setupTestGoalAndProof(t, db, tableName)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)
	approvalHandler := approval.NewHandler(approvalService)

	targetURL := fmt.Sprintf("/goals/%s/proofs/%s/approval", goalID, proofID)

	// 1. Unauthorized approver -> 403 Forbidden
	reqBody := fmt.Sprintf(`{"ownerId":"%s","status":"approved","comment":"unauthorized attempt"}`, ownerID)
	req := httptest.NewRequest("POST", targetURL, strings.NewReader(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "some-other-user"))
	rec := httptest.NewRecorder()
	approvalHandler.Decide(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d for unauthorized approver, got %d", http.StatusForbidden, rec.Code)
	}

	// 2. Goal owner attempting approval (self-approval) -> 403 Forbidden
	req = httptest.NewRequest("POST", targetURL, strings.NewReader(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
	rec = httptest.NewRecorder()
	approvalHandler.Decide(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d for self-approval, got %d", http.StatusForbidden, rec.Code)
	}

	// 3. Nonexistent goal -> 404 Not Found
	missingGoalURL := fmt.Sprintf("/goals/non-existent-goal/proofs/%s/approval", proofID)
	req = httptest.NewRequest("POST", missingGoalURL, strings.NewReader(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, approverID))
	rec = httptest.NewRecorder()
	approvalHandler.Decide(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d for nonexistent goal, got %d", http.StatusNotFound, rec.Code)
	}

	// 4. Nonexistent proof -> 404 Not Found
	missingProofURL := fmt.Sprintf("/goals/%s/proofs/non-existent-proof/approval", goalID)
	req = httptest.NewRequest("POST", missingProofURL, strings.NewReader(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, approverID))
	rec = httptest.NewRecorder()
	approvalHandler.Decide(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d for nonexistent proof, got %d", http.StatusNotFound, rec.Code)
	}

	// 5. Invalid decision status -> 400 Bad Request
	badStatusBody := fmt.Sprintf(`{"ownerId":"%s","status":"maybe","comment":"invalid status"}`, ownerID)
	req = httptest.NewRequest("POST", targetURL, strings.NewReader(badStatusBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, approverID))
	rec = httptest.NewRecorder()
	approvalHandler.Decide(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for invalid decision, got %d", http.StatusBadRequest, rec.Code)
	}

	// 6. Legitimate approval -> 201 Created
	validBody := fmt.Sprintf(`{"ownerId":"%s","status":"approved","comment":"legitimate decision"}`, ownerID)
	req = httptest.NewRequest("POST", targetURL, strings.NewReader(validBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, approverID))
	rec = httptest.NewRecorder()
	approvalHandler.Decide(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d for valid approval, got %d (body: %s)", http.StatusCreated, rec.Code, rec.Body.String())
	}

	// 7. Duplicate approval attempt -> 409 Conflict
	dupBody := fmt.Sprintf(`{"ownerId":"%s","status":"rejected","comment":"duplicate attempt"}`, ownerID)
	req = httptest.NewRequest("POST", targetURL, strings.NewReader(dupBody))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, approverID))
	rec = httptest.NewRecorder()
	approvalHandler.Decide(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected status %d for already decided proof, got %d (body: %s)", http.StatusConflict, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "proof has already been decided") {
		t.Errorf("expected body to contain 'proof has already been decided', got %s", rec.Body.String())
	}
}

// TestApproval_DecisionListenerNotification verifies that DecisionListeners
// are invoked only when TransactWriteItems succeeds and never on failure.
func TestApproval_DecisionListenerNotification(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, approverID, goalID, proofID := setupTestGoalAndProof(t, db, tableName)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	var listenerCalls int
	var receivedApp *approval.Approval
	var receivedGoal *goals.Goal

	approvalService.AddDecisionListener(approval.DecisionListenerFunc(func(ctx context.Context, app *approval.Approval, goal *goals.Goal) {
		listenerCalls++
		receivedApp = app
		receivedGoal = goal
	}))

	// 1. Valid decision -> listener must be called once with correct payload
	app, err := approvalService.Decide(
		context.Background(),
		approverID,
		ownerID,
		goalID,
		proofID,
		"approved",
		"Listener test approval",
	)
	if err != nil {
		t.Fatalf("unexpected error on decide: %v", err)
	}

	if listenerCalls != 1 {
		t.Fatalf("expected listener to be called 1 time, got %d", listenerCalls)
	}
	if receivedApp == nil || receivedApp.ID != app.ID {
		t.Errorf("expected listener to receive created approval, got %+v", receivedApp)
	}
	if receivedGoal == nil || receivedGoal.ID != goalID {
		t.Errorf("expected listener to receive target goal, got %+v", receivedGoal)
	}

	// 2. Duplicate decision -> should fail with ErrAlreadyDecided and NOT call listener again
	_, err = approvalService.Decide(
		context.Background(),
		approverID,
		ownerID,
		goalID,
		proofID,
		"rejected",
		"Duplicate attempt",
	)
	if err == nil {
		t.Fatalf("expected duplicate decision to fail")
	}
	if listenerCalls != 1 {
		t.Errorf("expected listener calls to remain 1 after failure, got %d", listenerCalls)
	}
}

type testApprovalPublisher struct {
	events []events.ApprovalDecidedEvent
}

func (p *testApprovalPublisher) PublishProofSubmitted(ctx context.Context, event events.ProofSubmittedEvent) error {
	return nil
}

func (p *testApprovalPublisher) PublishApprovalDecided(ctx context.Context, event events.ApprovalDecidedEvent) error {
	p.events = append(p.events, event)
	return nil
}

func TestApproval_PublisherEventEmission(t *testing.T) {
	db, tableName := getTestDynamoClient(t)
	ownerID, approverID, goalID, proofID := setupTestGoalAndProof(t, db, tableName)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	mockPub := &testApprovalPublisher{}
	approvalService.SetPublisher(mockPub)

	// 1. Successful decision -> exactly one event emitted
	app, err := approvalService.Decide(
		context.Background(),
		approverID,
		ownerID,
		goalID,
		proofID,
		"approved",
		"Publisher test approval",
	)
	if err != nil {
		t.Fatalf("unexpected error on decide: %v", err)
	}

	if len(mockPub.events) != 1 {
		t.Fatalf("expected exactly 1 event emitted, got %d", len(mockPub.events))
	}

	ev := mockPub.events[0]
	if ev.EventID != "approval-"+app.ID {
		t.Errorf("expected stable event ID 'approval-%s', got %q", app.ID, ev.EventID)
	}
	if ev.GoalID != goalID {
		t.Errorf("expected goalID %q, got %q", goalID, ev.GoalID)
	}
	if ev.OwnerID != ownerID {
		t.Errorf("expected ownerID %q, got %q", ownerID, ev.OwnerID)
	}
	if ev.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", ev.Status)
	}

	// 2. Duplicate decision attempt -> fails and emits 0 new events
	_, err = approvalService.Decide(
		context.Background(),
		approverID,
		ownerID,
		goalID,
		proofID,
		"rejected",
		"Duplicate attempt",
	)
	if err == nil {
		t.Fatalf("expected duplicate decision to fail")
	}

	if len(mockPub.events) != 1 {
		t.Errorf("expected event count to remain 1 after duplicate failure, got %d", len(mockPub.events))
	}
}
