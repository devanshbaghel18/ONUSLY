package notifications

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func getUserID(r *http.Request) string {
	if uid, ok := r.Context().Value(middleware.UserIDKey).(string); ok && uid != "" {
		return uid
	}
	if uid, ok := r.Context().Value("userID").(string); ok && uid != "" {
		return uid
	}
	return ""
}

// List handles GET /notifications.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if userID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	resp, err := h.service.List(r.Context(), userID, 50)
	if err != nil {
		http.Error(w, `{"error":"failed to load notifications"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// MarkAsRead handles PATCH /notifications/{id}/read.
func (h *Handler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if userID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	notifID := strings.TrimSpace(r.PathValue("id"))
	if notifID == "" {
		http.Error(w, `{"error":"missing notification id"}`, http.StatusBadRequest)
		return
	}

	err := h.service.MarkAsRead(r.Context(), userID, notifID)
	if err != nil {
		if errors.Is(err, ErrNotificationNotFound) {
			http.Error(w, `{"error":"notification not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to mark notification as read"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// MarkAllAsRead handles POST /notifications/read-all.
func (h *Handler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if userID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	err := h.service.MarkAllAsRead(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"failed to mark all notifications as read"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
