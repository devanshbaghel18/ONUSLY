package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

func TestAuthMiddleware(t *testing.T) {
	jwtSecret := "test-secret-key-12345"
	authMiddleware := middleware.Auth(jwtSecret)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserID(r.Context())
		if !ok || userID == "" {
			http.Error(w, "missing context user", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok:" + userID))
	})

	protected := authMiddleware(nextHandler)

	// 1. Missing header -> 401
	req := httptest.NewRequest("GET", "/protected", nil)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing header, got %d", rec.Code)
	}

	// 2. Non-Bearer header -> 401
	req = httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-bearer header, got %d", rec.Code)
	}

	// 3. Invalid signature -> 401
	wrongSecretToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signedWrong, _ := wrongSecretToken.SignedString([]byte("wrong-secret"))
	req = httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+signedWrong)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid signature, got %d", rec.Code)
	}

	// 4. Expired token -> 401
	expiredToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-123",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	signedExpired, _ := expiredToken.SignedString([]byte(jwtSecret))
	req = httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+signedExpired)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", rec.Code)
	}

	// 5. Missing sub claim -> 401
	noSubToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"email": "user@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	signedNoSub, _ := noSubToken.SignedString([]byte(jwtSecret))
	req = httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+signedNoSub)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for token missing sub claim, got %d", rec.Code)
	}

	// 6. Valid token -> 200 OK and extracts userID
	validToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "legitimate-user-789",
		"email": "legit@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	signedValid, _ := validToken.SignedString([]byte(jwtSecret))
	req = httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+signedValid)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for valid token, got %d", rec.Code)
	}
	if rec.Body.String() != "ok:legitimate-user-789" {
		t.Errorf("expected body 'ok:legitimate-user-789', got %s", rec.Body.String())
	}
}
