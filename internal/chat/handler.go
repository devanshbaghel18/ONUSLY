package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
	"github.com/google/uuid"
)

type ChatRepository interface {
	GetConversationHistory(ctx context.Context, userKey, peerKey string, limit int32, before ...string) ([]Message, error)
	SaveMessage(ctx context.Context, msg realtime.StoredMessage, isRecipientOnline bool) error
	DeleteMessage(ctx context.Context, callerEmail, callerHandle, callerID, msgID string) error
	ClearConversation(ctx context.Context, userKey, peerKey string) error
	SaveCommunityMessage(ctx context.Context, communityID string, msg realtime.StoredMessage) error
	GetCommunityHistory(ctx context.Context, communityID string, limit int32, before ...string) ([]Message, error)
}

func isCommunityID(id string) bool {
	clean := strings.TrimSpace(id)
	return strings.HasPrefix(clean, "comm-") || strings.HasPrefix(clean, "comm_")
}

type UserResolver interface {
	ResolveUser(ctx context.Context, identifier string) (userID, email, handle string, err error)
}

type MessageBroadcaster interface {
	SendToUser(userID string, event realtime.Event) error
}

type Handler struct {
	repo         ChatRepository
	userResolver UserResolver
	broadcaster  MessageBroadcaster
}

func NewHandler(repo ChatRepository) *Handler {
	return &Handler{
		repo: repo,
	}
}

func (h *Handler) SetUserResolver(resolver UserResolver) {
	h.userResolver = resolver
}

func (h *Handler) SetBroadcaster(b MessageBroadcaster) {
	h.broadcaster = b
}

func normalizeHandleKey(handle string) string {
	clean := strings.TrimPrefix(strings.TrimSpace(handle), "@")
	if clean == "" {
		return ""
	}
	return "@" + clean
}

func getPeerVariants(peer string) []string {
	clean := strings.TrimSpace(peer)
	if clean == "" {
		return nil
	}
	variants := []string{clean}
	if !strings.Contains(clean, "@") {
		variants = append(variants, "@"+clean)
	} else if strings.HasPrefix(clean, "@") {
		variants = append(variants, strings.TrimPrefix(clean, "@"))
	}
	return variants
}

