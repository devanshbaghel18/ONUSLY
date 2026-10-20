package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

var (
	ErrUserOffline = errors.New("user has no active connections")
)

// StoredMessage represents a chat message persisted in storage.
type StoredMessage struct {
	ID              string      `json:"id" dynamodbav:"ID"`
	SenderEmail     string      `json:"senderEmail" dynamodbav:"SenderEmail"`
	SenderHandle    string      `json:"senderHandle,omitempty" dynamodbav:"SenderHandle,omitempty"`
	SenderID        string      `json:"senderId" dynamodbav:"SenderID"`
	RecipientEmail  string      `json:"recipientEmail" dynamodbav:"RecipientEmail"`
	RecipientHandle string      `json:"recipientHandle,omitempty" dynamodbav:"RecipientHandle,omitempty"`
	RecipientID     string      `json:"recipientId,omitempty" dynamodbav:"RecipientID,omitempty"`
	Text            string      `json:"text" dynamodbav:"Text"`
	IsProof         bool        `json:"isProof,omitempty" dynamodbav:"IsProof,omitempty"`
	GoalID          string      `json:"goalId,omitempty" dynamodbav:"GoalID,omitempty"`
	GoalTitle       string      `json:"goalTitle,omitempty" dynamodbav:"GoalTitle,omitempty"`
	OwnerID         string      `json:"ownerId,omitempty" dynamodbav:"OwnerID,omitempty"`
	Images          []string    `json:"images,omitempty" dynamodbav:"Images,omitempty"`
	Files           interface{} `json:"files,omitempty" dynamodbav:"Files,omitempty"`
	ExternalLink    string      `json:"externalLink,omitempty" dynamodbav:"ExternalLink,omitempty"`
	Approved        bool        `json:"approved,omitempty" dynamodbav:"Approved,omitempty"`
	ReplyTo         interface{} `json:"replyTo,omitempty" dynamodbav:"ReplyTo,omitempty"`
	Time            string      `json:"time" dynamodbav:"Time"`
	Delivered       bool        `json:"delivered" dynamodbav:"Delivered"`
}

// UserResolver resolves user identifiers (ID, email, handle) to an immutable UserID.
type UserResolver interface {
	ResolveUser(ctx context.Context, identifier string) (userID, email, handle string, err error)
}

// MessageStore is an optional persistent storage layer for chat messages.
type MessageStore interface {
	SaveMessage(ctx context.Context, msg StoredMessage, isRecipientOnline bool) error
	GetPendingMessages(ctx context.Context, recipientEmail string) ([]StoredMessage, error)
	MarkMessagesDelivered(ctx context.Context, recipientEmail string, msgIDs []string) error
	DeleteMessage(ctx context.Context, callerEmail, callerHandle, callerID, msgID string) error
}

// GoalVerifier validates whether a goal has been legitimately approved by the backend.
type GoalVerifier interface {
	VerifyApprovedGoal(ctx context.Context, callerID, callerEmail, callerHandle, goalID string) (ownerID, goalTitle string, ok bool, err error)
}

// Event represents a standard real-time message payload sent to clients.
type Event struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// CommunityRecord stores live state of a community across all connected users.
type CommunityRecord struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Code        string                 `json:"code"`
	Category    string                 `json:"category"`
	Description string                 `json:"description"`
	CreatorID   string                 `json:"creatorId"`
	Data        map[string]interface{} `json:"data"`
	Members     map[string]bool        `json:"members"` // normalized handle/email -> true
}

// Hub maintains the set of active clients keyed by UserID, Email, and Handle, and coordinates
// real-time event dispatching to specific users.
type Hub struct {
	mu sync.RWMutex

	// users maps userID -> set of active *Client connections (supports multi-tab)
	users map[string]map[*Client]bool

	// emails maps lowercased email -> set of active *Client connections
	emails map[string]map[*Client]bool

	// handles maps lowercased handle -> set of active *Client connections
	handles map[string]map[*Client]bool

	// communities maps communityID -> set of active *Client connections
	communities map[string]map[*Client]bool

	// commRecords maps communityID -> *CommunityRecord
	commRecords map[string]*CommunityRecord

	// commByCode maps uppercase code -> communityID
	commByCode map[string]string

	// commMsgAuthors maps "communityID#messageID" -> author userID
	commMsgAuthors map[string]string

	// msgStore is an optional persistent message store for offline delivery
	msgStore MessageStore

	// goalVerifier validates goal approval authority against backend state
	goalVerifier GoalVerifier

	// userResolver resolves handles/emails to immutable UserID
	userResolver UserResolver
}

// NewHub initializes and returns a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		users:          make(map[string]map[*Client]bool),
		emails:         make(map[string]map[*Client]bool),
		handles:        make(map[string]map[*Client]bool),
		communities:    make(map[string]map[*Client]bool),
		commRecords:    make(map[string]*CommunityRecord),
		commByCode:     make(map[string]string),
		commMsgAuthors: make(map[string]string),
	}
}

func generateCommunityMessageID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("comm-msg-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

func isPublicTrendingCommunity(id string) bool {
	switch id {
	case "comm-early-risers", "comm-leetcode", "comm-fitness", "comm-deep-work", "comm-book-reading":
		return true
	default:
		return false
	}
}

// SetMessageStore attaches a persistent MessageStore to the Hub.
func (h *Hub) SetMessageStore(store MessageStore) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgStore = store
}

// SetGoalVerifier attaches a GoalVerifier to the Hub for validating approval broadcasts.
func (h *Hub) SetGoalVerifier(verifier GoalVerifier) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.goalVerifier = verifier
}

