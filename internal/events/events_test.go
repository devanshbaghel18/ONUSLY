package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/events"
)

type mockPublisher struct {
	proofEvents    []events.ProofSubmittedEvent
	approvalEvents []events.ApprovalDecidedEvent
	errToReturn    error
}

func (m *mockPublisher) PublishProofSubmitted(ctx context.Context, event events.ProofSubmittedEvent) error {
	if m.errToReturn != nil {
		return m.errToReturn
	}
	m.proofEvents = append(m.proofEvents, event)
	return nil
}

func (m *mockPublisher) PublishApprovalDecided(ctx context.Context, event events.ApprovalDecidedEvent) error {
	if m.errToReturn != nil {
		return m.errToReturn
	}
	m.approvalEvents = append(m.approvalEvents, event)
	return nil
}

func TestCompositePublisher_FanOut(t *testing.T) {
	pub1 := &mockPublisher{}
	pub2 := &mockPublisher{}

	comp := events.NewCompositePublisher(pub1, pub2)

	// 1. ProofSubmitted fanout
	proofEv := events.ProofSubmittedEvent{
		EventID:     "proof-123",
		GoalID:      "goal-456",
		ProofID:     "123",
		OwnerID:     "owner-1",
		ApproverID:  "approver-1",
		Title:       "Test Goal",
		SubmittedAt: time.Now().UTC(),
	}

	err := comp.PublishProofSubmitted(context.Background(), proofEv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub1.proofEvents) != 1 || len(pub2.proofEvents) != 1 {
		t.Fatalf("expected both publishers to receive event, got %d and %d", len(pub1.proofEvents), len(pub2.proofEvents))
	}
	if pub1.proofEvents[0].EventID != "proof-123" || pub2.proofEvents[0].EventID != "proof-123" {
		t.Errorf("expected stable event ID 'proof-123'")
	}

	// 2. ApprovalDecided fanout
	appEv := events.ApprovalDecidedEvent{
		EventID:    "approval-789",
		GoalID:     "goal-456",
		ProofID:    "123",
		OwnerID:    "owner-1",
		ApproverID: "approver-1",
		Status:     "approved",
		Title:      "Test Goal",
		DecidedAt:  time.Now().UTC(),
	}

	err = comp.PublishApprovalDecided(context.Background(), appEv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub1.approvalEvents) != 1 || len(pub2.approvalEvents) != 1 {
		t.Fatalf("expected both publishers to receive event, got %d and %d", len(pub1.approvalEvents), len(pub2.approvalEvents))
	}
	if pub1.approvalEvents[0].EventID != "approval-789" {
		t.Errorf("expected stable event ID 'approval-789'")
	}
}

func TestCompositePublisher_ErrorCollection(t *testing.T) {
	pub1 := &mockPublisher{errToReturn: errors.New("pub1 failed")}
	pub2 := &mockPublisher{}

	comp := events.NewCompositePublisher(pub1, pub2)

	err := comp.PublishProofSubmitted(context.Background(), events.ProofSubmittedEvent{EventID: "proof-999"})
	if err == nil {
		t.Fatalf("expected error from failing publisher, got nil")
	}

	// Pub2 should still receive the event even if Pub1 fails
	if len(pub2.proofEvents) != 1 {
		t.Fatalf("expected pub2 to still receive event despite pub1 failure")
	}
}
