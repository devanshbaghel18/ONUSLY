package approval

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/events"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
	"github.com/google/uuid"
)

var (
	ErrInvalidDecision  = errors.New("invalid approval decision")
	ErrUnauthorized     = errors.New("user is not the assigned approver")
	ErrSelfApproval     = errors.New("goal owner cannot approve their own goal")
	ErrGoalNotFound     = errors.New("goal not found")
	ErrProofNotFound    = errors.New("proof not found")
	ErrInvalidProof     = errors.New("invalid proof")
	ErrAlreadyDecided   = errors.New("proof has already been decided")
	ErrApprovalNotFound = errors.New("approval not found")
)

type Service struct {
	repo         *Repository
	goalService  *goals.Service
	proofService *proof.Service
	listeners    []DecisionListener
	publisher    events.Publisher
}

func NewService(
	repo *Repository,
	goalService *goals.Service,
	proofService *proof.Service,
) *Service {
	return &Service{
		repo:         repo,
		goalService:  goalService,
		proofService: proofService,
		publisher:    &events.NoopPublisher{},
	}
}

// SetPublisher assigns an events.Publisher for post-commit event dispatch.
func (s *Service) SetPublisher(p events.Publisher) {
	if p != nil {
		s.publisher = p
	}
}

// AddDecisionListener registers a listener to be notified after a decision is committed.
func (s *Service) AddDecisionListener(l DecisionListener) {
	if l != nil {
		s.listeners = append(s.listeners, l)
	}
}

func (s *Service) Decide(
	ctx context.Context,
	approverID string,
	ownerID string,
	goalID string,
	proofID string,
	status string,
	comment string,
) (*Approval, error) {

	approverID = strings.TrimSpace(approverID)
	ownerID = strings.TrimSpace(ownerID)
	goalID = strings.TrimSpace(goalID)
	proofID = strings.TrimSpace(proofID)
	status = strings.TrimSpace(status)
	comment = strings.TrimSpace(comment)

	if approverID == "" || goalID == "" || proofID == "" {
		return nil, errors.New("invalid approval request")
	}

	if status != "approved" && status != "rejected" {
		return nil, ErrInvalidDecision
	}

	if len(comment) > 2000 {
		return nil, errors.New("comment exceeds maximum allowed length of 2000 characters")
	}

	goal, err := s.goalService.GetByID(ctx, ownerID, goalID)
	if err != nil || goal == nil {
		goal, err = s.goalService.GetByID(ctx, "", goalID)
		if err != nil || goal == nil {
			return nil, ErrGoalNotFound
		}
	}

	// 1. Goal owner must NEVER be able to approve their own goal.
	if goal.OwnerID == approverID {
		return nil, ErrSelfApproval
	}

	// 2. Goal must have a valid configured approver and caller must match.
	callerEmail, _ := middleware.GetUserEmail(ctx)
	callerHandle, _ := middleware.GetUserHandle(ctx)
	if !goal.IsApprover(approverID, callerEmail, callerHandle) {
		return nil, ErrUnauthorized
	}

	// 3. Goal must belong to the specified owner (if provided by caller, must not be forged).
	if ownerID != "" && goal.OwnerID != ownerID {
		return nil, ErrUnauthorized
	}
	ownerID = goal.OwnerID

	// 4. Goal must currently be in proof_submitted state.
	if goal.Status != "proof_submitted" {
		return nil, ErrAlreadyDecided
	}

	// 5. Proof must exist.
	p, err := s.proofService.GetByID(
		ctx,
		ownerID,
		goalID,
		proofID,
	)
	if err != nil {
		if errors.Is(err, proof.ErrProofNotFound) {
			return nil, ErrProofNotFound
		}
		return nil, ErrInvalidProof
	}

	// 6. Proof must belong to this specific goal and legitimate goal owner.
	if p.GoalID != goalID || p.OwnerID != goal.OwnerID || p.OwnerID != ownerID {
		return nil, ErrInvalidProof
	}

	approvalID := uuid.NewString()

	approval := Approval{
		PK:         "USER#" + goal.OwnerID,
		SK:         "APPROVAL#" + goalID + "#" + approvalID,
		ID:         approvalID,
		GoalID:     goalID,
		ProofID:    proofID,
		OwnerID:    goal.OwnerID,
		ApproverID: approverID,
		Status:     status,
		Comment:    comment,
		DecidedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	goalStatus := "active"
	if status == "approved" {
		goalStatus = "completed"
	}

	if err := s.repo.CreateDecision(
		ctx,
		approval,
		"proof_submitted",
		goalStatus,
	); err != nil {
		return nil, err
	}

	for _, listener := range s.listeners {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Approval] Recovered from listener panic: %v", r)
				}
			}()
			listener.OnDecision(ctx, &approval, goal)
		}()
	}

	if s.publisher != nil {
		decidedTime, _ := time.Parse(time.RFC3339, approval.DecidedAt)
		if decidedTime.IsZero() {
			decidedTime = time.Now().UTC()
		}
		event := events.ApprovalDecidedEvent{
			EventID:    "approval-" + approval.ID,
			GoalID:     approval.GoalID,
			ProofID:    approval.ProofID,
			OwnerID:    approval.OwnerID,
			ApproverID: approval.ApproverID,
			Status:     approval.Status,
			Title:      goal.Title,
			DecidedAt:  decidedTime,
		}
		_ = s.publisher.PublishApprovalDecided(ctx, event)
	}

	return &approval, nil
}

func (s *Service) GetByID(
	ctx context.Context,
	ownerID string,
	goalID string,
	approvalID string,
) (*Approval, error) {

	approval, err := s.repo.GetByID(
		ctx,
		ownerID,
		goalID,
		approvalID,
	)

	if err != nil {
		return nil, err
	}

	if approval == nil {
		return nil, errors.New("approval not found")
	}

	return approval, nil
}

// ListByGoalAuthorized validates that caller is goal owner or assigned approver before listing approvals.
func (s *Service) ListByGoalAuthorized(
	ctx context.Context,
	callerID, callerEmail, callerHandle, goalID string,
) ([]Approval, error) {
	goal, err := s.goalService.GetByID(ctx, "", goalID)
	if err != nil || goal == nil {
		return nil, ErrGoalNotFound
	}

	if !goal.CanAccess(callerID, callerEmail, callerHandle) {
		return nil, ErrUnauthorized
	}

	return s.repo.ListByGoal(ctx, goal.OwnerID, goalID)
}

// GetByIDAuthorized validates that caller is goal owner or assigned approver before returning approval details.
func (s *Service) GetByIDAuthorized(
	ctx context.Context,
	callerID, callerEmail, callerHandle, goalID, approvalID string,
) (*Approval, error) {
	goal, err := s.goalService.GetByID(ctx, "", goalID)
	if err != nil || goal == nil {
		return nil, ErrGoalNotFound
	}

	if !goal.CanAccess(callerID, callerEmail, callerHandle) {
		return nil, ErrUnauthorized
	}

	app, err := s.repo.GetByID(ctx, goal.OwnerID, goalID, approvalID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, ErrApprovalNotFound
	}
	if app.GoalID != goalID || app.OwnerID != goal.OwnerID {
		return nil, ErrApprovalNotFound
	}

	return app, nil
}

func (s *Service) ListByGoal(
	ctx context.Context,
	ownerID string,
	goalID string,
) ([]Approval, error) {

	return s.repo.ListByGoal(
		ctx,
		ownerID,
		goalID,
	)
}
