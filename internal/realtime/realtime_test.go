package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
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

	// Valid token in Authorization header
	req, _ := http.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	userID, err := ExtractAndValidateToken(req, testSecret)
	if err != nil || userID != "user-abc" {
		t.Fatalf("expected user-abc via header, got %s (err: %v)", userID, err)
	}

	// Query param token is rejected for security
	req, _ = http.NewRequest("GET", "/ws?token="+validToken, nil)
	_, err = ExtractAndValidateToken(req, testSecret)
	if err == nil || !strings.Contains(err.Error(), "disabled for security") {
		t.Fatalf("expected query parameter disabled error, got: %v", err)
	}

	// Expired token in Authorization header
	req, _ = http.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
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
	req, _ = http.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
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

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	validToken := createTestToken("test-user-ws", false)

	// 1. Connection with query param token is REJECTED with 401 Unauthorized
	_, resp, err := websocket.DefaultDialer.Dial(u.String()+"?token="+validToken, nil)
	if err == nil {
		t.Fatalf("expected rejection when token passed in query parameter")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for query param token, got %d", resp.StatusCode)
	}

	// 2. Connect with first-message handshake
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to connect to ws endpoint: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}

	// Send auth handshake message
	authPayload := map[string]string{
		"type":  "auth",
		"token": validToken,
	}
	if err := conn.WriteJSON(authPayload); err != nil {
		t.Fatalf("failed to write auth handshake: %v", err)
	}

	// Expect auth.success confirmation
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, authResp, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read auth response: %v", err)
	}
	if !strings.Contains(string(authResp), "auth.success") {
		t.Fatalf("expected auth.success in response, got: %s", string(authResp))
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

func TestServeWS_HandshakeFailures(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)

	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// 1. First message is not "auth" -> rejected
	conn1, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn1.Close()

	_ = conn1.WriteJSON(map[string]string{"type": "chat.message", "text": "hello"})
	_ = conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn1.ReadMessage()
	if err == nil {
		t.Fatalf("expected connection to be closed by server on non-auth first message")
	}

	// 2. Auth message has expired token -> rejected
	conn2, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn2.Close()

	expiredToken := createTestToken("expired-user", true)
	_ = conn2.WriteJSON(map[string]string{"type": "auth", "token": expiredToken})
	_ = conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn2.ReadMessage()
	if err == nil {
		t.Fatalf("expected connection to be closed on expired token")
	}

	// 3. Direct Authorization header upgrade -> success
	validToken := createTestToken("direct-user", false)
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+validToken)
	conn3, resp, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		t.Fatalf("failed to dial with Authorization header: %v", err)
	}
	defer conn3.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}
	time.Sleep(50 * time.Millisecond)
	if !hub.IsUserOnline("direct-user") {
		t.Fatalf("expected direct-user to be registered online via header")
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

	// Connect Alice via handshake
	connAlice, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Alice: %v", err)
	}
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage() // auth.success

	// Connect Bob via handshake
	connBob, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Bob: %v", err)
	}
	defer connBob.Close()
	_ = connBob.WriteJSON(map[string]string{"type": "auth", "token": bobToken})
	_, _, _ = connBob.ReadMessage() // auth.success

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

func TestPresenceQuery(t *testing.T) {
	jwtSecret := "test-presence-secret"
	hub := NewHub()

	server := httptest.NewServer(ServeWS(hub, jwtSecret))
	defer server.Close()

	claims := jwt.MapClaims{
		"sub":    "alice-presence-id",
		"email":  "alice@test.com",
		"handle": "alice_presence",
		"exp":    time.Now().Add(1 * time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenAlice, _ := tok.SignedString([]byte(jwtSecret))
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	// Handshake
	_ = conn.WriteJSON(map[string]string{"type": "auth", "token": tokenAlice})
	_, _, _ = conn.ReadMessage() // auth.success

	// Query presence
	queryMsg := map[string]interface{}{
		"type": "presence.query",
	}
	if err := conn.WriteJSON(queryMsg); err != nil {
		t.Fatalf("failed to send presence query: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, respBytes, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read presence response: %v", err)
	}

	respStr := string(respBytes)
	if !strings.Contains(respStr, "presence.list") || !strings.Contains(respStr, "alice_presence") {
		t.Fatalf("unexpected presence list response: %s", respStr)
	}
	// Security: Presence responses must never expose user email addresses
	if strings.Contains(respStr, "alice@test.com") {
		t.Fatalf("security violation: presence response leaked user email: %s", respStr)
	}
}

func TestCommunityRoomAuthorization(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)

	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Alice registers community "comm-100" with member "bob@example.com"
	aliceToken := createTestToken("alice-sub", false)
	connAlice, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Alice: %v", err)
	}
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage()

	// Register community
	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.register",
		"payload": map[string]interface{}{
			"community": map[string]interface{}{
				"id":   "comm-100",
				"name": "Secret Study Group",
			},
			"members": []map[string]string{
				{"email": "bob@example.com"},
			},
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Eve (unrelated user) attempts to join "comm-100"
	eveClaims := jwt.MapClaims{
		"sub":   "eve-sub",
		"email": "eve@example.com",
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
	}
	tokEve := jwt.NewWithClaims(jwt.SigningMethodHS256, eveClaims)
	eveToken, _ := tokEve.SignedString([]byte(testSecret))

	connEve, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Eve: %v", err)
	}
	defer connEve.Close()
	_ = connEve.WriteJSON(map[string]string{"type": "auth", "token": eveToken})
	_, _, _ = connEve.ReadMessage()

	// Eve tries to join room
	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "community.join_room",
		"payload": map[string]string{
			"communityId": "comm-100",
		},
	})

	_ = connEve.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, eveMsg, err := connEve.ReadMessage()
	if err != nil {
		t.Fatalf("expected error event sent to Eve: %v", err)
	}
	if !strings.Contains(string(eveMsg), "community.error") || !strings.Contains(string(eveMsg), "unauthorized") {
		t.Fatalf("expected unauthorized community error for Eve, got: %s", string(eveMsg))
	}
}

