package realtime

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

const testSecret = "test-super-secret-key-12345"

func createTestToken(userID string, expired bool) string {
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": "test@example.com",
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
	}
	if expired {
		claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))
	return tokenString
}

func TestHub_RegisterAndSend(t *testing.T) {
	hub := NewHub()
	userID := "user-123"

	// Initially offline
	if hub.IsUserOnline(userID) {
		t.Fatalf("expected user to be offline initially")
	}

	client1 := &Client{
		hub:    hub,
		userID: userID,
		send:   make(chan []byte, 10),
	}
	hub.Register(client1)

	if !hub.IsUserOnline(userID) {
		t.Fatalf("expected user to be online after registration")
	}
	if hub.ActiveConnections(userID) != 1 {
		t.Fatalf("expected 1 connection, got %d", hub.ActiveConnections(userID))
	}

	// Register 2nd tab
	client2 := &Client{
		hub:    hub,
		userID: userID,
		send:   make(chan []byte, 10),
	}
	hub.Register(client2)

	if hub.ActiveConnections(userID) != 2 {
		t.Fatalf("expected 2 connections, got %d", hub.ActiveConnections(userID))
	}

	// Send an event
	testEvent := Event{
		Type: "goal.unlocked",
		Payload: map[string]string{
			"goalId": "goal-abc",
		},
	}
	err := hub.SendToUser(userID, testEvent)
	if err != nil {
		t.Fatalf("unexpected error sending event: %v", err)
	}

	// Both clients should receive the event
	select {
	case msg := <-client1.send:
		if !strings.Contains(string(msg), "goal.unlocked") {
			t.Fatalf("expected goal.unlocked in client1 payload, got: %s", string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("timeout waiting for client1 message")
	}

	select {
	case msg := <-client2.send:
		if !strings.Contains(string(msg), "goal.unlocked") {
			t.Fatalf("expected goal.unlocked in client2 payload, got: %s", string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("timeout waiting for client2 message")
	}

	// Unregister 1 client
	hub.Unregister(client1)
	if hub.ActiveConnections(userID) != 1 {
		t.Fatalf("expected 1 connection after 1 unregister, got %d", hub.ActiveConnections(userID))
	}

	// Unregister 2nd client
	hub.Unregister(client2)
	if hub.IsUserOnline(userID) {
		t.Fatalf("expected user to be offline after all unregisters")
	}

	// Sending to offline user should return ErrUserOffline
	err = hub.SendToUser(userID, testEvent)
	if err != ErrUserOffline {
		t.Fatalf("expected ErrUserOffline, got: %v", err)
	}
}

func TestExtractAndValidateToken(t *testing.T) {
	validToken := createTestToken("user-abc", false)
	expiredToken := createTestToken("user-abc", true)

	// Valid token in query param
	req, _ := http.NewRequest("GET", "/ws?token="+validToken, nil)
	userID, err := ExtractAndValidateToken(req, testSecret)
	if err != nil || userID != "user-abc" {
		t.Fatalf("expected user-abc, got %s (err: %v)", userID, err)
	}

	// Valid token in Authorization header
	req, _ = http.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	userID, err = ExtractAndValidateToken(req, testSecret)
	if err != nil || userID != "user-abc" {
		t.Fatalf("expected user-abc via header, got %s (err: %v)", userID, err)
	}

	// Expired token
	req, _ = http.NewRequest("GET", "/ws?token="+expiredToken, nil)
	_, err = ExtractAndValidateToken(req, testSecret)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got: %v", err)
	}

	// Missing token
	req, _ = http.NewRequest("GET", "/ws", nil)
	_, err = ExtractAndValidateToken(req, testSecret)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing token error, got: %v", err)
	}

	// Invalid secret
	req, _ = http.NewRequest("GET", "/ws?token="+validToken, nil)
	_, err = ExtractAndValidateToken(req, "wrong-secret")
	if err == nil {
		t.Fatalf("expected signature validation failure")
	}
}

func TestServeWS_EndToEnd(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)

	server := httptest.NewServer(handler)
	defer server.Close()

	// Convert http URL to ws URL
	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// 1. Attempt connection with NO token (should be rejected 401)
	_, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err == nil {
		t.Fatalf("expected connection rejection with no token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.StatusCode)
	}

	// 2. Connect with VALID token
	validToken := createTestToken("test-user-ws", false)
	wsURL := u.String() + "?token=" + validToken

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect with valid token: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}

	// Verify hub registered the user
	time.Sleep(50 * time.Millisecond)
	if !hub.IsUserOnline("test-user-ws") {
		t.Fatalf("expected test-user-ws to be online in hub")
	}

	// 3. Dispatch an unlock event through hub
	event := Event{
		Type: "goal.unlocked",
		Payload: map[string]interface{}{
			"goalId": "goal-999",
			"status": "completed",
		},
	}
	err = hub.SendToUser("test-user-ws", event)
	if err != nil {
		t.Fatalf("failed to send event to user: %v", err)
	}

	// Read message from client side
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message from websocket: %v", err)
	}

	if !strings.Contains(string(msg), "goal.unlocked") || !strings.Contains(string(msg), "goal-999") {
		t.Fatalf("unexpected message content received: %s", string(msg))
	}
}

func TestChatRouting_EndToEnd(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)

	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Alice token
	aliceClaims := jwt.MapClaims{
		"sub":   "alice-id",
		"email": "alice@example.com",
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
	}
	tokA := jwt.NewWithClaims(jwt.SigningMethodHS256, aliceClaims)
	aliceToken, _ := tokA.SignedString([]byte(testSecret))

	// Bob token
	bobClaims := jwt.MapClaims{
		"sub":   "bob-id",
		"email": "bob@example.com",
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
	}
	tokB := jwt.NewWithClaims(jwt.SigningMethodHS256, bobClaims)
	bobToken, _ := tokB.SignedString([]byte(testSecret))

	// Connect Alice
	connAlice, _, err := websocket.DefaultDialer.Dial(u.String()+"?token="+aliceToken, nil)
	if err != nil {
		t.Fatalf("failed to dial Alice: %v", err)
	}
	defer connAlice.Close()

	// Connect Bob
	connBob, _, err := websocket.DefaultDialer.Dial(u.String()+"?token="+bobToken, nil)
	if err != nil {
		t.Fatalf("failed to dial Bob: %v", err)
	}
	defer connBob.Close()

	time.Sleep(50 * time.Millisecond)
	if !hub.IsEmailOnline("alice@example.com") || !hub.IsEmailOnline("bob@example.com") {
		t.Fatalf("expected both Alice and Bob to be online")
	}

	// Alice sends a chat message to Bob
	chatMsg := map[string]interface{}{
		"type": "chat.message",
		"payload": map[string]string{
			"recipientEmail": "bob@example.com",
			"text":           "hello from Alice!",
		},
	}
	if err := connAlice.WriteJSON(chatMsg); err != nil {
		t.Fatalf("failed to write chat message from Alice: %v", err)
	}

	// Bob reads the message
	_ = connBob.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msgBob, err := connBob.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message on Bob's socket: %v", err)
	}

	receivedStr := string(msgBob)
	if !strings.Contains(receivedStr, "hello from Alice!") || !strings.Contains(receivedStr, "alice@example.com") {
		t.Fatalf("unexpected message on Bob: %s", receivedStr)
	}
}

