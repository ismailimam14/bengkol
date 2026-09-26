package history

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

// Handler handles HTTP requests for Service History endpoints
type Handler struct {
	service Service
	logger  *logger.Logger
}

// NewHandler creates a new Service History HTTP handler
func NewHandler(service Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// Create handles POST /api/v1/workshops/{id}/service-histories
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	role, _ := middleware.GetUserRole(ctx)

	var req CreateHistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "MALFORMED_JSON", "Invalid request body payload")
		return
	}

	record, valErrors, err := h.service.CreateHistory(ctx, userID, role, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrValidationFailed):
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid request parameters", valErrors)
		case errors.Is(err, ErrBookingNotFound):
			response.Error(w, http.StatusNotFound, "BOOKING_NOT_FOUND", "Booking not found")
		case errors.Is(err, ErrSparePartNotFound):
			response.Error(w, http.StatusNotFound, "SPARE_PART_NOT_FOUND", "One or more spare parts not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not own this workshop")
		case errors.Is(err, ErrHistoryExists):
			response.Error(w, http.StatusConflict, "HISTORY_ALREADY_EXISTS", "Service history is already recorded for this booking")
		case errors.Is(err, ErrInsufficientStock):
			response.Error(w, http.StatusConflict, "INSUFFICIENT_STOCK", "Insufficient stock for one or more requested spare parts")
		case errors.Is(err, ErrSparePartInactive):
			response.Error(w, http.StatusBadRequest, "SPARE_PART_INACTIVE", "One or more selected spare parts are inactive")
		case errors.Is(err, ErrSparePartWorkshopMismatch):
			response.Error(w, http.StatusBadRequest, "SPARE_PART_MISMATCH", "Spare part does not belong to this workshop")
		case errors.Is(err, ErrInvalidBookingStatus):
			response.Error(w, http.StatusBadRequest, "INVALID_BOOKING_STATUS", "Booking is not eligible to be completed")
		default:
			h.logger.WithContext(ctx).Error("failed to create service history", "error", err, "booking_id", req.BookingID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to record service history")
		}
		return
	}

	response.Success(w, http.StatusCreated, record)
}

// GetByID handles GET /api/v1/service-histories/{id}
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	historyIDStr := chi.URLParam(r, "id")
	historyID, err := uuid.Parse(historyIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid service history ID format")
		return
	}

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	role, _ := middleware.GetUserRole(ctx)

	record, err := h.service.GetHistoryByID(ctx, historyID, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrHistoryNotFound):
			response.Error(w, http.StatusNotFound, "HISTORY_NOT_FOUND", "Service history record not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to view this service record")
		default:
			h.logger.WithContext(ctx).Error("failed to get service history", "error", err, "history_id", historyID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve service history")
		}
		return
	}

	response.Success(w, http.StatusOK, record)
}

// GetByBookingID handles GET /api/v1/bookings/{id}/service-history
func (h *Handler) GetByBookingID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bookingIDStr := chi.URLParam(r, "id")
	bookingID, err := uuid.Parse(bookingIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid booking ID format")
		return
	}

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	role, _ := middleware.GetUserRole(ctx)

	record, err := h.service.GetHistoryByBookingID(ctx, bookingID, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrHistoryNotFound):
			response.Error(w, http.StatusNotFound, "HISTORY_NOT_FOUND", "Service history record not found for this booking")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to view this service record")
		default:
			h.logger.WithContext(ctx).Error("failed to get booking service history", "error", err, "booking_id", bookingID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve service history")
		}
		return
	}

	response.Success(w, http.StatusOK, record)
}

// ListCustomerHistories handles GET /api/v1/me/service-histories
func (h *Handler) ListCustomerHistories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	pagination := domain.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}

	list, meta, err := h.service.ListCustomerHistories(ctx, userID, pagination)
	if err != nil {
		h.logger.WithContext(ctx).Error("failed to list customer service histories", "error", err, "user_id", userID)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to list service histories")
		return
	}

	response.SuccessWithMeta(w, http.StatusOK, list, meta)
}

// ListWorkshopHistories handles GET /api/v1/workshops/{id}/service-histories
func (h *Handler) ListWorkshopHistories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsIDStr := chi.URLParam(r, "id")
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid workshop ID format")
		return
	}

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	role, _ := middleware.GetUserRole(ctx)

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	pagination := domain.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}

	startDateParam := r.URL.Query().Get("start_date")
	endDateParam := r.URL.Query().Get("end_date")
	var startDate, endDate *string
	if startDateParam != "" {
		startDate = &startDateParam
	}
	if endDateParam != "" {
		endDate = &endDateParam
	}

	list, meta, err := h.service.ListWorkshopHistories(ctx, wsID, pagination, startDate, endDate, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrWorkshopNotFound):
			response.Error(w, http.StatusNotFound, "WORKSHOP_NOT_FOUND", "Workshop not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not own this workshop")
		default:
			h.logger.WithContext(ctx).Error("failed to list workshop service histories", "error", err, "workshop_id", wsID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to list service histories")
		}
		return
	}

	response.SuccessWithMeta(w, http.StatusOK, list, meta)
}