func createCustomToken(sub, email, handle, name string) string {
	claims := jwt.MapClaims{
		"sub":    sub,
		"email":  email,
		"handle": handle,
		"name":   name,
		"exp":    time.Now().Add(1 * time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := tok.SignedString([]byte(testSecret))
	return s
}

func TestUserOnline_CannotSpoofIdentity(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)

	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Alice connects with valid token
	aliceToken := createCustomToken("alice-sub", "alice@example.com", "alice_h", "Alice Real")
	connAlice, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Alice: %v", err)
	}
	defer connAlice.Close()

	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage() // auth.success

	time.Sleep(20 * time.Millisecond)
	if !hub.IsEmailOnline("alice@example.com") || !hub.IsHandleOnline("alice_h") {
		t.Fatalf("expected Alice to be online under authenticated identity")
	}

	// Alice attempts to spoof Bob by sending a forged user.online frame
	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "user.online",
		"payload": map[string]string{
			"email":  "bob@example.com",
			"handle": "bob_h",
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Verify server rejected the spoof: Bob must NOT be registered online in the hub
	if hub.IsEmailOnline("bob@example.com") {
		t.Fatalf("security violation: forged email registered in hub!")
	}
	if hub.IsHandleOnline("bob_h") {
		t.Fatalf("security violation: forged handle registered in hub!")
	}

	// Alice must remain registered under her real authenticated identity
	if !hub.IsEmailOnline("alice@example.com") {
		t.Fatalf("expected Alice email to remain online")
	}
	if !hub.IsHandleOnline("alice_h") {
		t.Fatalf("expected Alice handle to remain online")
	}
}

func TestUserOnline_DisconnectDoesNotRemoveOtherUserPresence(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)

	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// 1. Bob connects legitimately
	bobToken := createCustomToken("bob-sub", "bob@example.com", "bob_h", "Bob Legit")
	connBob, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Bob: %v", err)
	}
	defer connBob.Close()
	_ = connBob.WriteJSON(map[string]string{"type": "auth", "token": bobToken})
	_, _, _ = connBob.ReadMessage()
	time.Sleep(20 * time.Millisecond)

	if !hub.IsEmailOnline("bob@example.com") || !hub.IsHandleOnline("bob_h") {
		t.Fatalf("expected Bob to be online")
	}

	// 2. Eve connects and tries to claim Bob's email/handle
	eveToken := createCustomToken("eve-sub", "eve@example.com", "eve_h", "Eve Attacker")
	connEve, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Eve: %v", err)
	}
	_ = connEve.WriteJSON(map[string]string{"type": "auth", "token": eveToken})
	_, _, _ = connEve.ReadMessage()

	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "user.online",
		"payload": map[string]string{
			"email":  "bob@example.com",
			"handle": "bob_h",
		},
	})
	time.Sleep(30 * time.Millisecond)

	// 3. Eve disconnects
	connEve.Close()
	time.Sleep(50 * time.Millisecond)

	// Bob must STILL be online! Eve's disconnect must NOT have unregistered Bob's presence!
	if !hub.IsEmailOnline("bob@example.com") {
		t.Fatalf("security violation: legitimate user email presence removed by spoofed client disconnect!")
	}
	if !hub.IsHandleOnline("bob_h") {
		t.Fatalf("security violation: legitimate user handle presence removed by spoofed client disconnect!")
	}
	if hub.IsEmailOnline("eve@example.com") {
		t.Fatalf("expected Eve email to be offline after disconnect")
	}
}

func TestUnauthenticatedConnectionCannotAnnouncePresence(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)

	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Connect without authentication handshake
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	// Try sending user.online directly
	_ = conn.WriteJSON(map[string]interface{}{
		"type": "user.online",
		"payload": map[string]string{
			"email":  "ghost@example.com",
			"handle": "ghost_h",
		},
	})

	// Server must close connection with policy violation ("first message must be auth")
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatalf("expected read error / close from server on unauthenticated message")
	}

	time.Sleep(30 * time.Millisecond)
	if hub.IsEmailOnline("ghost@example.com") || hub.IsHandleOnline("ghost_h") {
		t.Fatalf("security violation: unauthenticated connection registered presence!")
	}
}

func TestChatMessage_SenderIdentityDerivedStrictlyFromAuth(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)

	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Alice connects
	aliceToken := createCustomToken("alice-sub", "alice@example.com", "alice_h", "Alice User")
	connAlice, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Alice: %v", err)
	}
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage()

	// Eve connects
	eveToken := createCustomToken("eve-sub", "eve@example.com", "eve_h", "Eve Attacker")
	connEve, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Eve: %v", err)
	}
	defer connEve.Close()
	_ = connEve.WriteJSON(map[string]string{"type": "auth", "token": eveToken})
	_, _, _ = connEve.ReadMessage()

	time.Sleep(50 * time.Millisecond)

	// Eve attempts to send a message impersonating the system Administrator
	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "chat.message",
		"payload": map[string]interface{}{
			"recipientEmail": "alice@example.com",
			"senderEmail":    "admin@onusly.internal",
			"senderHandle":   "administrator",
			"senderName":     "Chief Admin",
			"senderId":       "admin-id",
			"text":           "Security Notice",
			"isProof":        true,
			"ownerId":        "forged-owner-id",
		},
	})

	_ = connAlice.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msgBytes, err := connAlice.ReadMessage()
	if err != nil {
		t.Fatalf("Alice failed to receive message: %v", err)
	}

	msgStr := string(msgBytes)

	// Verify server enforced Eve's real authenticated identity
	if strings.Contains(msgStr, "admin@onusly.internal") {
		t.Fatalf("security violation: forged senderEmail delivered to recipient! Msg: %s", msgStr)
	}
	if strings.Contains(msgStr, "administrator") {
		t.Fatalf("security violation: forged senderHandle delivered to recipient! Msg: %s", msgStr)
	}
	if !strings.Contains(msgStr, "eve@example.com") {
		t.Fatalf("expected authenticated sender email in payload: %s", msgStr)
	}
	if !strings.Contains(msgStr, "eve_h") {
		t.Fatalf("expected authenticated sender handle in payload: %s", msgStr)
	}
	if !strings.Contains(msgStr, "eve-sub") {
		t.Fatalf("expected authenticated sender ID in payload: %s", msgStr)
	}
	// For proof submissions, ownerId must be derived from sender's authenticated ID
	if strings.Contains(msgStr, "forged-owner-id") {
		t.Fatalf("security violation: forged ownerId allowed in proof chat message!")
	}
	if !strings.Contains(msgStr, `"ownerId":"eve-sub"`) {
		t.Fatalf("expected ownerId to be derived from authenticated user ID (eve-sub): %s", msgStr)
	}

	// Verify Eve's connection was NOT re-registered as "administrator"
	if hub.IsHandleOnline("administrator") {
		t.Fatalf("security violation: Eve claimed administrator handle in hub!")
	}
}

