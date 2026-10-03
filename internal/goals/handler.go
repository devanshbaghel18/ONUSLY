package goals

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

type createGoalRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req createGoalRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	goal, err := h.service.Create(
		r.Context(),
		userID,
		req.Title,
		req.Description,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(goal)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	goals, err := h.service.List(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to fetch goals", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(goals)
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	goalID := r.PathValue("id")
	if goalID == "" {
		goalID = strings.TrimPrefix(r.URL.Path, "/goals/")
	}

	goal, err := h.service.GetByID(
		r.Context(),
		userID,
		goalID,
	)

	if err != nil {
		http.Error(w, "goal not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(goal)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	goalID := r.PathValue("id")
	if goalID == "" {
		goalID = strings.TrimPrefix(r.URL.Path, "/goals/")
	}

	if err := h.service.Delete(
		r.Context(),
		userID,
		goalID,
	); err != nil {
		http.Error(w, "failed to delete goal", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	goalID := r.PathValue("id")
	if goalID == "" {
		goalID = strings.TrimPrefix(r.URL.Path, "/goals/")
	}

	var req struct {
		Title         *string `json:"title"`
		Description   *string `json:"description"`
		ApprovalType  *string `json:"approvalType"`
		ApproverEmail *string `json:"approverEmail"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	input := UpdateGoalInput{
		Title:         req.Title,
		Description:   req.Description,
		ApprovalType:  req.ApprovalType,
		ApproverEmail: req.ApproverEmail,
	}

	goal, err := h.service.Update(
		r.Context(),
		userID,
		goalID,
		input,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(goal)
}
