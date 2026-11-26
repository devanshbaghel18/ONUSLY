package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Helper to send JSON responses
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func (h *Handler) resolveFrontendURL() string {
	frontendURL := strings.TrimSpace(h.service.cfg.FrontendURL)
	if frontendURL == "" {
		frontendURL = strings.TrimSpace(os.Getenv("FRONTEND_URL"))
	}
	if frontendURL == "" {
		frontendURL = "http://localhost:5173"
	}
	return strings.TrimRight(frontendURL, "/")
}

func (h *Handler) buildSessionCookie(r *http.Request, name, value string, maxAge int) *http.Cookie {
	isSecure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	sameSite := http.SameSiteLaxMode
	if isSecure {
		frontendURL := h.resolveFrontendURL()
		if parsed, err := url.Parse(frontendURL); err == nil && parsed.Host != "" && !strings.EqualFold(parsed.Host, r.Host) {
			sameSite = http.SameSiteNoneMode
		}
	}
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: sameSite,
	}
	if maxAge < 0 {
		c.Expires = time.Unix(0, 0)
	}
	return c
}

func (h *Handler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	var req GoogleLoginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	token := req.IDToken
	if token == "" {
		token = req.Credential
	}

	resp, err := h.service.GoogleLogin(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": err.Error(),
		})
		return
	}

	http.SetCookie(w, h.buildSessionCookie(r, "token", resp.Token, 7*24*3600))

	writeJSON(w, http.StatusOK, resp)
}

// GoogleLoginRedirect initiates the OAuth2 authorization code flow with a CSRF state cookie.
func (h *Handler) GoogleLoginRedirect(w http.ResponseWriter, r *http.Request) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		http.Error(w, "failed to generate oauth state", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(buf)
	isSecure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, h.service.GetAuthURL(state), http.StatusFound)
}

func (h *Handler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	stateQuery := strings.TrimSpace(r.URL.Query().Get("state"))
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateQuery == "" || stateCookie.Value == "" ||
		subtle.ConstantTimeCompare([]byte(stateQuery), []byte(stateCookie.Value)) != 1 {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	isSecure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	token, err := h.service.HandleCallback(r.Context(), code)
	if err != nil {
		http.Error(w, "auth failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	frontendURL := h.resolveFrontendURL()

	http.SetCookie(w, h.buildSessionCookie(r, "token", token, 7*24*3600))

	// Pass token in URL fragment rather than query parameter to prevent server log and Referer leaks.
	http.Redirect(w, r, frontendURL+"/#token="+url.QueryEscape(token), http.StatusFound)
}

// Logout clears the HttpOnly session cookie
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, h.buildSessionCookie(r, "token", "", -1))
	http.SetCookie(w, h.buildSessionCookie(r, "onusly_token", "", -1))

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "logged out successfully",
	})
}

// RevokeAllSessions increments the user's session version in repository and closes
// any active sessions/cookies across all devices, returning 200 OK.
func (h *Handler) RevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "unauthorized",
		})
		return
	}

	newVersion, err := h.service.RevokeAllSessions(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed revoking sessions: " + err.Error(),
		})
		return
	}

	// Also clear current browser cookie
	http.SetCookie(w, h.buildSessionCookie(r, "token", "", -1))
	http.SetCookie(w, h.buildSessionCookie(r, "onusly_token", "", -1))

	writeJSON(w, http.StatusOK, map[string]any{
		"message":      "all sessions have been successfully revoked",
		"tokenVersion": newVersion,
	})
}

// UpdateHandle allows an authenticated user to customize their public @handle.
func (h *Handler) UpdateHandle(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "unauthorized",
		})
		return
	}

	var req struct {
		Handle string `json:"handle"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	profile, err := h.service.UpdateUserHandle(r.Context(), userID, req.Handle)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

// LookupUser queries a user by public @handle and returns their PublicProfile (protects email).
func (h *Handler) LookupUser(w http.ResponseWriter, r *http.Request) {
	handle := strings.TrimSpace(r.URL.Query().Get("handle"))
	if handle == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing handle query parameter",
		})
		return
	}

	profile, err := h.service.LookupUserByHandle(r.Context(), handle)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

// GetMe returns the authenticated caller's profile.
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "unauthorized",
		})
		return
	}

	user, err := h.service.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "user not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, user)
}