func TestCommunityDeleteMessage_UnauthorizedUserBlocked(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)

	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Alice registers community "comm-300"
	aliceToken := createCustomToken("alice-sub", "alice@example.com", "alice_h", "Alice")
	connAlice, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage()

	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.register",
		"payload": map[string]interface{}{
			"community": map[string]interface{}{
				"id":   "comm-300",
				"name": "Alice Private Room",
			},
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Eve (not a member) attempts to delete a message in "comm-300"
	eveToken := createCustomToken("eve-sub", "eve@example.com", "eve_h", "Eve")
	connEve, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connEve.Close()
	_ = connEve.WriteJSON(map[string]string{"type": "auth", "token": eveToken})
	_, _, _ = connEve.ReadMessage()

	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "community.delete_message",
		"payload": map[string]string{
			"communityId": "comm-300",
			"messageId":   "msg-target-123",
		},
	})

	_ = connEve.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, eveMsg, err := connEve.ReadMessage()
	if err != nil {
		t.Fatalf("expected error response to Eve: %v", err)
	}
	if !strings.Contains(string(eveMsg), "community.error") || !strings.Contains(string(eveMsg), "unauthorized") {
		t.Fatalf("expected unauthorized error for Eve, got: %s", string(eveMsg))
	}
}

type mockTestGoalVerifier struct {
	verifiedGoals map[string]struct {
		ownerID  string
		title    string
		approver string
	}
}

func (m *mockTestGoalVerifier) VerifyApprovedGoal(ctx context.Context, callerID, callerEmail, callerHandle, goalID string) (string, string, bool, error) {
	g, ok := m.verifiedGoals[goalID]
	if !ok {
		return "", "", false, nil
	}
	if g.approver != callerID {
		return "", "", false, nil
	}
	return g.ownerID, g.title, true, nil
}

func TestChatMessageApproved_ApproverIdentityDerivedFromAuth(t *testing.T) {
	hub := NewHub()
	verifier := &mockTestGoalVerifier{
		verifiedGoals: map[string]struct {
			ownerID  string
			title    string
			approver string
		}{
			"goal-99": {
				ownerID:  "alice-sub",
				title:    "My Goal",
				approver: "eve-sub",
			},
		},
	}
	hub.SetGoalVerifier(verifier)

	handler := ServeWS(hub, testSecret)

	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Alice connects
	aliceToken := createCustomToken("alice-sub", "alice@example.com", "alice_h", "Alice")
	connAlice, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage()

	// Eve connects and attempts to broadcast an approval spoofing the approver
	eveToken := createCustomToken("eve-sub", "eve@example.com", "eve_h", "Eve Verified")
	connEve, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connEve.Close()
	_ = connEve.WriteJSON(map[string]string{"type": "auth", "token": eveToken})
	_, _, _ = connEve.ReadMessage()

	time.Sleep(30 * time.Millisecond)

	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "chat.message.approved",
		"payload": map[string]string{
			"goalId":       "goal-99",
			"goalTitle":    "My Goal",
			"approverName": "Official Auditor", // Spoofed name
		},
	})

	_ = connAlice.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msgBytes, err := connAlice.ReadMessage()
	if err != nil {
		t.Fatalf("Alice failed to receive broadcast: %v", err)
	}
	msgStr := string(msgBytes)

	// Verify the approver name and ID were derived strictly from Eve's authenticated session
	if strings.Contains(msgStr, "Official Auditor") {
		t.Fatalf("security violation: spoofed approverName broadcasted! Msg: %s", msgStr)
	}
	if !strings.Contains(msgStr, "Eve Verified") {
		t.Fatalf("expected authenticated approver name in broadcast: %s", msgStr)
	}
	if !strings.Contains(msgStr, "eve-sub") {
		t.Fatalf("expected authenticated approver ID in broadcast: %s", msgStr)
	}
}

func TestChatMessageApproved_UnverifiedGoalBlocked(t *testing.T) {
	hub := NewHub()
	// Verifier configured, but goal-404 is NOT in verified list
	verifier := &mockTestGoalVerifier{
		verifiedGoals: map[string]struct {
			ownerID  string
			title    string
			approver string
		}{},
	}
	hub.SetGoalVerifier(verifier)

	handler := ServeWS(hub, testSecret)
	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	eveToken := createCustomToken("eve-sub", "eve@example.com", "eve_h", "Eve")
	connEve, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connEve.Close()
	_ = connEve.WriteJSON(map[string]string{"type": "auth", "token": eveToken})
	_, _, _ = connEve.ReadMessage()

	// Eve tries to broadcast approval for unverified goal
	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "chat.message.approved",
		"payload": map[string]string{
			"goalId": "goal-unverified",
		},
	})

	_ = connEve.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, msgBytes, err := connEve.ReadMessage()
	if err != nil {
		t.Fatalf("expected error response from server: %v", err)
	}
	msgStr := string(msgBytes)
	if !strings.Contains(msgStr, "approval.error") || !strings.Contains(msgStr, "forbidden") {
		t.Fatalf("expected forbidden approval.error, got: %s", msgStr)
	}
}

