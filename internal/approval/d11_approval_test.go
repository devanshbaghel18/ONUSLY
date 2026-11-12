package approval_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/approval"
	"github.com/devanshbaghel18/ONUSLY/internal/enforcement"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
)

type d11MockGoalRepoForApproval struct {
	mu    sync.Mutex
	goals map[string]goals.Goal
}

func newD11MockGoalRepoForApproval() *d11MockGoalRepoForApproval {
	return &d11MockGoalRepoForApproval{
		goals: make(map[string]goals.Goal),
	}
}

func (r *d11MockGoalRepoForApproval) Create(ctx context.Context, g goals.Goal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.goals[g.OwnerID+"#"+g.ID] = g
	return nil
}

func (r *d11MockGoalRepoForApproval) GetByID(ctx context.Context, ownerID, goalID string) (*goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ownerID == "" {
		for _, g := range r.goals {
			if g.ID == goalID {
				return &g, nil
			}
		}
		return nil, errors.New("goal not found")
	}
	g, ok := r.goals[ownerID+"#"+goalID]
	if !ok {
		return nil, errors.New("goal not found")
	}
	return &g, nil
}

func (r *d11MockGoalRepoForApproval) FindByID(ctx context.Context, goalID string) (*goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.goals {
		if g.ID == goalID {
			return &g, nil
		}
	}
	return nil, errors.New("goal not found")
}

func (r *d11MockGoalRepoForApproval) List(ctx context.Context, ownerID string) ([]goals.Goal, error) {
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

func (r *d11MockGoalRepoForApproval) ListForApprover(ctx context.Context, approverID, approverEmail string) ([]goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []goals.Goal
	for _, g := range r.goals {
		if (approverID != "" && g.ApproverID == approverID) || (approverEmail != "" && g.ApproverEmail == approverEmail) {
			list = append(list, g)
		}
	}
	return list, nil
}

func (r *d11MockGoalRepoForApproval) ListByApprover(ctx context.Context, approverID, approverEmail, approverHandle, statusFilter string) ([]goals.Goal, error) {
	return r.ListForApprover(ctx, approverID, approverEmail)
}

func (r *d11MockGoalRepoForApproval) Delete(ctx context.Context, ownerID, goalID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.goals, ownerID+"#"+goalID)
	return nil
}

func (r *d11MockGoalRepoForApproval) Update(ctx context.Context, ownerID, goalID string, input goals.UpdateGoalInput) (*goals.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.goals[ownerID+"#"+goalID]
	if !ok {
		return nil, errors.New("goal not found")
	}
	r.goals[ownerID+"#"+goalID] = g
	return &g, nil
}

func (r *d11MockGoalRepoForApproval) UpdateStatus(ctx context.Context, ownerID, goalID, status string) (*goals.Goal, error) {
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

type d11MockProofRepoForApproval struct {
	mu       sync.Mutex
	proofs   map[string]proof.Proof
	goalRepo *d11MockGoalRepoForApproval
}

func newD11MockProofRepoForApproval(goalRepo *d11MockGoalRepoForApproval) *d11MockProofRepoForApproval {
	return &d11MockProofRepoForApproval{
		proofs:   make(map[string]proof.Proof),
		goalRepo: goalRepo,
	}
}

func (r *d11MockProofRepoForApproval) Create(ctx context.Context, p proof.Proof) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.proofs[p.GoalID+"#"+p.ID] = p
	return nil
}

func (r *d11MockProofRepoForApproval) CreateWithGoalTransition(ctx context.Context, p proof.Proof, expectedGoalStatuses []string, newGoalStatus string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.goalRepo != nil {
		g, err := r.goalRepo.GetByID(ctx, p.OwnerID, p.GoalID)
		if err != nil {
			return errors.New("goal not found")
		}
		matched := false
		for _, exp := range expectedGoalStatuses {
			if g.Status == exp {
				matched = true
				break
			}
		}
		if !matched {
			return errors.New("ConditionalCheckFailedException: goal status does not match expected")
		}
		_, _ = r.goalRepo.UpdateStatus(ctx, p.OwnerID, p.GoalID, newGoalStatus)
	}

	r.proofs[p.GoalID+"#"+p.ID] = p
	return nil
}

func (r *d11MockProofRepoForApproval) GetByID(ctx context.Context, ownerID, goalID, proofID string) (*proof.Proof, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.proofs[goalID+"#"+proofID]
	if !ok || p.OwnerID != ownerID {
		return nil, proof.ErrProofNotFound
	}
	return &p, nil
}

