package realtime

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
)

var (
	ErrUserOffline = errors.New("user has no active connections")
)

// Event represents a standard real-time message payload sent to clients.
type Event struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Hub maintains the set of active clients keyed by UserID and coordinates
// real-time event dispatching to specific users.
type Hub struct {
	mu sync.RWMutex

	// users maps userID -> set of active *Client connections (supports multi-tab)
	users map[string]map[*Client]bool
}

// NewHub initializes and returns a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		users: make(map[string]map[*Client]bool),
	}
}

// Register adds a client connection under its authenticated UserID.
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	userConns, exists := h.users[c.userID]
	if !exists {
		userConns = make(map[*Client]bool)
		h.users[c.userID] = userConns
	}
	userConns[c] = true
	log.Printf("[WebSocket] User connected: %s (active tabs: %d)", c.userID, len(userConns))
}

// Unregister removes a client connection and cleans up empty user maps.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	userConns, exists := h.users[c.userID]
	if exists {
		delete(userConns, c)
		close(c.send)
		if len(userConns) == 0 {
			delete(h.users, c.userID)
			log.Printf("[WebSocket] User disconnected completely: %s", c.userID)
		} else {
			log.Printf("[WebSocket] User closed a tab: %s (remaining tabs: %d)", c.userID, len(userConns))
		}
	}
}

// SendToUser dispatches an Event to all active WebSocket connections for a given user.
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
			// Buffer full, connection might be stuck; will be reaped by reader/writer
			log.Printf("[WebSocket] Buffer full for client of user: %s", userID)
		}
	}

	return nil
}

// IsUserOnline returns true if the user has at least one active connection.
func (h *Hub) IsUserOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	conns, exists := h.users[userID]
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
