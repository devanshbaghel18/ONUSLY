package approval

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
	"github.com/google/uuid"
)

var (
	ErrInvalidDecision = errors.New("invalid approval decision")
	ErrUnauthorized    = errors.New("user is not the assigned approver")
	ErrInvalidProof    = errors.New("invalid proof")
	ErrAlreadyDecided  = errors.New("proof has already been decided")
)

type Service struct {
	repo         *Repository
	goalService  *goals.Service
	proofService *proof.Service
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

	goal, err := s.goalService.GetByID(ctx, ownerID, goalID)
	if err != nil {
		return nil, err
	}

	if goal.ApproverID != approverID {
		return nil, ErrUnauthorized
	}

	if goal.Status != "proof_submitted" {
		return nil, ErrAlreadyDecided
	}

	p, err := s.proofService.GetByID(
		ctx,
		ownerID,
		goalID,
		proofID,
	)
	if err != nil {
		return nil, ErrInvalidProof
	}

	if p.GoalID != goalID || p.OwnerID != ownerID {
		return nil, ErrInvalidProof
	}

	approvalID := uuid.NewString()

	approval := Approval{
		PK:         "USER#" + ownerID,
		SK:         "APPROVAL#" + goalID + "#" + approvalID,
		ID:         approvalID,
		GoalID:     goalID,
		ProofID:    proofID,
		OwnerID:    ownerID,
		ApproverID: approverID,
		Status:     status,
		Comment:    comment,
		DecidedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.repo.Create(ctx, approval); err != nil {
		return nil, err
	}

	goalStatus := "active"

	if status == "approved" {
		goalStatus = "completed"
	}

	if _, err := s.goalService.UpdateStatus(
		ctx,
		ownerID,
		goalID,
		goalStatus,
	); err != nil {
		return nil, err
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
