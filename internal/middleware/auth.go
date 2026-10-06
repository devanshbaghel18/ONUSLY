package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserIDKey contextKey = "userID"

func Auth(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			authHeader := r.Header.Get("Authorization")

			if authHeader == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)

			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "invalid authorization header", http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				if token.Method != jwt.SigningMethodHS256 {
					return nil, jwt.ErrSignatureInvalid
				}

				return []byte(jwtSecret), nil
			})

			if err != nil || !token.Valid {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "invalid token claims", http.StatusUnauthorized)
				return
			}

			userID, ok := claims["sub"].(string)
			if !ok || userID == "" {
				http.Error(w, "user ID missing from token", http.StatusUnauthorized)
				return
			}

			if exp, ok := claims["exp"].(float64); ok {
				if time.Now().Unix() > int64(exp) {
					http.Error(w, "token expired", http.StatusUnauthorized)
					return
				}
			}

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			if email, ok := claims["email"].(string); ok && email != "" {
				ctx = context.WithValue(ctx, UserEmailKey, strings.ToLower(strings.TrimSpace(email)))
			}
			if handle, ok := claims["handle"].(string); ok && handle != "" {
				ctx = context.WithValue(ctx, UserHandleKey, strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@")))
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type emailContextKey string
type handleContextKey string

const UserEmailKey emailContextKey = "user_email"
const UserHandleKey handleContextKey = "user_handle"

func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	return userID, ok
}

func GetUserEmail(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(UserEmailKey).(string)
	return email, ok
}

func GetUserHandle(ctx context.Context) (string, bool) {
	handle, ok := ctx.Value(UserHandleKey).(string)
	return handle, ok
}

