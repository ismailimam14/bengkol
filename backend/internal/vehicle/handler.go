package vehicle

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles vehicle REST API endpoints.
type Handler struct {
	service Service
	logger  *logger.Logger
}

// NewHandler creates a new Vehicle Handler.
func NewHandler(service Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// Create registers a new vehicle for the authenticated customer.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	var req CreateVehicleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	veh, valErrors, err := h.service.CreateVehicle(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", valErrors)
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to create vehicle", "user_id", userID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to create vehicle")
		return
	}

	response.Success(w, http.StatusCreated, veh)
}

// ListMyVehicles returns all registered vehicles for the authenticated user.
func (h *Handler) ListMyVehicles(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize == 0 {
		pageSize, _ = strconv.Atoi(r.URL.Query().Get("page_size"))
	}

	pagination := domain.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}

	list, meta, err := h.service.ListMyVehicles(r.Context(), userID, pagination)
	if err != nil {
		h.logger.WithContext(r.Context()).Error("failed to list vehicles", "user_id", userID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve vehicles")
		return
	}

	response.SuccessWithMeta(w, http.StatusOK, list, meta)
}

// GetByID returns the details of a single vehicle.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	role, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid vehicle ID")
		return
	}

	veh, err := h.service.GetVehicleByID(r.Context(), id, userID, role)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to view this vehicle")
			return
		}
		if errors.Is(err, ErrVehicleNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Vehicle not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to get vehicle", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve vehicle")
		return
	}

	response.Success(w, http.StatusOK, veh)
}

// Update modifies vehicle details.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	role, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid vehicle ID")
		return
	}

	var req UpdateVehicleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	veh, valErrors, err := h.service.UpdateVehicle(r.Context(), id, userID, role, req)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to update this vehicle")
			return
		}
		if errors.Is(err, ErrVehicleNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Vehicle not found")
			return
		}
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", valErrors)
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to update vehicle", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to update vehicle")
		return
	}

	response.Success(w, http.StatusOK, veh)
}

// Delete removes a registered vehicle.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	role, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid vehicle ID")
		return
	}

	if err := h.service.DeleteVehicle(r.Context(), id, userID, role); err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to delete this vehicle")
			return
		}
		if errors.Is(err, ErrVehicleNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Vehicle not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to delete vehicle", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to delete vehicle")
		return
	}

	response.Success(w, http.StatusOK, map[string]string{
		"message": "Vehicle successfully deleted",
	})
}
