package service

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles service REST API endpoints.
type Handler struct {
	service Service
	logger  *logger.Logger
}

// NewHandler creates a new Service Handler.
func NewHandler(service Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// ListByWorkshop returns all services for a specific workshop.
func (h *Handler) ListByWorkshop(w http.ResponseWriter, r *http.Request) {
	workshopIDStr := chi.URLParam(r, "id")
	workshopID, err := uuid.Parse(workshopIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	var reqRole *domain.UserRole
	var reqUserID *uuid.UUID

	if role, ok := middleware.GetUserRole(r.Context()); ok {
		reqRole = &role
	}
	if uID, ok := middleware.GetUserID(r.Context()); ok {
		reqUserID = &uID
	}

	list, err := h.service.GetServicesByWorkshop(r.Context(), workshopID, reqRole, reqUserID)
	if err != nil {
		h.logger.WithContext(r.Context()).Error("failed to list services", "workshop_id", workshopID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve services")
		return
	}

	response.Success(w, http.StatusOK, list)
}

// Create creates a new service in a workshop.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	role, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	workshopIDStr := chi.URLParam(r, "id")
	workshopID, err := uuid.Parse(workshopIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	var req CreateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	srv, valErrors, err := h.service.CreateService(r.Context(), workshopID, userID, role, req)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to manage this workshop's services")
			return
		}
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", valErrors)
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to create service", "workshop_id", workshopID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to create service")
		return
	}

	response.Success(w, http.StatusCreated, srv)
}

// GetByID returns the detail of a specific service.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid service ID")
		return
	}

	srv, err := h.service.GetServiceByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrServiceNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Service not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to get service", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve service")
		return
	}

	response.Success(w, http.StatusOK, srv)
}

// Update modifies an existing service.
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
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid service ID")
		return
	}

	var req UpdateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	srv, valErrors, err := h.service.UpdateService(r.Context(), id, userID, role, req)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to modify this service")
			return
		}
		if errors.Is(err, ErrServiceNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Service not found")
			return
		}
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", valErrors)
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to update service", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to update service")
		return
	}

	response.Success(w, http.StatusOK, srv)
}

// Delete removes a service.
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
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid service ID")
		return
	}

	if err := h.service.DeleteService(r.Context(), id, userID, role); err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to delete this service")
			return
		}
		if errors.Is(err, ErrServiceNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Service not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to delete service", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to delete service")
		return
	}

	response.Success(w, http.StatusOK, map[string]string{
		"message": "Service successfully deleted",
	})
}