func TestCommunityRegister_NonOwnerBlockedFromOverwriting(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)
	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Alice creates community comm-legit
	aliceToken := createCustomToken("alice-sub", "alice@example.com", "alice_h", "Alice")
	connAlice, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage()

	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.register",
		"payload": map[string]interface{}{
			"community": map[string]interface{}{
				"id":   "comm-legit",
				"name": "Alice Group",
			},
		},
	})
	time.Sleep(20 * time.Millisecond)

	// Eve attempts to overwrite comm-legit
	eveToken := createCustomToken("eve-sub", "eve@example.com", "eve_h", "Eve")
	connEve, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connEve.Close()
	_ = connEve.WriteJSON(map[string]string{"type": "auth", "token": eveToken})
	_, _, _ = connEve.ReadMessage()

	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "community.register",
		"payload": map[string]interface{}{
			"community": map[string]interface{}{
				"id":   "comm-legit",
				"name": "Eve Hijacked Group",
			},
		},
	})

	_ = connEve.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, msgBytes, err := connEve.ReadMessage()
	if err != nil {
		t.Fatalf("expected error response for overwrite attempt: %v", err)
	}
	msgStr := string(msgBytes)
	if !strings.Contains(msgStr, "community.error") || !strings.Contains(msgStr, "only the creator can update") {
		t.Fatalf("expected unauthorized error for non-owner overwrite attempt, got: %s", msgStr)
	}

	// Verify original metadata was NOT overwritten
	hub.mu.RLock()
	rec := hub.commRecords["comm-legit"]
	hub.mu.RUnlock()
	if rec.Name != "Alice Group" || rec.CreatorID != "alice-sub" {
		t.Fatalf("security violation: community metadata was corrupted! rec: %+v", rec)
	}
}

func TestCommunityJoinRoom_MissingRecordFailsClosed(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)
	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	eveToken := createCustomToken("eve-sub", "eve@example.com", "eve_h", "Eve")
	connEve, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connEve.Close()
	_ = connEve.WriteJSON(map[string]string{"type": "auth", "token": eveToken})
	_, _, _ = connEve.ReadMessage()

	// Eve tries to join a room that does not exist in memory
	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "community.join_room",
		"payload": map[string]string{
			"communityId": "nonexistent-comm",
		},
	})

	_ = connEve.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, msgBytes, err := connEve.ReadMessage()
	if err != nil {
		t.Fatalf("expected error response when joining missing room: %v", err)
	}
	msgStr := string(msgBytes)
	if !strings.Contains(msgStr, "community.error") || !strings.Contains(msgStr, "community not found or unauthorized") {
		t.Fatalf("expected fail closed error on missing community, got: %s", msgStr)
	}
}

type mockTestMessageStore struct {
	deleteFunc    func(ctx context.Context, callerEmail, callerHandle, callerID, msgID string) error
	pendingFunc   func(ctx context.Context, recipientKey string) ([]StoredMessage, error)
	saveFunc      func(ctx context.Context, msg StoredMessage, isRecipientOnline bool) error
	deliveredFunc func(ctx context.Context, recipientKey string, msgIDs []string) error
}

func (m *mockTestMessageStore) SaveMessage(ctx context.Context, msg StoredMessage, isRecipientOnline bool) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, msg, isRecipientOnline)
	}
	return nil
}

func (m *mockTestMessageStore) GetPendingMessages(ctx context.Context, recipientKey string) ([]StoredMessage, error) {
	if m.pendingFunc != nil {
		return m.pendingFunc(ctx, recipientKey)
	}
	return nil, nil
}

func (m *mockTestMessageStore) MarkMessagesDelivered(ctx context.Context, recipientKey string, msgIDs []string) error {
	if m.deliveredFunc != nil {
		return m.deliveredFunc(ctx, recipientKey, msgIDs)
	}
	return nil
}

func (m *mockTestMessageStore) DeleteMessage(ctx context.Context, callerEmail, callerHandle, callerID, msgID string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, callerEmail, callerHandle, callerID, msgID)
	}
	return nil
}

func TestChatDelete_SenderAuthorizationEnforced(t *testing.T) {
	hub := NewHub()
	deleteCalledWithCaller := ""
	store := &mockTestMessageStore{
		deleteFunc: func(ctx context.Context, callerEmail, callerHandle, callerID, msgID string) error {
			deleteCalledWithCaller = callerID
			if callerID != "alice-sub" {
				return errors.New("unauthorized")
			}
			return nil
		},
	}
	hub.SetMessageStore(store)

	handler := ServeWS(hub, testSecret)
	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Eve connects and tries to delete Alice's message msg-123
	eveToken := createCustomToken("eve-sub", "eve@example.com", "eve_h", "Eve")
	connEve, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connEve.Close()
	_ = connEve.WriteJSON(map[string]string{"type": "auth", "token": eveToken})
	_, _, _ = connEve.ReadMessage()

	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "chat.delete",
		"payload": map[string]string{
			"id": "msg-123",
		},
	})

	_ = connEve.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, msgBytes, err := connEve.ReadMessage()
	if err != nil {
		t.Fatalf("expected error response when unauthorized user deletes message: %v", err)
	}
	msgStr := string(msgBytes)
	if !strings.Contains(msgStr, "chat.error") || !strings.Contains(msgStr, "unauthorized") {
		t.Fatalf("expected chat.error for unauthorized deletion, got: %s", msgStr)
	}
	if deleteCalledWithCaller != "eve-sub" {
		t.Fatalf("expected delete to be verified with authenticated caller eve-sub, got %s", deleteCalledWithCaller)
	}
}

