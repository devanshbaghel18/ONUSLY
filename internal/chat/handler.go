package chat

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{
		repo: repo,
	}
}

func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	callerEmail, ok := middleware.GetUserEmail(r.Context())
	if !ok || strings.TrimSpace(callerEmail) == "" {
		http.Error(w, "unauthorized: email required in token", http.StatusUnauthorized)
		return
	}

	peer := strings.TrimSpace(r.URL.Query().Get("peer"))
	if peer == "" {
		http.Error(w, "missing required query parameter: peer", http.StatusBadRequest)
		return
	}

	msgs, err := h.repo.GetConversationHistory(r.Context(), callerEmail, peer, 100)
	if err != nil {
		http.Error(w, "failed to load chat history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if msgs == nil {
		msgs = []Message{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}
