package realtime

import (
	"context"
	"log"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/events"
)

// WebSocketPublisher implements events.Publisher by pushing events over active WebSockets.
// This preserves the exact Slice A real-time unlock delivery behavior.
type WebSocketPublisher struct {
	hub *Hub
}

// NewWebSocketPublisher creates a new WebSocketPublisher.
func NewWebSocketPublisher(hub *Hub) *WebSocketPublisher {
	return &WebSocketPublisher{hub: hub}
}

// PublishProofSubmitted delivers real-time proof submission notification to the approver if online.
func (p *WebSocketPublisher) PublishProofSubmitted(ctx context.Context, event events.ProofSubmittedEvent) error {
	if p.hub == nil {
		return nil
	}

	wsEvent := Event{
		Type: "proof.submitted",
		Payload: map[string]interface{}{
			"eventId":     event.EventID,
			"goalId":      event.GoalID,
			"proofId":     event.ProofID,
			"ownerId":     event.OwnerID,
			"approverId":  event.ApproverID,
			"title":       event.Title,
			"submittedAt": event.SubmittedAt.Format(time.RFC3339),
		},
	}

	if event.ApproverID != "" {
		_ = p.hub.SendToUser(event.ApproverID, wsEvent)
	}

	return nil
}

// PublishApprovalDecided delivers goal.unlocked event to the owner (and approver) upon approval.
// This fulfills the core Slice A real-time unlock requirement.
func (p *WebSocketPublisher) PublishApprovalDecided(ctx context.Context, event events.ApprovalDecidedEvent) error {
	if p.hub == nil {
		return nil
	}

	if event.Status == "approved" {
		log.Printf("[Realtime] Dispatching goal.unlocked event to owner %s for goal %s", event.OwnerID, event.GoalID)
		unlockEvent := Event{
			Type: "goal.unlocked",
			Payload: map[string]interface{}{
				"goalId":     event.GoalID,
				"ownerId":    event.OwnerID,
				"approverId": event.ApproverID,
				"status":     "completed",
				"title":      event.Title,
				"unlockedAt": event.DecidedAt.Format(time.RFC3339),
			},
		}

		_ = p.hub.SendToUser(event.OwnerID, unlockEvent)
		if event.ApproverID != "" && event.ApproverID != event.OwnerID {
			_ = p.hub.SendToUser(event.ApproverID, unlockEvent)
		}
	}

	return nil
}

var _ events.Publisher = (*WebSocketPublisher)(nil)