// 4. Offline delivery to recycled handle:
// User A sends an offline message to @bob (original user ID).
// Before User B logs in, @bob is transferred to User D.
// When User D logs in, User D does not receive the message.
// When User B logs in (with their new handle or email), User B receives the message.
func TestRegression_4_OfflineDeliveryToRecycledHandle(t *testing.T) {
	hub := NewHub()

	// Store has offline message intended for original Bob (user-b)
	pendingMsg := StoredMessage{
		ID:              "msg-offline-1",
		SenderID:        "user-a",
		RecipientID:     "user-b",
		SenderEmail:     "alice@example.com",
		RecipientEmail:  "bob@example.com",
		RecipientHandle: "bob",
		Text:            "Offline message intended for original Bob",
		Time:            "2026-10-20T11:00:00Z",
	}

	queriedKeys := []string{}
	var mu sync.Mutex

	store := &mockTestMessageStore{
		pendingFunc: func(ctx context.Context, recipientKey string) ([]StoredMessage, error) {
			mu.Lock()
			queriedKeys = append(queriedKeys, recipientKey)
			mu.Unlock()
			if recipientKey == "user-b" {
				return []StoredMessage{pendingMsg}, nil
			}
			return nil, nil
		},
	}
	hub.SetMessageStore(store)

	handler := ServeWS(hub, testSecret)
	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// 1. User D (user-d) who now holds recycled handle "bob" connects
	dToken := createCustomToken("user-d", "david@example.com", "bob", "David")
	connD, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer connD.Close()

	_ = connD.WriteJSON(map[string]string{"type": "auth", "token": dToken})
	_, _, _ = connD.ReadMessage() // auth.success

	_ = connD.WriteJSON(map[string]interface{}{
		"type": "user.online",
		"payload": map[string]string{
			"handle": "bob",
		},
	})

	// Give a short window for any async delivery
	_ = connD.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, msgBytes, err := connD.ReadMessage()
	// User D must NOT receive any offline message!
	if err == nil {
		var evt Event
		_ = json.Unmarshal(msgBytes, &evt)
		if evt.Type == "chat.message" {
			t.Fatalf("CRITICAL: User D received offline message intended for User B! payload: %s", string(msgBytes))
		}
	}

	// 2. User B (user-b) connects with new handle "bob_new"
	bToken := createCustomToken("user-b", "bob@example.com", "bob_new", "Bob")
	connB, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer connB.Close()

	_ = connB.WriteJSON(map[string]string{"type": "auth", "token": bToken})
	_, _, _ = connB.ReadMessage() // auth.success

	_ = connB.WriteJSON(map[string]interface{}{
		"type": "user.online",
		"payload": map[string]string{
			"handle": "bob_new",
		},
	})

	_ = connB.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, bBytes, err := connB.ReadMessage()
	if err != nil {
		t.Fatalf("User B did not receive expected offline message: %v", err)
	}

	var bEvt Event
	if err := json.Unmarshal(bBytes, &bEvt); err != nil {
		t.Fatalf("failed to parse event for User B: %v", err)
	}
	if bEvt.Type != "chat.message" {
		t.Fatalf("expected chat.message event for User B, got %s", bEvt.Type)
	}

	// Verify mutable handle @bob was NEVER queried in pending messages
	mu.Lock()
	defer mu.Unlock()
	for _, k := range queriedKeys {
		if k == "@bob" || k == "bob" {
			t.Fatalf("CRITICAL: Store was queried with mutable handle alias: %s", k)
		}
	}
}

type mockTestUserResolver struct {
	resolveFunc func(ctx context.Context, identifier string) (string, string, string, error)
}

func (m *mockTestUserResolver) ResolveUser(ctx context.Context, identifier string) (string, string, string, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(ctx, identifier)
	}
	return "", "", "", nil
}

// Priority 3 Regression Test: chat.message unconditionally binds recipient metadata to authoritative resolver output
func TestRegression_Priority3_RecipientMetadataSpoofingPrevention(t *testing.T) {
	hub := NewHub()

	resolver := &mockTestUserResolver{
		resolveFunc: func(ctx context.Context, identifier string) (string, string, string, error) {
			if identifier == "user-bob-id" || identifier == "@bob_official" || identifier == "bob.authoritative@example.com" {
				return "user-bob-id", "bob.authoritative@example.com", "bob_official", nil
			}
			return "", "", "", errors.New("user not found")
		},
	}
	hub.SetUserResolver(resolver)

	var saveMu sync.Mutex
	var savedMsg StoredMessage
	store := &mockTestMessageStore{
		saveFunc: func(ctx context.Context, msg StoredMessage, isRecipientOnline bool) error {
			saveMu.Lock()
			savedMsg = msg
			saveMu.Unlock()
			return nil
		},
	}
	hub.SetMessageStore(store)

	handler := ServeWS(hub, testSecret)
	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Bob connects
	bobToken := createCustomToken("user-bob-id", "bob.authoritative@example.com", "bob_official", "Bob")
	connBob, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial bob: %v", err)
	}
	defer connBob.Close()
	_ = connBob.WriteJSON(map[string]string{"type": "auth", "token": bobToken})
	_, _, _ = connBob.ReadMessage() // auth.success

	// Alice connects and sends a message attempting to spoof recipient email/handle
	aliceToken := createCustomToken("user-alice-id", "alice@example.com", "alice", "Alice")
	connAlice, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial alice: %v", err)
	}
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage() // auth.success

	// Alice sends message with spoofed recipient metadata
	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "chat.message",
		"payload": map[string]interface{}{
			"id":              "msg-spoof-1",
			"recipientId":     "user-bob-id",
			"recipientEmail":  "spoofed.admin@evil.com",
			"recipientHandle": "evil_admin",
			"text":            "Hello Bob",
		},
	})

	_ = connBob.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, bBytes, err := connBob.ReadMessage()
	if err != nil {
		t.Fatalf("Bob failed to receive message: %v", err)
	}

	var bEvt Event
	if err := json.Unmarshal(bBytes, &bEvt); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	payload := bEvt.Payload.(map[string]interface{})
	if payload["recipientEmail"] != "bob.authoritative@example.com" {
		t.Fatalf("CRITICAL: Recipient email was not authoritatively bound! got %v", payload["recipientEmail"])
	}
	if payload["recipientHandle"] != "bob_official" {
		t.Fatalf("CRITICAL: Recipient handle was not authoritatively bound! got %v", payload["recipientHandle"])
	}

	// Verify persistent store also received authoritative values
	saveMu.Lock()
	copiedSaved := savedMsg
	saveMu.Unlock()
	if copiedSaved.RecipientEmail != "bob.authoritative@example.com" || copiedSaved.RecipientHandle != "bob_official" {
		t.Fatalf("CRITICAL: Stored message contains unverified recipient metadata: %+v", copiedSaved)
	}
}

