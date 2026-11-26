package wellbeing

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := h.service.Ingest(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrInvalidDate) || errors.Is(err, ErrInvalidDeviceID) || errors.Is(err, ErrInvalidClientType) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to record wellbeing telemetry", http.StatusInternalServerError)
		return
	}

	writeJSON(w, res)
}

func (h *Handler) GetSummary(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	rng := q.Get("range")
	deviceFilter := q.Get("deviceId")
	tzOffset := 0
	if rawTz := q.Get("tzOffset"); rawTz != "" {
		if parsed, err := strconv.Atoi(rawTz); err == nil {
			tzOffset = parsed
		}
	}

	res, err := h.service.GetSummary(r.Context(), userID, rng, tzOffset, deviceFilter)
	if err != nil {
		if errors.Is(err, ErrInvalidRange) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to load wellbeing summary", http.StatusInternalServerError)
		return
	}

	writeJSON(w, res)
}

func (h *Handler) GetRules(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var tzPtr *int
	if rawTz := r.URL.Query().Get("tzOffset"); rawTz != "" {
		if parsed, err := strconv.Atoi(rawTz); err == nil {
			tzPtr = &parsed
		}
	}

	res, err := h.service.getRulesInternal(r.Context(), userID, tzPtr, tzPtr != nil)
	if err != nil {
		http.Error(w, "failed to load wellbeing rules", http.StatusInternalServerError)
		return
	}

	writeJSON(w, res)
}

func (h *Handler) UpsertLimit(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	var req UpsertLimitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := h.service.UpsertLimit(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrInvalidLimit) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to save limit", http.StatusInternalServerError)
		return
	}

	writeJSON(w, res)
}

func (h *Handler) DeleteLimit(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rawID := r.PathValue("id")
	if rawID == "" {
		rawID = r.URL.Query().Get("id")
	}
	if decoded, err := url.PathUnescape(rawID); err == nil {
		rawID = decoded
	}

	if err := h.service.DeleteLimit(r.Context(), userID, rawID); err != nil {
		if errors.Is(err, ErrInvalidLimit) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to delete limit", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "deleted"})
}

func (h *Handler) SaveSchedule(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	var req FocusScheduleConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	res, err := h.service.SaveSchedule(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrInvalidSchedule) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to save focus schedule", http.StatusInternalServerError)
		return
	}

	writeJSON(w, res)
}
