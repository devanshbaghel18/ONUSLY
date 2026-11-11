package middleware_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

func TestD13_StrictAlgorithmAllowlist(t *testing.T) {
	jwtSecret := "secure-test-jwt-secret-key-32chars"
	authMiddleware := middleware.Auth(jwtSecret)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("authorized"))
	})
	protected := authMiddleware(nextHandler)

	// 1. Rejection of 'none' algorithm
	noneToken := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "attacker",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signedNone, _ := noneToken.SignedString(jwt.UnsafeAllowNoneSignatureType)

	reqNone := httptest.NewRequest("GET", "/test", nil)
	reqNone.Header.Set("Authorization", "Bearer "+signedNone)
	recNone := httptest.NewRecorder()
	protected.ServeHTTP(recNone, reqNone)
	if recNone.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for alg 'none', got %d", recNone.Code)
	}

	// 2. Rejection of fake/tampered algorithm header
	hs256Token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-456",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	hs256Token.Header["alg"] = "HS384" // Header mismatch
	signedTampered, _ := hs256Token.SignedString([]byte(jwtSecret))

	reqTampered := httptest.NewRequest("GET", "/test", nil)
	reqTampered.Header.Set("Authorization", "Bearer "+signedTampered)
	recTampered := httptest.NewRecorder()
	protected.ServeHTTP(recTampered, reqTampered)
	if recTampered.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for tampered alg header, got %d", recTampered.Code)
	}
}

func TestD13_TokenTypeSeparationAndEnforcement(t *testing.T) {
	jwtSecret := "secure-test-jwt-secret-key-32chars"
	authMiddleware := middleware.Auth(jwtSecret)

	// Handler requiring user token
	userOnlyHandler := authMiddleware(middleware.RequireUserToken(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenType, _ := middleware.GetTokenType(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("user-route:" + tokenType))
	})))

	// Handler requiring device token
	deviceOnlyHandler := authMiddleware(middleware.RequireDeviceToken(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenType, _ := middleware.GetTokenType(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("device-route:" + tokenType))
	})))

	// Create valid user token
	userToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":        "user-abc",
		"token_type": "user",
		"exp":        time.Now().Add(time.Hour).Unix(),
	})
	signedUser, _ := userToken.SignedString([]byte(jwtSecret))

	// Create valid device token
	deviceToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":        "user-abc",
		"device_id":  "device-xyz",
		"token_type": "device",
		"exp":        time.Now().Add(time.Hour).Unix(),
	})
	signedDevice, _ := deviceToken.SignedString([]byte(jwtSecret))

	// 1. User token accesses user route -> 200 OK
	req1 := httptest.NewRequest("GET", "/user-only", nil)
	req1.Header.Set("Authorization", "Bearer "+signedUser)
	rec1 := httptest.NewRecorder()
	userOnlyHandler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK || !strings.Contains(rec1.Body.String(), "user-route:user") {
		t.Fatalf("expected 200 for user token on user route, got %d: %s", rec1.Code, rec1.Body.String())
	}

	// 2. Device token accesses user route -> 403 Forbidden
	req2 := httptest.NewRequest("GET", "/user-only", nil)
	req2.Header.Set("Authorization", "Bearer "+signedDevice)
	rec2 := httptest.NewRecorder()
	userOnlyHandler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for device token on user route, got %d", rec2.Code)
	}

	// 3. Device token accesses device route -> 200 OK
	req3 := httptest.NewRequest("GET", "/device-only", nil)
	req3.Header.Set("Authorization", "Bearer "+signedDevice)
	rec3 := httptest.NewRecorder()
	deviceOnlyHandler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK || !strings.Contains(rec3.Body.String(), "device-route:device") {
		t.Fatalf("expected 200 for device token on device route, got %d: %s", rec3.Code, rec3.Body.String())
	}

	// 4. User token accesses device route -> 403 Forbidden
	req4 := httptest.NewRequest("GET", "/device-only", nil)
	req4.Header.Set("Authorization", "Bearer "+signedUser)
	rec4 := httptest.NewRecorder()
	deviceOnlyHandler.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for user token on device route, got %d", rec4.Code)
	}

	// 5. Unrecognized token type -> 401
	bogusToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":        "user-abc",
		"token_type": "superadmin_bypass",
		"exp":        time.Now().Add(time.Hour).Unix(),
	})
	signedBogus, _ := bogusToken.SignedString([]byte(jwtSecret))
	req5 := httptest.NewRequest("GET", "/user-only", nil)
	req5.Header.Set("Authorization", "Bearer "+signedBogus)
	rec5 := httptest.NewRecorder()
	userOnlyHandler.ServeHTTP(rec5, req5)
	if rec5.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token_type, got %d", rec5.Code)
	}
}

func TestD13_SecurityHeaders(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/any", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	headers := rec.Header()
	if headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing nosniff header, got %q", headers.Get("X-Content-Type-Options"))
	}
	if headers.Get("X-Frame-Options") != "DENY" {
		t.Errorf("missing DENY header, got %q", headers.Get("X-Frame-Options"))
	}
	if headers.Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Errorf("missing Referrer-Policy header, got %q", headers.Get("Referrer-Policy"))
	}
	if !strings.Contains(headers.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Errorf("CSP missing frame-ancestors 'none', got %q", headers.Get("Content-Security-Policy"))
	}
}

func TestD13_MaxBytesLimiter(t *testing.T) {
	// Limit to 100 bytes
	limiter := middleware.MaxBytesLimiter(100)
	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "payload too large: "+err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("read bytes: "))
		_, _ = w.Write(body)
	}))

	// 1. Small payload passes
	smallData := []byte("hello world small payload")
	reqSmall := httptest.NewRequest("POST", "/data", bytes.NewReader(smallData))
	recSmall := httptest.NewRecorder()
	handler.ServeHTTP(recSmall, reqSmall)
	if recSmall.Code != http.StatusOK {
		t.Fatalf("expected 200 for small payload, got %d", recSmall.Code)
	}

	// 2. Oversized payload is truncated/rejected
	oversizedData := bytes.Repeat([]byte("A"), 500)
	reqOver := httptest.NewRequest("POST", "/data", bytes.NewReader(oversizedData))
	recOver := httptest.NewRecorder()
	handler.ServeHTTP(recOver, reqOver)
	if recOver.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized payload, got %d", recOver.Code)
	}
}

func TestD13_RateLimiter(t *testing.T) {
	// Limit to 3 requests per 10 seconds
	rl := middleware.NewRateLimiter(3, 10*time.Second)
	handler := rl.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest("GET", "/limited", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d should have succeeded with 200, got %d", i, rec.Code)
		}
	}

	// 4th request must be rate limited (429)
	req4 := httptest.NewRequest("GET", "/limited", nil)
	req4.RemoteAddr = "192.168.1.100:12345"
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", rec4.Code)
	}
	if rec4.Header().Get("Retry-After") != "60" {
		t.Fatalf("expected Retry-After: 60 header, got %q", rec4.Header().Get("Retry-After"))
	}

	// Different IP should still succeed
	reqOther := httptest.NewRequest("GET", "/limited", nil)
	reqOther.RemoteAddr = "10.0.0.5:12345"
	recOther := httptest.NewRecorder()
	handler.ServeHTTP(recOther, reqOther)
	if recOther.Code != http.StatusOK {
		t.Fatalf("different IP should have succeeded with 200, got %d", recOther.Code)
	}
}