func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	callerID, _ := middleware.GetUserID(r.Context())
	callerEmail, _ := middleware.GetUserEmail(r.Context())
	callerHandle, _ := middleware.GetUserHandle(r.Context())

	callerIDKey := strings.TrimSpace(callerID)
	callerEmailKey := strings.TrimSpace(strings.ToLower(callerEmail))
	callerHandleKey := normalizeHandleKey(callerHandle)

	if callerIDKey == "" && callerEmailKey == "" && callerHandleKey == "" {
		http.Error(w, "unauthorized: email or handle required in token", http.StatusUnauthorized)
		return
	}

	peer := strings.TrimSpace(r.URL.Query().Get("peer"))
	if peer == "" {
		http.Error(w, "missing required query parameter: peer", http.StatusBadRequest)
		return
	}
	before := strings.TrimSpace(r.URL.Query().Get("before"))

	limit := 100
	if lStr := strings.TrimSpace(r.URL.Query().Get("limit")); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 {
			limit = parsed
			if limit > 100 {
				limit = 100
			}
		}
	}

	// Community chat messages
	if isCommunityID(peer) {
		commMsgs, err := h.repo.GetCommunityHistory(r.Context(), peer, int32(limit), before)
		if err != nil {
			http.Error(w, "failed to load community messages: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if commMsgs == nil {
			commMsgs = make([]Message, 0)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(commMsgs)
		return
	}

	seen := make(map[string]bool)
	var allMsgs []Message

	// If UserResolver is configured, use strict immutable identity resolution and isolation
	if h.userResolver != nil {
		peerID, peerEmail, _, err := h.userResolver.ResolveUser(r.Context(), peer)
		if err != nil || peerID == "" {
			http.Error(w, "peer user not found", http.StatusNotFound)
			return
		}

		primaryCallerKey := callerIDKey
		if primaryCallerKey == "" {
			primaryCallerKey = callerEmailKey
		}

		// 1. Primary query: authenticated user partition with peer user ID
		primaryMsgs, err := h.repo.GetConversationHistory(r.Context(), primaryCallerKey, peerID, int32(limit), before)
		if err != nil {
			http.Error(w, "failed to load chat history: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if len(primaryMsgs) > 0 {
			allMsgs = primaryMsgs
		} else if callerEmailKey != "" && peerEmail != "" && primaryCallerKey != callerEmailKey {
			// 2. Backward compatibility: read legacy email partition only if canonical messages are exhausted
			// and peer email is known. Strictly verify ownership on every legacy record (fail-closed).
			legacyMsgs, err := h.repo.GetConversationHistory(r.Context(), callerEmailKey, peerEmail, int32(limit), before)
			if err == nil {
				for _, m := range legacyMsgs {
					isOwner := (strings.EqualFold(m.SenderEmail, callerEmailKey) && strings.EqualFold(m.RecipientEmail, peerEmail)) ||
						(strings.EqualFold(m.SenderEmail, peerEmail) && strings.EqualFold(m.RecipientEmail, callerEmailKey)) ||
						m.SenderID == callerIDKey || m.RecipientID == callerIDKey
					if isOwner && !seen[m.ID] {
						seen[m.ID] = true
						allMsgs = append(allMsgs, m)
					}
				}
			}
		}
	} else {
		// Fallback for mock unit tests without UserResolver
		var callerKeys []string
		if callerEmailKey != "" {
			callerKeys = append(callerKeys, callerEmailKey)
		}
		if callerHandleKey != "" && callerHandleKey != callerEmailKey {
			callerKeys = append(callerKeys, callerHandleKey)
		}
		if callerIDKey != "" && callerIDKey != callerEmailKey && callerIDKey != callerHandleKey {
			callerKeys = append(callerKeys, callerIDKey)
		}

		peerVariants := getPeerVariants(peer)
		for _, cKey := range callerKeys {
			for _, pKey := range peerVariants {
				msgs, err := h.repo.GetConversationHistory(r.Context(), cKey, pKey, int32(limit), before)
				if err != nil {
					http.Error(w, "failed to load chat history: "+err.Error(), http.StatusInternalServerError)
					return
				}
				for _, m := range msgs {
					if !seen[m.ID] {
						seen[m.ID] = true
						allMsgs = append(allMsgs, m)
					}
				}
			}
		}
		if len(allMsgs) > limit {
			allMsgs = allMsgs[len(allMsgs)-limit:]
		}
	}

	// Sort chronologically by timestamp with deterministic ID tie-breaking
	sort.Slice(allMsgs, func(i, j int) bool {
		if allMsgs[i].Time == allMsgs[j].Time {
			return allMsgs[i].ID < allMsgs[j].ID
		}
		return allMsgs[i].Time < allMsgs[j].Time
	})

	if allMsgs == nil {
		allMsgs = []Message{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allMsgs)
}

func (h *Handler) ClearHistory(w http.ResponseWriter, r *http.Request) {
	callerID, _ := middleware.GetUserID(r.Context())
	callerEmail, _ := middleware.GetUserEmail(r.Context())
	callerHandle, _ := middleware.GetUserHandle(r.Context())

	callerIDKey := strings.TrimSpace(callerID)
	callerEmailKey := strings.TrimSpace(strings.ToLower(callerEmail))
	callerHandleKey := normalizeHandleKey(callerHandle)

	if callerIDKey == "" && callerEmailKey == "" && callerHandleKey == "" {
		http.Error(w, "unauthorized: authenticated user identity required in token", http.StatusUnauthorized)
		return
	}

	peer := strings.TrimSpace(r.URL.Query().Get("peer"))
	if peer == "" {
		http.Error(w, "missing required query parameter: peer", http.StatusBadRequest)
		return
	}

	if isCommunityID(peer) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		return
	}

	if h.userResolver != nil {
		peerID, peerEmail, _, err := h.userResolver.ResolveUser(r.Context(), peer)
		if err != nil || peerID == "" {
			http.Error(w, "peer user not found", http.StatusNotFound)
			return
		}

		primaryCallerKey := callerIDKey
		if primaryCallerKey == "" {
			primaryCallerKey = callerEmailKey
		}

		if err := h.repo.ClearConversation(r.Context(), primaryCallerKey, peerID); err != nil {
			http.Error(w, "failed to clear chat: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if callerEmailKey != "" && peerEmail != "" && primaryCallerKey != callerEmailKey {
			_ = h.repo.ClearConversation(r.Context(), callerEmailKey, peerEmail)
		}
	} else {
		var callerKeys []string
		if callerEmailKey != "" {
			callerKeys = append(callerKeys, callerEmailKey)
		}
		if callerHandleKey != "" && callerHandleKey != callerEmailKey {
			callerKeys = append(callerKeys, callerHandleKey)
		}
		if callerIDKey != "" && callerIDKey != callerEmailKey && callerIDKey != callerHandleKey {
			callerKeys = append(callerKeys, callerIDKey)
		}

		peerVariants := getPeerVariants(peer)
		for _, cKey := range callerKeys {
			for _, pKey := range peerVariants {
				if err := h.repo.ClearConversation(r.Context(), cKey, pKey); err != nil {
					http.Error(w, "failed to clear chat: "+err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// SendMessageRequest represents the payload for POST /chat/messages.
type SendMessageRequest struct {
	Recipient string `json:"recipient"`
	Text      string `json:"text"`
	IsProof   bool   `json:"isProof,omitempty"`
	GoalID    string `json:"goalId,omitempty"`
	GoalTitle string `json:"goalTitle,omitempty"`
	OwnerID   string `json:"ownerId,omitempty"`
}

// SendMessage stores a message in DynamoDB and broadcasts it via WebSocket if broadcaster is configured.
func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	callerID, _ := middleware.GetUserID(r.Context())
	callerEmail, _ := middleware.GetUserEmail(r.Context())
	callerHandle, _ := middleware.GetUserHandle(r.Context())

	callerID = strings.TrimSpace(callerID)
	callerEmail = strings.TrimSpace(callerEmail)
	callerHandle = strings.TrimSpace(callerHandle)

	if callerID == "" && callerEmail == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	cleanRecipient := strings.TrimSpace(req.Recipient)
	cleanText := strings.TrimSpace(req.Text)

	if cleanRecipient == "" {
		http.Error(w, "missing required field: recipient", http.StatusBadRequest)
		return
	}
	if cleanText == "" && !req.IsProof {
		http.Error(w, "message text cannot be empty", http.StatusBadRequest)
		return
	}

	msgID := "msg-" + uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	ownerID := req.OwnerID
	if ownerID == "" && req.IsProof {
		ownerID = callerID
	}

	// Community message handling
	if isCommunityID(cleanRecipient) {
		stored := realtime.StoredMessage{
			ID:           msgID,
			SenderID:     callerID,
			SenderEmail:  callerEmail,
			SenderHandle: callerHandle,
			RecipientID:  cleanRecipient,
			Text:         cleanText,
			IsProof:      req.IsProof,
			GoalID:       req.GoalID,
			GoalTitle:    req.GoalTitle,
			OwnerID:      ownerID,
			Time:         now,
		}

		if err := h.repo.SaveCommunityMessage(r.Context(), cleanRecipient, stored); err != nil {
			http.Error(w, "failed to save community message: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if h.broadcaster != nil {
			senderName := callerHandle
			if senderName == "" {
				senderName = callerEmail
			}
			evt := realtime.Event{
				Type: "community.message",
				Payload: map[string]interface{}{
					"id":           msgID,
					"communityId":  cleanRecipient,
					"senderId":     callerID,
					"senderEmail":  callerEmail,
					"senderHandle": callerHandle,
					"senderName":   senderName,
					"text":         cleanText,
					"isProof":      req.IsProof,
					"goalId":       req.GoalID,
					"goalTitle":    req.GoalTitle,
					"ownerId":      ownerID,
					"time":         now,
				},
			}
			if cb, ok := h.broadcaster.(interface {
				SendToCommunity(string, realtime.Event) error
			}); ok {
				_ = cb.SendToCommunity(cleanRecipient, evt)
			}
			if bc, ok := h.broadcaster.(interface {
				Broadcast(realtime.Event)
			}); ok {
				bc.Broadcast(evt)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Message{
			ID:           msgID,
			SenderID:     callerID,
			SenderEmail:  callerEmail,
			SenderHandle: callerHandle,
			RecipientID:  cleanRecipient,
			Text:         cleanText,
			IsProof:      req.IsProof,
			GoalID:       req.GoalID,
			GoalTitle:    req.GoalTitle,
			OwnerID:      ownerID,
			Time:         now,
			Delivered:    true,
		})
		return
	}

	var recipientID, recipientEmail, recipientHandle string
	if h.userResolver != nil {
		id, em, hd, err := h.userResolver.ResolveUser(r.Context(), cleanRecipient)
		if err != nil || id == "" {
			http.Error(w, "recipient not found", http.StatusNotFound)
			return
		}
		recipientID = id
		recipientEmail = em
		recipientHandle = hd
	} else {
		recipientEmail = cleanRecipient
	}

	stored := realtime.StoredMessage{
		ID:              msgID,
		SenderID:        callerID,
		SenderEmail:     callerEmail,
		SenderHandle:    callerHandle,
		RecipientID:     recipientID,
		RecipientEmail:  recipientEmail,
		RecipientHandle: recipientHandle,
		Text:            cleanText,
		IsProof:         req.IsProof,
		GoalID:          req.GoalID,
		GoalTitle:       req.GoalTitle,
		OwnerID:         ownerID,
		Time:            now,
	}

	if err := h.repo.SaveMessage(r.Context(), stored, false); err != nil {
		http.Error(w, "failed to save message: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if h.broadcaster != nil {
		senderName := callerHandle
		if senderName == "" {
			senderName = callerEmail
		}
		evt := realtime.Event{
			Type: "chat.message",
			Payload: map[string]interface{}{
				"id":              msgID,
				"senderId":        callerID,
				"senderEmail":     callerEmail,
				"senderHandle":    callerHandle,
				"senderName":      senderName,
				"recipientId":     recipientID,
				"recipientEmail":  recipientEmail,
				"recipientHandle": recipientHandle,
				"text":            cleanText,
				"isProof":         req.IsProof,
				"goalId":          req.GoalID,
				"goalTitle":       req.GoalTitle,
				"ownerId":         ownerID,
				"time":            now,
			},
		}
		if recipientID != "" {
			_ = h.broadcaster.SendToUser(recipientID, evt)
		}
		if callerID != "" && callerID != recipientID {
			_ = h.broadcaster.SendToUser(callerID, evt)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(stored)
}
