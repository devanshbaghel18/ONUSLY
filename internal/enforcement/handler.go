package enforcement

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

func (h *Handler) GetBlocklist(w http.ResponseWriter, r *http.Request) {
	// Identity MUST come from JWT context, never from path or query
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	res, err := h.service.GetBlocklist(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	etag := `"` + res.Version + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")

	ifNoneMatch := r.Header.Get("If-None-Match")
	if ifNoneMatch != "" {
		clientETag := strings.TrimSpace(ifNoneMatch)
		clientETag = strings.TrimPrefix(clientETag, "W/")
		clientETag = strings.Trim(clientETag, `"`)

		if clientETag == res.Version {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}
