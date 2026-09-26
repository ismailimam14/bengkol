package notification

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/go-chi/chi/v5"
)

// Handler handles HTTP requests for device token registrations.
type Handler struct {
	service Service
	logger  *logger.Logger
}

// NewHandler creates a new notification handler.
func NewHandler(svc Service, log *logger.Logger) *Handler {
	return &Handler{
		service: svc,
		logger:  log,
	}
}

// RegisterDevice registers or updates an FCM/APNs push notification token.
func (h *Handler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	var req RegisterDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid JSON payload")
		return
	}

	device, valErrors, err := h.service.RegisterDevice(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Validation failed", valErrors)
			return
		}
		h.logger.Error("failed to register device token", "error", err, "user_id", userID)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to register device token")
		return
	}

	response.Success(w, http.StatusCreated, device)
}

// UnregisterDevice removes a registered device token.
func (h *Handler) UnregisterDevice(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	token := chi.URLParam(r, "token")
	if token == "" {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Device token path parameter is required")
		return
	}

	if err := h.service.UnregisterDevice(r.Context(), userID, token); err != nil {
		if errors.Is(err, ErrDeviceNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Device token not found")
			return
		}
		h.logger.Error("failed to unregister device token", "error", err, "user_id", userID)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to unregister device token")
		return
	}

	response.Success(w, http.StatusOK, map[string]string{
		"message": "Device token unregistered successfully",
	})
}

// GetMyDevices returns all active device tokens registered by the current user.
func (h *Handler) GetMyDevices(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	devices, err := h.service.GetMyDevices(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to fetch user devices", "error", err, "user_id", userID)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve registered devices")
		return
	}

	response.Success(w, http.StatusOK, devices)
}