// Priority 6 Regression Test: chat.delete dispatches exclusively via SendToUser and does not leak to recycled handles
func TestRegression_Priority6_ChatDelete_ImmutableIDOnly(t *testing.T) {
	hub := NewHub()

	store := &mockTestMessageStore{
		deleteFunc: func(ctx context.Context, callerEmail, callerHandle, callerID, msgID string) error {
			return nil
		},
	}
	hub.SetMessageStore(store)

	handler := ServeWS(hub, testSecret)
	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Bob connects with immutable ID "user-bob-id"
	bobToken := createCustomToken("user-bob-id", "bob@example.com", "bob_current", "Bob")
	connBob, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Bob: %v", err)
	}
	defer connBob.Close()
	_ = connBob.WriteJSON(map[string]string{"type": "auth", "token": bobToken})
	_, _, _ = connBob.ReadMessage()

	// Eve connects with recycled handle "bob_former" and old email "bob.old@example.com"
	eveToken := createCustomToken("user-eve-id", "bob.old@example.com", "bob_former", "Eve")
	connEve, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Eve: %v", err)
	}
	defer connEve.Close()
	_ = connEve.WriteJSON(map[string]string{"type": "auth", "token": eveToken})
	_, _, _ = connEve.ReadMessage()

	// Alice connects and deletes a message
	aliceToken := createCustomToken("user-alice-id", "alice@example.com", "alice", "Alice")
	connAlice, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial Alice: %v", err)
	}
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage()

	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "chat.delete",
		"payload": map[string]interface{}{
			"id":              "msg-target-to-delete",
			"recipientId":     "user-bob-id",
			"recipientHandle": "bob_former",
			"recipientEmail":  "bob.old@example.com",
		},
	})

	// Bob must receive chat.delete
	_ = connBob.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, bBytes, err := connBob.ReadMessage()
	if err != nil {
		t.Fatalf("Bob did not receive chat.delete: %v", err)
	}
	var bEvt Event
	_ = json.Unmarshal(bBytes, &bEvt)
	if bEvt.Type != "chat.delete" {
		t.Fatalf("expected chat.delete event for Bob, got %s", bEvt.Type)
	}

	// Eve (who holds the mutable handle/email) must NOT receive chat.delete
	_ = connEve.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err = connEve.ReadMessage()
	if err == nil {
		t.Fatalf("CRITICAL: Eve received chat.delete broadcast via mutable handle/email!")
	}
}

