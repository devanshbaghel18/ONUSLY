package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type ChatRepository interface {
	GetConversationHistory(ctx context.Context, userKey, peerKey string, limit int32) ([]Message, error)
	ClearConversation(ctx context.Context, userKey, peerKey string) error
}

type Handler struct {
	repo ChatRepository
}

func NewHandler(repo ChatRepository) *Handler {
	return &Handler{
		repo: repo,
	}
}

func normalizeHandleKey(handle string) string {
	clean := strings.TrimPrefix(strings.TrimSpace(handle), "@")
	if clean == "" {
		return ""
	}
	return "@" + clean
}

func getPeerVariants(peer string) []string {
	clean := strings.TrimSpace(peer)
	if clean == "" {
		return nil
	}
	variants := []string{clean}
	if !strings.Contains(clean, "@") {
		variants = append(variants, "@"+clean)
	} else if strings.HasPrefix(clean, "@") {
		variants = append(variants, strings.TrimPrefix(clean, "@"))
	}
	return variants
}

func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	callerEmail, _ := middleware.GetUserEmail(r.Context())
	callerHandle, _ := middleware.GetUserHandle(r.Context())

	callerHandleKey := normalizeHandleKey(callerHandle)
	callerKey := strings.TrimSpace(callerEmail)
	if callerKey == "" && callerHandleKey != "" {
		callerKey = callerHandleKey
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

	// 1. Primary query
	msgs, err := h.repo.GetConversationHistory(r.Context(), callerKey, peer, 100)
	if err != nil {
		http.Error(w, "failed to load chat history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Caller handle fallback if primary returned 0 messages
	if len(msgs) == 0 && callerHandleKey != "" && callerKey != callerHandleKey {
		altMsgs, altErr := h.repo.GetConversationHistory(r.Context(), callerHandleKey, peer, 100)
		if altErr != nil {
			http.Error(w, "failed to load chat history: "+altErr.Error(), http.StatusInternalServerError)
			return
		}
		if len(altMsgs) > 0 {
			msgs = altMsgs
		}
	}

	// 3. Peer variant fallback if still empty
	if len(msgs) == 0 {
		for _, altPeer := range getPeerVariants(peer) {
			if altPeer == peer {
				continue
			}
			altMsgs, altErr := h.repo.GetConversationHistory(r.Context(), callerKey, altPeer, 100)
			if altErr != nil {
				http.Error(w, "failed to load chat history: "+altErr.Error(), http.StatusInternalServerError)
				return
			}
			if len(altMsgs) > 0 {
				msgs = altMsgs
				break
			}
			if callerHandleKey != "" && callerKey != callerHandleKey {
				altMsgs2, altErr2 := h.repo.GetConversationHistory(r.Context(), callerHandleKey, altPeer, 100)
				if altErr2 != nil {
					http.Error(w, "failed to load chat history: "+altErr2.Error(), http.StatusInternalServerError)
					return
				}
				if len(altMsgs2) > 0 {
					msgs = altMsgs2
					break
				}
			}
		}
	}

	if msgs == nil {
		msgs = []Message{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}

func (h *Handler) ClearHistory(w http.ResponseWriter, r *http.Request) {
	callerEmail, _ := middleware.GetUserEmail(r.Context())
	callerHandle, _ := middleware.GetUserHandle(r.Context())

	callerHandleKey := normalizeHandleKey(callerHandle)
	callerKey := strings.TrimSpace(callerEmail)
	if callerKey == "" && callerHandleKey != "" {
		callerKey = callerHandleKey
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

	for _, p := range getPeerVariants(peer) {
		if err := h.repo.ClearConversation(r.Context(), callerKey, p); err != nil {
			http.Error(w, "failed to clear chat: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if callerHandleKey != "" && callerKey != callerHandleKey {
			if err := h.repo.ClearConversation(r.Context(), callerHandleKey, p); err != nil {
				http.Error(w, "failed to clear chat: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
