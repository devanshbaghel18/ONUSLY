package proof

import (
	"context"
	"errors"
	"net/url"
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

type ProofRepository interface {
	Create(ctx context.Context, p Proof) error
	CreateWithGoalTransition(ctx context.Context, p Proof, expectedGoalStatuses []string, newGoalStatus string) error
	GetByID(ctx context.Context, ownerID, goalID, proofID string) (*Proof, error)
	ListByGoal(ctx context.Context, ownerID, goalID string) ([]Proof, error)
}

type Service struct {
	repo        ProofRepository
	goalService *goals.Service
	publisher   events.Publisher
}

func NewService(repo ProofRepository, goalService *goals.Service) *Service {
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
	return s.SubmitWithFiles(ctx, ownerID, goalID, proofType, textExplanation, externalLink, photoURL, nil)
}

func (s *Service) SubmitWithFiles(
	ctx context.Context,
	ownerID string,
	goalID string,
	proofType string,
	textExplanation string,
	externalLink string,
	photoURL string,
	files interface{},
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
		if len(externalLink) > 2048 {
			return nil, errors.New("external link exceeds maximum length of 2048 characters")
		}
		parsedURL, err := url.ParseRequestURI(externalLink)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
			return nil, errors.New("external link must be a valid http:// or https:// URL")
		}
	case "photo", "document":
		if photoURL == "" && textExplanation == "" {
			return nil, ErrInvalidProof
		}
		if photoURL != "" {
			lower := strings.ToLower(photoURL)
			if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
				if len(photoURL) > 2048 {
					return nil, errors.New("photo URL exceeds maximum length of 2048 characters")
				}
				parsedURL, err := url.ParseRequestURI(photoURL)
				if err != nil || parsedURL.Host == "" {
					return nil, errors.New("photo URL must be a valid http(s) URL")
				}
			} else if strings.HasPrefix(lower, "data:image/") || strings.HasPrefix(lower, "data:application/pdf") {
				// Base64 data URI validation
				if len(photoURL) > 2*1024*1024 {
					return nil, errors.New("photo data URI exceeds maximum size limit of 2MB")
				}
				validMIMEPrefixes := []string{
					"data:image/jpeg;base64,",
					"data:image/jpg;base64,",
					"data:image/png;base64,",
					"data:image/webp;base64,",
					"data:image/gif;base64,",
					"data:application/pdf;base64,",
				}
				hasValidPrefix := false
				for _, prefix := range validMIMEPrefixes {
					if strings.HasPrefix(lower, prefix) {
						hasValidPrefix = true
						break
					}
				}
				if !hasValidPrefix {
					return nil, errors.New("photo data URI must have a valid MIME prefix (jpeg, png, webp, gif, pdf)")
				}
			} else {
				return nil, errors.New("photo URL must be an http(s) URL or valid image data URI")
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

	if goal.Status != "active" && goal.Status != "rejected" {
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
		Files:           files,
		SubmittedAt:     now,
	}

	if err := s.repo.CreateWithGoalTransition(
		ctx,
		p,
		[]string{"active", "rejected"},
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
			Files:       files,
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
