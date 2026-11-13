package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders applies protective HTTP response headers mitigating XSS, clickjacking,
// MIME-type sniffing, and cross-origin resource leakage.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// Allow embedded preview of uploaded verification proof documents in dashboard iframes
		if strings.HasPrefix(r.URL.Path, "/uploads/") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'self' http://localhost:* http://127.0.0.1:* https://*.onusly.com;")
		} else {
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' ws: wss: http: https:; frame-ancestors 'none';")
		}

		next.ServeHTTP(w, r)
	})
}

// MaxBytesLimiter wraps incoming request bodies with http.MaxBytesReader to enforce a strict
// ceiling on JSON and API payload sizes, preventing memory exhaustion and denial-of-service.
func MaxBytesLimiter(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && maxBytes > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
