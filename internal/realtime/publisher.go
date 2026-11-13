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

// PublishProofSubmitted delivers real-time proof submission notification to the approver if online.
// It also persists and broadcasts a chat verification message so the proof request appears directly in the 1-on-1 chat.
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

	if event.ApproverID != "" {
		_ = p.hub.SendToUser(event.ApproverID, wsEvent)
		_ = p.hub.SendToEmail(event.ApproverID, wsEvent)
		_ = p.hub.SendToHandle(event.ApproverID, wsEvent)
	}
	// Broadcast so all connected approver clients receive live inbox update immediately
	p.hub.Broadcast(wsEvent)

	// Synthesize and dispatch chat message so verification appears directly in the chat between owner and approver
	if p.hub.msgStore != nil && event.ApproverID != "" && event.OwnerID != "" {
		ownerID := event.OwnerID
		ownerEmail := ""
		ownerHandle := ""
		approverID := event.ApproverID
		approverEmail := ""
		approverHandle := ""

		if p.hub.userResolver != nil {
			if _, e, h, err := p.hub.userResolver.ResolveUser(ctx, event.OwnerID); err == nil {
				ownerEmail = e
				ownerHandle = h
			}
			if _, e, h, err := p.hub.userResolver.ResolveUser(ctx, event.ApproverID); err == nil {
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

		p.hub.mu.RLock()
		_, isRecipientOnline := p.hub.users[approverID]
		p.hub.mu.RUnlock()

		_ = p.hub.msgStore.SaveMessage(ctx, storedMsg, isRecipientOnline)

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

		_ = p.hub.SendToUser(event.OwnerID, unlockEvent)
		if event.ApproverID != "" && event.ApproverID != event.OwnerID {
			_ = p.hub.SendToUser(event.ApproverID, unlockEvent)
		}

		// Also broadcast authentic chat.message.approved event platform-wide
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
		p.hub.Broadcast(approveEvent)
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

		_ = p.hub.SendToUser(event.OwnerID, rejectEvent)
		if event.ApproverID != "" && event.ApproverID != event.OwnerID {
			_ = p.hub.SendToUser(event.ApproverID, rejectEvent)
		}
		p.hub.Broadcast(rejectEvent)
	}

	return nil
}

var _ events.Publisher = (*WebSocketPublisher)(nil)