func (r *d11MockProofRepoForApproval) ListByGoal(ctx context.Context, ownerID, goalID string) ([]proof.Proof, error) {
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

type d11MockApprovalRepo struct {
	mu        sync.Mutex
	approvals map[string]approval.Approval
	goalRepo  *d11MockGoalRepoForApproval
}

func newD11MockApprovalRepo(goalRepo *d11MockGoalRepoForApproval) *d11MockApprovalRepo {
	return &d11MockApprovalRepo{
		approvals: make(map[string]approval.Approval),
		goalRepo:  goalRepo,
	}
}

func (r *d11MockApprovalRepo) Create(ctx context.Context, a approval.Approval) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvals[a.GoalID+"#"+a.ID] = a
	return nil
}

func (r *d11MockApprovalRepo) CreateDecision(ctx context.Context, a approval.Approval, expectedGoalStatus string, newGoalStatus string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	g, err := r.goalRepo.GetByID(ctx, a.OwnerID, a.GoalID)
	if err != nil {
		return errors.New("goal not found")
	}

	if g.Status != expectedGoalStatus {
		return errors.New("ConditionalCheckFailedException: goal status does not match expected")
	}

	_, _ = r.goalRepo.UpdateStatus(ctx, a.OwnerID, a.GoalID, newGoalStatus)
	r.approvals[a.GoalID+"#"+a.ID] = a
	return nil
}

func (r *d11MockApprovalRepo) GetByID(ctx context.Context, ownerID, goalID, approvalID string) (*approval.Approval, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.approvals[goalID+"#"+approvalID]
	if !ok || a.OwnerID != ownerID {
		return nil, errors.New("approval not found")
	}
	return &a, nil
}

func (r *d11MockApprovalRepo) ListByGoal(ctx context.Context, ownerID, goalID string) ([]approval.Approval, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []approval.Approval
	for _, a := range r.approvals {
		if a.GoalID == goalID && a.OwnerID == ownerID {
			list = append(list, a)
		}
	}
	return list, nil
}

// -------------------------------------------------------------
// D11 APPROVAL INTEGRITY & ENFORCEMENT INTEGRATION TESTS
// -------------------------------------------------------------

func setupD11TestEnvironment() (*d11MockGoalRepoForApproval, *goals.Service, *proof.Service, *approval.Service, *enforcement.Service) {
	goalRepo := newD11MockGoalRepoForApproval()
	goalService := goals.NewService(goalRepo, nil)
	proofRepo := newD11MockProofRepoForApproval(goalRepo)
	proofService := proof.NewService(proofRepo, goalService)
	approvalRepo := newD11MockApprovalRepo(goalRepo)
	approvalService := approval.NewService(approvalRepo, goalService, proofService)
	enforcementService := enforcement.NewService(goalService)

	return goalRepo, goalService, proofService, approvalService, enforcementService
}

func TestD11_Approval_Approve_TransitionsCompleted(t *testing.T) {
	ctx := context.Background()
	goalRepo, _, proofService, approvalService, _ := setupD11TestEnvironment()

	ownerID := "user-alice"
	approverID := "user-bob"
	goalID := "goal-201"

	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   approverID,
		ApprovalType: "friend",
		Title:        "Read Clean Architecture Chapters",
		Status:       "active",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	p, err := proofService.Submit(ctx, ownerID, goalID, "text", "Finished chapter 1 to 5", "", "")
	if err != nil {
		t.Fatalf("expected proof submission to succeed, got: %v", err)
	}

	// Bob approves
	app, err := approvalService.Decide(ctx, approverID, ownerID, goalID, p.ID, "approved", "Great work!")
	if err != nil {
		t.Fatalf("expected approval to succeed, got: %v", err)
	}

	if app.Status != "approved" {
		t.Fatalf("expected decision status 'approved', got: %s", app.Status)
	}

	// Goal must be completed
	g, err := goalRepo.GetByID(ctx, ownerID, goalID)
	if err != nil || g.Status != "completed" {
		t.Fatalf("expected goal status 'completed', got: %s", g.Status)
	}
}

