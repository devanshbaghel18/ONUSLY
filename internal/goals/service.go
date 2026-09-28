package goals

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{
		repo: repo,
	}
}

func (s *Service) Create(
	ctx context.Context,
	ownerID string,
	title string,
	description string,
) (*Goal, error) {

	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)

	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	if title == "" {
		return nil, errors.New("title is required")
	}

	goalID := uuid.NewString()

	goal := Goal{
		PK:          "USER#" + ownerID,
		SK:          "GOAL#" + goalID,
		ID:          goalID,
		OwnerID:     ownerID,
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
	status string,
) (*Goal, error) {

	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	if goalID == "" {
		return nil, errors.New("goal ID is required")
	}

	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	status = strings.TrimSpace(status)

	if title == "" {
		return nil, errors.New("title is required")
	}

	if status == "" {
		return nil, errors.New("status is required")
	}

	return s.repo.Update(
		ctx,
		ownerID,
		goalID,
		title,
		description,
		status,
	)
}
