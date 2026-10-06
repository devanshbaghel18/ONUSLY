package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/realtime"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

func TestWebSocketRouteIntegration(t *testing.T) {
	jwtSecret := "server-test-secret"
	hub := realtime.NewHub()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /ws", realtime.ServeWS(hub, jwtSecret))
	mux.HandleFunc("GET /ws/", realtime.ServeWS(hub, jwtSecret))

	handler := cors(mux)
	server := httptest.NewServer(handler)
	defer server.Close()

	// 1. Test /health
	resp, err := http.Get(server.URL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /health, got status %v, err %v", resp.StatusCode, err)
	}

	// 2. Test /ws without token -> 401
	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"
	u.Path = "/ws"

	_, resp, err = websocket.DefaultDialer.Dial(u.String(), nil)
	if err == nil {
		t.Fatalf("expected dial without token to fail")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}

	// 3. Test /ws with valid token -> 101 Switching Protocols
	claims := jwt.MapClaims{
		"sub": "test-server-user",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokString, _ := tok.SignedString([]byte(jwtSecret))

	wsURL := u.String() + "?token=" + tokString
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial /ws with valid token: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}

	// Check user is registered in hub
	time.Sleep(50 * time.Millisecond)
	if !hub.IsUserOnline("test-server-user") {
		t.Fatalf("expected test-server-user to be online in hub")
	}
}
