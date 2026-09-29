package goals

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/auth"
	"github.com/google/uuid"
)

type Service struct {
	repo     *Repository
	authRepo auth.Repository
}

func NewService(repo *Repository, authRepo auth.Repository) *Service {
	return &Service{
		repo:     repo,
		authRepo: authRepo,
	}
}
func (s *Service) Create(
	ctx context.Context,
	ownerID string,
	title string,
	description string,
	approverEmail string,
) (*Goal, error) {

	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	approverEmail = strings.TrimSpace(approverEmail)

	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	if title == "" {
		return nil, errors.New("title is required")
	}

	if approverEmail == "" {
		return nil, errors.New("approver email is required")
	}

	approver, err := s.authRepo.GetUserByEmail(ctx, approverEmail)
	if err != nil {
		return nil, errors.New("failed to find approver")
	}

	if approver == nil {
		return nil, errors.New("approver not found")
	}

	if approver.ID == ownerID {
		return nil, errors.New("you cannot be your own approver")
	}

	goalID := uuid.NewString()

	goal := Goal{
		PK:          "USER#" + ownerID,
		SK:          "GOAL#" + goalID,
		ID:          goalID,
		OwnerID:     ownerID,
		ApproverID:  approver.ID,
		Title:       title,
		Description: description,
		Status:      "active",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.repo.Create(ctx, goal); err != nil {
		return nil, err
	}

	return &goal, nil
}
func (s *Service) GetByID(
	ctx context.Context,
	ownerID string,
	goalID string,
) (*Goal, error) {

	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	if goalID == "" {
		return nil, errors.New("goal ID is required")
	}

	return s.repo.GetByID(ctx, ownerID, goalID)
}

func (s *Service) List(
	ctx context.Context,
	ownerID string,
) ([]Goal, error) {

	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	return s.repo.List(ctx, ownerID)
}

func (s *Service) Delete(
	ctx context.Context,
	ownerID string,
	goalID string,
) error {

	if ownerID == "" {
		return errors.New("owner ID is required")
	}

	if goalID == "" {
		return errors.New("goal ID is required")
	}

	return s.repo.Delete(ctx, ownerID, goalID)
}

func (s *Service) Update(
	ctx context.Context,
	ownerID string,
	goalID string,
	title string,
	description string,
) (*Goal, error) {

	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	if goalID == "" {
		return nil, errors.New("goal ID is required")
	}

	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)

	if title == "" {
		return nil, errors.New("title is required")
	}

	return s.repo.Update(
		ctx,
		ownerID,
		goalID,
		title,
		description,
	)
}
func (s *Service) UpdateStatus(
	ctx context.Context,
	ownerID string,
	goalID string,
	status string,
) (*Goal, error) {

	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	if goalID == "" {
		return nil, errors.New("goal ID is required")
	}

	status = strings.TrimSpace(status)

	if status == "" {
		return nil, errors.New("status is required")
	}

	return s.repo.UpdateStatus(
		ctx,
		ownerID,
		goalID,
		status,
	)
}
