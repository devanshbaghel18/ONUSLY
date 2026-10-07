package notifications

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/events"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// CreateFromProofSubmitted generates and persists a notification for the approver.
func (s *Service) CreateFromProofSubmitted(ctx context.Context, event events.ProofSubmittedEvent) error {
	approverID := strings.TrimSpace(event.ApproverID)
	if approverID == "" {
		return nil // Goal has no assigned approver; nothing to deliver
	}

	submittedAt := event.SubmittedAt
	if submittedAt.IsZero() {
		submittedAt = time.Now().UTC()
	}

	notif := &Notification{
		ID:          event.EventID,
		RecipientID: approverID,
		Type:        "proof_submitted",
		Title:       "New Proof Needs Review",
		Message:     fmt.Sprintf("New proof submitted for %q. Review evidence.", event.Title),
		GoalID:      event.GoalID,
		ProofID:     event.ProofID,
		Read:        false,
		CreatedAt:   submittedAt.Format(time.RFC3339),
	}

	err := s.repo.Create(ctx, notif)
	if err != nil && errors.Is(err, ErrAlreadyExists) {
		// Idempotency: duplicate processing treated as already-completed
		return nil
	}
	return err
}

// CreateFromApprovalDecided generates and persists a notification for the goal owner.
func (s *Service) CreateFromApprovalDecided(ctx context.Context, event events.ApprovalDecidedEvent) error {
	ownerID := strings.TrimSpace(event.OwnerID)
	if ownerID == "" {
		return nil
	}

	decidedAt := event.DecidedAt
	if decidedAt.IsZero() {
		decidedAt = time.Now().UTC()
	}

	var title, message string
	if event.Status == "approved" {
		title = "Proof Approved"
		message = fmt.Sprintf("Your proof for %q was approved! Focus locks lifted.", event.Title)
	} else {
		title = "Proof Rejected"
		message = fmt.Sprintf("Your proof for %q was rejected.", event.Title)
	}

	notif := &Notification{
		ID:          event.EventID,
		RecipientID: ownerID,
		Type:        "approval_decided",
		Title:       title,
		Message:     message,
		GoalID:      event.GoalID,
		ProofID:     event.ProofID,
		Read:        false,
		CreatedAt:   decidedAt.Format(time.RFC3339),
	}

	err := s.repo.Create(ctx, notif)
	if err != nil && errors.Is(err, ErrAlreadyExists) {
		// Idempotency: duplicate processing treated as already-completed
		return nil
	}
	return err
}

// List returns notifications for the recipient along with total unread count.
func (s *Service) List(ctx context.Context, recipientID string, limit int) (*NotificationListResponse, error) {
	if strings.TrimSpace(recipientID) == "" {
		return &NotificationListResponse{Notifications: []Notification{}, UnreadCount: 0}, nil
	}

	notifs, err := s.repo.List(ctx, recipientID, limit)
	if err != nil {
		return nil, err
	}

	unread := 0
	for _, n := range notifs {
		if !n.Read {
			unread++
		}
	}

	return &NotificationListResponse{
		Notifications: notifs,
		UnreadCount:   unread,
	}, nil
}

// MarkAsRead marks a specific notification as read.
func (s *Service) MarkAsRead(ctx context.Context, recipientID, notifID string) error {
	if strings.TrimSpace(recipientID) == "" || strings.TrimSpace(notifID) == "" {
		return errors.New("recipient and notification ID required")
	}
	return s.repo.MarkAsRead(ctx, recipientID, notifID)
}

// MarkAllAsRead marks all unread notifications as read.
func (s *Service) MarkAllAsRead(ctx context.Context, recipientID string) error {
	if strings.TrimSpace(recipientID) == "" {
		return errors.New("recipient ID required")
	}
	return s.repo.MarkAllAsRead(ctx, recipientID)
}
