package realtime

import (
	"context"
	"fmt"
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

func deliverToParticipants(hub *Hub, ownerID, approverID string, ev Event) {
	if ownerID != "" {
		_ = hub.SendToUser(ownerID, ev)
		_ = hub.SendToEmail(ownerID, ev)
		_ = hub.SendToHandle(ownerID, ev)
	}
	if approverID != "" && approverID != ownerID {
		_ = hub.SendToUser(approverID, ev)
		_ = hub.SendToEmail(approverID, ev)
		_ = hub.SendToHandle(approverID, ev)
	}
}

// PublishProofSubmitted delivers real-time proof submission notification to the approver and owner if online.
// It also persists and dispatches a chat verification message so the proof request appears directly in the 1-on-1 chat.
func (p *WebSocketPublisher) PublishProofSubmitted(ctx context.Context, event events.ProofSubmittedEvent) error {
	if p.hub == nil {
		return nil
	}

	payload := map[string]interface{}{
		"eventId":     event.EventID,
		"goalId":      event.GoalID,
		"proofId":     event.ProofID,
		"ownerId":     event.OwnerID,
		"approverId":  event.ApproverID,
		"title":       event.Title,
		"submittedAt": event.SubmittedAt.Format(time.RFC3339),
	}
	if event.Files != nil {
		payload["files"] = event.Files
	}

	wsEvent := Event{
		Type:    "proof.submitted",
		Payload: payload,
	}

	deliverToParticipants(p.hub, event.OwnerID, event.ApproverID, wsEvent)

	// Synthesize and dispatch chat message so verification appears directly in the chat between owner and approver
	msgStore := p.hub.MessageStore()
	resolver := p.hub.UserResolver()
	if msgStore != nil && event.ApproverID != "" && event.OwnerID != "" {
		ownerID := event.OwnerID
		ownerEmail := ""
		ownerHandle := ""
		approverID := event.ApproverID
		approverEmail := ""
		approverHandle := ""

		if resolver != nil {
			if _, e, h, err := resolver.ResolveUser(ctx, event.OwnerID); err == nil {
				ownerEmail = e
				ownerHandle = h
			}
			if _, e, h, err := resolver.ResolveUser(ctx, event.ApproverID); err == nil {
				approverEmail = e
				approverHandle = h
			}
		}

		nowStr := event.SubmittedAt.UTC().Format(time.RFC3339)
		msgID := "proof-" + event.ProofID
		proofText := fmt.Sprintf("Please verify my goal: \"%s\"", event.Title)

		senderName := ownerHandle
		if senderName == "" {
			senderName = ownerEmail
		}
		if senderName == "" {
			senderName = "Partner"
		}

		storedMsg := StoredMessage{
			ID:              msgID,
			SenderID:        ownerID,
			SenderEmail:     ownerEmail,
			SenderHandle:    ownerHandle,
			RecipientID:     approverID,
			RecipientEmail:  approverEmail,
			RecipientHandle: approverHandle,
			Text:            proofText,
			IsProof:         true,
			GoalID:          event.GoalID,
			GoalTitle:       event.Title,
			OwnerID:         event.OwnerID,
			Files:           event.Files,
			Time:            nowStr,
		}

		isRecipientOnline := p.hub.IsUserOnline(approverID)

		_ = msgStore.SaveMessage(ctx, storedMsg, isRecipientOnline)

		chatEvent := Event{
			Type: "chat.message",
			Payload: map[string]interface{}{
				"id":              msgID,
				"senderId":        ownerID,
				"senderEmail":     ownerEmail,
				"senderHandle":    ownerHandle,
				"senderName":      senderName,
				"recipientEmail":  approverEmail,
				"recipientHandle": approverHandle,
				"recipientId":     approverID,
				"text":            proofText,
				"isProof":         true,
				"goalId":          event.GoalID,
				"goalTitle":       event.Title,
				"ownerId":         event.OwnerID,
				"files":           event.Files,
				"time":            nowStr,
			},
		}

		_ = p.hub.SendToUser(approverID, chatEvent)
		_ = p.hub.SendToUser(ownerID, chatEvent)
	}

	return nil
}

// PublishApprovalDecided delivers goal.unlocked event to the owner (and approver) upon approval,
// and delivers goal.rejected upon rejection to maintain active restrictions and notify the owner.
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

		deliverToParticipants(p.hub, event.OwnerID, event.ApproverID, unlockEvent)

		// Route goal approval chat notification strictly to the owner and approver
		approveEvent := Event{
			Type: "chat.message.approved",
			Payload: map[string]interface{}{
				"goalId":       event.GoalID,
				"ownerId":      event.OwnerID,
				"approverId":   event.ApproverID,
				"goalTitle":    event.Title,
				"approverName": event.ApproverID,
				"status":       "completed",
			},
		}
		deliverToParticipants(p.hub, event.OwnerID, event.ApproverID, approveEvent)
	} else if event.Status == "rejected" {
		log.Printf("[Realtime] Dispatching goal.rejected event to owner %s for goal %s", event.OwnerID, event.GoalID)
		rejectEvent := Event{
			Type: "goal.rejected",
			Payload: map[string]interface{}{
				"goalId":     event.GoalID,
				"ownerId":    event.OwnerID,
				"approverId": event.ApproverID,
				"status":     "rejected",
				"comment":    event.Comment,
				"title":      event.Title,
				"decidedAt":  event.DecidedAt.Format(time.RFC3339),
			},
		}

		deliverToParticipants(p.hub, event.OwnerID, event.ApproverID, rejectEvent)
	}

	return nil
}

var _ events.Publisher = (*WebSocketPublisher)(nil)
