package realtime

import (
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

// Event represents a standard real-time message payload sent to clients.
type Event struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Hub maintains the set of active clients keyed by UserID and Email, and coordinates
// real-time event dispatching to specific users.
type Hub struct {
	mu sync.RWMutex

	// users maps userID -> set of active *Client connections (supports multi-tab)
	users map[string]map[*Client]bool

	// emails maps lowercased email -> set of active *Client connections
	emails map[string]map[*Client]bool
}

// NewHub initializes and returns a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		users:  make(map[string]map[*Client]bool),
		emails: make(map[string]map[*Client]bool),
	}
}

// Register adds a client connection under its authenticated UserID and Email.
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

	log.Printf("[WebSocket] Connected: userID=%s email=%s (user tabs: %d)", c.userID, c.email, len(userConns))
}

// Unregister removes a client connection and cleans up empty user/email maps.
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

	close(c.send)
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
			Email string `json:"email"`
		}
		if err := json.Unmarshal(base.Payload, &req); err == nil && req.Email != "" {
			normalized := strings.TrimSpace(strings.ToLower(req.Email))
			c.email = normalized
			h.mu.Lock()
			emailConns, exists := h.emails[normalized]
			if !exists {
				emailConns = make(map[*Client]bool)
				h.emails[normalized] = emailConns
			}
			emailConns[c] = true
			h.mu.Unlock()
			log.Printf("[WebSocket] Presence confirmed: user %s registered email %s", c.userID, normalized)
		}

	case "chat.message":
		var req struct {
			RecipientEmail string `json:"recipientEmail"`
			SenderEmail    string `json:"senderEmail"`
			Text           string `json:"text"`
		}
		if err := json.Unmarshal(base.Payload, &req); err != nil {
			log.Printf("[WebSocket] Failed to parse chat.message payload: %v", err)
			return
		}

		req.RecipientEmail = strings.TrimSpace(strings.ToLower(req.RecipientEmail))
		req.SenderEmail = strings.TrimSpace(strings.ToLower(req.SenderEmail))
		req.Text = strings.TrimSpace(req.Text)

		senderEmail := c.email
		if senderEmail == "" {
			senderEmail = req.SenderEmail
		}

		if senderEmail != "" && c.email == "" {
			c.email = senderEmail
			h.mu.Lock()
			emailConns, exists := h.emails[senderEmail]
			if !exists {
				emailConns = make(map[*Client]bool)
				h.emails[senderEmail] = emailConns
			}
			emailConns[c] = true
			h.mu.Unlock()
			log.Printf("[WebSocket] Dynamically indexed user %s to email %s", c.userID, senderEmail)
		}

		if req.RecipientEmail == "" || req.Text == "" {
			log.Printf("[WebSocket] Invalid chat message: recipientEmail=%q, text=%q", req.RecipientEmail, req.Text)
			return
		}

		log.Printf("[WebSocket] Chat from %q to %q: %q", senderEmail, req.RecipientEmail, req.Text)

		now := time.Now().UTC().Format(time.RFC3339)
		msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())

		outEvent := Event{
			Type: "chat.message",
			Payload: map[string]interface{}{
				"id":             msgID,
				"senderEmail":    senderEmail,
				"senderId":       c.userID,
				"recipientEmail": req.RecipientEmail,
				"text":           req.Text,
				"time":           now,
			},
		}

		// Forward to recipient's live connection
		if err := h.SendToEmail(req.RecipientEmail, outEvent); err != nil {
			log.Printf("[WebSocket] Delivery failed: recipient %q is offline. Currently online emails: %v", req.RecipientEmail, h.GetOnlineEmails())
		} else {
			log.Printf("[WebSocket] Successfully delivered chat message to %q", req.RecipientEmail)
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

// ActiveConnections returns the number of open connections for a given user.
func (h *Hub) ActiveConnections(userID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if conns, exists := h.users[userID]; exists {
		return len(conns)
	}
	return 0
}
