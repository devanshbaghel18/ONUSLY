package proof

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

type submitProofRequest struct {
	ProofType       string `json:"proofType"`
	TextExplanation string `json:"textExplanation"`
	ExternalLink    string `json:"externalLink"`
	PhotoURL        string `json:"photoUrl"`
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	goalID := extractGoalID(r.URL.Path)

	var req submitProofRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	proof, err := h.service.Submit(
		r.Context(),
		userID,
		goalID,
		req.ProofType,
		req.TextExplanation,
		req.ExternalLink,
		req.PhotoURL,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(proof)
}

func (h *Handler) ListByGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	goalID := extractGoalID(r.URL.Path)

	proofs, err := h.service.ListByGoal(
		r.Context(),
		userID,
		goalID,
	)

	if err != nil {
		http.Error(w, "failed to fetch proofs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(proofs)
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	goalID, proofID := extractGoalAndProofID(r.URL.Path)

	proof, err := h.service.GetByID(
		r.Context(),
		userID,
		goalID,
		proofID,
	)

	if err != nil {
		http.Error(w, "proof not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(proof)
}

func extractGoalID(path string) string {
	path = strings.TrimPrefix(path, "/goals/")
	path = strings.TrimSuffix(path, "/proofs")

	return strings.Trim(path, "/")
}

func extractGoalAndProofID(path string) (string, string) {
	path = strings.TrimPrefix(path, "/goals/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) >= 3 && parts[1] == "proofs" {
		return parts[0], parts[2]
	}

	return "", ""
}
