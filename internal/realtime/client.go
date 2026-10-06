package realtime

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer (in bytes).
	maxMessageSize = 4096
)

// Client represents a single active WebSocket connection for an authenticated user.
type Client struct {
	hub              *Hub
	conn             *websocket.Conn
	userID           string
	email            string
	handle           string
	deliveredOffline bool
	mu               sync.Mutex
	send             chan []byte
}

// NewClient initializes a new Client with authenticated userID, email, and handle.
func NewClient(hub *Hub, conn *websocket.Conn, userID string, email string, handle string) *Client {
	return &Client{
		hub:    hub,
		conn:   conn,
		userID: userID,
		email:  strings.TrimSpace(strings.ToLower(email)),
		handle: strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@")),
		send:   make(chan []byte, 64),
	}
}

// SendEvent serializes and sends an Event directly to this client.
func (c *Client) SendEvent(event Event) {
	defer func() {
		// Prevent runtime panic if sending on closed channel during disconnect
		_ = recover()
	}()
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

// ReadPump pumps messages from the websocket connection to the hub.
// When an error occurs or connection drops, it unregisters from the hub.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WebSocket] Read error for user %s: %v", c.userID, err)
			}
			break
		}

		// Handle client-to-server messages (e.g. real-time chat)
		c.hub.HandleClientMessage(c, message)
	}
}

// WritePump pumps messages from the hub to the websocket connection.
// Periodically sends a ping message to keep the connection alive.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
