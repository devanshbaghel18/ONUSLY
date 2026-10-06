package realtime

import (
	"context"
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
	ID              string `json:"id" dynamodbav:"ID"`
	SenderEmail     string `json:"senderEmail" dynamodbav:"SenderEmail"`
	SenderHandle    string `json:"senderHandle,omitempty" dynamodbav:"SenderHandle,omitempty"`
	SenderID        string `json:"senderId" dynamodbav:"SenderID"`
	RecipientEmail  string `json:"recipientEmail" dynamodbav:"RecipientEmail"`
	RecipientHandle string `json:"recipientHandle,omitempty" dynamodbav:"RecipientHandle,omitempty"`
	Text            string `json:"text" dynamodbav:"Text"`
	Time            string `json:"time" dynamodbav:"Time"`
	Delivered       bool   `json:"delivered" dynamodbav:"Delivered"`
}

// MessageStore is an optional persistent storage layer for chat messages.
type MessageStore interface {
	SaveMessage(ctx context.Context, msg StoredMessage, isRecipientOnline bool) error
	GetPendingMessages(ctx context.Context, recipientEmail string) ([]StoredMessage, error)
	MarkMessagesDelivered(ctx context.Context, recipientEmail string, msgIDs []string) error
	DeleteMessage(ctx context.Context, msgID string) error
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

	// msgStore is an optional persistent message store for offline delivery
	msgStore MessageStore
}

// NewHub initializes and returns a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		users:       make(map[string]map[*Client]bool),
		emails:      make(map[string]map[*Client]bool),
		handles:     make(map[string]map[*Client]bool),
		communities: make(map[string]map[*Client]bool),
		commRecords: make(map[string]*CommunityRecord),
		commByCode:  make(map[string]string),
	}
}