// SetUserResolver attaches a UserResolver to the Hub for resolving handles/emails to immutable IDs.
func (h *Hub) SetUserResolver(resolver UserResolver) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.userResolver = resolver
}

// Register adds a client connection under its authenticated UserID, Email, and Handle.
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	userConns, exists := h.users[c.userID]
	if !exists {
		userConns = make(map[*Client]bool)
		h.users[c.userID] = userConns
	}
	userConns[c] = true

	if c.email != "" {
		emailConns, exists := h.emails[c.email]
		if !exists {
			emailConns = make(map[*Client]bool)
			h.emails[c.email] = emailConns
		}
		emailConns[c] = true
	}

	if c.handle != "" {
		handleConns, exists := h.handles[c.handle]
		if !exists {
			handleConns = make(map[*Client]bool)
			h.handles[c.handle] = handleConns
		}
		handleConns[c] = true
	}

	log.Printf("[WebSocket] Connected: userID=%s email=%s handle=%s (user tabs: %d)", c.userID, c.email, c.handle, len(userConns))
}

// Unregister removes a client connection and cleans up empty user/email/handle maps.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	userConns, exists := h.users[c.userID]
	if exists {
		delete(userConns, c)
		if len(userConns) == 0 {
			delete(h.users, c.userID)
			log.Printf("[WebSocket] User disconnected completely: %s", c.userID)
		}
	}

	if c.email != "" {
		emailConns, exists := h.emails[c.email]
		if exists {
			delete(emailConns, c)
			if len(emailConns) == 0 {
				delete(h.emails, c.email)
			}
		}
	}

	if c.handle != "" {
		handleConns, exists := h.handles[c.handle]
		if exists {
			delete(handleConns, c)
			if len(handleConns) == 0 {
				delete(h.handles, c.handle)
			}
		}
	}

	for commID, conns := range h.communities {
		delete(conns, c)
		if len(conns) == 0 {
			delete(h.communities, commID)
		}
	}

	close(c.send)
}

