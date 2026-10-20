package friends

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// SendRequest handles POST /friends/request.
func (h *Handler) SendRequest(w http.ResponseWriter, r *http.Request) {
	callerID, ok := middleware.GetUserID(r.Context())
	if !ok || callerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	callerEmail, _ := middleware.GetUserEmail(r.Context())
	callerHandle, _ := middleware.GetUserHandle(r.Context())

	var input SendFriendRequestInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req, err := h.service.SendRequest(r.Context(), callerID, callerEmail, callerHandle, input.Target)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, req)
}

// GetRequests handles GET /friends/requests.
func (h *Handler) GetRequests(w http.ResponseWriter, r *http.Request) {
	callerID, ok := middleware.GetUserID(r.Context())
	if !ok || callerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	incoming, err := h.service.GetIncomingRequests(r.Context(), callerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	outgoing, err := h.service.GetOutgoingRequests(r.Context(), callerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"incoming": incoming,
		"outgoing": outgoing,
	})
}

// RespondRequest handles POST /friends/request/respond.
func (h *Handler) RespondRequest(w http.ResponseWriter, r *http.Request) {
	callerID, ok := middleware.GetUserID(r.Context())
	if !ok || callerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input RespondFriendRequestInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.service.RespondRequest(r.Context(), callerID, input.RequestID, input.Action); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "action": input.Action})
}

// GetFriends handles GET /friends.
func (h *Handler) GetFriends(w http.ResponseWriter, r *http.Request) {
	callerID, ok := middleware.GetUserID(r.Context())
	if !ok || callerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	friendsList, err := h.service.GetFriends(r.Context(), callerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, friendsList)
}

// DeleteFriend handles DELETE /friends/{friendId}.
func (h *Handler) DeleteFriend(w http.ResponseWriter, r *http.Request) {
	callerID, ok := middleware.GetUserID(r.Context())
	if !ok || callerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	friendID := strings.TrimPrefix(r.URL.Path, "/friends/")
	if friendID == "" || friendID == "/friends" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "friendId required"})
		return
	}

	if err := h.service.DeleteFriend(r.Context(), callerID, friendID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// GetTrendingCommunities handles GET /communities/trending.
func (h *Handler) GetTrendingCommunities(w http.ResponseWriter, r *http.Request) {
	callerID, _ := middleware.GetUserID(r.Context())

	communities, err := h.service.GetTrendingCommunities(r.Context(), callerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, communities)
}

// GetJoinedCommunities handles GET /communities/joined.
func (h *Handler) GetJoinedCommunities(w http.ResponseWriter, r *http.Request) {
	callerID, ok := middleware.GetUserID(r.Context())
	if !ok || callerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	joined, err := h.service.GetJoinedCommunities(r.Context(), callerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, joined)
}

// JoinCommunity handles POST /communities/join.
func (h *Handler) JoinCommunity(w http.ResponseWriter, r *http.Request) {
	callerID, ok := middleware.GetUserID(r.Context())
	if !ok || callerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input JoinCommunityInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.service.JoinCommunity(r.Context(), callerID, input.CommunityID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "joined", "communityId": input.CommunityID})
}

// LeaveCommunity handles POST /communities/leave.
func (h *Handler) LeaveCommunity(w http.ResponseWriter, r *http.Request) {
	callerID, ok := middleware.GetUserID(r.Context())
	if !ok || callerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input JoinCommunityInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.service.LeaveCommunity(r.Context(), callerID, input.CommunityID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "left", "communityId": input.CommunityID})
}
