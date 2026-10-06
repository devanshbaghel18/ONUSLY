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
	callerEmail, _ := middleware.GetUserEmail(r.Context())
	callerHandle, _ := middleware.GetUserHandle(r.Context())

	callerKey := strings.TrimSpace(callerEmail)
	if callerKey == "" && callerHandle != "" {
		callerKey = "@" + callerHandle
	}
	if callerKey == "" {
		http.Error(w, "unauthorized: email or handle required in token", http.StatusUnauthorized)
		return
	}

	peer := strings.TrimSpace(r.URL.Query().Get("peer"))
	if peer == "" {
		http.Error(w, "missing required query parameter: peer", http.StatusBadRequest)
		return
	}

	msgs, err := h.repo.GetConversationHistory(r.Context(), callerKey, peer, 100)
	if (msgs == nil || len(msgs) == 0) && callerHandle != "" && callerKey != ("@"+callerHandle) {
		if altMsgs, _ := h.repo.GetConversationHistory(r.Context(), "@"+callerHandle, peer, 100); len(altMsgs) > 0 {
			msgs = altMsgs
		}
	}
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
