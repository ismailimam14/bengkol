package sparepart

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

// Handler handles spare parts REST API endpoints.
type Handler struct {
	service Service
	logger  *logger.Logger
}

// NewHandler creates a new SparePart Handler.
func NewHandler(service Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// ListByWorkshop returns spare parts for a specific workshop.
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

	list, err := h.service.GetSparePartsByWorkshop(r.Context(), workshopID, reqRole, reqUserID)
	if err != nil {
		h.logger.WithContext(r.Context()).Error("failed to list spare parts", "workshop_id", workshopID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve spare parts")
		return
	}

	response.Success(w, http.StatusOK, list)
}

// Create adds a new spare part to a workshop's inventory.
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

	var req CreateSparePartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	sp, valErrors, err := h.service.CreateSparePart(r.Context(), workshopID, userID, role, req)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to manage this workshop's spare parts")
			return
		}
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", valErrors)
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to create spare part", "workshop_id", workshopID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to create spare part")
		return
	}

	response.Success(w, http.StatusCreated, sp)
}

// GetByID returns a specific spare part's details.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid spare part ID")
		return
	}

	sp, err := h.service.GetSparePartByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrSparePartNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Spare part not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to get spare part", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve spare part")
		return
	}

	response.Success(w, http.StatusOK, sp)
}

// Update modifies a spare part record.
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
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid spare part ID")
		return
	}

	var req UpdateSparePartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	sp, valErrors, err := h.service.UpdateSparePart(r.Context(), id, userID, role, req)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to modify this spare part")
			return
		}
		if errors.Is(err, ErrSparePartNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Spare part not found")
			return
		}
		if errors.Is(err, ErrValidationFailed) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", valErrors)
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to update spare part", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to update spare part")
		return
	}

	response.Success(w, http.StatusOK, sp)
}

// Delete removes a spare part.
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
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid spare part ID")
		return
	}

	if err := h.service.DeleteSparePart(r.Context(), id, userID, role); err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to delete this spare part")
			return
		}
		if errors.Is(err, ErrSparePartNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Spare part not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to delete spare part", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to delete spare part")
		return
	}

	response.Success(w, http.StatusOK, map[string]string{
		"message": "Spare part successfully deleted",
	})
}