func TestD11_Approval_Reject_TransitionsRejected(t *testing.T) {
	ctx := context.Background()
	goalRepo, _, proofService, approvalService, _ := setupD11TestEnvironment()

	ownerID := "user-alice"
	approverID := "user-bob"
	goalID := "goal-202"

	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   approverID,
		ApprovalType: "friend",
		Title:        "Workout Session",
		Status:       "active",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	p, err := proofService.Submit(ctx, ownerID, goalID, "text", "Went to gym", "", "")
	if err != nil {
		t.Fatalf("expected proof submission to succeed, got: %v", err)
	}

	// Bob rejects with reason
	app, err := approvalService.Decide(ctx, approverID, ownerID, goalID, p.ID, "rejected", "Need screenshot of heart-rate log")
	if err != nil {
		t.Fatalf("expected rejection to succeed, got: %v", err)
	}

	if app.Status != "rejected" {
		t.Fatalf("expected decision status 'rejected', got: %s", app.Status)
	}

	// Goal must be transitioned to 'rejected'
	g, err := goalRepo.GetByID(ctx, ownerID, goalID)
	if err != nil || g.Status != "rejected" {
		t.Fatalf("expected goal status 'rejected', got: %s", g.Status)
	}
}

func TestD11_Approval_SelfApproval_Forbidden(t *testing.T) {
	ctx := context.Background()
	goalRepo, _, proofService, approvalService, _ := setupD11TestEnvironment()

	ownerID := "user-alice"
	goalID := "goal-203"

	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   ownerID, // Self-configured attempt
		ApprovalType: "friend",
		Title:        "Self-Check",
		Status:       "active",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	p, err := proofService.Submit(ctx, ownerID, goalID, "text", "Self evidence", "", "")
	if err != nil {
		t.Fatalf("expected proof submission to succeed, got: %v", err)
	}

	// Alice tries to approve her own goal
	_, err = approvalService.Decide(ctx, ownerID, ownerID, goalID, p.ID, "approved", "Approving myself")
	if !errors.Is(err, approval.ErrSelfApproval) {
		t.Fatalf("expected ErrSelfApproval when owner approves own goal, got: %v", err)
	}
}

func TestD11_Approval_UnauthorizedApprover_Forbidden(t *testing.T) {
	ctx := context.Background()
	goalRepo, _, proofService, approvalService, _ := setupD11TestEnvironment()

	ownerID := "user-alice"
	approverID := "user-bob"
	eveID := "user-eve"
	goalID := "goal-204"

	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   approverID,
		ApprovalType: "friend",
		Title:        "Study German",
		Status:       "active",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	p, err := proofService.Submit(ctx, ownerID, goalID, "text", "Duolingo lesson", "", "")
	if err != nil {
		t.Fatalf("expected proof submission to succeed, got: %v", err)
	}

	// Eve tries to approve
	_, err = approvalService.Decide(ctx, eveID, ownerID, goalID, p.ID, "approved", "Eve approving")
	if !errors.Is(err, approval.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for unauthorized approver, got: %v", err)
	}
}

func TestD11_Approval_AlreadyDecided_Rejection(t *testing.T) {
	ctx := context.Background()
	goalRepo, _, proofService, approvalService, _ := setupD11TestEnvironment()

	ownerID := "user-alice"
	approverID := "user-bob"
	goalID := "goal-205"

	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   approverID,
		ApprovalType: "friend",
		Title:        "Project Alpha",
		Status:       "active",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	p, err := proofService.Submit(ctx, ownerID, goalID, "text", "Evidence", "", "")
	if err != nil {
		t.Fatalf("expected proof submission to succeed, got: %v", err)
	}

	// First decision succeeds
	_, err = approvalService.Decide(ctx, approverID, ownerID, goalID, p.ID, "approved", "First approval")
	if err != nil {
		t.Fatalf("first approval failed: %v", err)
	}

	// Second decision must fail with ErrAlreadyDecided because status is now "completed"
	_, err = approvalService.Decide(ctx, approverID, ownerID, goalID, p.ID, "approved", "Second approval")
	if !errors.Is(err, approval.ErrAlreadyDecided) {
		t.Fatalf("expected ErrAlreadyDecided, got: %v", err)
	}
}

