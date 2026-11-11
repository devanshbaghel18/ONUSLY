package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/metrics"
)

// IdempotencyRecord stores the cached response for an idempotent operation.
type IdempotencyRecord struct {
	Key          string            `json:"key"`
	Scope        string            `json:"scope"`
	RequestHash  string            `json:"requestHash"`
	StatusCode   int               `json:"statusCode"`
	Headers      map[string]string `json:"headers"`
	ResponseBody []byte            `json:"responseBody"`
	CreatedAt    time.Time         `json:"createdAt"`
	ExpiresAt    time.Time         `json:"expiresAt"`
}

// IdempotencyStore defines the storage contract for idempotency tracking.
type IdempotencyStore interface {
	Get(ctx context.Context, scope, key string) (*IdempotencyRecord, error)
	Put(ctx context.Context, record IdempotencyRecord) error
	Lock(ctx context.Context, scope, key string, ttl time.Duration) (bool, error)
	Unlock(ctx context.Context, scope, key string) error
}

// MemoryIdempotencyStore implements an in-memory, thread-safe idempotency store with automatic TTL.
type MemoryIdempotencyStore struct {
	mu      sync.RWMutex
	records map[string]IdempotencyRecord // compositeKey -> record
	locks   map[string]time.Time         // compositeKey -> lockExpiration
	ttl     time.Duration
}

// NewMemoryIdempotencyStore creates an in-memory store with the given retention TTL.
func NewMemoryIdempotencyStore(ttl time.Duration) *MemoryIdempotencyStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	s := &MemoryIdempotencyStore{
		records: make(map[string]IdempotencyRecord),
		locks:   make(map[string]time.Time),
		ttl:     ttl,
	}
	// Background cleanup of expired entries
	go s.cleanupLoop()
	return s
}

func (s *MemoryIdempotencyStore) compositeKey(scope, key string) string {
	return scope + "::" + key
}

func (s *MemoryIdempotencyStore) Get(ctx context.Context, scope, key string) (*IdempotencyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ck := s.compositeKey(scope, key)
	rec, ok := s.records[ck]
	if !ok {
		return nil, nil
	}
	if time.Now().After(rec.ExpiresAt) {
		return nil, nil
	}
	return &rec, nil
}

func (s *MemoryIdempotencyStore) Put(ctx context.Context, record IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ck := s.compositeKey(record.Scope, record.Key)
	if record.ExpiresAt.IsZero() {
		record.ExpiresAt = time.Now().Add(s.ttl)
	}
	s.records[ck] = record
	delete(s.locks, ck)
	return nil
}

func (s *MemoryIdempotencyStore) Lock(ctx context.Context, scope, key string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ck := s.compositeKey(scope, key)

	// Check if already locked and not expired
	if exp, ok := s.locks[ck]; ok && time.Now().Before(exp) {
		return false, nil // Lock currently held
	}

	// Check if already completed
	if rec, ok := s.records[ck]; ok && time.Now().Before(rec.ExpiresAt) {
		return false, nil
	}

	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	s.locks[ck] = time.Now().Add(ttl)
	return true, nil
}

func (s *MemoryIdempotencyStore) Unlock(ctx context.Context, scope, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ck := s.compositeKey(scope, key)
	delete(s.locks, ck)
	return nil
}

func (s *MemoryIdempotencyStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for k, rec := range s.records {
			if now.After(rec.ExpiresAt) {
				delete(s.records, k)
			}
		}
		for k, exp := range s.locks {
			if now.After(exp) {
				delete(s.locks, k)
			}
		}
		s.mu.Unlock()
	}
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (w *responseRecorder) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseRecorder) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// Idempotency returns HTTP middleware enforcing idempotent semantics on mutation requests.
func Idempotency(store IdempotencyStore) func(http.Handler) http.Handler {
	if store == nil {
		store = NewMemoryIdempotencyStore(24 * time.Hour)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Idempotency strictly applies to mutation operations
			switch r.Method {
			case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
			default:
				next.ServeHTTP(w, r)
				return
			}

			// Extract idempotency key from canonical or X- prefix header
			key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
			if key == "" {
				key = strings.TrimSpace(r.Header.Get("X-Idempotency-Key"))
			}

			// If no idempotency key supplied, pass through transparently
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Determine tenant scope
			scope := "public:" + r.URL.Path
			if userID, ok := GetUserID(r.Context()); ok && userID != "" {
				scope = "user:" + userID
			}

			// Read and hash request payload
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read request body", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			hash := sha256.Sum256(bodyBytes)
			requestHash := hex.EncodeToString(hash[:])

			// 1. Check if previously completed
			rec, err := store.Get(r.Context(), scope, key)
			if err == nil && rec != nil {
				// Verify payload integrity
				if rec.RequestHash != requestHash {
					metrics.DefaultRegistry.RecordIdempotencyConflict(r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnprocessableEntity)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"error":   "IDEMPOTENCY_PAYLOAD_MISMATCH",
						"message": "Idempotency key was previously used with a different request payload.",
					})
					return
				}

				// Replay cached response
				metrics.DefaultRegistry.RecordIdempotencyHit(r.URL.Path)
				for hK, hV := range rec.Headers {
					w.Header().Set(hK, hV)
				}
				w.Header().Set("Idempotent-Replay", "true")
				w.WriteHeader(rec.StatusCode)
				_, _ = w.Write(rec.ResponseBody)
				return
			}

			// 2. Acquire in-flight execution lock
			locked, err := store.Lock(r.Context(), scope, key, 30*time.Second)
			if err != nil || !locked {
				// Another request with this key is currently being processed
				metrics.DefaultRegistry.RecordIdempotencyConflict(r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":   "IDEMPOTENCY_IN_FLIGHT",
					"message": "A request with this idempotency key is currently in flight. Please retry shortly.",
				})
				return
			}

			defer func() {
				_ = store.Unlock(r.Context(), scope, key)
			}()

			// 3. Execute downstream handler and buffer response
			recWriter := &responseRecorder{
				ResponseWriter: w,
				statusCode:     http.StatusOK, // default if WriteHeader not called explicitly
				body:           bytes.NewBuffer(nil),
			}

			next.ServeHTTP(recWriter, r)

			// 4. Record successful or completed response (status codes < 500)
			if recWriter.statusCode < 500 {
				cachedHeaders := make(map[string]string)
				for hK := range w.Header() {
					cachedHeaders[hK] = w.Header().Get(hK)
				}

				_ = store.Put(r.Context(), IdempotencyRecord{
					Key:          key,
					Scope:        scope,
					RequestHash:  requestHash,
					StatusCode:   recWriter.statusCode,
					Headers:      cachedHeaders,
					ResponseBody: recWriter.body.Bytes(),
					CreatedAt:    time.Now().UTC(),
					ExpiresAt:    time.Now().UTC().Add(24 * time.Hour),
				})
			}
		})
	}
}

