package middleware_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

func TestIdempotencyMiddleware_ReplayIdenticalRequest(t *testing.T) {
	store := middleware.NewMemoryIdempotencyStore(1 * time.Hour)
	var handlerExecutions int32

	backendHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&handlerExecutions, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"goal-123","status":"created"}`))
	})

	idempotentHandler := middleware.Idempotency(store)(backendHandler)

	reqBody := `{"title":"Read Book"}`

	// First Request
	req1 := httptest.NewRequest("POST", "/goals", bytes.NewBufferString(reqBody))
	req1.Header.Set("Idempotency-Key", "key-test-1")
	rec1 := httptest.NewRecorder()
	idempotentHandler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusCreated {
		t.Fatalf("first request expected 201, got %d", rec1.Code)
	}
	if rec1.Header().Get("Idempotent-Replay") == "true" {
		t.Fatalf("first request should not be flagged as replay")
	}
	if atomic.LoadInt32(&handlerExecutions) != 1 {
		t.Fatalf("expected handler executed once, got %d", handlerExecutions)
	}

	// Second (Replay) Request with identical key and payload
	req2 := httptest.NewRequest("POST", "/goals", bytes.NewBufferString(reqBody))
	req2.Header.Set("Idempotency-Key", "key-test-1")
	rec2 := httptest.NewRecorder()
	idempotentHandler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Fatalf("replay request expected 201, got %d", rec2.Code)
	}
	if rec2.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("expected Idempotent-Replay: true header on second request")
	}
	if rec2.Body.String() != `{"id":"goal-123","status":"created"}` {
		t.Fatalf("expected identical cached body, got: %s", rec2.Body.String())
	}
	// Handler must NOT have executed a second time!
	if atomic.LoadInt32(&handlerExecutions) != 1 {
		t.Fatalf("handler must NOT execute again on replay, got %d executions", handlerExecutions)
	}
}

func TestIdempotencyMiddleware_PayloadMismatchRejection(t *testing.T) {
	store := middleware.NewMemoryIdempotencyStore(1 * time.Hour)

	backendHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	idempotentHandler := middleware.Idempotency(store)(backendHandler)

	// First Request
	req1 := httptest.NewRequest("POST", "/goals", bytes.NewBufferString(`{"title":"Initial"}`))
	req1.Header.Set("Idempotency-Key", "key-mismatch-test")
	rec1 := httptest.NewRecorder()
	idempotentHandler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec1.Code)
	}

	// Second Request with same key but DIFFERENT payload
	req2 := httptest.NewRequest("POST", "/goals", bytes.NewBufferString(`{"title":"TAMPERED"}`))
	req2.Header.Set("Idempotency-Key", "key-mismatch-test")
	rec2 := httptest.NewRecorder()
	idempotentHandler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for payload mismatch, got %d", rec2.Code)
	}
}

func TestIdempotencyMiddleware_InFlightConflict(t *testing.T) {
	store := middleware.NewMemoryIdempotencyStore(1 * time.Hour)

	started := make(chan struct{})
	release := make(chan struct{})

	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusCreated)
	})

	idempotentHandler := middleware.Idempotency(store)(slowHandler)

	var wg sync.WaitGroup
	wg.Add(1)

	var rec1 *httptest.ResponseRecorder
	go func() {
		defer wg.Done()
		req1 := httptest.NewRequest("POST", "/goals", bytes.NewBufferString(`{"title":"Slow"}`))
		req1.Header.Set("Idempotency-Key", "key-inflight-test")
		rec1 = httptest.NewRecorder()
		idempotentHandler.ServeHTTP(rec1, req1)
	}()

	// Wait until request 1 is actively inside the handler holding the lock
	<-started

	// Request 2 arrives concurrently with the same key
	req2 := httptest.NewRequest("POST", "/goals", bytes.NewBufferString(`{"title":"Slow"}`))
	req2.Header.Set("Idempotency-Key", "key-inflight-test")
	rec2 := httptest.NewRecorder()
	idempotentHandler.ServeHTTP(rec2, req2)

	// Release request 1
	close(release)
	wg.Wait()

	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for in-flight request, got %d", rec2.Code)
	}
	if rec1.Code != http.StatusCreated {
		t.Fatalf("expected original request 1 to succeed with 201, got %d", rec1.Code)
	}
}

func TestIdempotencyMiddleware_UserIsolation(t *testing.T) {
	store := middleware.NewMemoryIdempotencyStore(1 * time.Hour)

	backendHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := middleware.GetUserID(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"user":"` + userID + `"}`))
	})

	idempotentHandler := middleware.Idempotency(store)(backendHandler)

	// User 1 uses key-A
	req1 := httptest.NewRequest("POST", "/goals", bytes.NewBufferString(`{"payload":1}`))
	req1.Header.Set("Idempotency-Key", "key-shared")
	ctx1 := context.WithValue(req1.Context(), middleware.UserIDKey, "user-111")
	rec1 := httptest.NewRecorder()
	idempotentHandler.ServeHTTP(rec1, req1.WithContext(ctx1))

	// User 2 uses SAME key-A
	req2 := httptest.NewRequest("POST", "/goals", bytes.NewBufferString(`{"payload":2}`))
	req2.Header.Set("Idempotency-Key", "key-shared")
	ctx2 := context.WithValue(req2.Context(), middleware.UserIDKey, "user-222")
	rec2 := httptest.NewRecorder()
	idempotentHandler.ServeHTTP(rec2, req2.WithContext(ctx2))

	if rec1.Body.String() != `{"user":"user-111"}` {
		t.Fatalf("expected user-111 response, got %s", rec1.Body.String())
	}
	// User 2 must NOT collide with User 1's key because scope is tenant-isolated
	if rec2.Body.String() != `{"user":"user-222"}` {
		t.Fatalf("expected user-222 response, got %s", rec2.Body.String())
	}
}