// SetMessageStore attaches a persistent MessageStore to the Hub.
func (h *Hub) SetMessageStore(store MessageStore) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgStore = store
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
		if err := json.Unmarshal(base.Payload, &req); err == nil {
			normalizedEmail := strings.TrimSpace(strings.ToLower(req.Email))
			cleanHandle := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(req.Handle)), "@")

			h.mu.Lock()
			changed := false
			if normalizedEmail != "" && c.email != normalizedEmail {
				c.email = normalizedEmail
				emailConns, exists := h.emails[normalizedEmail]
				if !exists {
					emailConns = make(map[*Client]bool)
					h.emails[normalizedEmail] = emailConns
				}
				emailConns[c] = true
				changed = true
			}

			if cleanHandle != "" && c.handle != cleanHandle {
				if c.handle != "" {
					if oldConns, exists := h.handles[c.handle]; exists {
						delete(oldConns, c)
						if len(oldConns) == 0 {
							delete(h.handles, c.handle)
						}
					}
				}
				c.handle = cleanHandle
				handleConns, exists := h.handles[cleanHandle]
				if !exists {
					handleConns = make(map[*Client]bool)
					h.handles[cleanHandle] = handleConns
				}
				handleConns[c] = true
				changed = true
			}
			store := h.msgStore
			h.mu.Unlock()

			c.mu.Lock()
			shouldDeliver := !c.deliveredOffline && store != nil
			if shouldDeliver {
				c.deliveredOffline = true
			}
			c.mu.Unlock()

			if changed || shouldDeliver {
				log.Printf("[WebSocket] Presence confirmed: user %s registered email=%s handle=@%s", c.userID, c.email, c.handle)
			}

			// Deliver any stored offline messages to this user once per connection
			if shouldDeliver {
				go func(targetEmail, targetHandle string, cl *Client, s MessageStore) {
					var allPending []StoredMessage
					if targetEmail != "" {
						if pending, err := s.GetPendingMessages(context.Background(), targetEmail); err == nil {
							allPending = append(allPending, pending...)
						}
					}
					if targetHandle != "" {
						if pendingH, err := s.GetPendingMessages(context.Background(), "@"+targetHandle); err == nil {
							allPending = append(allPending, pendingH...)
						}
					}

					if len(allPending) > 0 {
						var deliveredIDs []string
						seen := make(map[string]bool)
						for _, m := range allPending {
							if seen[m.ID] {
								continue
							}
							seen[m.ID] = true
							cl.SendEvent(Event{
								Type: "chat.message",
								Payload: map[string]interface{}{
									"id":              m.ID,
									"senderEmail":     m.SenderEmail,
									"senderHandle":    m.SenderHandle,
									"senderId":        m.SenderID,
									"recipientEmail":  m.RecipientEmail,
									"recipientHandle": m.RecipientHandle,
									"text":            m.Text,
									"time":            m.Time,
									"isOffline":       true,
								},
							})
							deliveredIDs = append(deliveredIDs, m.ID)
						}
						if targetEmail != "" {
							_ = s.MarkMessagesDelivered(context.Background(), targetEmail, deliveredIDs)
						}
						if targetHandle != "" {
							_ = s.MarkMessagesDelivered(context.Background(), "@"+targetHandle, deliveredIDs)
						}
						log.Printf("[WebSocket] Delivered %d offline messages to user (email=%s, handle=@%s)", len(deliveredIDs), targetEmail, targetHandle)
					}
				}(c.email, c.handle, c, store)
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
		}

	case "presence.query":
		c.SendEvent(Event{
			Type: "presence.list",
			Payload: map[string]interface{}{
				"onlineEmails":  h.GetOnlineEmails(),
				"onlineHandles": h.GetOnlineHandles(),
			},
		})

	case "chat.message":
		var req struct {
			RecipientEmail  string   `json:"recipientEmail"`
			RecipientHandle string   `json:"recipientHandle"`
			SenderEmail     string   `json:"senderEmail"`
			SenderHandle    string   `json:"senderHandle"`
			SenderName      string   `json:"senderName"`
			Text            string   `json:"text"`
			IsProof         bool     `json:"isProof"`
			GoalID          string   `json:"goalId"`
			GoalTitle       string   `json:"goalTitle"`
			Images          []string `json:"images"`
			ExternalLink    string   `json:"externalLink"`
		}
		if err := json.Unmarshal(base.Payload, &req); err != nil {
			log.Printf("[WebSocket] Failed to parse chat.message payload: %v", err)
			return
		}

		req.RecipientEmail = strings.TrimSpace(strings.ToLower(req.RecipientEmail))
		req.RecipientHandle = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(req.RecipientHandle)), "@")
		req.SenderEmail = strings.TrimSpace(strings.ToLower(req.SenderEmail))
		req.SenderHandle = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(req.SenderHandle)), "@")
		req.SenderName = strings.TrimSpace(req.SenderName)
		req.Text = strings.TrimSpace(req.Text)

		// Support if recipient handle was passed in RecipientEmail field as @handle
		if req.RecipientHandle == "" && strings.HasPrefix(req.RecipientEmail, "@") {
			req.RecipientHandle = strings.TrimPrefix(req.RecipientEmail, "@")
		}

		senderEmail := c.email
		if senderEmail == "" {
			senderEmail = req.SenderEmail
		}

		senderHandle := c.handle
		if req.SenderHandle != "" {
			senderHandle = req.SenderHandle
			if c.handle != senderHandle {
				h.mu.Lock()
				c.handle = senderHandle
				handleConns, exists := h.handles[senderHandle]
				if !exists {
					handleConns = make(map[*Client]bool)
					h.handles[senderHandle] = handleConns
				}
				handleConns[c] = true
				h.mu.Unlock()
			}
		}

		senderName := req.SenderName
		if senderName == "" {
			senderName = senderHandle
			if senderName == "" {
				senderName = senderEmail
			}
		}

		if (req.RecipientEmail == "" && req.RecipientHandle == "") || (req.Text == "" && !req.IsProof && len(req.Images) == 0) {
			log.Printf("[WebSocket] Invalid chat message: recipientEmail=%q, recipientHandle=%q, text=%q", req.RecipientEmail, req.RecipientHandle, req.Text)
			return
		}

		log.Printf("[WebSocket] Chat from (%s/@%s/%s) to (%s/@%s): %q (proof=%v)", senderEmail, senderHandle, senderName, req.RecipientEmail, req.RecipientHandle, req.Text, req.IsProof)

		now := time.Now().UTC().Format(time.RFC3339)
		msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

		outEvent := Event{
			Type: "chat.message",
			Payload: map[string]interface{}{
				"id":              msgID,
				"senderEmail":     senderEmail,
				"senderHandle":    senderHandle,
				"senderName":      senderName,
				"senderId":        c.userID,
				"recipientEmail":  req.RecipientEmail,
				"recipientHandle": req.RecipientHandle,
				"text":            req.Text,
				"isProof":         req.IsProof,
				"goalId":          req.GoalID,
				"goalTitle":       req.GoalTitle,
				"images":          req.Images,
				"externalLink":    req.ExternalLink,
				"time":            now,
			},
		}

		// Forward directly to recipient's active live connection
		delivered := false
		if req.RecipientHandle != "" {
			if err := h.SendToHandle(req.RecipientHandle, outEvent); err == nil {
				delivered = true
				log.Printf("[WebSocket] Live delivered message to handle @%s", req.RecipientHandle)
			}
		}
		if !delivered && req.RecipientEmail != "" {
			if err := h.SendToEmail(req.RecipientEmail, outEvent); err == nil {
				delivered = true
				log.Printf("[WebSocket] Live delivered message to email %s", req.RecipientEmail)
			}
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
				RecipientEmail:  req.RecipientEmail,
				RecipientHandle: req.RecipientHandle,
				Text:            req.Text,
				Time:            now,
				Delivered:       delivered,
			}
			if err := store.SaveMessage(context.Background(), stored, delivered); err != nil {
				log.Printf("[WebSocket] Failed to persist chat message: %v", err)
			}
		}

		if !delivered {
			target := req.RecipientHandle
			if target == "" {
				target = req.RecipientEmail
			}
			log.Printf("[WebSocket] Recipient %s is offline. Message saved in DynamoDB for delivery on login.", target)
		}

	case "chat.delete":
		var req struct {
			ID              string `json:"id"`
			RecipientEmail  string `json:"recipientEmail"`
			RecipientHandle string `json:"recipientHandle"`
		}
		if err := json.Unmarshal(base.Payload, &req); err == nil && req.ID != "" {
			req.RecipientEmail = strings.TrimSpace(strings.ToLower(req.RecipientEmail))
			req.RecipientHandle = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(req.RecipientHandle)), "@")
			if req.RecipientHandle == "" && strings.HasPrefix(req.RecipientEmail, "@") {
				req.RecipientHandle = strings.TrimPrefix(req.RecipientEmail, "@")
			}
			delEvent := Event{
				Type: "chat.delete",
				Payload: map[string]interface{}{
					"id": req.ID,
				},
			}
			if req.RecipientHandle != "" {
				_ = h.SendToHandle(req.RecipientHandle, delEvent)
			}
			if req.RecipientEmail != "" {
				_ = h.SendToEmail(req.RecipientEmail, delEvent)
			}

			// Also delete from persistent store if configured
			h.mu.RLock()
			store := h.msgStore
			h.mu.RUnlock()
			if store != nil {
				go func(msgID string, s MessageStore) {
					_ = s.DeleteMessage(context.Background(), msgID)
				}(req.ID, store)
			}
			log.Printf("[WebSocket] Chat message deleted: id=%s (notified recipient @%s / %s)", req.ID, req.RecipientHandle, req.RecipientEmail)
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
				if !exists {
					rec = &CommunityRecord{
						ID:        cleanID,
						Name:      commName,
						Code:      cleanCode,
						CreatorID: c.userID,
						Data:      req.Community,
						Members:   make(map[string]bool),
					}
					h.commRecords[cleanID] = rec
				} else {
					rec.Data = req.Community
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
			h.JoinCommunity(req.CommunityID, c)
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
			ID            string   `json:"id"`
			CommunityID   string   `json:"communityId"`
			CommunityName string   `json:"communityName"`
			CommunityCode string   `json:"communityCode"`
			SenderName    string   `json:"senderName"`
			SenderEmail   string   `json:"senderEmail"`
			SenderHandle  string   `json:"senderHandle"`
			Text          string   `json:"text"`
			IsProof       bool     `json:"isProof"`
			GoalID        string   `json:"goalId"`
			GoalTitle     string   `json:"goalTitle"`
			Images        []string `json:"images"`
			ExternalLink  string   `json:"externalLink"`
			Time          string   `json:"time"`
		}
		if err := json.Unmarshal(base.Payload, &req); err != nil || strings.TrimSpace(req.CommunityID) == "" {
			log.Printf("[WebSocket] Failed to parse community.message payload: %v", err)
			return
		}

		cleanCommID := strings.TrimSpace(req.CommunityID)
		senderEmail := c.email
		if senderEmail == "" {
			senderEmail = req.SenderEmail
		}
		senderHandle := c.handle
		if senderHandle == "" {
			senderHandle = req.SenderHandle
		}
		senderName := req.SenderName
		if senderName == "" {
			senderName = senderHandle
			if senderName == "" {
				senderName = senderEmail
			}
		}

		now := time.Now().UTC().Format(time.RFC3339)
		if req.Time != "" {
			now = req.Time
		}
		msgID := strings.TrimSpace(req.ID)
		if msgID == "" {
			msgID = fmt.Sprintf("comm-msg-%d", time.Now().UnixNano())
		}

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
				"externalLink":  req.ExternalLink,
				"time":          now,
			},
		}

		// Broadcast to all active community room listeners except the sender client
		_ = h.SendToCommunityExcept(cleanCommID, c, outEvent)

		// Direct forward to all registered members who may not currently be in the room
		cleanCHandle := strings.TrimPrefix(strings.ToLower(senderHandle), "@")
		cleanCEmail := strings.ToLower(senderEmail)

		h.mu.RLock()
		rec := h.commRecords[cleanCommID]
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
			_ = h.SendToCommunity(strings.TrimSpace(req.CommunityID), Event{
				Type: "community.delete_message",
				Payload: map[string]interface{}{
					"communityId": req.CommunityID,
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

