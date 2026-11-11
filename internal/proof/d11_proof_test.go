package proof_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
)

type d11MockGoalRepo struct {
	mu    sync.Mutex
	goals map[string]goals.Goal
}

func newD11MockGoalRepo() *d11MockGoalRepo {
	return &d11MockGoalRepo{
		goals: make(map[string]goals.Goal),
	}
}

func (r *d11MockGoalRepo) Create(ctx context.Context, g goals.Goal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.goals[g.OwnerID+"#"+g.ID] = g
	return nil
}

func (r *d11MockGoalRepo) GetByID(ctx context.Context, ownerID, goalID string) (*goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.goals[ownerID+"#"+goalID]
	if !ok {
		return nil, errors.New("goal not found")
	}
	return &g, nil
}

func (r *d11MockGoalRepo) FindByID(ctx context.Context, goalID string) (*goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.goals {
		if g.ID == goalID {
			return &g, nil
		}
	}
	return nil, errors.New("goal not found")
}

func (r *d11MockGoalRepo) List(ctx context.Context, ownerID string) ([]goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []goals.Goal
	for _, g := range r.goals {
		if g.OwnerID == ownerID {
			list = append(list, g)
		}
	}
	return list, nil
}

func (r *d11MockGoalRepo) Delete(ctx context.Context, ownerID, goalID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.goals, ownerID+"#"+goalID)
	return nil
}

func (r *d11MockGoalRepo) Update(ctx context.Context, ownerID, goalID string, input goals.UpdateGoalInput) (*goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.goals[ownerID+"#"+goalID]
	if !ok {
		return nil, errors.New("goal not found")
	}
	if input.Title != nil {
		g.Title = *input.Title
	}
	if input.Description != nil {
		g.Description = *input.Description
	}
	r.goals[ownerID+"#"+goalID] = g
	return &g, nil
}

func (r *d11MockGoalRepo) UpdateStatus(ctx context.Context, ownerID, goalID, status string) (*goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.goals[ownerID+"#"+goalID]
	if !ok {
		return nil, errors.New("goal not found")
	}
	g.Status = status
	r.goals[ownerID+"#"+goalID] = g
	return &g, nil
}

type d11MockProofRepo struct {
	mu       sync.Mutex
	proofs   map[string]proof.Proof
	goalRepo *d11MockGoalRepo
}

func newD11MockProofRepo(goalRepo *d11MockGoalRepo) *d11MockProofRepo {
	return &d11MockProofRepo{
		proofs:   make(map[string]proof.Proof),
		goalRepo: goalRepo,
	}
}

func (r *d11MockProofRepo) Create(ctx context.Context, p proof.Proof) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.proofs[p.GoalID+"#"+p.ID] = p
	return nil
}

func (r *d11MockProofRepo) CreateWithGoalTransition(ctx context.Context, p proof.Proof, expectedGoalStatuses []string, newGoalStatus string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	g, err := r.goalRepo.GetByID(ctx, p.OwnerID, p.GoalID)
	if err != nil {
		return errors.New("goal not found")
	}

	match := false
	for _, s := range expectedGoalStatuses {
		if g.Status == s {
			match = true
			break
		}
	}
	if !match {
		if g.Status == "proof_submitted" {
			return proof.ErrProofAlreadySubmitted
		}
		return proof.ErrGoalNotActive
	}

	_, _ = r.goalRepo.UpdateStatus(ctx, p.OwnerID, p.GoalID, newGoalStatus)
	r.proofs[p.GoalID+"#"+p.ID] = p
	return nil
}

func (r *d11MockProofRepo) GetByID(ctx context.Context, ownerID, goalID, proofID string) (*proof.Proof, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.proofs[goalID+"#"+proofID]
	if !ok || p.OwnerID != ownerID {
		return nil, proof.ErrProofNotFound
	}
	return &p, nil
}

func (r *d11MockProofRepo) ListByGoal(ctx context.Context, ownerID, goalID string) ([]proof.Proof, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []proof.Proof
	for _, p := range r.proofs {
		if p.GoalID == goalID && p.OwnerID == ownerID {
			list = append(list, p)
		}
	}
	return list, nil
}

// -------------------------------------------------------------
// D11 PROOF LIFECYCLE TESTS
// -------------------------------------------------------------

func TestD11_ProofSubmission_ActiveGoal(t *testing.T) {
	ctx := context.Background()
	goalRepo := newD11MockGoalRepo()
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := newD11MockProofRepo(goalRepo)
	proofService := proof.NewService(proofRepo, goalService)

	ownerID := "user-alice"
	goalID := "goal-101"
	approverID := "user-bob"

	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   approverID,
		ApprovalType: "friend",
		Title:        "Complete Go Concurrency Milestone",
		Status:       "active",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	p, err := proofService.Submit(ctx, ownerID, goalID, "text", "Submitted PR for review", "", "")
	if err != nil {
		t.Fatalf("expected proof submission to succeed, got: %v", err)
	}

	if p.ID == "" {
		t.Fatalf("expected non-empty proof ID")
	}

	// Verify goal transitioned to proof_submitted
	g, err := goalRepo.GetByID(ctx, ownerID, goalID)
	if err != nil || g.Status != "proof_submitted" {
		t.Fatalf("expected goal status 'proof_submitted', got: %v (err: %v)", g.Status, err)
	}
}

