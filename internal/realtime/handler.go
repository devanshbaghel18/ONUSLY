package realtime

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins for development and reverse proxy flexibility
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ExtractAndValidateToken extracts the JWT from query param "token" or "Authorization" header
// and validates it against the provided HS256 secret.
func ExtractAndValidateToken(r *http.Request, jwtSecret string) (string, error) {
	tokenString := r.URL.Query().Get("token")
	if tokenString == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return "", errors.New("missing authentication token")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(jwtSecret), nil
	})

	if err != nil || !token.Valid {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", errors.New("token expired")
		}
		return "", errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims")
	}

	userID, ok := claims["sub"].(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return "", errors.New("user ID missing from token claims")
	}

	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return "", errors.New("token expired")
		}
	}

	return userID, nil
}

// ServeWS handles WebSocket connection upgrade requests.
func ServeWS(hub *Hub, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := ExtractAndValidateToken(r, jwtSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// Upgrade failed (e.g. not a websocket handshake)
			return
		}

		client := NewClient(hub, conn, userID)
		hub.Register(client)

		// Start reader and writer pumps
		go client.WritePump()
		go client.ReadPump()
	}
}