func TestD11_Approval_ConcurrentDecisions_RaceSafe(t *testing.T) {
	ctx := context.Background()
	goalRepo, _, proofService, approvalService, _ := setupD11TestEnvironment()

	ownerID := "user-alice"
	approverID := "user-bob"
	goalID := "goal-206"

	_ = goalRepo.Create(ctx, goals.Goal{
		ID:           goalID,
		OwnerID:      ownerID,
		ApproverID:   approverID,
		ApprovalType: "friend",
		Title:        "Concurrent Race Milestone",
		Status:       "active",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})

	p, err := proofService.Submit(ctx, ownerID, goalID, "text", "Race evidence", "", "")
	if err != nil {
		t.Fatalf("expected proof submission to succeed, got: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)

	// Approver 1 tries to approve
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := approvalService.Decide(ctx, approverID, ownerID, goalID, p.ID, "approved", "First click")
		results <- err
	}()

	// Approver 2 (or rapid second click) tries to reject concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := approvalService.Decide(ctx, approverID, ownerID, goalID, p.ID, "rejected", "Second click")
		results <- err
	}()

	wg.Wait()
	close(results)

	var successes, failures int
	for err := range results {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}

	if successes != 1 || failures != 1 {
		t.Fatalf("expected exactly 1 success and 1 failure under concurrent decisions, got successes: %d, failures: %d", successes, failures)
	}
}

func TestD11_Approval_EnforcementLifecycleContract(t *testing.T) {
	ctx := context.Background()
	goalRepo, _, proofService, approvalService, enforcementService := setupD11TestEnvironment()

	ownerID := "user-alice"
	approverID := "user-bob"
	goalID := "goal-207"

	// 1. Goal Created (Active) -> Must be INCLUDED in blocklist
	_ = goalRepo.Create(ctx, goals.Goal{
		ID:             goalID,
		OwnerID:        ownerID,
		ApproverID:     approverID,
		ApprovalType:   "friend",
		Title:          "Deep Work Sprint",
		Status:         "active",
		BlockedDomains: []string{"youtube.com", "reddit.com"},
		BlockedApps:    []string{"steam.exe"},
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	})

	policy1, err := enforcementService.GetBlocklist(ctx, ownerID)
	if err != nil || len(policy1.Targets) != 1 {
		t.Fatalf("step 1 (active): expected 1 target in blocklist, got: %d (err: %v)", len(policy1.Targets), err)
	}

	// 2. Proof Submitted -> Must remain INCLUDED in blocklist (not released yet)
	p1, err := proofService.Submit(ctx, ownerID, goalID, "text", "Completed session", "", "")
	if err != nil {
		t.Fatalf("proof submit failed: %v", err)
	}

	policy2, err := enforcementService.GetBlocklist(ctx, ownerID)
	if err != nil || len(policy2.Targets) != 1 {
		t.Fatalf("step 2 (proof_submitted): expected target to remain in blocklist, got: %d", len(policy2.Targets))
	}

	// 3. Rejected -> Must remain INCLUDED in blocklist (restrictions preserved!)
	_, err = approvalService.Decide(ctx, approverID, ownerID, goalID, p1.ID, "rejected", "Not enough hours")
	if err != nil {
		t.Fatalf("rejection failed: %v", err)
	}

	policy3, err := enforcementService.GetBlocklist(ctx, ownerID)
	if err != nil || len(policy3.Targets) != 1 {
		t.Fatalf("step 3 (rejected): expected target to remain in blocklist upon rejection, got: %d", len(policy3.Targets))
	}
	if policy3.Targets[0].Status != "rejected" {
		t.Fatalf("expected target status 'rejected', got: %s", policy3.Targets[0].Status)
	}

	// 4. Resubmitted from rejected state -> Still in blocklist
	p2, err := proofService.Submit(ctx, ownerID, goalID, "text", "Revised proof with additional hours", "", "")
	if err != nil {
		t.Fatalf("proof resubmission from rejected state failed: %v", err)
	}

	policy4, _ := enforcementService.GetBlocklist(ctx, ownerID)
	if len(policy4.Targets) != 1 {
		t.Fatalf("step 4 (resubmitted): expected target in blocklist, got: %d", len(policy4.Targets))
	}

	// 5. Approved (Completed) -> Immediately EXCLUDED from blocklist!
	_, err = approvalService.Decide(ctx, approverID, ownerID, goalID, p2.ID, "approved", "Great revised proof!")
	if err != nil {
		t.Fatalf("approval failed: %v", err)
	}

	policy5, err := enforcementService.GetBlocklist(ctx, ownerID)
	if err != nil {
		t.Fatalf("fetch blocklist failed: %v", err)
	}
	if len(policy5.Targets) != 0 {
		t.Fatalf("step 5 (completed): expected goal to be EXCLUDED from blocklist upon approval, found %d targets", len(policy5.Targets))
	}
}
