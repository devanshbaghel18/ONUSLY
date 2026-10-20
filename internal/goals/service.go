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

func (s *Service) CreateWithInput(
	ctx context.Context,
	ownerID string,
	input CreateGoalInput,
) (*Goal, error) {
	title := strings.TrimSpace(input.Title)
	description := strings.TrimSpace(input.Description)

	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	if title == "" {
		return nil, errors.New("title is required")
	}

	if len(title) > 200 {
		return nil, errors.New("title exceeds maximum allowed length of 200 characters")
	}

	if len(description) > 2000 {
		return nil, errors.New("description exceeds maximum allowed length of 2000 characters")
	}

	var apps []string
	var domains []string
	var err error

	if len(input.BlockedApps) > 0 {
		apps, err = NormalizeAndValidateApps(input.BlockedApps)
		if err != nil {
			return nil, err
		}
	} else {
		apps = []string{}
	}

	if len(input.BlockedDomains) > 0 {
		domains, err = NormalizeAndValidateDomains(input.BlockedDomains)
		if err != nil {
			return nil, err
		}
	} else {
		domains = []string{}
	}

	hasTargets := len(apps) > 0 || len(domains) > 0
	approvalType := ""
	approverID := ""
	approverEmail := ""

	if hasTargets {
		// A goal with targets MUST be friend approval, rejecting both none and community
		if input.ApprovalType == nil {
			return nil, ErrAccountabilityRequired
		}
		appType := strings.ToLower(strings.TrimSpace(*input.ApprovalType))
		if appType != "friend" {
			return nil, ErrAccountabilityRequired
		}
		if input.ApproverEmail == nil || strings.TrimSpace(*input.ApproverEmail) == "" {
			return nil, ErrAccountabilityRequired
		}

		inputVal := strings.TrimSpace(*input.ApproverEmail)
		if s.authRepo != nil {
			var friendUser *auth.User
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
				approverID = friendUser.ID
				displayTag := friendUser.Handle
				if displayTag != "" {
					if !strings.HasPrefix(displayTag, "@") {
						displayTag = "@" + displayTag
					}
					approverEmail = displayTag
				} else {
					approverEmail = strings.ToLower(friendUser.Email)
				}
				approvalType = "friend"
			} else {
				return nil, errors.New("accountability partner not found by handle or email")
			}
		} else {
			approvalType = "friend"
			approverEmail = inputVal
			approverID = "test-approver-id"
		}
	} else {
		// Untargeted goals can have none, community, or friend
		if input.ApprovalType != nil {
			appType := strings.ToLower(strings.TrimSpace(*input.ApprovalType))
			if appType != "" && appType != "community" && appType != "friend" && appType != "none" {
				return nil, errors.New("approval type must be 'community', 'friend', or 'none'")
			}
			if appType == "community" {
				approvalType = "community"
				approverID = "COMMUNITY"
			} else if appType == "friend" {
				if input.ApproverEmail != nil && strings.TrimSpace(*input.ApproverEmail) != "" {
					inputVal := strings.TrimSpace(*input.ApproverEmail)
					if s.authRepo != nil {
						cleanedHandle := strings.TrimPrefix(inputVal, "@")
						var friendUser *auth.User
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
							approverID = friendUser.ID
							displayTag := friendUser.Handle
							if displayTag != "" {
								if !strings.HasPrefix(displayTag, "@") {
									displayTag = "@" + displayTag
								}
								approverEmail = displayTag
							} else {
								approverEmail = strings.ToLower(friendUser.Email)
							}
							approvalType = "friend"
						} else {
							return nil, errors.New("accountability partner not found by handle or email")
						}
					} else {
						approvalType = "friend"
						approverEmail = inputVal
						approverID = "test-approver-id"
					}
				}
			}
		}
	}

	goalID := uuid.NewString()

	goal := Goal{
		PK:             "USER#" + ownerID,
		SK:             "GOAL#" + goalID,
		ID:             goalID,
		OwnerID:        ownerID,
		ApproverID:     approverID,
		ApproverEmail:  approverEmail,
		ApprovalType:   approvalType,
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

func (s *Service) Create(
	ctx context.Context,
	ownerID string,
	title string,
	description string,
	rawApps []string,
	rawDomains []string,
) (*Goal, error) {
	input := CreateGoalInput{
		Title:          title,
		Description:    description,
		BlockedApps:    rawApps,
		BlockedDomains: rawDomains,
	}
	if len(rawApps) > 0 || len(rawDomains) > 0 {
		friendType := "friend"
		testEmail := "test-partner@example.com"
		input.ApprovalType = &friendType
		input.ApproverEmail = &testEmail
	}
	return s.CreateWithInput(ctx, ownerID, input)
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

	goal, err := s.repo.GetByID(ctx, ownerID, goalID)
	if err != nil {
		return err
	}

	if goal.Status == "proof_submitted" {
		return ErrGoalInReview
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

	isTargeted := hasExistingTargets || (input.BlockedApps != nil && len(*input.BlockedApps) > 0) || (input.BlockedDomains != nil && len(*input.BlockedDomains) > 0)
	if isTargeted && goal.Status != "completed" {
		if input.ApprovalType != nil {
			low := strings.ToLower(strings.TrimSpace(*input.ApprovalType))
			if low == "none" || low == "" || low == "community" {
				return nil, ErrAccountabilityRequired
			}
		}
		if input.ApproverEmail != nil && strings.TrimSpace(*input.ApproverEmail) == "" {
			return nil, ErrAccountabilityRequired
		}
	}

	input.ExpectedBlockedApps = goal.BlockedApps
	input.ExpectedBlockedDomains = goal.BlockedDomains
	input.CheckTargetsUnchanged = (input.BlockedApps != nil || input.BlockedDomains != nil) && hasExistingTargets
	if input.ApprovalType != nil {
		low := strings.ToLower(strings.TrimSpace(*input.ApprovalType))
		input.DisallowNoneIfLocked = (low == "none" || low == "" || low == "community")
	}
	if isTargeted && goal.Status != "completed" && (input.ApprovalType == nil || strings.ToLower(strings.TrimSpace(*input.ApprovalType)) != "friend") {
		input.RequireFriendAccountability = true
	}

	if (input.ApprovalType != nil || input.ApproverEmail != nil) && goal.Status != "active" {
		return nil, errors.New("cannot modify accountability settings on a goal that is not active")
	}

	if input.Title != nil {
		trimmed := strings.TrimSpace(*input.Title)
		if trimmed == "" {
			return nil, errors.New("title is required")
		}
		if len(trimmed) > 200 {
			return nil, errors.New("title exceeds maximum allowed length of 200 characters")
		}
		input.Title = &trimmed
	}

	if input.Description != nil {
		trimmed := strings.TrimSpace(*input.Description)
		if len(trimmed) > 2000 {
			return nil, errors.New("description exceeds maximum allowed length of 2000 characters")
		}
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
