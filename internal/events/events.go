package events

import (
	"context"
	"errors"
	"time"
)

// ProofSubmittedEvent represents a domain event emitted after a proof is persisted to DynamoDB.
type ProofSubmittedEvent struct {
	EventID     string    `json:"eventId"`
	GoalID      string    `json:"goalId"`
	ProofID     string    `json:"proofId"`
	OwnerID     string    `json:"ownerId"`
	ApproverID  string    `json:"approverId"`
	Title       string    `json:"title"`
	SubmittedAt time.Time `json:"submittedAt"`
}

// ApprovalDecidedEvent represents a domain event emitted after an approval decision commits to DynamoDB.
type ApprovalDecidedEvent struct {
	EventID    string    `json:"eventId"`
	GoalID     string    `json:"goalId"`
	ProofID    string    `json:"proofId"`
	OwnerID    string    `json:"ownerId"`
	ApproverID string    `json:"approverId"`
	Status     string    `json:"status"` // "approved" | "rejected"
	Title      string    `json:"title"`
	DecidedAt  time.Time `json:"decidedAt"`
}

// Publisher defines the contract for domain event publication.
// Neither proof nor approval services know about SQS or WebSockets directly.
type Publisher interface {
	PublishProofSubmitted(ctx context.Context, event ProofSubmittedEvent) error
	PublishApprovalDecided(ctx context.Context, event ApprovalDecidedEvent) error
}

// CompositePublisher fans out domain events to multiple downstream publishers
// (e.g., WebSocket Publisher and SQS Queue Publisher).
type CompositePublisher struct {
	publishers []Publisher
}

// NewCompositePublisher creates a new CompositePublisher wrapping the provided publishers.
func NewCompositePublisher(publishers ...Publisher) *CompositePublisher {
	var valid []Publisher
	for _, p := range publishers {
		if p != nil {
			valid = append(valid, p)
		}
	}
	return &CompositePublisher{publishers: valid}
}

// Add appends a publisher to the composite.
func (c *CompositePublisher) Add(p Publisher) {
	if p != nil {
		c.publishers = append(c.publishers, p)
	}
}

// PublishProofSubmitted broadcasts the event to all registered publishers.
func (c *CompositePublisher) PublishProofSubmitted(ctx context.Context, event ProofSubmittedEvent) error {
	var errs []error
	for _, p := range c.publishers {
		if err := p.PublishProofSubmitted(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// PublishApprovalDecided broadcasts the event to all registered publishers.
func (c *CompositePublisher) PublishApprovalDecided(ctx context.Context, event ApprovalDecidedEvent) error {
	var errs []error
	for _, p := range c.publishers {
		if err := p.PublishApprovalDecided(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// NoopPublisher is a stub publisher that discards all events.
type NoopPublisher struct{}

func (n *NoopPublisher) PublishProofSubmitted(ctx context.Context, event ProofSubmittedEvent) error {
	return nil
}

func (n *NoopPublisher) PublishApprovalDecided(ctx context.Context, event ApprovalDecidedEvent) error {
	return nil
}
