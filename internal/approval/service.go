package approval

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
	"github.com/google/uuid"
)

var (
	ErrInvalidDecision = errors.New("invalid approval decision")
	ErrUnauthorized    = errors.New("user is not the assigned approver")
	ErrSelfApproval    = errors.New("goal owner cannot approve their own goal")
	ErrGoalNotFound    = errors.New("goal not found")
	ErrProofNotFound   = errors.New("proof not found")
	ErrInvalidProof    = errors.New("invalid proof")
	ErrAlreadyDecided  = errors.New("proof has already been decided")
)

type Service struct {
	repo         *Repository
	goalService  *goals.Service
	proofService *proof.Service
	listeners    []DecisionListener
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

	if approverID == "" || ownerID == "" || goalID == "" || proofID == "" {
		return nil, errors.New("invalid approval request")
	}

	if status != "approved" && status != "rejected" {
		return nil, ErrInvalidDecision
	}

	if len(comment) > 2000 {
		return nil, errors.New("comment exceeds maximum allowed length of 2000 characters")
	}

	goal, err := s.goalService.GetByID(ctx, ownerID, goalID)
	if err != nil {
		return nil, ErrGoalNotFound
	}

	// 1. Goal owner must NEVER be able to approve their own goal.
	if goal.OwnerID == approverID {
		return nil, ErrSelfApproval
	}

	// 2. Goal must have a valid configured approver and caller must match.
	callerEmail, _ := middleware.GetUserEmail(ctx)
	callerHandle, _ := middleware.GetUserHandle(ctx)
	isAuthorized := (goal.ApproverID == "COMMUNITY") ||
		(goal.ApprovalType == "community") ||
		(goal.ApproverID == "" && goal.ApproverEmail == "") ||
		(goal.ApproverID != "" && goal.ApproverID == approverID) ||
		(goal.ApproverEmail != "" && callerEmail != "" && strings.EqualFold(goal.ApproverEmail, callerEmail)) ||
		(goal.ApproverEmail != "" && callerHandle != "" && (strings.EqualFold(goal.ApproverEmail, "@"+callerHandle) || strings.EqualFold(goal.ApproverEmail, callerHandle)))

	if !isAuthorized {
		return nil, ErrUnauthorized
	}

	// 3. Goal must belong to the specified owner.
	if goal.OwnerID != ownerID {
		return nil, ErrUnauthorized
	}

	// 4. Goal must currently be in proof_submitted or active state.
	if goal.Status != "proof_submitted" && goal.Status != "active" {
		return nil, ErrAlreadyDecided
	}

	// 5. Proof check if provided and not direct chat approval
	if proofID != "" && proofID != "direct-approval" && proofID != "chat-direct" {
		p, err := s.proofService.GetByID(
			ctx,
			ownerID,
			goalID,
			proofID,
		)
		if err != nil {
			if !errors.Is(err, proof.ErrProofNotFound) {
				return nil, ErrInvalidProof
			}
		} else if p != nil {
			// 6. Proof must belong to this specific goal and legitimate goal owner.
			if p.GoalID != goalID || p.OwnerID != goal.OwnerID || p.OwnerID != ownerID {
				return nil, ErrInvalidProof
			}
		}
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
		goal.Status,
		goalStatus,
	); err != nil {
		return nil, err
	}

	for _, listener := range s.listeners {
		listener.OnDecision(ctx, &approval, goal)
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
