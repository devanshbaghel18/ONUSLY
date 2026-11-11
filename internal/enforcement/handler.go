package enforcement

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

func (h *Handler) GetBlocklist(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	res, err := h.service.GetBlocklist(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrDeviceRevoked) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "DEVICE_REVOKED",
				"code":    "device_revoked",
				"message": "This device has been revoked by the account owner",
			})
			return
		}
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

func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16384) // Max 16KB

	var req HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	device, err := h.service.ProcessHeartbeat(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrDeviceRevoked) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "DEVICE_REVOKED",
				"code":    "device_revoked",
				"message": "This device has been revoked by the account owner",
			})
			return
		}
		if errors.Is(err, ErrInvalidUUID) || errors.Is(err, ErrInvalidClientType) ||
			errors.Is(err, ErrInvalidPlatform) || errors.Is(err, ErrInvalidBrowser) ||
			errors.Is(err, ErrInvalidSentAt) || errors.Is(err, ErrInvalidIssues) ||
			strings.Contains(err.Error(), "must be") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(device)
}

func (h *Handler) RevokeDevice(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	deviceID := r.PathValue("id")
	if deviceID == "" {
		http.Error(w, "missing device id", http.StatusBadRequest)
		return
	}

	var req RevokeDeviceRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	resp, err := h.service.RevokeDevice(r.Context(), userID, deviceID, req.Reason)
	if err != nil {
		if errors.Is(err, ErrDeviceNotFound) {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrDeviceOwnershipMismatch) {
			http.Error(w, "forbidden: device belongs to a different account", http.StatusForbidden)
			return
		}
		if errors.Is(err, ErrInvalidUUID) {
			http.Error(w, "invalid device id format", http.StatusBadRequest)
			return
		}
		http.Error(w, "internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetDevices(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	resp, err := h.service.GetDevices(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetGoalEnforcementStatus(w http.ResponseWriter, r *http.Request) {
	callerID, ok := middleware.GetUserID(r.Context())
	if !ok || callerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	callerEmail, _ := middleware.GetUserEmail(r.Context())
	callerHandle, _ := middleware.GetUserHandle(r.Context())

	goalID := r.PathValue("id")
	if goalID == "" {
		http.Error(w, "missing goal id", http.StatusBadRequest)
		return
	}

	resp, err := h.service.GetGoalEnforcementStatus(r.Context(), callerID, callerEmail, callerHandle, goalID)
	if err != nil {
		if errors.Is(err, ErrGoalNotFound) {
			http.Error(w, "goal not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrUnauthorizedGoal) {
			http.Error(w, "forbidden: caller is neither owner nor approver", http.StatusForbidden)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

type PairRequestInput struct {
	ClientType string `json:"clientType"`
	Platform   string `json:"platform"`
	DeviceID   string `json:"deviceId"`
}

func (h *Handler) RequestPairing(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	var in PairRequestInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	session, err := h.service.InitiatePairing(r.Context(), in.ClientType, in.Platform, in.DeviceID)
	if err != nil {
		http.Error(w, "failed to initiate pairing: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":      session.Code,
		"expiresAt": session.ExpiresAt,
	})
}

type ConfirmPairInput struct {
	Code string `json:"code"`
}

func (h *Handler) ConfirmPairing(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userEmail, _ := middleware.GetUserEmail(r.Context())
	userHandle, _ := middleware.GetUserHandle(r.Context())
	userName, _ := middleware.GetUserName(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var in ConfirmPairInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.ConfirmPairing(r.Context(), in.Code, userID, userEmail, userHandle, userName); err != nil {
		http.Error(w, "failed to confirm pairing: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "confirmed",
	})
}

type ExchangePairInput struct {
	Code string `json:"code"`
}

func (h *Handler) ExchangePairing(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var in ExchangePairInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	token, err := h.service.ExchangePairingToken(r.Context(), in.Code)
	if err != nil {
		http.Error(w, "exchange failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"token": token,
	})
}
