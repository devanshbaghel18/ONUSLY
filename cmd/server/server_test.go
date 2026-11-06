package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"
	u.Path = "/ws"

	claims := jwt.MapClaims{
		"sub": "test-server-user",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokString, _ := tok.SignedString([]byte(jwtSecret))

	// 2. Test /ws with query param token -> 401 Unauthorized (token URL leakage prevented)
	wsURLQuery := u.String() + "?token=" + tokString
	_, resp, err = websocket.DefaultDialer.Dial(wsURLQuery, nil)
	if err == nil {
		t.Fatalf("expected query parameter token to be rejected")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for query parameter token, got %d", resp.StatusCode)
	}

	// 3. Test /ws with first-message handshake -> 101 Switching Protocols + auth.success
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial /ws: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}

	_ = conn.WriteJSON(map[string]string{
		"type":  "auth",
		"token": tokString,
	})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, authMsg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read handshake response: %v", err)
	}
	if !strings.Contains(string(authMsg), "auth.success") {
		t.Fatalf("expected auth.success in handshake, got: %s", string(authMsg))
	}

	// Check user is registered in hub
	time.Sleep(50 * time.Millisecond)
	if !hub.IsUserOnline("test-server-user") {
		t.Fatalf("expected test-server-user to be online in hub")
	}

	// 4. Test /ws with Authorization header directly
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tokString)
	connHeader, respHeader, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		t.Fatalf("failed to dial with Authorization header: %v", err)
	}
	defer connHeader.Close()
	if respHeader.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols with header, got %d", respHeader.StatusCode)
	}
}

func TestDownloadsRouteIntegration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)

	downloadsDir := "../../frontend/public/downloads"
	fileServer := http.FileServer(http.Dir(downloadsDir))
	mux.Handle("GET /downloads/", http.StripPrefix("/downloads/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".crx") {
			w.Header().Set("Content-Type", "application/x-chrome-extension")
		} else if strings.HasSuffix(r.URL.Path, ".xml") {
			w.Header().Set("Content-Type", "application/xml")
		}
		fileServer.ServeHTTP(w, r)
	})))

	server := httptest.NewServer(mux)
	defer server.Close()

	// 1. Test updates.xml
	resp, err := http.Get(server.URL + "/downloads/updates.xml")
	if err != nil {
		t.Fatalf("failed to GET updates.xml: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for updates.xml, got %v", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Errorf("expected xml Content-Type, got %v", ct)
	}

	// 2. Test onusly.crx
	respCRX, err := http.Get(server.URL + "/downloads/onusly.crx")
	if err != nil {
		t.Fatalf("failed to GET onusly.crx: %v", err)
	}
	defer respCRX.Body.Close()
	if respCRX.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for onusly.crx, got %v", respCRX.StatusCode)
	}
	if ct := respCRX.Header.Get("Content-Type"); ct != "application/x-chrome-extension" {
		t.Errorf("expected application/x-chrome-extension, got %v", ct)
	}
}