// Priority 8 Regression Test: community.delete_message requires community creator or original message author
func TestRegression_Priority8_CommunityDeleteMessage_Authorization(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)
	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Alice creates community "comm-priv-100" (Alice is Creator)
	aliceToken := createCustomToken("user-alice-id", "alice@example.com", "alice", "Alice")
	connAlice, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage()

	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.register",
		"payload": map[string]interface{}{
			"community": map[string]interface{}{
				"id":   "comm-priv-100",
				"name": "Secret Room",
			},
			"members": []map[string]string{
				{"handle": "bob", "email": "bob@example.com"},
				{"handle": "charlie", "email": "charlie@example.com"},
			},
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Bob joins "comm-priv-100"
	bobToken := createCustomToken("user-bob-id", "bob@example.com", "bob", "Bob")
	connBob, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connBob.Close()
	_ = connBob.WriteJSON(map[string]string{"type": "auth", "token": bobToken})
	_, _, _ = connBob.ReadMessage()

	_ = connBob.WriteJSON(map[string]interface{}{
		"type": "community.join_room",
		"payload": map[string]string{
			"communityId": "comm-priv-100",
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Charlie joins "comm-priv-100"
	charlieToken := createCustomToken("user-charlie-id", "charlie@example.com", "charlie", "Charlie")
	connCharlie, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connCharlie.Close()
	_ = connCharlie.WriteJSON(map[string]string{"type": "auth", "token": charlieToken})
	_, _, _ = connCharlie.ReadMessage()

	_ = connCharlie.WriteJSON(map[string]interface{}{
		"type": "community.join_room",
		"payload": map[string]string{
			"communityId": "comm-priv-100",
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Bob sends a message in "comm-priv-100"
	_ = connBob.WriteJSON(map[string]interface{}{
		"type": "community.message",
		"payload": map[string]interface{}{
			"id":          "msg-bob-exclusive-1",
			"communityId": "comm-priv-100",
			"text":        "Bob original message",
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Consume broadcast of Bob's message on Alice and Charlie
	_ = connAlice.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, _ = connAlice.ReadMessage()
	_ = connCharlie.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, _ = connCharlie.ReadMessage()

	// 1. Charlie (member, but NOT creator and NOT author) attempts to delete Bob's message
	_ = connCharlie.WriteJSON(map[string]interface{}{
		"type": "community.delete_message",
		"payload": map[string]string{
			"communityId": "comm-priv-100",
			"messageId":   "msg-bob-exclusive-1",
		},
	})

	var cErrBytes []byte
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = connCharlie.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, msg, err := connCharlie.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(msg), "community.error") {
			cErrBytes = msg
			break
		}
	}
	if len(cErrBytes) == 0 {
		t.Fatalf("expected community.error response to Charlie, got none")
	}
	if !strings.Contains(string(cErrBytes), "only the community creator or original message author") {
		t.Fatalf("expected unauthorized error for Charlie, got: %s", string(cErrBytes))
	}

	// 2. Bob (original author) deletes his own message -> SUCCEEDS
	_ = connBob.WriteJSON(map[string]interface{}{
		"type": "community.delete_message",
		"payload": map[string]string{
			"communityId": "comm-priv-100",
			"messageId":   "msg-bob-exclusive-1",
		},
	})

	// Alice (room listener) receives community.delete_message broadcast
	var aDelBytes []byte
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = connAlice.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, msg, err := connAlice.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(msg), "community.delete_message") {
			aDelBytes = msg
			break
		}
	}
	if len(aDelBytes) == 0 {
		t.Fatalf("Alice failed to receive delete broadcast")
	}
	var aEvt Event
	_ = json.Unmarshal(aDelBytes, &aEvt)
	if aEvt.Type != "community.delete_message" {
		t.Fatalf("expected community.delete_message, got: %s", aEvt.Type)
	}

	// 3. Bob sends another message, and Alice (community creator) deletes it -> SUCCEEDS
	_ = connBob.WriteJSON(map[string]interface{}{
		"type": "community.message",
		"payload": map[string]interface{}{
			"id":          "msg-bob-exclusive-2",
			"communityId": "comm-priv-100",
			"text":        "Bob second message",
		},
	})
	time.Sleep(50 * time.Millisecond)

	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.delete_message",
		"payload": map[string]string{
			"communityId": "comm-priv-100",
			"messageId":   "msg-bob-exclusive-2",
		},
	})

	var bDelBytes []byte
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = connBob.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, msg, err := connBob.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(msg), "community.delete_message") {
			bDelBytes = msg
			break
		}
	}
	if len(bDelBytes) == 0 {
		t.Fatalf("Bob failed to receive delete broadcast from creator Alice")
	}
	var bDelEvt Event
	_ = json.Unmarshal(bDelBytes, &bDelEvt)
	if bDelEvt.Type != "community.delete_message" {
		t.Fatalf("expected community.delete_message, got: %s", bDelEvt.Type)
	}
}

// Slice 5I Regression Test: ID collision prevention, authorship overwrite immunity, and deletion authorization
func TestRegression_Slice5I_CommunityMessageIDCollision_EveCannotOverwriteAuthorship(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)
	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Alice creates community "comm-collide-100" (Alice is Creator)
	aliceToken := createCustomToken("user-alice-id", "alice@example.com", "alice", "Alice")
	connAlice, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage()

	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.register",
		"payload": map[string]interface{}{
			"community": map[string]interface{}{
				"id":   "comm-collide-100",
				"name": "Collision Test Room",
			},
			"members": []map[string]string{
				{"handle": "eve", "email": "eve@example.com"},
			},
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Eve joins "comm-collide-100"
	eveToken := createCustomToken("user-eve-id", "eve@example.com", "eve", "Eve")
	connEve, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connEve.Close()
	_ = connEve.WriteJSON(map[string]string{"type": "auth", "token": eveToken})
	_, _, _ = connEve.ReadMessage()

	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "community.join_room",
		"payload": map[string]string{
			"communityId": "comm-collide-100",
		},
	})
	time.Sleep(50 * time.Millisecond)

	// 1. Alice publishes a legitimate message with ID "alice-original-msg-123"
	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.message",
		"payload": map[string]interface{}{
			"id":          "alice-original-msg-123",
			"communityId": "comm-collide-100",
			"text":        "Secret plans by Alice",
		},
	})

	// Eve receives Alice's broadcast
	_ = connEve.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, msgBytes, err := connEve.ReadMessage()
	if err != nil {
		t.Fatalf("Eve failed to receive Alice's broadcast: %v", err)
	}
	var aEvt Event
	_ = json.Unmarshal(msgBytes, &aEvt)
	if aEvt.Type != "community.message" {
		t.Fatalf("expected community.message, got %s", aEvt.Type)
	}

	// Verify server recorded Alice as author
	hub.mu.RLock()
	recordedAuthor := hub.commMsgAuthors["comm-collide-100#alice-original-msg-123"]
	hub.mu.RUnlock()
	if recordedAuthor != "user-alice-id" {
		t.Fatalf("expected author to be Alice, got %q", recordedAuthor)
	}

	// 2. Eve attempts to reuse Alice's message ID to overwrite authorship
	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "community.message",
		"payload": map[string]interface{}{
			"id":          "alice-original-msg-123",
			"communityId": "comm-collide-100",
			"text":        "Eve hostile overwrite attempt",
		},
	})

	// Eve must receive an immediate community.error rejecting the duplicate/colliding ID
	var eveErrBytes []byte
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = connEve.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, msg, err := connEve.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(msg), "community.error") {
			eveErrBytes = msg
			break
		}
	}
	if len(eveErrBytes) == 0 {
		t.Fatalf("expected Eve to receive community.error for colliding ID, got none")
	}
	if !strings.Contains(string(eveErrBytes), "duplicate or colliding message ID") {
		t.Fatalf("expected 'duplicate or colliding message ID' in error, got: %s", string(eveErrBytes))
	}

	// Alice must NOT receive any broadcast of Eve's colliding message (no duplicate message broadcast)
	_ = connAlice.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err = connAlice.ReadMessage()
	if err == nil {
		t.Fatalf("CRITICAL: Colliding message was broadcast to Alice!")
	}

	// Check: Eve DID NOT become the recorded author
	hub.mu.RLock()
	stillAliceAuthor := hub.commMsgAuthors["comm-collide-100#alice-original-msg-123"]
	hub.mu.RUnlock()
	if stillAliceAuthor != "user-alice-id" {
		t.Fatalf("CRITICAL: Authorship was overwritten! Expected Alice, got %q", stillAliceAuthor)
	}

	// 3. Eve attempts to delete Alice's message using the reused ID -> MUST FAIL
	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "community.delete_message",
		"payload": map[string]string{
			"communityId": "comm-collide-100",
			"messageId":   "alice-original-msg-123",
		},
	})

	var eveDelErrBytes []byte
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = connEve.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, msg, err := connEve.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(msg), "community.error") {
			eveDelErrBytes = msg
			break
		}
	}
	if len(eveDelErrBytes) == 0 {
		t.Fatalf("expected Eve to receive community.error when attempting unauthorized deletion")
	}
	if !strings.Contains(string(eveDelErrBytes), "only the community creator or original message author") {
		t.Fatalf("expected unauthorized error for Eve, got: %s", string(eveDelErrBytes))
	}

	// 4. Alice (original author) deletes her message -> MUST SUCCEED
	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.delete_message",
		"payload": map[string]string{
			"communityId": "comm-collide-100",
			"messageId":   "alice-original-msg-123",
		},
	})

	var eveDelNotif []byte
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = connEve.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, msg, err := connEve.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(msg), "community.delete_message") {
			eveDelNotif = msg
			break
		}
	}
	if len(eveDelNotif) == 0 {
		t.Fatalf("expected Eve to receive community.delete_message broadcast from Alice's deletion")
	}

	// 5. Creator retains deletion permission for other members' messages
	_ = connEve.WriteJSON(map[string]interface{}{
		"type": "community.message",
		"payload": map[string]interface{}{
			"id":          "eve-legit-unique-999",
			"communityId": "comm-collide-100",
			"text":        "Eve unique legitimate message",
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Alice (creator) deletes Eve's message
	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.delete_message",
		"payload": map[string]string{
			"communityId": "comm-collide-100",
			"messageId":   "eve-legit-unique-999",
		},
	})

	var eveDelNotif2 []byte
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = connEve.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, msg, err := connEve.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(msg), "community.delete_message") {
			eveDelNotif2 = msg
			break
		}
	}
	if len(eveDelNotif2) == 0 {
		t.Fatalf("expected Eve to receive community.delete_message broadcast from Creator Alice")
	}
}

