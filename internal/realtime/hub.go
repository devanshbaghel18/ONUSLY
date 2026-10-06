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
}

// Event represents a standard real-time message payload sent to clients.
type Event struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
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

	// msgStore is an optional persistent message store for offline delivery
	msgStore MessageStore
}

// NewHub initializes and returns a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		users:   make(map[string]map[*Client]bool),
		emails:  make(map[string]map[*Client]bool),
		handles: make(map[string]map[*Client]bool),
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

	close(c.send)
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
			if normalizedEmail != "" {
				c.email = normalizedEmail
				emailConns, exists := h.emails[normalizedEmail]
				if !exists {
					emailConns = make(map[*Client]bool)
					h.emails[normalizedEmail] = emailConns
				}
				emailConns[c] = true
			}

			if cleanHandle != "" {
				c.handle = cleanHandle
				handleConns, exists := h.handles[cleanHandle]
				if !exists {
					handleConns = make(map[*Client]bool)
					h.handles[cleanHandle] = handleConns
				}
				handleConns[c] = true
			}
			store := h.msgStore
			h.mu.Unlock()

			log.Printf("[WebSocket] Presence confirmed: user %s registered email=%s handle=@%s", c.userID, c.email, c.handle)

			// Deliver any stored offline messages to this user
			if store != nil {
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
			RecipientEmail  string `json:"recipientEmail"`
			RecipientHandle string `json:"recipientHandle"`
			SenderEmail     string `json:"senderEmail"`
			SenderHandle    string `json:"senderHandle"`
			Text            string `json:"text"`
		}
		if err := json.Unmarshal(base.Payload, &req); err != nil {
			log.Printf("[WebSocket] Failed to parse chat.message payload: %v", err)
			return
		}

		req.RecipientEmail = strings.TrimSpace(strings.ToLower(req.RecipientEmail))
		req.RecipientHandle = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(req.RecipientHandle)), "@")
		req.SenderEmail = strings.TrimSpace(strings.ToLower(req.SenderEmail))
		req.SenderHandle = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(req.SenderHandle)), "@")
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
		if senderHandle == "" {
			senderHandle = req.SenderHandle
		}

		if (req.RecipientEmail == "" && req.RecipientHandle == "") || req.Text == "" {
			log.Printf("[WebSocket] Invalid chat message: recipientEmail=%q, recipientHandle=%q, text=%q", req.RecipientEmail, req.RecipientHandle, req.Text)
			return
		}

		log.Printf("[WebSocket] Chat from (%s/@%s) to (%s/@%s): %q", senderEmail, senderHandle, req.RecipientEmail, req.RecipientHandle, req.Text)

		now := time.Now().UTC().Format(time.RFC3339)
		msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

		outEvent := Event{
			Type: "chat.message",
			Payload: map[string]interface{}{
				"id":              msgID,
				"senderEmail":     senderEmail,
				"senderHandle":    senderHandle,
				"senderId":        c.userID,
				"recipientEmail":  req.RecipientEmail,
				"recipientHandle": req.RecipientHandle,
				"text":            req.Text,
				"time":            now,
			},
		}

		isRecipientOnline := false
		if req.RecipientHandle != "" && h.IsHandleOnline(req.RecipientHandle) {
			isRecipientOnline = true
		} else if req.RecipientEmail != "" && h.IsEmailOnline(req.RecipientEmail) {
			isRecipientOnline = true
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
				Delivered:       isRecipientOnline,
			}
			if err := store.SaveMessage(context.Background(), stored, isRecipientOnline); err != nil {
				log.Printf("[WebSocket] Failed to persist chat message: %v", err)
			}
		}

		// Forward to recipient's live connection
		if isRecipientOnline {
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
		} else {
			target := req.RecipientHandle
			if target == "" {
				target = req.RecipientEmail
			}
			log.Printf("[WebSocket] Recipient %s is offline. Message saved in DynamoDB for delivery on login.", target)
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

