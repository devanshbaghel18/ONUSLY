package approval_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/approval"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
)

func TestApprovalSecurity_UnrelatedUserBlockedFromDeciding(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	ownerID := "test-owner-" + uuid.NewString()
	approverID := "test-approver-" + uuid.NewString()
	eveID := "test-eve-" + uuid.NewString()

	goalID, proofID := setupCustomTestGoalAndProof(t, db, tableName, "proof_submitted", ownerID, approverID)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)

	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)

	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)
	handler := approval.NewHandler(approvalService)

	// 1. Eve attempts to decide proof
	body := strings.NewReader(fmt.Sprintf(`{"ownerId":"%s","status":"approved","comment":"I approve this"}`, ownerID))
	url := fmt.Sprintf("/goals/%s/proofs/%s/approval", goalID, proofID)
	req := httptest.NewRequest("POST", url, body)
	req.SetPathValue("id", goalID)
	req.SetPathValue("proofId", proofID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, eveID))
	rec := httptest.NewRecorder()

	handler.Decide(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when unrelated user attempts decision, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestApprovalSecurity_OwnerCannotSelfApprove(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	ownerID := "test-owner-" + uuid.NewString()
	approverID := "test-approver-" + uuid.NewString()

	goalID, proofID := setupCustomTestGoalAndProof(t, db, tableName, "proof_submitted", ownerID, approverID)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)

	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)

	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)
	handler := approval.NewHandler(approvalService)

	// Goal owner tries to approve their own goal
	body := strings.NewReader(fmt.Sprintf(`{"ownerId":"%s","status":"approved","comment":"Self approved"}`, ownerID))
	url := fmt.Sprintf("/goals/%s/proofs/%s/approval", goalID, proofID)
	req := httptest.NewRequest("POST", url, body)
	req.SetPathValue("id", goalID)
	req.SetPathValue("proofId", proofID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, ownerID))
	rec := httptest.NewRecorder()

	handler.Decide(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when owner attempts self-approval, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestApprovalSecurity_ForgedOwnerIDOrInconsistentProof_Rejected(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	ownerID := "test-owner-" + uuid.NewString()
	approverID := "test-approver-" + uuid.NewString()

	goalID, proofID := setupCustomTestGoalAndProof(t, db, tableName, "proof_submitted", ownerID, approverID)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)

	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)

	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)
	handler := approval.NewHandler(approvalService)

	// 1. Forged owner ID in request
	bodyForged := strings.NewReader(`{"ownerId":"wrong-owner-id","status":"approved","comment":"Forged owner"}`)
	url := fmt.Sprintf("/goals/%s/proofs/%s/approval", goalID, proofID)
	reqForged := httptest.NewRequest("POST", url, bodyForged)
	reqForged.SetPathValue("id", goalID)
	reqForged.SetPathValue("proofId", proofID)
	reqForged = reqForged.WithContext(context.WithValue(reqForged.Context(), middleware.UserIDKey, approverID))
	recForged := httptest.NewRecorder()

	handler.Decide(recForged, reqForged)

	if recForged.Code != http.StatusForbidden && recForged.Code != http.StatusNotFound {
		t.Fatalf("expected 403 Forbidden or 404 Not Found for forged owner ID, got %d: %s", recForged.Code, recForged.Body.String())
	}

	// 2. Inconsistent proof relationship (nonexistent proof ID)
	bodyBadProof := strings.NewReader(fmt.Sprintf(`{"ownerId":"%s","status":"approved","comment":"Valid"}`, ownerID))
	urlBadProof := fmt.Sprintf("/goals/%s/proofs/%s/approval", goalID, "non-existent-proof-id")
	reqBadProof := httptest.NewRequest("POST", urlBadProof, bodyBadProof)
	reqBadProof.SetPathValue("id", goalID)
	reqBadProof.SetPathValue("proofId", "non-existent-proof-id")
	reqBadProof = reqBadProof.WithContext(context.WithValue(reqBadProof.Context(), middleware.UserIDKey, approverID))
	recBadProof := httptest.NewRecorder()

	handler.Decide(recBadProof, reqBadProof)

	if recBadProof.Code != http.StatusNotFound && recBadProof.Code != http.StatusBadRequest {
		t.Fatalf("expected 404 or 400 for nonexistent proof ID, got %d: %s", recBadProof.Code, recBadProof.Body.String())
	}
}

func TestApprovalSecurity_UnrelatedUserBlockedFromViewingApprovals(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	ownerID := "test-owner-" + uuid.NewString()
	approverID := "test-approver-" + uuid.NewString()
	eveID := "test-eve-" + uuid.NewString()

	goalID, proofID := setupCustomTestGoalAndProof(t, db, tableName, "proof_submitted", ownerID, approverID)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)

	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)

	approvalRepo := approval.NewRepository(db, tableName)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)
	handler := approval.NewHandler(approvalService)

	// Legitimate approval by assigned approver
	_, err := approvalService.Decide(
		context.Background(),
		approverID,
		ownerID,
		goalID,
		proofID,
		"approved",
		"Looks good",
	)
	if err != nil {
		t.Fatalf("failed to record approval: %v", err)
	}

	// 1. Eve attempts GET /goals/{id}/approvals
	urlList := fmt.Sprintf("/goals/%s/approvals", goalID)
	reqList := httptest.NewRequest("GET", urlList, nil)
	reqList.SetPathValue("id", goalID)
	reqList = reqList.WithContext(context.WithValue(reqList.Context(), middleware.UserIDKey, eveID))
	recList := httptest.NewRecorder()

	handler.ListByGoal(recList, reqList)

	if recList.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Eve listing approvals, got %d: %s", recList.Code, recList.Body.String())
	}

	// 2. Approver attempts GET /goals/{id}/approvals -> 200 OK
	reqApprover := httptest.NewRequest("GET", urlList, nil)
	reqApprover.SetPathValue("id", goalID)
	reqApprover = reqApprover.WithContext(context.WithValue(reqApprover.Context(), middleware.UserIDKey, approverID))
	recApprover := httptest.NewRecorder()

	handler.ListByGoal(recApprover, reqApprover)

	if recApprover.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for approver listing approvals, got %d: %s", recApprover.Code, recApprover.Body.String())
	}
}
