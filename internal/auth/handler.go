package auth

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

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

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
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

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:5173"
	}

	// redirect to frontend with token
	http.Redirect(w, r, frontendURL+"/?token="+token, http.StatusFound)
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