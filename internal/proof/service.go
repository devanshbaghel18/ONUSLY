package proof

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/google/uuid"
)

var (
	ErrInvalidProofType      = errors.New("invalid proof type")
	ErrInvalidProof          = errors.New("invalid proof")
	ErrProofNotFound         = errors.New("proof not found")
	ErrGoalNotFound          = errors.New("goal not found")
	ErrProofAlreadySubmitted = errors.New("proof has already been submitted for this goal")
	ErrGoalNotActive         = errors.New("goal is not active")
)

type Service struct {
	repo        *Repository
	goalService *goals.Service
}

func NewService(repo *Repository, goalService *goals.Service) *Service {
	return &Service{
		repo:        repo,
		goalService: goalService,
	}
}

func (s *Service) Submit(
	ctx context.Context,
	ownerID string,
	goalID string,
	proofType string,
	textExplanation string,
	externalLink string,
	photoURL string,
) (*Proof, error) {

	if strings.TrimSpace(ownerID) == "" {
		return nil, ErrInvalidProof
	}

	if strings.TrimSpace(goalID) == "" {
		return nil, ErrInvalidProof
	}

	proofType = strings.TrimSpace(proofType)

	switch proofType {
	case "text", "link", "photo":
	default:
		return nil, ErrInvalidProofType
	}

	switch proofType {
	case "text":
		if strings.TrimSpace(textExplanation) == "" {
			return nil, ErrInvalidProof
		}
	case "link":
		if strings.TrimSpace(externalLink) == "" {
			return nil, ErrInvalidProof
		}
	case "photo":
		if strings.TrimSpace(photoURL) == "" {
			return nil, ErrInvalidProof
		}
	}

	// Verify that the goal belongs to the authenticated user and is in "active" state.
	goal, err := s.goalService.GetByID(ctx, ownerID, goalID)
	if err != nil {
		return nil, ErrGoalNotFound
	}

	if goal.Status == "proof_submitted" {
		return nil, ErrProofAlreadySubmitted
	}

	if goal.Status != "active" {
		return nil, ErrGoalNotActive
	}

	proofID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	p := Proof{
		PK:              "USER#" + ownerID,
		SK:              "PROOF#" + goalID + "#" + proofID,
		ID:              proofID,
		GoalID:          goalID,
		OwnerID:         ownerID,
		ProofType:       proofType,
		TextExplanation: strings.TrimSpace(textExplanation),
		ExternalLink:    strings.TrimSpace(externalLink),
		PhotoURL:        strings.TrimSpace(photoURL),
		SubmittedAt:     now,
	}

	if err := s.repo.CreateWithGoalTransition(
		ctx,
		p,
		"active",
		"proof_submitted",
	); err != nil {
		return nil, err
	}

	return &p, nil
}

func (s *Service) GetByID(
	ctx context.Context,
	ownerID string,
	goalID string,
	proofID string,
) (*Proof, error) {

	p, err := s.repo.GetByID(ctx, ownerID, goalID, proofID)
	if err != nil {
		return nil, err
	}

	if p == nil {
		return nil, ErrProofNotFound
	}

	return p, nil
}

func (s *Service) ListByGoal(
	ctx context.Context,
	ownerID string,
	goalID string,
) ([]Proof, error) {
	if ownerID == "" {
		g, err := s.goalService.GetByID(ctx, "", goalID)
		if err == nil && g != nil {
			ownerID = g.OwnerID
		}
	}

	proofs, err := s.repo.ListByGoal(ctx, ownerID, goalID)
	if (err != nil || len(proofs) == 0) && goalID != "" {
		g, gErr := s.goalService.GetByID(ctx, "", goalID)
		if gErr == nil && g != nil && g.OwnerID != ownerID {
			return s.repo.ListByGoal(ctx, g.OwnerID, goalID)
		}
	}

	return proofs, err
}