func TestD11_ProofSubmission_RejectedGoal_Resubmission(t *testing.T) {
	ctx := context.Background()
	goalRepo := newD11MockGoalRepo()
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := newD11MockProofRepo(goalRepo)
	proofService := proof.NewService(proofRepo, goalService)

	ownerID := "user-alice"
	goalID := "goal-102"
	approverID := "user-bob"

	// Goal previously rejected by approver
	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   approverID,
		ApprovalType: "friend",
		Title:        "Exercise 45 minutes",
		Status:       "rejected",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	// Resubmission must succeed and transition goal back to proof_submitted
	p, err := proofService.Submit(ctx, ownerID, goalID, "link", "Updated Strava activity link", "https://strava.com/activities/123456", "")
	if err != nil {
		t.Fatalf("expected proof resubmission on rejected goal to succeed, got: %v", err)
	}

	if p.ExternalLink != "https://strava.com/activities/123456" {
		t.Fatalf("unexpected external link: %s", p.ExternalLink)
	}

	g, err := goalRepo.GetByID(ctx, ownerID, goalID)
	if err != nil || g.Status != "proof_submitted" {
		t.Fatalf("expected goal status 'proof_submitted' after resubmission, got: %v", g.Status)
	}
}

func TestD11_ProofSubmission_CompletedGoal_Fails(t *testing.T) {
	ctx := context.Background()
	goalRepo := newD11MockGoalRepo()
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := newD11MockProofRepo(goalRepo)
	proofService := proof.NewService(proofRepo, goalService)

	ownerID := "user-alice"
	goalID := "goal-103"
	approverID := "user-bob"

	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   approverID,
		ApprovalType: "friend",
		Title:        "Completed Task",
		Status:       "completed",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	_, err := proofService.Submit(ctx, ownerID, goalID, "text", "Submitting proof on completed goal", "", "")
	if !errors.Is(err, proof.ErrGoalNotActive) {
		t.Fatalf("expected ErrGoalNotActive when submitting on completed goal, got: %v", err)
	}
}

func TestD11_ProofSubmission_AlreadySubmitted_Fails(t *testing.T) {
	ctx := context.Background()
	goalRepo := newD11MockGoalRepo()
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := newD11MockProofRepo(goalRepo)
	proofService := proof.NewService(proofRepo, goalService)

	ownerID := "user-alice"
	goalID := "goal-104"
	approverID := "user-bob"

	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   approverID,
		ApprovalType: "friend",
		Title:        "Pending Task",
		Status:       "proof_submitted",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	_, err := proofService.Submit(ctx, ownerID, goalID, "text", "Duplicate submission attempt", "", "")
	if !errors.Is(err, proof.ErrProofAlreadySubmitted) {
		t.Fatalf("expected ErrProofAlreadySubmitted, got: %v", err)
	}
}

func TestD11_ProofSubmission_Unauthorized_NonOwner(t *testing.T) {
	ctx := context.Background()
	goalRepo := newD11MockGoalRepo()
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := newD11MockProofRepo(goalRepo)
	proofService := proof.NewService(proofRepo, goalService)

	ownerID := "user-alice"
	eveID := "user-eve"
	goalID := "goal-105"
	approverID := "user-bob"

	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   approverID,
		ApprovalType: "friend",
		Title:        "Confidential Project",
		Status:       "active",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	// Eve tries to submit proof for Alice's goal
	_, err := proofService.Submit(ctx, eveID, goalID, "text", "Forged proof", "", "")
	if err == nil {
		t.Fatalf("expected non-owner proof submission to be rejected, but succeeded")
	}
}

func TestD11_ProofAccess_Authorization(t *testing.T) {
	ctx := context.Background()
	goalRepo := newD11MockGoalRepo()
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := newD11MockProofRepo(goalRepo)
	proofService := proof.NewService(proofRepo, goalService)

	ownerID := "user-alice"
	approverID := "user-bob"
	eveID := "user-eve"
	goalID := "goal-106"

	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   approverID,
		ApprovalType: "friend",
		Title:        "Design System Sprint",
		Status:       "active",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	p, err := proofService.Submit(ctx, ownerID, goalID, "text", "Figma prototype attached", "", "")
	if err != nil {
		t.Fatalf("failed to submit proof: %v", err)
	}

	// 1. Owner can access
	pOwner, err := proofService.GetByIDAuthorized(ctx, ownerID, "alice@example.com", "alice", goalID, p.ID)
	if err != nil || pOwner == nil {
		t.Fatalf("owner failed to access proof: %v", err)
	}

	// 2. Assigned approver can access
	pApprover, err := proofService.GetByIDAuthorized(ctx, approverID, "bob@example.com", "bob", goalID, p.ID)
	if err != nil || pApprover == nil {
		t.Fatalf("approver failed to access proof: %v", err)
	}

	// 3. Unrelated caller (Eve) is blocked with ErrUnauthorized
	_, err = proofService.GetByIDAuthorized(ctx, eveID, "eve@example.com", "eve", goalID, p.ID)
	if !errors.Is(err, proof.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for unrelated user, got: %v", err)
	}

	// 4. Listing proofs: Eve blocked
	_, err = proofService.ListByGoalAuthorized(ctx, eveID, "eve@example.com", "eve", goalID)
	if !errors.Is(err, proof.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for unrelated user list, got: %v", err)
	}
}