// JoinCommunity registers a client connection to a specific community room.
func (h *Hub) JoinCommunity(communityID string, c *Client) {
	cleanID := strings.TrimSpace(communityID)
	if cleanID == "" {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	conns, exists := h.communities[cleanID]
	if !exists {
		conns = make(map[*Client]bool)
		h.communities[cleanID] = conns
	}
	conns[c] = true
	log.Printf("[WebSocket] User %s joined community room %s (total online: %d)", c.userID, cleanID, len(conns))
}

// LeaveCommunity unregisters a client connection from a specific community room.
func (h *Hub) LeaveCommunity(communityID string, c *Client) {
	cleanID := strings.TrimSpace(communityID)
	if cleanID == "" {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	conns, exists := h.communities[cleanID]
	if exists {
		delete(conns, c)
		if len(conns) == 0 {
			delete(h.communities, cleanID)
		}
		log.Printf("[WebSocket] User %s left community room %s", c.userID, cleanID)
	}
}

// SendToCommunity dispatches an Event to all active clients currently in a community room.
func (h *Hub) SendToCommunity(communityID string, event Event) error {
	return h.SendToCommunityExcept(communityID, nil, event)
}

// SendToCommunityExcept dispatches an Event to active clients in a community room except a specific sender connection.
func (h *Hub) SendToCommunityExcept(communityID string, except *Client, event Event) error {
	cleanID := strings.TrimSpace(communityID)
	if cleanID == "" {
		return errors.New("empty community id")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	conns, exists := h.communities[cleanID]
	if !exists || len(conns) == 0 {
		return nil
	}

	for client := range conns {
		if except != nil && client == except {
			continue
		}
		select {
		case client.send <- data:
		default:
			log.Printf("[WebSocket] Buffer full for client in community %s", cleanID)
		}
	}

	return nil
}

// Broadcast sends an event to all connected clients across the entire hub.
func (h *Hub) Broadcast(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, conns := range h.users {
		for client := range conns {
			select {
			case client.send <- data:
			default:
			}
		}
	}
}

// SendToUser dispatches an Event to all active WebSocket connections for a given user ID.
func (h *Hub) SendToUser(userID string, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	userConns, exists := h.users[userID]
	if !exists || len(userConns) == 0 {
		return ErrUserOffline
	}

	for client := range userConns {
		select {
		case client.send <- data:
		default:
			log.Printf("[WebSocket] Buffer full for client of user: %s", userID)
		}
	}

	return nil
}

// SendToEmail dispatches an Event to all active WebSocket connections for a given email.
func (h *Hub) SendToEmail(email string, event Event) error {
	normalized := strings.TrimSpace(strings.ToLower(email))
	if normalized == "" {
		return errors.New("empty email")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	emailConns, exists := h.emails[normalized]
	if !exists || len(emailConns) == 0 {
		return ErrUserOffline
	}

	for client := range emailConns {
		select {
		case client.send <- data:
		default:
			log.Printf("[WebSocket] Buffer full for client of email: %s", normalized)
		}
	}

	return nil
}

// SendToHandle dispatches an Event to all active WebSocket connections for a given handle.
func (h *Hub) SendToHandle(handle string, event Event) error {
	normalized := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(handle)), "@")
	if normalized == "" {
		return errors.New("empty handle")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	handleConns, exists := h.handles[normalized]
	if !exists || len(handleConns) == 0 {
		return ErrUserOffline
	}

	for client := range handleConns {
		select {
		case client.send <- data:
		default:
			log.Printf("[WebSocket] Buffer full for client of handle: %s", normalized)
		}
	}

	return nil
}

// HandleClientMessage processes incoming WebSocket messages sent from a connected client.
func (h *Hub) HandleClientMessage(c *Client, message []byte) {
	var base struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(message, &base); err != nil {
		log.Printf("[WebSocket] Failed to parse client message JSON: %v", err)
		return
	}

	switch base.Type {
	case "user.online":
		var req struct {
			Email  string `json:"email"`
			Handle string `json:"handle"`
		}
		_ = json.Unmarshal(base.Payload, &req)

		// Security: NEVER overwrite c.email or c.handle from unverified client payload.
		// Identity is bound exclusively from the validated JWT token claims on the connection.
		// If client supplies an email or handle that does not match authenticated identity, log and ignore it.
		clientEmail := strings.TrimSpace(strings.ToLower(req.Email))
		clientHandle := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(req.Handle)), "@")
		if (clientEmail != "" && clientEmail != c.email) || (clientHandle != "" && clientHandle != c.handle) {
			log.Printf("[WebSocket] user.online: rejected spoofed identity from user %s (payload: email=%q, handle=%q; authenticated: email=%q, handle=%q)",
				c.userID, clientEmail, clientHandle, c.email, c.handle)
		}

		c.mu.Lock()
		store := h.msgStore
		shouldDeliver := !c.deliveredOffline && store != nil
		if shouldDeliver {
			c.deliveredOffline = true
		}
		c.mu.Unlock()

		if shouldDeliver {
			log.Printf("[WebSocket] Presence confirmed: user %s registered email=%s handle=@%s", c.userID, c.email, c.handle)

			// Deliver any stored offline messages to this user once per connection.
			// Security: Offline messages are retrieved exclusively by the authenticated user's
			// immutable user ID (and provable email when configured), NEVER by mutable handle aliases.
			go func(targetUserID, targetEmail string, cl *Client, s MessageStore) {
				var allPending []StoredMessage
				seen := make(map[string]bool)

				if targetUserID != "" {
					if pending, err := s.GetPendingMessages(context.Background(), targetUserID); err == nil {
						for _, m := range pending {
							if !seen[m.ID] {
								seen[m.ID] = true
								allPending = append(allPending, m)
							}
						}
					}
				}
				if targetEmail != "" {
					if pending, err := s.GetPendingMessages(context.Background(), targetEmail); err == nil {
						for _, m := range pending {
							if !seen[m.ID] {
								seen[m.ID] = true
								allPending = append(allPending, m)
							}
						}
					}
				}

				if len(allPending) > 0 {
					var deliveredIDs []string
					for _, m := range allPending {
						cl.SendEvent(Event{
							Type: "chat.message",
							Payload: map[string]interface{}{
								"id":              m.ID,
								"senderEmail":     m.SenderEmail,
								"senderHandle":    m.SenderHandle,
								"senderId":        m.SenderID,
								"recipientEmail":  m.RecipientEmail,
								"recipientHandle": m.RecipientHandle,
								"recipientId":     m.RecipientID,
								"text":            m.Text,
								"isProof":         m.IsProof,
								"goalId":          m.GoalID,
								"goalTitle":       m.GoalTitle,
								"ownerId":         m.OwnerID,
								"images":          m.Images,
								"externalLink":    m.ExternalLink,
								"approved":        m.Approved,
								"time":            m.Time,
								"isOffline":       true,
							},
						})
						deliveredIDs = append(deliveredIDs, m.ID)
					}
					if targetUserID != "" {
						_ = s.MarkMessagesDelivered(context.Background(), targetUserID, deliveredIDs)
					}
					if targetEmail != "" {
						_ = s.MarkMessagesDelivered(context.Background(), targetEmail, deliveredIDs)
					}
					log.Printf("[WebSocket] Delivered %d offline messages to user (userID=%s, email=%s)", len(deliveredIDs), targetUserID, targetEmail)
				}
			}(c.userID, c.email, c, store)
		}

		// Sync all communities the connecting user is registered in
		h.mu.RLock()
		var userCommunities []map[string]interface{}
		for _, rec := range h.commRecords {
			isMember := (c.handle != "" && rec.Members[strings.ToLower(c.handle)]) ||
				(c.email != "" && rec.Members[strings.ToLower(c.email)]) ||
				rec.CreatorID == c.userID
			if isMember && rec.Data != nil {
				userCommunities = append(userCommunities, rec.Data)
			}
		}
		h.mu.RUnlock()

		for _, commData := range userCommunities {
			commID, _ := commData["id"].(string)
			if commID != "" {
				h.JoinCommunity(commID, c)
			}
			c.SendEvent(Event{
				Type: "community.sync",
				Payload: map[string]interface{}{
					"community": commData,
				},
			})
		}

	case "presence.query":
		c.SendEvent(Event{
			Type: "presence.list",
			Payload: map[string]interface{}{
				"onlineHandles": h.GetOnlineHandles(),
			},
		})

	case "chat.message":
		var req struct {
			ID              string      `json:"id"`
			RecipientEmail  string      `json:"recipientEmail"`
			RecipientHandle string      `json:"recipientHandle"`
			RecipientID     string      `json:"recipientId"`
			SenderEmail     string      `json:"senderEmail"`
			SenderHandle    string      `json:"senderHandle"`
			SenderName      string      `json:"senderName"`
			Text            string      `json:"text"`
			IsProof         bool        `json:"isProof"`
			GoalID          string      `json:"goalId"`
			GoalTitle       string      `json:"goalTitle"`
			OwnerID         string      `json:"ownerId"`
			Images          []string    `json:"images"`
			Files           interface{} `json:"files"`
			ExternalLink    string      `json:"externalLink"`
			ReplyTo         interface{} `json:"replyTo"`
		}
		if err := json.Unmarshal(base.Payload, &req); err != nil {
			log.Printf("[WebSocket] Failed to parse chat.message payload: %v", err)
			return
		}

		req.RecipientEmail = strings.TrimSpace(strings.ToLower(req.RecipientEmail))
		req.RecipientHandle = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(req.RecipientHandle)), "@")
		req.RecipientID = strings.TrimSpace(req.RecipientID)
		req.SenderEmail = strings.TrimSpace(strings.ToLower(req.SenderEmail))
		req.SenderHandle = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(req.SenderHandle)), "@")
		req.SenderName = strings.TrimSpace(req.SenderName)
		req.Text = strings.TrimSpace(req.Text)

		// Support if recipient handle was passed in RecipientEmail field as @handle
		if req.RecipientHandle == "" && strings.HasPrefix(req.RecipientEmail, "@") {
			req.RecipientHandle = strings.TrimPrefix(req.RecipientEmail, "@")
		}

		// Security: Sender identity is bound strictly to the authenticated client connection.
		// NEVER trust client-supplied senderEmail, senderHandle, or senderId.
		senderEmail := c.email
		senderHandle := c.handle
		senderName := c.name
		if senderName == "" {
			if senderHandle != "" {
				senderName = "@" + senderHandle
			} else {
				senderName = senderEmail
			}
		}

		// Resolve recipient to immutable UserID if resolver is available
		h.mu.RLock()
		resolver := h.userResolver
		h.mu.RUnlock()

		recipientID := req.RecipientID
		resolvedEmail := req.RecipientEmail
		resolvedHandle := req.RecipientHandle

		if resolver != nil {
			if req.RecipientID != "" {
				if rID, rEmail, rHandle, err := resolver.ResolveUser(context.Background(), req.RecipientID); err == nil && rID != "" {
					recipientID = rID
					resolvedEmail = rEmail
					resolvedHandle = rHandle
				}
			}
			if recipientID == "" && req.RecipientHandle != "" {
				if rID, rEmail, rHandle, err := resolver.ResolveUser(context.Background(), req.RecipientHandle); err == nil && rID != "" {
					recipientID = rID
					resolvedEmail = rEmail
					resolvedHandle = rHandle
				}
			}
			if recipientID == "" && req.RecipientEmail != "" {
				if rID, rEmail, rHandle, err := resolver.ResolveUser(context.Background(), req.RecipientEmail); err == nil && rID != "" {
					recipientID = rID
					resolvedEmail = rEmail
					resolvedHandle = rHandle
				}
			}
		}

		// Fallback: check connected clients by handle or email if still unresolved
		if recipientID == "" {
			h.mu.RLock()
			if req.RecipientHandle != "" {
				cleanH := strings.TrimPrefix(strings.ToLower(req.RecipientHandle), "@")
				for cl := range h.handles[cleanH] {
					if cl.userID != "" {
						recipientID = cl.userID
						resolvedEmail = cl.email
						resolvedHandle = cl.handle
						break
					}
				}
			}
			if recipientID == "" && req.RecipientEmail != "" {
				for cl := range h.emails[strings.ToLower(req.RecipientEmail)] {
					if cl.userID != "" {
						recipientID = cl.userID
						resolvedEmail = cl.email
						resolvedHandle = cl.handle
						break
					}
				}
			}
			h.mu.RUnlock()
		}

		// Fail closed: if recipient cannot be resolved, reject cleanly
		if recipientID == "" {
			log.Printf("[WebSocket] chat.message: unresolvable recipient (email=%q, handle=%q, id=%q) from sender %s",
				req.RecipientEmail, req.RecipientHandle, req.RecipientID, c.userID)
			return
		}

		// If this is a proof submission, ensure ownerId is bound to the authenticated user ID
		ownerID := req.OwnerID
		if req.IsProof {
			ownerID = c.userID
		}

		if (resolvedEmail == "" && resolvedHandle == "" && recipientID == "") || (req.Text == "" && !req.IsProof && len(req.Images) == 0) {
			log.Printf("[WebSocket] Invalid chat message: recipientID=%q, recipientEmail=%q, recipientHandle=%q, text=%q", recipientID, resolvedEmail, resolvedHandle, req.Text)
			return
		}

		log.Printf("[WebSocket] Chat from (%s/@%s/%s) to (%s/@%s/%s): %q (proof=%v)", senderEmail, senderHandle, senderName, resolvedEmail, resolvedHandle, recipientID, req.Text, req.IsProof)

		now := time.Now().UTC().Format(time.RFC3339)
		msgID := strings.TrimSpace(req.ID)
		if msgID == "" {
			msgID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
		}

		outEvent := Event{
			Type: "chat.message",
			Payload: map[string]interface{}{
				"id":              msgID,
				"senderEmail":     senderEmail,
				"senderHandle":    senderHandle,
				"senderName":      senderName,
				"senderId":        c.userID,
				"recipientEmail":  resolvedEmail,
				"recipientHandle": resolvedHandle,
				"recipientId":     recipientID,
				"text":            req.Text,
				"isProof":         req.IsProof,
				"goalId":          req.GoalID,
				"goalTitle":       req.GoalTitle,
				"ownerId":         ownerID,
				"images":          req.Images,
				"files":           req.Files,
				"externalLink":    req.ExternalLink,
				"replyTo":         req.ReplyTo,
				"time":            now,
			},
		}

		// Forward directly to recipient's active live connection across all tabs/devices
		// Bound to immutable recipientID when available to prevent delivery to recycled handles.
		delivered := false
		targetClients := make(map[*Client]bool)

		h.mu.RLock()
		if recipientID != "" {
			for client := range h.users[recipientID] {
				targetClients[client] = true
			}
		} else {
			if resolvedHandle != "" {
				for client := range h.handles[resolvedHandle] {
					targetClients[client] = true
				}
			}
			if resolvedEmail != "" {
				for client := range h.emails[resolvedEmail] {
					targetClients[client] = true
				}
			}
		}
		h.mu.RUnlock()

		data, err := json.Marshal(outEvent)
		if err == nil {
			for target := range targetClients {
				select {
				case target.send <- data:
					delivered = true
				default:
					log.Printf("[WebSocket] Buffer full for recipient client (%s/@%s)", target.email, target.handle)
				}
			}
		}

		if delivered {
			log.Printf("[WebSocket] Live delivered message to recipient (%s/@%s/%s) on %d active connection(s)", resolvedEmail, resolvedHandle, recipientID, len(targetClients))
		}

		// Persist message in store if configured
		h.mu.RLock()
		store := h.msgStore
		h.mu.RUnlock()

		if store != nil {
			stored := StoredMessage{
				ID:              msgID,
				SenderEmail:     senderEmail,
				SenderHandle:    senderHandle,
				SenderID:        c.userID,
				RecipientEmail:  resolvedEmail,
				RecipientHandle: resolvedHandle,
				RecipientID:     recipientID,
				Text:            req.Text,
				IsProof:         req.IsProof,
				GoalID:          req.GoalID,
				GoalTitle:       req.GoalTitle,
				OwnerID:         ownerID,
				Images:          req.Images,
				Files:           req.Files,
				ExternalLink:    req.ExternalLink,
				ReplyTo:         req.ReplyTo,
				Time:            now,
				Delivered:       delivered,
			}
			if err := store.SaveMessage(context.Background(), stored, delivered); err != nil {
				log.Printf("[WebSocket] Failed to persist chat message: %v", err)
			}
		}

		if !delivered {
			target := resolvedHandle
			if target == "" {
				target = resolvedEmail
			}
			if target == "" {
				target = recipientID
			}
			log.Printf("[WebSocket] Recipient %s is offline. Message saved in DynamoDB for delivery on login.", target)
		}

	case "chat.delete":
		var req struct {
			ID              string `json:"id"`
			RecipientEmail  string `json:"recipientEmail"`
			RecipientHandle string `json:"recipientHandle"`
			RecipientID     string `json:"recipientId"`
		}
		if err := json.Unmarshal(base.Payload, &req); err == nil && req.ID != "" {
			req.RecipientEmail = strings.TrimSpace(strings.ToLower(req.RecipientEmail))
			req.RecipientHandle = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(req.RecipientHandle)), "@")
			req.RecipientID = strings.TrimSpace(req.RecipientID)
			if req.RecipientHandle == "" && strings.HasPrefix(req.RecipientEmail, "@") {
				req.RecipientHandle = strings.TrimPrefix(req.RecipientEmail, "@")
			}

			// Security: Require sender authorization for message deletion against persistent store
			h.mu.RLock()
			store := h.msgStore
			resolver := h.userResolver
			h.mu.RUnlock()

			if store != nil {
				if err := store.DeleteMessage(context.Background(), c.email, c.handle, c.userID, req.ID); err != nil {
					log.Printf("[WebSocket] User %s unauthorized to delete chat message %s: %v", c.userID, req.ID, err)
					c.SendEvent(Event{
						Type: "chat.error",
						Payload: map[string]interface{}{
							"error": "unauthorized: cannot delete this message",
							"id":    req.ID,
						},
					})
					return
				}
			}

			// Resolve recipient ID if not directly provided
			if req.RecipientID == "" && resolver != nil {
				target := req.RecipientHandle
				if target == "" {
					target = req.RecipientEmail
				}
				if target != "" {
					if rID, _, _, err := resolver.ResolveUser(context.Background(), target); err == nil {
						req.RecipientID = rID
					}
				}
			}

			delEvent := Event{
				Type: "chat.delete",
				Payload: map[string]interface{}{
					"id": req.ID,
				},
			}
			if req.RecipientID != "" {
				_ = h.SendToUser(req.RecipientID, delEvent)
			}

			log.Printf("[WebSocket] Chat message deleted: id=%s by sender %s (notified recipient %s)", req.ID, c.userID, req.RecipientID)
		}

	case "chat.message.approved":
		var req struct {
			MessageID string `json:"messageId"`
			GoalID    string `json:"goalId"`
			GoalTitle string `json:"goalTitle"`
		}
		if err := json.Unmarshal(base.Payload, &req); err == nil && req.GoalID != "" {
			h.mu.RLock()
			verifier := h.goalVerifier
			h.mu.RUnlock()

			if verifier == nil {
				c.SendEvent(Event{
					Type: "approval.error",
					Payload: map[string]interface{}{
						"error":  "unauthorized: approval broadcast requires server verification",
						"goalId": req.GoalID,
					},
				})
				log.Printf("[WebSocket] Client %s attempted approval broadcast without goal verifier configured", c.userID)
				return
			}

			ownerID, goalTitle, ok, err := verifier.VerifyApprovedGoal(context.Background(), c.userID, c.email, c.handle, req.GoalID)
			if err != nil || !ok {
				c.SendEvent(Event{
					Type: "approval.error",
					Payload: map[string]interface{}{
						"error":  "forbidden: you are not authorized to broadcast approval for this goal",
						"goalId": req.GoalID,
					},
				})
				log.Printf("[WebSocket] User %s blocked from broadcasting unverified approval for goal %s (ok=%v, err=%v)", c.userID, req.GoalID, ok, err)
				return
			}

			approverName := c.name
			if approverName == "" {
				if c.handle != "" {
					approverName = "@" + c.handle
				} else {
					approverName = c.email
				}
			}
			if goalTitle == "" {
				goalTitle = req.GoalTitle
			}

			approveEvent := Event{
				Type: "chat.message.approved",
				Payload: map[string]interface{}{
					"messageId":    req.MessageID,
					"goalId":       req.GoalID,
					"goalTitle":    goalTitle,
					"ownerId":      ownerID,
					"approverName": approverName,
					"approverId":   c.userID,
				},
			}
			h.Broadcast(approveEvent)
		}

	case "community.register":
		var req struct {
			Community map[string]interface{} `json:"community"`
			Members   []struct {
				Handle string `json:"handle"`
				Email  string `json:"email"`
				Name   string `json:"name"`
			} `json:"members"`
		}
		if err := json.Unmarshal(base.Payload, &req); err == nil && req.Community != nil {
			commID, _ := req.Community["id"].(string)
			commName, _ := req.Community["name"].(string)
			commCode, _ := req.Community["code"].(string)
			cleanID := strings.TrimSpace(commID)
			cleanCode := strings.ToUpper(strings.TrimSpace(commCode))

			if cleanID != "" {
				h.mu.Lock()
				rec, exists := h.commRecords[cleanID]
				if exists {
					// Security: Only the original creator can update community metadata or invited members
					if rec.CreatorID != "" && rec.CreatorID != c.userID {
						h.mu.Unlock()
						c.SendEvent(Event{
							Type: "community.error",
							Payload: map[string]interface{}{
								"error":       "unauthorized: only the creator can update community settings",
								"communityId": cleanID,
							},
						})
						log.Printf("[WebSocket] User %s blocked from overwriting community %s created by %s", c.userID, cleanID, rec.CreatorID)
						return
					}
					rec.Data = req.Community
				} else {
					rec = &CommunityRecord{
						ID:        cleanID,
						Name:      commName,
						Code:      cleanCode,
						CreatorID: c.userID,
						Data:      req.Community,
						Members:   make(map[string]bool),
					}
					h.commRecords[cleanID] = rec
				}

				if cleanCode != "" {
					h.commByCode[cleanCode] = cleanID
				}

				// Always record creator as member
				if c.handle != "" {
					rec.Members[strings.ToLower(c.handle)] = true
				}
				if c.email != "" {
					rec.Members[strings.ToLower(c.email)] = true
				}

				// Record all invited members
				var targetsToNotify []string
				for _, m := range req.Members {
					cleanH := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(m.Handle)), "@")
					cleanE := strings.TrimSpace(strings.ToLower(m.Email))
					if cleanH != "" {
						rec.Members[cleanH] = true
						targetsToNotify = append(targetsToNotify, "@"+cleanH)
					}
					if cleanE != "" {
						rec.Members[cleanE] = true
						targetsToNotify = append(targetsToNotify, cleanE)
					}
				}
				h.mu.Unlock()

				// Join creator client to room
				h.JoinCommunity(cleanID, c)

				// Realtime Auto-Join: Forward community.sync directly to each invited friend!
				syncEvent := Event{
					Type: "community.sync",
					Payload: map[string]interface{}{
						"community": req.Community,
					},
				}
				for _, target := range targetsToNotify {
					if strings.HasPrefix(target, "@") {
						_ = h.SendToHandle(strings.TrimPrefix(target, "@"), syncEvent)
					} else {
						_ = h.SendToEmail(target, syncEvent)
					}
				}
				log.Printf("[WebSocket] Community registered: %q (code: %s), auto-synced to %d friends", commName, cleanCode, len(targetsToNotify))
			}
		}

	case "community.join_by_code":
		var req struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(base.Payload, &req); err == nil {
			cleanCode := strings.ToUpper(strings.TrimSpace(req.Code))
			h.mu.Lock()
			commID, exists := h.commByCode[cleanCode]
			var rec *CommunityRecord
			if exists {
				rec = h.commRecords[commID]
			}
			if rec != nil {
				if c.handle != "" {
					rec.Members[strings.ToLower(c.handle)] = true
				}
				if c.email != "" {
					rec.Members[strings.ToLower(c.email)] = true
				}
			}
			h.mu.Unlock()

			if exists && rec != nil {
				h.JoinCommunity(commID, c)
				c.SendEvent(Event{
					Type: "community.join_success",
					Payload: map[string]interface{}{
						"community": rec.Data,
					},
				})
				log.Printf("[WebSocket] User %s successfully joined community %s by code %s", c.userID, rec.Name, cleanCode)
			} else {
				c.SendEvent(Event{
					Type: "community.join_error",
					Payload: map[string]interface{}{
						"code":  cleanCode,
						"error": fmt.Sprintf("No community found with invite code %q.", cleanCode),
					},
				})
				log.Printf("[WebSocket] Code lookup failed for code %s", cleanCode)
			}
		}

	case "community.join_room":
		var req struct {
			CommunityID string `json:"communityId"`
		}
		if err := json.Unmarshal(base.Payload, &req); err == nil && req.CommunityID != "" {
			cleanID := strings.TrimSpace(req.CommunityID)
			h.mu.RLock()
			rec, recExists := h.commRecords[cleanID]
			h.mu.RUnlock()

			if !recExists {
				c.SendEvent(Event{
					Type: "community.error",
					Payload: map[string]interface{}{
						"error":       "community not found or unauthorized",
						"communityId": cleanID,
					},
				})
				log.Printf("[WebSocket] User %s blocked from joining nonexistent/unregistered community %s", c.userID, cleanID)
				return
			}

			cleanH := strings.TrimPrefix(strings.ToLower(c.handle), "@")
			cleanE := strings.ToLower(c.email)

			// Allow any authenticated user to join public/trending communities
			if isPublicTrendingCommunity(cleanID) || rec.CreatorID == "" {
				h.mu.Lock()
				if cleanH != "" {
					rec.Members[cleanH] = true
					rec.Members["@"+cleanH] = true
				}
				if cleanE != "" {
					rec.Members[cleanE] = true
				}
				h.mu.Unlock()
			}

			isMember := c.userID == rec.CreatorID ||
				rec.CreatorID == "" ||
				isPublicTrendingCommunity(cleanID) ||
				(cleanH != "" && (rec.Members[cleanH] || rec.Members["@"+cleanH])) ||
				(cleanE != "" && rec.Members[cleanE])

			if !isMember {
				c.SendEvent(Event{
					Type: "community.error",
					Payload: map[string]interface{}{
						"error":       "unauthorized: not a member of this community",
						"communityId": cleanID,
					},
				})
				log.Printf("[WebSocket] User %s blocked from joining unauthorized community %s", c.userID, cleanID)
				return
			}

			h.JoinCommunity(cleanID, c)
		}

	case "community.leave_room":
		var req struct {
			CommunityID string `json:"communityId"`
		}
		if err := json.Unmarshal(base.Payload, &req); err == nil && req.CommunityID != "" {
			h.LeaveCommunity(req.CommunityID, c)
		}

	case "community.message":
		var req struct {
			ID            string      `json:"id"`
			CommunityID   string      `json:"communityId"`
			CommunityName string      `json:"communityName"`
			CommunityCode string      `json:"communityCode"`
			SenderName    string      `json:"senderName"`
			SenderEmail   string      `json:"senderEmail"`
			SenderHandle  string      `json:"senderHandle"`
			Text          string      `json:"text"`
			IsProof       bool        `json:"isProof"`
			GoalID        string      `json:"goalId"`
			GoalTitle     string      `json:"goalTitle"`
			Images        []string    `json:"images"`
			Files         interface{} `json:"files"`
			ExternalLink  string      `json:"externalLink"`
			ReplyTo       interface{} `json:"replyTo"`
			Time          string      `json:"time"`
		}
		if err := json.Unmarshal(base.Payload, &req); err != nil || strings.TrimSpace(req.CommunityID) == "" {
			log.Printf("[WebSocket] Failed to parse community.message payload: %v", err)
			return
		}

		cleanCommID := strings.TrimSpace(req.CommunityID)

		// Verify sender is authorized for this community before broadcasting
		h.mu.RLock()
		rec, recExists := h.commRecords[cleanCommID]
		h.mu.RUnlock()

		if !recExists {
			c.SendEvent(Event{
				Type: "community.error",
				Payload: map[string]interface{}{
					"error":       "community not found or unauthorized",
					"communityId": cleanCommID,
				},
			})
			log.Printf("[WebSocket] User %s blocked from broadcasting to nonexistent/unregistered community %s", c.userID, cleanCommID)
			return
		}

		cleanH := strings.TrimPrefix(strings.ToLower(c.handle), "@")
		cleanE := strings.ToLower(c.email)
		isMember := c.userID == rec.CreatorID ||
			(cleanH != "" && (rec.Members[cleanH] || rec.Members["@"+cleanH])) ||
			(cleanE != "" && rec.Members[cleanE])

		if !isMember {
			c.SendEvent(Event{
				Type: "community.error",
				Payload: map[string]interface{}{
					"error":       "unauthorized: cannot broadcast to a community you are not a member of",
					"communityId": cleanCommID,
				},
			})
			log.Printf("[WebSocket] User %s blocked from broadcasting to unauthorized community %s", c.userID, cleanCommID)
			return
		}
		// Security: Derive sender identity strictly from the authenticated connection
		senderEmail := c.email
		senderHandle := c.handle
		senderName := c.name
		if senderName == "" {
			if senderHandle != "" {
				senderName = "@" + senderHandle
			} else {
				senderName = senderEmail
			}
		}

		cleanText := strings.TrimSpace(req.Text)
		if cleanText == "" && !req.IsProof && len(req.Images) == 0 {
			c.SendEvent(Event{
				Type: "community.error",
				Payload: map[string]interface{}{
					"error":       "empty message content",
					"communityId": cleanCommID,
				},
			})
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		if req.Time != "" {
			now = req.Time
		}

		rawID := strings.TrimSpace(req.ID)
		var msgID string

		h.mu.Lock()
		if rawID == "" {
			// Generate server-side unique ID
			for {
				candidate := generateCommunityMessageID()
				if _, exists := h.commMsgAuthors[cleanCommID+"#"+candidate]; !exists {
					msgID = candidate
					break
				}
			}
		} else {
			// Client provided an ID: check for collision and reject duplicate IDs
			key := cleanCommID + "#" + rawID
			if _, exists := h.commMsgAuthors[key]; exists {
				h.mu.Unlock()
				c.SendEvent(Event{
					Type: "community.error",
					Payload: map[string]interface{}{
						"error":       "duplicate or colliding message ID",
						"communityId": cleanCommID,
						"messageId":   rawID,
					},
				})
				log.Printf("[WebSocket] Rejected colliding community.message with ID %q from user %s in community %s", rawID, c.userID, cleanCommID)
				return
			}
			msgID = rawID
		}

		h.commMsgAuthors[cleanCommID+"#"+msgID] = c.userID
		h.mu.Unlock()

		outEvent := Event{
			Type: "community.message",
			Payload: map[string]interface{}{
				"id":            msgID,
				"communityId":   cleanCommID,
				"communityName": req.CommunityName,
				"communityCode": req.CommunityCode,
				"senderName":    senderName,
				"senderHandle":  senderHandle,
				"senderEmail":   senderEmail,
				"senderId":      c.userID,
				"text":          strings.TrimSpace(req.Text),
				"isProof":       req.IsProof,
				"goalId":        req.GoalID,
				"goalTitle":     req.GoalTitle,
				"images":        req.Images,
				"files":         req.Files,
				"externalLink":  req.ExternalLink,
				"replyTo":       req.ReplyTo,
				"time":          now,
			},
		}

		// Broadcast to all active community room listeners except the sender client
		_ = h.SendToCommunityExcept(cleanCommID, c, outEvent)

		// Direct forward to all registered members who may not currently be in the room
		cleanCHandle := strings.TrimPrefix(strings.ToLower(senderHandle), "@")
		cleanCEmail := strings.ToLower(senderEmail)

		h.mu.RLock()
		rec = h.commRecords[cleanCommID]
		var memberTargets []string
		if rec != nil {
			for target := range rec.Members {
				cleanTarget := strings.TrimPrefix(strings.ToLower(target), "@")
				if cleanTarget != cleanCHandle && cleanTarget != cleanCEmail {
					memberTargets = append(memberTargets, target)
				}
			}
		}
		h.mu.RUnlock()

		for _, member := range memberTargets {
			if strings.Contains(member, "@") && strings.Contains(member, ".") {
				_ = h.SendToEmail(member, outEvent)
			} else {
				_ = h.SendToHandle(member, outEvent)
			}
		}

	case "community.delete_message":
		var req struct {
			CommunityID string `json:"communityId"`
			MessageID   string `json:"messageId"`
		}
		if err := json.Unmarshal(base.Payload, &req); err == nil && req.CommunityID != "" && req.MessageID != "" {
			cleanCommID := strings.TrimSpace(req.CommunityID)

			// Authorization: Verify caller is a member of the community
			h.mu.RLock()
			rec, recExists := h.commRecords[cleanCommID]
			h.mu.RUnlock()

			if !recExists {
				c.SendEvent(Event{
					Type: "community.error",
					Payload: map[string]interface{}{
						"error":       "community not found or unauthorized",
						"communityId": cleanCommID,
					},
				})
				log.Printf("[WebSocket] User %s blocked from deleting message in nonexistent/unregistered community %s", c.userID, cleanCommID)
				return
			}

			cleanH := strings.TrimPrefix(strings.ToLower(c.handle), "@")
			cleanE := strings.ToLower(c.email)

			// Allow any authenticated user to join public/trending communities
			if isPublicTrendingCommunity(cleanCommID) || rec.CreatorID == "" {
				h.mu.Lock()
				if cleanH != "" {
					rec.Members[cleanH] = true
					rec.Members["@"+cleanH] = true
				}
				if cleanE != "" {
					rec.Members[cleanE] = true
				}
				h.mu.Unlock()
			}

			isMember := c.userID == rec.CreatorID ||
				rec.CreatorID == "" ||
				isPublicTrendingCommunity(cleanCommID) ||
				(cleanH != "" && (rec.Members[cleanH] || rec.Members["@"+cleanH])) ||
				(cleanE != "" && rec.Members[cleanE])

			if !isMember {
				c.SendEvent(Event{
					Type: "community.error",
					Payload: map[string]interface{}{
						"error":       "unauthorized: not a member of this community",
						"communityId": cleanCommID,
					},
				})
				log.Printf("[WebSocket] User %s blocked from deleting message in unauthorized community %s", c.userID, cleanCommID)
				return
			}

			// Authorization: Only community creator or original message author may delete community messages
			h.mu.RLock()
			authorID := h.commMsgAuthors[cleanCommID+"#"+req.MessageID]
			h.mu.RUnlock()

			isCreator := c.userID == rec.CreatorID
			isAuthor := authorID != "" && c.userID == authorID

			if !isCreator && !isAuthor {
				c.SendEvent(Event{
					Type: "community.error",
					Payload: map[string]interface{}{
						"error":       "unauthorized: only the community creator or original message author may delete this message",
						"communityId": cleanCommID,
					},
				})
				log.Printf("[WebSocket] User %s blocked from deleting message %s in community %s (not creator or author)", c.userID, req.MessageID, cleanCommID)
				return
			}

			h.mu.Lock()
			delete(h.commMsgAuthors, cleanCommID+"#"+req.MessageID)
			h.mu.Unlock()

			_ = h.SendToCommunity(cleanCommID, Event{
				Type: "community.delete_message",
				Payload: map[string]interface{}{
					"communityId": cleanCommID,
					"messageId":   req.MessageID,
				},
			})
		}
	}
}

