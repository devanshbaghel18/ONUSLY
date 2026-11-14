package approval_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/approval"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
)

type mockSubmissionsApprovalRepo struct {
	approvals []approval.Approval
}

func (m *mockSubmissionsApprovalRepo) Create(ctx context.Context, app approval.Approval) error {
	m.approvals = append(m.approvals, app)
	return nil
}

func (m *mockSubmissionsApprovalRepo) CreateDecision(ctx context.Context, app approval.Approval, expectedGoalStatus string, newGoalStatus string) error {
	m.approvals = append(m.approvals, app)
	return nil
}

func (m *mockSubmissionsApprovalRepo) GetByID(ctx context.Context, ownerID, goalID, approvalID string) (*approval.Approval, error) {
	for _, a := range m.approvals {
		if a.ID == approvalID {
			return &a, nil
		}
	}
	return nil, approval.ErrApprovalNotFound
}

func (m *mockSubmissionsApprovalRepo) ListByGoal(ctx context.Context, ownerID, goalID string) ([]approval.Approval, error) {
	var list []approval.Approval
	for _, a := range m.approvals {
		if a.GoalID == goalID {
			list = append(list, a)
		}
	}
	return list, nil
}

type mockSubmissionsProofRepo struct {
	proofs []proof.Proof
}

func (m *mockSubmissionsProofRepo) Create(ctx context.Context, p proof.Proof) error {
	m.proofs = append(m.proofs, p)
	return nil
}

func (m *mockSubmissionsProofRepo) CreateWithGoalTransition(ctx context.Context, p proof.Proof, expected []string, newStatus string) error {
	m.proofs = append(m.proofs, p)
	return nil
}

func (m *mockSubmissionsProofRepo) GetByID(ctx context.Context, ownerID, goalID, proofID string) (*proof.Proof, error) {
	for _, p := range m.proofs {
		if p.ID == proofID {
			return &p, nil
		}
	}
	return nil, proof.ErrProofNotFound
}

func (m *mockSubmissionsProofRepo) ListByGoal(ctx context.Context, ownerID, goalID string) ([]proof.Proof, error) {
	var list []proof.Proof
	for _, p := range m.proofs {
		if p.GoalID == goalID {
			list = append(list, p)
		}
	}
	return list, nil
}

func TestGetSubmissions_MultiAttemptChronology(t *testing.T) {
	ctx := context.Background()
	ownerID := "user-alice"
	approverID := "user-bob"
	goalID := "goal-100"

	goalRepo := newD11MockGoalRepoForApproval()
	goalService := goals.NewService(goalRepo, nil)
	_ = goalRepo.Create(ctx, goals.Goal{
		ID:            goalID,
		OwnerID:       ownerID,
		ApproverID:    approverID,
		ApproverEmail: "bob@example.com",
		Title:         "Study Go Concurrency",
		Status:        "proof_submitted",
	})

	t1 := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	t2 := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	t3 := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)

	proofRepo := &mockSubmissionsProofRepo{
		proofs: []proof.Proof{
			{
				ID:              "proof-1",
				GoalID:          goalID,
				OwnerID:         ownerID,
				ProofType:       "photo",
				TextExplanation: "Initial code snippet",
				PhotoURL:        "/uploads/proofs/1.png",
				SubmittedAt:     t1,
			},
			{
				ID:              "proof-2",
				GoalID:          goalID,
				OwnerID:         ownerID,
				ProofType:       "photo",
				TextExplanation: "Updated with benchmarks",
				PhotoURL:        "/uploads/proofs/2.png",
				SubmittedAt:     t2,
			},
			{
				ID:              "proof-3",
				GoalID:          goalID,
				OwnerID:         ownerID,
				ProofType:       "text",
				TextExplanation: "Final verification with all tests passing",
				SubmittedAt:     t3,
			},
		},
	}
	proofService := proof.NewService(proofRepo, goalService)

	decidedT1 := time.Now().Add(-90 * time.Minute).UTC().Format(time.RFC3339)
	decidedT2 := time.Now().Add(-30 * time.Minute).UTC().Format(time.RFC3339)

	approvalRepo := &mockSubmissionsApprovalRepo{
		approvals: []approval.Approval{
			{
				ID:         "app-1",
				GoalID:     goalID,
				ProofID:    "proof-1",
				OwnerID:    ownerID,
				ApproverID: approverID,
				Status:     "rejected",
				Comment:    "Screenshot unclear, missing compiler output.",
				DecidedAt:  decidedT1,
			},
			{
				ID:         "app-2",
				GoalID:     goalID,
				ProofID:    "proof-2",
				OwnerID:    ownerID,
				ApproverID: approverID,
				Status:     "rejected",
				Comment:    "Benchmarks need 1000 iterations minimum.",
				DecidedAt:  decidedT2,
			},
		},
	}
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	// Fetch dossier as owner
	dossier, err := approvalService.GetSubmissions(ctx, ownerID, "alice@example.com", "alice", goalID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if dossier.TotalAttempts != 3 {
		t.Errorf("expected 3 total attempts, got %d", dossier.TotalAttempts)
	}

	if len(dossier.Attempts) != 3 {
		t.Fatalf("expected 3 attempts in slice, got %d", len(dossier.Attempts))
	}

	// Verify Attempt #1
	a1 := dossier.Attempts[0]
	if a1.AttemptNumber != 1 || a1.Status != "rejected" || a1.Comment != "Screenshot unclear, missing compiler output." {
		t.Errorf("attempt 1 mismatch: %+v", a1)
	}
	if a1.IsLatest {
		t.Errorf("attempt 1 should not be latest")
	}

	// Verify Attempt #2
	a2 := dossier.Attempts[1]
	if a2.AttemptNumber != 2 || a2.Status != "rejected" || a2.Comment != "Benchmarks need 1000 iterations minimum." {
		t.Errorf("attempt 2 mismatch: %+v", a2)
	}
	if a2.IsLatest {
		t.Errorf("attempt 2 should not be latest")
	}

	// Verify Attempt #3
	a3 := dossier.Attempts[2]
	if a3.AttemptNumber != 3 || a3.Status != "pending" {
		t.Errorf("attempt 3 mismatch: %+v", a3)
	}
	if !a3.IsLatest {
		t.Errorf("attempt 3 should be marked latest")
	}

	// Verify PendingAttempt & LatestDecision pointers
	if dossier.PendingAttempt == nil || dossier.PendingAttempt.ProofID != "proof-3" {
		t.Errorf("expected pending attempt proof-3, got %v", dossier.PendingAttempt)
	}
	if dossier.LatestDecision == nil || dossier.LatestDecision.Comment != "Benchmarks need 1000 iterations minimum." {
		t.Errorf("expected latest decision from attempt 2, got %v", dossier.LatestDecision)
	}

	// Verify AuditLog
	if len(dossier.AuditLog) != 2 {
		t.Errorf("expected 2 audit log receipts, got %d", len(dossier.AuditLog))
	}
	if dossier.AuditLog[0].AttemptIndex != 1 || dossier.AuditLog[1].AttemptIndex != 2 {
		t.Errorf("unexpected audit log attempt indices: %+v", dossier.AuditLog)
	}
}

