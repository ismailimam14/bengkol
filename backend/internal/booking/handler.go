package booking

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles booking REST API endpoints.
type Handler struct {
	service Service
	logger  *logger.Logger
}

// NewHandler creates a new Booking Handler.
func NewHandler(service Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// GetAvailableSlots returns available booking slots for a workshop on a date.
func (h *Handler) GetAvailableSlots(w http.ResponseWriter, r *http.Request) {
	workshopIDStr := chi.URLParam(r, "id")
	workshopID, err := uuid.Parse(workshopIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid workshop ID")
		return
	}

	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	slotsResp, err := h.service.GetAvailableSlots(r.Context(), workshopID, dateStr)
	if err != nil {
		h.logger.WithContext(r.Context()).Error("failed to get available slots", "workshop_id", workshopID, "date", dateStr, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve available slots")
		return
	}

	response.Success(w, http.StatusOK, slotsResp)
}

// Create handles reservation creation by authenticated customers.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	customerID, ok := middleware.GetUserID(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	var req CreateBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid request body")
		return
	}

	b, valErrors, err := h.service.CreateBooking(r.Context(), customerID, req)
	if err != nil {
		if errors.Is(err, ErrSlotUnavailable) {
			response.Error(w, http.StatusConflict, "BOOKING_SLOT_UNAVAILABLE", "The selected slot is no longer available")
			return
		}
		if errors.Is(err, ErrWorkshopClosed) {
			response.Error(w, http.StatusBadRequest, "WORKSHOP_CLOSED", "The workshop is closed on the selected date or time slot")
			return
		}
		if errors.Is(err, ErrValidationFailed) || errors.Is(err, ErrPastDateNotAllowed) || errors.Is(err, ErrServiceInactive) || errors.Is(err, ErrServiceWorkshopMismatch) {
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, response.ErrCodeValidationFailed, "Input validation failed", valErrors)
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to create booking", "customer_id", customerID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to create booking")
		return
	}

	response.Success(w, http.StatusCreated, b)
}

// GetByID returns the details of a booking.
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
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid booking ID")
		return
	}

	b, err := h.service.GetBookingByID(r.Context(), id, userID, role)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to view this booking")
			return
		}
		if errors.Is(err, ErrBookingNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Booking not found")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to get booking", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve booking")
		return
	}

	response.Success(w, http.StatusOK, b)
}

// Cancel cancels a booking.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	role, _ := middleware.GetUserRole(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Invalid booking ID")
		return
	}

	if err := h.service.CancelBooking(r.Context(), id, userID, role); err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to cancel this booking")
			return
		}
		if errors.Is(err, ErrBookingNotFound) {
			response.Error(w, http.StatusNotFound, response.ErrCodeNotFound, "Booking not found")
			return
		}
		if errors.Is(err, ErrInvalidOperation) {
			response.Error(w, http.StatusBadRequest, response.ErrCodeBadRequest, "Cannot cancel a booking that is completed or in service")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to cancel booking", "id", id, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to cancel booking")
		return
	}

	response.Success(w, http.StatusOK, map[string]string{
		"message": "Booking successfully cancelled",
	})
}

// ListMyBookings returns the authenticated customer's bookings.
func (h *Handler) ListMyBookings(w http.ResponseWriter, r *http.Request) {
	customerID, ok := middleware.GetUserID(r.Context())
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

	list, meta, err := h.service.ListCustomerBookings(r.Context(), customerID, pagination)
	if err != nil {
		h.logger.WithContext(r.Context()).Error("failed to list customer bookings", "customer_id", customerID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve bookings")
		return
	}

	response.SuccessWithMeta(w, http.StatusOK, list, meta)
}

// ListWorkshopBookings returns bookings for a workshop.
func (h *Handler) ListWorkshopBookings(w http.ResponseWriter, r *http.Request) {
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

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize == 0 {
		pageSize, _ = strconv.Atoi(r.URL.Query().Get("page_size"))
	}

	dateParam := r.URL.Query().Get("date")
	var dateFilter *string
	if dateParam != "" {
		dateFilter = &dateParam
	}

	pagination := domain.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}

	list, meta, err := h.service.ListWorkshopBookings(r.Context(), workshopID, userID, role, pagination, dateFilter)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			response.Error(w, http.StatusForbidden, response.ErrCodeForbidden, "You do not have permission to view this workshop's bookings")
			return
		}
		h.logger.WithContext(r.Context()).Error("failed to list workshop bookings", "workshop_id", workshopID, "error", err)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve bookings")
		return
	}

	response.SuccessWithMeta(w, http.StatusOK, list, meta)
}
