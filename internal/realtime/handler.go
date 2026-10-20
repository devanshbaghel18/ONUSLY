package realtime

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	// Allow all origins for development and reverse proxy flexibility
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// AuthUser represents the authenticated identity extracted from a JWT.
type AuthUser struct {
	ID     string
	Email  string
	Handle string
	Name   string
}

// AuthHandshakeMessage defines the expected format for first-message WebSocket authentication.
type AuthHandshakeMessage struct {
	Type    string          `json:"type"`
	Token   string          `json:"token,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type AuthHandshakePayload struct {
	Token string `json:"token"`
}

const authTimeout = 5 * time.Second

// ValidateToken parses and validates a JWT token string against the provided HS256 secret.
func ValidateToken(tokenString, jwtSecret string) (*AuthUser, error) {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, errors.New("missing authentication token")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(jwtSecret), nil
	})

	if err != nil || !token.Valid {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errors.New("token expired")
		}
		return nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	userID, ok := claims["sub"].(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return nil, errors.New("user ID missing from token claims")
	}

	email, _ := claims["email"].(string)
	handle, _ := claims["handle"].(string)
	name, _ := claims["name"].(string)

	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, errors.New("token expired")
		}
	}

	return &AuthUser{
		ID:     strings.TrimSpace(userID),
		Email:  strings.TrimSpace(strings.ToLower(email)),
		Handle: strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@")),
		Name:   strings.TrimSpace(name),
	}, nil
}

// ExtractAndValidateAuthUser extracts the JWT from the Authorization header and validates it.
// Query parameter tokens are explicitly rejected for security hardening.
func ExtractAndValidateAuthUser(r *http.Request, jwtSecret string) (*AuthUser, error) {
	if r.URL.Query().Get("token") != "" {
		return nil, errors.New("token in query parameters is disabled for security; use first-message authentication handshake or Authorization header")
	}

	authHeader := r.Header.Get("Authorization")
	var tokenString string
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenString = strings.TrimPrefix(authHeader, "Bearer ")
	}

	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, errors.New("missing authentication token")
	}

	return ValidateToken(tokenString, jwtSecret)
}

// ExtractAndValidateToken is kept for backward-compatibility with tests.
func ExtractAndValidateToken(r *http.Request, jwtSecret string) (string, error) {
	user, err := ExtractAndValidateAuthUser(r, jwtSecret)
	if err != nil {
		return "", err
	}
	return user.ID, nil
}

// ServeWS handles WebSocket connection upgrade requests.
func ServeWS(hub *Hub, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Reject query parameter tokens to prevent URL leakage
		if r.URL.Query().Get("token") != "" {
			http.Error(w, "token in query parameters is disabled for security; use first-message authentication handshake or Authorization header", http.StatusUnauthorized)
			return
		}

		// Check if Authorization header is provided for direct authentication (e.g. non-browser clients)
		var preAuthUser *AuthUser
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			user, err := ValidateToken(strings.TrimPrefix(authHeader, "Bearer "), jwtSecret)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
			preAuthUser = user
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// Upgrade failed (e.g. not a websocket handshake)
			return
		}

		if preAuthUser != nil {
			client := NewClient(hub, conn, preAuthUser.ID, preAuthUser.Email, preAuthUser.Handle, preAuthUser.Name)
			hub.Register(client)
			go client.WritePump()
			go client.ReadPump()
			return
		}

		// First-message handshake for browser clients (where custom headers cannot be set in WebSocket constructor)
		go handleHandshake(hub, conn, jwtSecret)
	}
}

func handleHandshake(hub *Hub, conn *websocket.Conn, jwtSecret string) {
	conn.SetReadLimit(maxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(authTimeout))

	messageType, message, err := conn.ReadMessage()
	if err != nil {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authentication timeout or read failure"),
			time.Now().Add(time.Second),
		)
		_ = conn.Close()
		return
	}

	if messageType != websocket.TextMessage {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid message format"),
			time.Now().Add(time.Second),
		)
		_ = conn.Close()
		return
	}

	var authMsg AuthHandshakeMessage
	if err := json.Unmarshal(message, &authMsg); err != nil || authMsg.Type != "auth" {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "first message must be auth"),
			time.Now().Add(time.Second),
		)
		_ = conn.Close()
		return
	}

	tokenStr := strings.TrimSpace(authMsg.Token)
	if tokenStr == "" && len(authMsg.Payload) > 0 {
		var p AuthHandshakePayload
		if err := json.Unmarshal(authMsg.Payload, &p); err == nil {
			tokenStr = strings.TrimSpace(p.Token)
		}
	}

	if tokenStr == "" {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "missing token"),
			time.Now().Add(time.Second),
		)
		_ = conn.Close()
		return
	}

	user, err := ValidateToken(tokenStr, jwtSecret)
	if err != nil {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, err.Error()),
			time.Now().Add(time.Second),
		)
		_ = conn.Close()
		return
	}

	// Successful authentication - reset deadline and send confirmation
	_ = conn.SetReadDeadline(time.Time{})

	resp, _ := json.Marshal(Event{
		Type: "auth.success",
		Payload: map[string]interface{}{
			"userId": user.ID,
			"handle": user.Handle,
		},
	})
	_ = conn.WriteMessage(websocket.TextMessage, resp)

	client := NewClient(hub, conn, user.ID, user.Email, user.Handle, user.Name)
	hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