// Slice 5I Regression Test: Concurrency and simultaneous duplicate ID submission race test
func TestRegression_Slice5I_ConcurrentDuplicateIDSubmissions(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)
	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Register community "comm-race-100"
	aliceToken := createCustomToken("user-alice-id", "alice@example.com", "alice", "Alice")
	connAlice, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage()

	var members []map[string]string
	for i := 0; i < 10; i++ {
		members = append(members, map[string]string{
			"handle": fmt.Sprintf("user%d", i),
			"email":  fmt.Sprintf("user%d@example.com", i),
		})
	}

	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.register",
		"payload": map[string]interface{}{
			"community": map[string]interface{}{
				"id":   "comm-race-100",
				"name": "Race Test Room",
			},
			"members": members,
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Spin up 10 concurrent clients submitting with the EXACT SAME ID
	conflictID := "race-target-conflict-id"
	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			token := createCustomToken(fmt.Sprintf("user-sub-%d", idx), fmt.Sprintf("user%d@example.com", idx), fmt.Sprintf("user%d", idx), fmt.Sprintf("User%d", idx))
			conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_ = conn.WriteJSON(map[string]string{"type": "auth", "token": token})
			_, _, _ = conn.ReadMessage()

			_ = conn.WriteJSON(map[string]interface{}{
				"type": "community.join_room",
				"payload": map[string]string{
					"communityId": "comm-race-100",
				},
			})
			time.Sleep(20 * time.Millisecond)

			_ = conn.WriteJSON(map[string]interface{}{
				"type": "community.message",
				"payload": map[string]interface{}{
					"id":          conflictID,
					"communityId": "comm-race-100",
					"text":        fmt.Sprintf("Concurrent message from %d", idx),
				},
			})

			// Check if this client receives a community.error
			_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					break
				}
				if strings.Contains(string(msg), "duplicate or colliding message ID") {
					atomic.AddInt64(&errorCount, 1)
					return
				}
			}
			atomic.AddInt64(&successCount, 1)
		}(i)
	}
	wg.Wait()

	// Exactly 1 winner should succeed and 9 should be rejected
	if errorCount < 9 {
		t.Fatalf("expected at least 9 clients rejected due to ID collision, got errors=%d, successes=%d", errorCount, successCount)
	}

	// Authorship in hub must be exactly 1 record
	hub.mu.RLock()
	author := hub.commMsgAuthors["comm-race-100#"+conflictID]
	hub.mu.RUnlock()
	if author == "" {
		t.Fatalf("expected one author recorded for %s, got empty", conflictID)
	}
}

// Slice 5I Regression Test: Empty ID generates unique server-side ID, records author, and allows deletion
func TestRegression_Slice5I_EmptyMessageID_ServerGeneratedAndCollisionFree(t *testing.T) {
	hub := NewHub()
	handler := ServeWS(hub, testSecret)
	server := httptest.NewServer(handler)
	defer server.Close()

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"

	// Alice registers and joins "comm-gen-100"
	aliceToken := createCustomToken("user-alice-id", "alice@example.com", "alice", "Alice")
	connAlice, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connAlice.Close()
	_ = connAlice.WriteJSON(map[string]string{"type": "auth", "token": aliceToken})
	_, _, _ = connAlice.ReadMessage()

	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.register",
		"payload": map[string]interface{}{
			"community": map[string]interface{}{
				"id":   "comm-gen-100",
				"name": "Auto ID Room",
			},
			"members": []map[string]string{
				{"handle": "bob", "email": "bob@example.com"},
			},
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Bob joins
	bobToken := createCustomToken("user-bob-id", "bob@example.com", "bob", "Bob")
	connBob, _, _ := websocket.DefaultDialer.Dial(u.String(), nil)
	defer connBob.Close()
	_ = connBob.WriteJSON(map[string]string{"type": "auth", "token": bobToken})
	_, _, _ = connBob.ReadMessage()

	_ = connBob.WriteJSON(map[string]interface{}{
		"type": "community.join_room",
		"payload": map[string]string{
			"communityId": "comm-gen-100",
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Alice sends with empty id
	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.message",
		"payload": map[string]interface{}{
			"id":          "",
			"communityId": "comm-gen-100",
			"text":        "Message with no client ID",
		},
	})

	// Bob receives broadcast
	_ = connBob.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, msgBytes, err := connBob.ReadMessage()
	if err != nil {
		t.Fatalf("Bob failed to receive broadcast: %v", err)
	}

	var bEvt Event
	_ = json.Unmarshal(msgBytes, &bEvt)
	payload := bEvt.Payload.(map[string]interface{})
	generatedID := payload["id"].(string)

	if !strings.HasPrefix(generatedID, "comm-msg-") {
		t.Fatalf("expected server-generated ID starting with 'comm-msg-', got %q", generatedID)
	}

	// Verify authorship was recorded under generatedID
	hub.mu.RLock()
	author := hub.commMsgAuthors["comm-gen-100#"+generatedID]
	hub.mu.RUnlock()
	if author != "user-alice-id" {
		t.Fatalf("expected author for generated ID to be Alice, got %q", author)
	}

	// Alice deletes using generated ID -> SUCCEEDS
	_ = connAlice.WriteJSON(map[string]interface{}{
		"type": "community.delete_message",
		"payload": map[string]string{
			"communityId": "comm-gen-100",
			"messageId":   generatedID,
		},
	})

	var delEvt Event
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = connBob.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, delBytes, err := connBob.ReadMessage()
		if err != nil {
			break
		}
		if strings.Contains(string(delBytes), "community.delete_message") {
			_ = json.Unmarshal(delBytes, &delEvt)
			break
		}
	}
	if delEvt.Type != "community.delete_message" {
		t.Fatalf("expected community.delete_message, got %s", delEvt.Type)
	}
}
