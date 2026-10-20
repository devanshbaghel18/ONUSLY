package proof

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/events"
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
	ErrUnauthorized          = errors.New("forbidden: you are not authorized to access proofs for this goal")
)

type Service struct {
	repo        *Repository
	goalService *goals.Service
	publisher   events.Publisher
}

func NewService(repo *Repository, goalService *goals.Service) *Service {
	return &Service{
		repo:        repo,
		goalService: goalService,
		publisher:   &events.NoopPublisher{},
	}
}

// SetPublisher assigns an events.Publisher for post-commit event dispatch.
func (s *Service) SetPublisher(p events.Publisher) {
	if p != nil {
		s.publisher = p
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
	case "text", "link", "photo", "document":
	default:
		return nil, ErrInvalidProofType
	}

	textExplanation = strings.TrimSpace(textExplanation)
	externalLink = strings.TrimSpace(externalLink)
	photoURL = strings.TrimSpace(photoURL)

	if len(textExplanation) > 5000 {
		return nil, errors.New("proof explanation exceeds maximum length of 5000 characters")
	}

	switch proofType {
	case "text":
		if textExplanation == "" {
			return nil, ErrInvalidProof
		}
	case "link":
		if externalLink == "" {
			return nil, ErrInvalidProof
		}
		lower := strings.ToLower(externalLink)
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return nil, errors.New("external link must start with http:// or https://")
		}
	case "photo", "document":
		if photoURL == "" && textExplanation == "" {
			return nil, ErrInvalidProof
		}
		if photoURL != "" {
			lower := strings.ToLower(photoURL)
			if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "data:image/") {
				return nil, errors.New("photo URL must be an http(s) URL or image data URI")
			}
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

	if s.publisher != nil {
		submittedTime, _ := time.Parse(time.RFC3339, now)
		if submittedTime.IsZero() {
			submittedTime = time.Now().UTC()
		}
		event := events.ProofSubmittedEvent{
			EventID:     "proof-" + p.ID,
			GoalID:      p.GoalID,
			ProofID:     p.ID,
			OwnerID:     p.OwnerID,
			ApproverID:  goal.ApproverID,
			Title:       goal.Title,
			SubmittedAt: submittedTime,
		}
		_ = s.publisher.PublishProofSubmitted(ctx, event)
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

// GetByIDAuthorized enforces that only the goal owner or designated approver can view a proof.
func (s *Service) GetByIDAuthorized(
	ctx context.Context,
	callerID string,
	callerEmail string,
	callerHandle string,
	goalID string,
	proofID string,
) (*Proof, error) {
	goal, err := s.goalService.GetByID(ctx, "", goalID)
	if err != nil || goal == nil {
		return nil, ErrGoalNotFound
	}

	if !goal.CanAccess(callerID, callerEmail, callerHandle) {
		return nil, ErrUnauthorized
	}

	p, err := s.repo.GetByID(ctx, goal.OwnerID, goalID, proofID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrProofNotFound
	}
	if p.GoalID != goalID || p.OwnerID != goal.OwnerID {
		return nil, ErrProofNotFound
	}

	return p, nil
}

// ListByGoalAuthorized enforces that only the goal owner or designated approver can list proofs.
func (s *Service) ListByGoalAuthorized(
	ctx context.Context,
	callerID string,
	callerEmail string,
	callerHandle string,
	goalID string,
) ([]Proof, error) {
	goal, err := s.goalService.GetByID(ctx, "", goalID)
	if err != nil || goal == nil {
		return nil, ErrGoalNotFound
	}

	if !goal.CanAccess(callerID, callerEmail, callerHandle) {
		return nil, ErrUnauthorized
	}

	return s.repo.ListByGoal(ctx, goal.OwnerID, goalID)
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
