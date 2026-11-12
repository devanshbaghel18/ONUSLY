package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

	// 7. Valid cookie ("token") -> 200 OK without Authorization header
	req = httptest.NewRequest("GET", "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: signedValid})
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for valid cookie token, got %d", rec.Code)
	}
	if rec.Body.String() != "ok:legitimate-user-789" {
		t.Errorf("expected body 'ok:legitimate-user-789', got %s", rec.Body.String())
	}

	// 8. Valid cookie ("onusly_token") -> 200 OK without Authorization header
	req = httptest.NewRequest("GET", "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "onusly_token", Value: signedValid})
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for valid onusly_token cookie, got %d", rec.Code)
	}
	if rec.Body.String() != "ok:legitimate-user-789" {
		t.Errorf("expected body 'ok:legitimate-user-789', got %s", rec.Body.String())
	}
}

type mockSessionValidator struct {
	versions map[string]int
}

func (m *mockSessionValidator) GetTokenVersion(ctx context.Context, userID string) (int, error) {
	if v, ok := m.versions[userID]; ok {
		return v, nil
	}
	return 1, nil
}

func TestAuthMiddleware_SessionVersionRevocation(t *testing.T) {
	jwtSecret := "test-secret-version-12345"
	validator := &mockSessionValidator{
		versions: map[string]int{
			"user-alice": 1,
			"user-bob":   2, // user-bob revoked prior sessions!
		},
	}

	authMiddleware := middleware.Auth(jwtSecret, middleware.WithSessionValidator(validator))
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := middleware.GetUserID(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok:" + uid))
	})
	protected := authMiddleware(nextHandler)

	makeToken := func(userID string, ver int) string {
		claims := jwt.MapClaims{
			"sub":           userID,
			"token_type":    "user",
			"token_version": ver,
			"exp":           time.Now().Add(time.Hour).Unix(),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		s, _ := tok.SignedString([]byte(jwtSecret))
		return s
	}

	// 1. User Alice with version 1 -> matches expected version 1 -> 200 OK
	aliceTokenV1 := makeToken("user-alice", 1)
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+aliceTokenV1)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for matching session version, got %d", rec.Code)
	}

	// 2. User Bob presenting older token with version 1 (stolen or prior session) -> expected is 2 -> 401 Unauthorized
	bobTokenV1 := makeToken("user-bob", 1)
	req = httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+bobTokenV1)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked session version, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "session revoked") {
		t.Fatalf("expected 'session revoked' message, got: %s", rec.Body.String())
	}

	// 3. User Bob with newly issued token version 2 -> matches expected 2 -> 200 OK
	bobTokenV2 := makeToken("user-bob", 2)
	req = httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+bobTokenV2)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for updated session version, got %d", rec.Code)
	}

	// 4. Also works via Cookie
	req = httptest.NewRequest("GET", "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: bobTokenV2})
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for cookie with updated session version, got %d", rec.Code)
	}

	// 5. Stolen cookie with old version 1 -> 401 Unauthorized
	req = httptest.NewRequest("GET", "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: bobTokenV1})
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for stolen cookie with old session version, got %d", rec.Code)
	}
}