// GetOnlineEmails returns a list of all emails with currently active connections.
func (h *Hub) GetOnlineEmails() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var list []string
	for email, conns := range h.emails {
		if len(conns) > 0 {
			list = append(list, email)
		}
	}
	return list
}

// GetOnlineHandles returns a list of all handles with currently active connections.
func (h *Hub) GetOnlineHandles() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var list []string
	for handle, conns := range h.handles {
		if len(conns) > 0 {
			list = append(list, handle)
		}
	}
	return list
}

// IsUserOnline returns true if the user ID has at least one active connection.
func (h *Hub) IsUserOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	conns, exists := h.users[userID]
	return exists && len(conns) > 0
}

// IsEmailOnline returns true if the email has at least one active connection.
func (h *Hub) IsEmailOnline(email string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	normalized := strings.TrimSpace(strings.ToLower(email))
	conns, exists := h.emails[normalized]
	return exists && len(conns) > 0
}

// IsHandleOnline returns true if the handle has at least one active connection.
func (h *Hub) IsHandleOnline(handle string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	normalized := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(handle)), "@")
	conns, exists := h.handles[normalized]
	return exists && len(conns) > 0
}

// ActiveConnections returns the number of open connections for a given user.
func (h *Hub) ActiveConnections(userID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if conns, exists := h.users[userID]; exists {
		return len(conns)
	}
	return 0
}
