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
		ApproverID:  "",
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
	if goalID == "" {
		return nil, errors.New("goal ID is required")
	}

	if ownerID != "" {
		goal, err := s.repo.GetByID(ctx, ownerID, goalID)
		if err == nil && goal != nil {
			return goal, nil
		}
	}

	return s.repo.FindByID(ctx, goalID)
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
	input UpdateGoalInput,
) (*Goal, error) {

	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	if goalID == "" {
		return nil, errors.New("goal ID is required")
	}

	// Verify goal exists
	goal, err := s.repo.GetByID(ctx, ownerID, goalID)
	if err != nil {
		return nil, err
	}

	if (input.ApprovalType != nil || input.ApproverEmail != nil) && goal.Status != "active" {
		return nil, errors.New("cannot modify accountability settings on a goal that is not active")
	}

	if input.Title != nil {
		trimmed := strings.TrimSpace(*input.Title)
		if trimmed == "" {
			return nil, errors.New("title is required")
		}
		input.Title = &trimmed
	}

	if input.Description != nil {
		trimmed := strings.TrimSpace(*input.Description)
		input.Description = &trimmed
	}

	if input.ApprovalType != nil {
		appType := strings.ToLower(strings.TrimSpace(*input.ApprovalType))
		if appType != "" && appType != "community" && appType != "friend" && appType != "none" {
			return nil, errors.New("approval type must be 'community', 'friend', or 'none'")
		}
		if appType == "none" {
			appType = ""
		}
		input.ApprovalType = &appType

		if appType == "community" {
			comm := "COMMUNITY"
			empty := ""
			input.ApproverID = &comm
			input.ApproverEmail = &empty
		} else if appType == "friend" {
			if input.ApproverEmail != nil {
				inputVal := strings.TrimSpace(*input.ApproverEmail)
				if inputVal != "" {
					if s.authRepo != nil {
						var friendUser *auth.User
						var err error

						cleanedHandle := strings.TrimPrefix(inputVal, "@")
						if strings.HasPrefix(inputVal, "@") || !strings.Contains(inputVal, ".") {
							friendUser, err = s.authRepo.GetUserByHandle(ctx, cleanedHandle)
						} else {
							friendUser, err = s.authRepo.GetUserByEmail(ctx, inputVal)
							if err == nil && friendUser == nil {
								friendUser, err = s.authRepo.GetUserByHandle(ctx, cleanedHandle)
							}
						}

						if err == nil && friendUser != nil {
							if friendUser.ID == ownerID {
								return nil, errors.New("you cannot assign yourself as your accountability partner")
							}
							input.ApproverID = &friendUser.ID

							// Privacy Protection: Store and display @handle instead of raw Gmail address
							displayTag := friendUser.Handle
							if displayTag != "" {
								if !strings.HasPrefix(displayTag, "@") {
									displayTag = "@" + displayTag
								}
								input.ApproverEmail = &displayTag
							} else {
								low := strings.ToLower(friendUser.Email)
								input.ApproverEmail = &low
							}
						} else {
							return nil, errors.New("accountability partner not found by handle or email")
						}
					}
				} else {
					empty := ""
					input.ApproverID = &empty
					input.ApproverEmail = &empty
				}
			}
		} else if appType == "" {
			empty := ""
			input.ApproverEmail = &empty
			input.ApproverID = &empty
		}
	}

	input.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	return s.repo.Update(
		ctx,
		ownerID,
		goalID,
		input,
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
