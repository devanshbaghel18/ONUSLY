package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	UserIDKey    contextKey = "userID"
	TokenTypeKey contextKey = "token_type"
)

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
				// Strict algorithm allowlist: strictly require HS256 to prevent algorithm confusion attacks
				if token.Method != jwt.SigningMethodHS256 {
					return nil, jwt.ErrSignatureInvalid
				}
				if alg, ok := token.Header["alg"].(string); !ok || alg != "HS256" {
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
			userID = strings.TrimSpace(userID)
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

			// Token type derivation and validation
			var tokenType string
			if rawType, ok := claims["token_type"].(string); ok && rawType != "" {
				rawType = strings.ToLower(strings.TrimSpace(rawType))
				if rawType != "user" && rawType != "device" {
					http.Error(w, "invalid token claims: unrecognized token type", http.StatusUnauthorized)
					return
				}
				tokenType = rawType
			} else {
				// Backward compatibility for existing tokens without explicit token_type
				if devID, ok := claims["device_id"].(string); ok && strings.TrimSpace(devID) != "" {
					tokenType = "device"
				} else {
					tokenType = "user"
				}
			}

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			ctx = context.WithValue(ctx, TokenTypeKey, tokenType)

			if email, ok := claims["email"].(string); ok && email != "" {
				ctx = context.WithValue(ctx, UserEmailKey, strings.ToLower(strings.TrimSpace(email)))
			}
			if handle, ok := claims["handle"].(string); ok && handle != "" {
				ctx = context.WithValue(ctx, UserHandleKey, strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@")))
			}
			if name, ok := claims["name"].(string); ok && name != "" {
				ctx = context.WithValue(ctx, UserNameKey, strings.TrimSpace(name))
			}
			if devID, ok := claims["device_id"].(string); ok && devID != "" {
				ctx = context.WithValue(ctx, DeviceIDKey, strings.TrimSpace(devID))
			} else if headerDevID := r.Header.Get("X-Device-ID"); headerDevID != "" && tokenType == "device" {
				ctx = context.WithValue(ctx, DeviceIDKey, strings.TrimSpace(headerDevID))
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireUserToken ensures that the authenticated caller has a "user" token type.
func RequireUserToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenType, ok := GetTokenType(r.Context())
		if !ok || tokenType != "user" {
			http.Error(w, "forbidden: user session token required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireDeviceToken ensures that the authenticated caller has a "device" token type.
func RequireDeviceToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenType, ok := GetTokenType(r.Context())
		if !ok || tokenType != "device" {
			http.Error(w, "forbidden: device session token required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type emailContextKey string
type handleContextKey string
type nameContextKey string
type deviceIDContextKey string

const UserEmailKey emailContextKey = "user_email"
const UserHandleKey handleContextKey = "user_handle"
const UserNameKey nameContextKey = "user_name"
const DeviceIDKey deviceIDContextKey = "device_id"

func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	return userID, ok
}

func GetTokenType(ctx context.Context) (string, bool) {
	tokenType, ok := ctx.Value(TokenTypeKey).(string)
	return tokenType, ok
}

func GetDeviceID(ctx context.Context) (string, bool) {
	deviceID, ok := ctx.Value(DeviceIDKey).(string)
	return deviceID, ok
}

func GetUserEmail(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(UserEmailKey).(string)
	return email, ok
}

func GetUserHandle(ctx context.Context) (string, bool) {
	handle, ok := ctx.Value(UserHandleKey).(string)
	return handle, ok
}

func GetUserName(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(UserNameKey).(string)
	return name, ok
}
