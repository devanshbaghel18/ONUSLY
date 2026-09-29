package approval

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

type decideApprovalRequest struct {
	Status  string `json:"status"`
	Comment string `json:"comment"`
}

func (h *Handler) Decide(w http.ResponseWriter, r *http.Request) {
	approverID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	goalID, proofID := extractGoalAndProofID(r.URL.Path)

	if goalID == "" || proofID == "" {
		http.Error(w, "invalid goal or proof ID", http.StatusBadRequest)
		return
	}

	// The owner ID is supplied through the request because the approver
	// is reviewing someone else's goal.
	var req struct {
		OwnerID string `json:"ownerId"`
		Status  string `json:"status"`
		Comment string `json:"comment"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	approval, err := h.service.Decide(
		r.Context(),
		approverID,
		req.OwnerID,
		goalID,
		proofID,
		req.Status,
		req.Comment,
	)

	if err != nil {
		switch err {
		case ErrUnauthorized:
			http.Error(w, "forbidden", http.StatusForbidden)

		case ErrInvalidDecision:
			http.Error(w, err.Error(), http.StatusBadRequest)

		case ErrAlreadyDecided:
			http.Error(w, err.Error(), http.StatusConflict)

		case ErrInvalidProof:
			http.Error(w, err.Error(), http.StatusBadRequest)

		default:
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(approval)
}

func (h *Handler) ListByGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	goalID := extractGoalID(r.URL.Path)

	approvals, err := h.service.ListByGoal(
		r.Context(),
		userID,
		goalID,
	)
	if err != nil {
		http.Error(w, "failed to fetch approvals", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(approvals)
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	goalID, approvalID := extractGoalAndApprovalID(r.URL.Path)

	approval, err := h.service.GetByID(
		r.Context(),
		userID,
		goalID,
		approvalID,
	)
	if err != nil {
		http.Error(w, "approval not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(approval)
}

func extractGoalID(path string) string {
	path = strings.TrimPrefix(path, "/goals/")
	path = strings.TrimSuffix(path, "/approvals")
	return strings.Trim(path, "/")
}

func extractGoalAndProofID(path string) (string, string) {
	path = strings.TrimPrefix(path, "/goals/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) >= 4 &&
		parts[1] == "proofs" &&
		parts[3] == "approval" {
		return parts[0], parts[2]
	}

	return "", ""
}

func extractGoalAndApprovalID(path string) (string, string) {
	path = strings.TrimPrefix(path, "/goals/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) >= 3 &&
		parts[1] == "approvals" {
		return parts[0], parts[2]
	}

	return "", ""
}
