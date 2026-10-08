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
	rawApps []string,
	rawDomains []string,
) (*Goal, error) {

	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)

	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	if title == "" {
		return nil, errors.New("title is required")
	}

	var apps []string
	var domains []string
	var err error

	if len(rawApps) > 0 {
		apps, err = NormalizeAndValidateApps(rawApps)
		if err != nil {
			return nil, err
		}
	} else {
		apps = []string{}
	}

	if len(rawDomains) > 0 {
		domains, err = NormalizeAndValidateDomains(rawDomains)
		if err != nil {
			return nil, err
		}
	} else {
		domains = []string{}
	}

	goalID := uuid.NewString()

	goal := Goal{
		PK:             "USER#" + ownerID,
		SK:             "GOAL#" + goalID,
		ID:             goalID,
		OwnerID:        ownerID,
		ApproverID:     "",
		Title:          title,
		Description:    description,
		Status:         "active",
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		BlockedApps:    apps,
		BlockedDomains: domains,
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
		return s.repo.GetByID(ctx, ownerID, goalID)
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

	hasExistingTargets := len(goal.BlockedApps) > 0 || len(goal.BlockedDomains) > 0

	if goal.Status == "completed" {
		if input.BlockedApps != nil || input.BlockedDomains != nil {
			return nil, errors.New("cannot modify targets on a completed goal")
		}
	}

	if input.BlockedApps != nil {
		normalizedApps, err := NormalizeAndValidateApps(*input.BlockedApps)
		if err != nil {
			return nil, err
		}
		if goal.Status != "completed" && hasExistingTargets {
			if !CheckTargetsSubset(goal.BlockedApps, normalizedApps) {
				return nil, ErrTargetsLocked
			}
		}
		input.BlockedApps = &normalizedApps
	}

	if input.BlockedDomains != nil {
		normalizedDomains, err := NormalizeAndValidateDomains(*input.BlockedDomains)
		if err != nil {
			return nil, err
		}
		if goal.Status != "completed" && hasExistingTargets {
			if !CheckTargetsSubset(goal.BlockedDomains, normalizedDomains) {
				return nil, ErrTargetsLocked
			}
		}
		input.BlockedDomains = &normalizedDomains
	}

	if hasExistingTargets && goal.Status != "completed" {
		if input.ApprovalType != nil {
			low := strings.ToLower(strings.TrimSpace(*input.ApprovalType))
			if low == "none" || low == "" {
				return nil, ErrAccountabilityLocked
			}
		}
		if input.ApproverEmail != nil && strings.TrimSpace(*input.ApproverEmail) == "" && input.ApprovalType == nil && goal.ApprovalType != "community" {
			return nil, ErrAccountabilityLocked
		}
	}

	input.ExpectedBlockedApps = goal.BlockedApps
	input.ExpectedBlockedDomains = goal.BlockedDomains
	input.CheckTargetsUnchanged = (input.BlockedApps != nil || input.BlockedDomains != nil) && hasExistingTargets
	if input.ApprovalType != nil {
		low := strings.ToLower(strings.TrimSpace(*input.ApprovalType))
		input.DisallowNoneIfLocked = (low == "none" || low == "")
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
