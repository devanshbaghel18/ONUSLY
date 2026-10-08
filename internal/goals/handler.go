package goals

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
	return &Handler{
		service: service,
	}
}

type createGoalRequest struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	BlockedApps    []string `json:"blockedApps"`
	BlockedDomains []string `json:"blockedDomains"`
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
		req.BlockedApps,
		req.BlockedDomains,
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
	callerID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	goalID := r.PathValue("id")
	if goalID == "" {
		goalID = strings.TrimPrefix(r.URL.Path, "/goals/")
	}

	var goal *Goal
	var err error
	if qOwner := strings.TrimSpace(r.URL.Query().Get("ownerId")); qOwner != "" {
		goal, err = h.service.GetByID(r.Context(), qOwner, goalID)
	} else {
		goal, err = h.service.GetByID(r.Context(), callerID, goalID)
		if err != nil {
			goal, err = h.service.GetByID(r.Context(), "", goalID)
		}
	}

	if err != nil {
		http.Error(w, "goal not found", http.StatusNotFound)
		return
	}

	// Security: caller must be either the goal owner or the designated approver
	callerEmail, _ := middleware.GetUserEmail(r.Context())
	callerHandle, _ := middleware.GetUserHandle(r.Context())
	isApprover := (goal.ApproverID == "COMMUNITY") ||
		(goal.ApprovalType == "community") ||
		(goal.ApproverID != "" && goal.ApproverID == callerID) ||
		(goal.ApproverEmail != "" && callerEmail != "" && strings.EqualFold(goal.ApproverEmail, callerEmail)) ||
		(goal.ApproverEmail != "" && callerHandle != "" && (strings.EqualFold(goal.ApproverEmail, "@"+callerHandle) || strings.EqualFold(goal.ApproverEmail, callerHandle)))

	if goal.OwnerID != callerID && !isApprover {
		http.Error(w, "forbidden: you are not authorized to view this goal", http.StatusForbidden)
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
		if errors.Is(err, ErrGoalLocked) || strings.Contains(err.Error(), "goal_locked") {
			http.Error(w, "goal_locked", http.StatusConflict)
			return
		}
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
		Title          *string   `json:"title"`
		Description    *string   `json:"description"`
		ApprovalType   *string   `json:"approvalType"`
		ApproverEmail  *string   `json:"approverEmail"`
		BlockedApps    *[]string `json:"blockedApps"`
		BlockedDomains *[]string `json:"blockedDomains"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	input := UpdateGoalInput{
		Title:          req.Title,
		Description:    req.Description,
		ApprovalType:  req.ApprovalType,
		ApproverEmail: req.ApproverEmail,
		BlockedApps:    req.BlockedApps,
		BlockedDomains: req.BlockedDomains,
	}

	goal, err := h.service.Update(
		r.Context(),
		userID,
		goalID,
		input,
	)

	if err != nil {
		if errors.Is(err, ErrTargetsLocked) || strings.Contains(err.Error(), "targets_locked") {
			http.Error(w, "targets_locked", http.StatusConflict)
			return
		}
		if errors.Is(err, ErrGoalLocked) || strings.Contains(err.Error(), "goal_locked") {
			http.Error(w, "goal_locked", http.StatusConflict)
			return
		}
		if errors.Is(err, ErrAccountabilityLocked) || strings.Contains(err.Error(), "cannot change accountability") {
			http.Error(w, "cannot change accountability to none while targets are locked", http.StatusConflict)
			return
		}
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(goal)
}
