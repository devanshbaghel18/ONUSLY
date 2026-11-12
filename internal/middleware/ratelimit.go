package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type clientRecord struct {
	timestamps []time.Time
}

// RateLimiter implements an in-memory sliding-window rate limiter per client key (IP or user ID).
type RateLimiter struct {
	mu          sync.Mutex
	limit       int
	window      time.Duration
	records     map[string]*clientRecord
	lastCleanup time.Time
	keyFunc     func(r *http.Request) string
}

// NewRateLimiter creates a new RateLimiter with the specified maximum requests per window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:       limit,
		window:      window,
		records:     make(map[string]*clientRecord),
		lastCleanup: time.Now(),
	}
}

// NewUserRateLimiter creates a RateLimiter keyed by authenticated user ID (from context),
// falling back to client IP for unauthenticated callers or when user ID is unavailable.
func NewUserRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		limit:       limit,
		window:      window,
		records:     make(map[string]*clientRecord),
		lastCleanup: time.Now(),
	}
	rl.keyFunc = func(r *http.Request) string {
		if uid, ok := GetUserID(r.Context()); ok && strings.TrimSpace(uid) != "" {
			return "user:" + strings.TrimSpace(uid)
		}
		return "ip:" + rl.getClientIP(r)
	}
	return rl
}

func (rl *RateLimiter) getClientIP(r *http.Request) string {
	// If X-Forwarded-For is provided, extract the first client IP
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

// Limit returns a middleware handler that enforces the rate limit.
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var key string
		if rl.keyFunc != nil {
			key = rl.keyFunc(r)
		} else {
			key = rl.getClientIP(r)
		}
		now := time.Now()

		rl.mu.Lock()

		// Periodic cleanup every 5 minutes to prevent unbounded memory growth
		if now.Sub(rl.lastCleanup) > 5*time.Minute {
			cutoff := now.Add(-rl.window)
			for k, v := range rl.records {
				var valid []time.Time
				for _, t := range v.timestamps {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(rl.records, k)
				} else {
					v.timestamps = valid
				}
			}
			rl.lastCleanup = now
		}

		rec, exists := rl.records[key]
		if !exists {
			rec = &clientRecord{}
			rl.records[key] = rec
		}

		// Prune entries outside current window
		cutoff := now.Add(-rl.window)
		var valid []time.Time
		for _, t := range rec.timestamps {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= rl.limit {
			rl.mu.Unlock()
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "rate_limit_exceeded",
				"message": "Too many requests. Please try again later.",
			})
			return
		}

		valid = append(valid, now)
		rec.timestamps = valid
		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