func TestGetSubmissions_Authorization(t *testing.T) {
	ctx := context.Background()
	ownerID := "user-alice"
	approverID := "user-bob"
	goalID := "goal-200"

	goalRepo := newD11MockGoalRepoForApproval()
	goalService := goals.NewService(goalRepo, nil)
	_ = goalRepo.Create(ctx, goals.Goal{
		ID:            goalID,
		OwnerID:       ownerID,
		ApproverID:    approverID,
		ApproverEmail: "bob@example.com",
		Title:         "Private Goal",
		Status:        "active",
	})

	proofRepo := &mockSubmissionsProofRepo{}
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := &mockSubmissionsApprovalRepo{}
	approvalService := approval.NewService(approvalRepo, goalService, proofService)

	// Random unauthorized user
	_, err := approvalService.GetSubmissions(ctx, "stranger-charlie", "charlie@example.com", "charlie", goalID)
	if err != approval.ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}

	// Non-existent goal
	_, err = approvalService.GetSubmissions(ctx, ownerID, "alice@example.com", "alice", "non-existent")
	if err != approval.ErrGoalNotFound {
		t.Errorf("expected ErrGoalNotFound, got %v", err)
	}
}

func TestHandler_GetSubmissions(t *testing.T) {
	ctx := context.Background()
	ownerID := "user-alice"
	approverID := "user-bob"
	goalID := "goal-handler-test"

	goalRepo := newD11MockGoalRepoForApproval()
	goalService := goals.NewService(goalRepo, nil)
	_ = goalRepo.Create(ctx, goals.Goal{
		ID:            goalID,
		OwnerID:       ownerID,
		ApproverID:    approverID,
		ApproverEmail: "bob@example.com",
		Title:         "Handler Test Goal",
		Status:        "active",
	})

	proofRepo := &mockSubmissionsProofRepo{}
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := &mockSubmissionsApprovalRepo{}
	approvalService := approval.NewService(approvalRepo, goalService, proofService)
	handler := approval.NewHandler(approvalService)

	// 1. Authorized Request
	req := httptest.NewRequest("GET", "/goals/"+goalID+"/submissions", nil)
	req.SetPathValue("id", goalID)
	reqCtx := context.WithValue(req.Context(), middleware.UserIDKey, ownerID)
	reqCtx = context.WithValue(reqCtx, middleware.UserEmailKey, "alice@example.com")
	reqCtx = context.WithValue(reqCtx, middleware.UserHandleKey, "alice")
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()
	handler.GetSubmissions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Unauthorized Request (no claims)
	unauthReq := httptest.NewRequest("GET", "/goals/"+goalID+"/submissions", nil)
	unauthRec := httptest.NewRecorder()
	handler.GetSubmissions(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", unauthRec.Code)
	}

	// 3. Forbidden Request (different user)
	forbidReq := httptest.NewRequest("GET", "/goals/"+goalID+"/submissions", nil)
	forbidReq.SetPathValue("id", goalID)
	forbidCtx := context.WithValue(forbidReq.Context(), middleware.UserIDKey, "random-stranger")
	forbidCtx = context.WithValue(forbidCtx, middleware.UserEmailKey, "stranger@example.com")
	forbidReq = forbidReq.WithContext(forbidCtx)
	forbidRec := httptest.NewRecorder()
	handler.GetSubmissions(forbidRec, forbidReq)
	if forbidRec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden, got %d", forbidRec.Code)
	}
}

