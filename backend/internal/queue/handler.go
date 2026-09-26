package queue

import (
	"errors"
	"net/http"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/middleware"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for Queue endpoints
type Handler struct {
	service Service
	logger  *logger.Logger
}

// NewHandler creates a new Queue HTTP handler
func NewHandler(service Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// CheckIn handles POST /api/v1/bookings/{id}/check-in
func (h *Handler) CheckIn(w http.ResponseWriter, r *http.Request) {
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

	q, err := h.service.CheckInBooking(ctx, bookingID, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrBookingNotFound):
			response.Error(w, http.StatusNotFound, "BOOKING_NOT_FOUND", "Booking not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to check in for this booking")
		case errors.Is(err, ErrInvalidBookingStatus):
			response.Error(w, http.StatusBadRequest, "INVALID_BOOKING_STATUS", "Only confirmed bookings can check in")
		case errors.Is(err, ErrInvalidCheckInDate):
			response.Error(w, http.StatusBadRequest, "INVALID_CHECK_IN_DATE", "Check-in is only available on the scheduled booking date")
		default:
			h.logger.WithContext(ctx).Error("failed to check in booking", "error", err, "booking_id", bookingID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to check in booking")
		}
		return
	}

	response.Success(w, http.StatusCreated, q)
}

// GetByID handles GET /api/v1/queues/{id}
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queueIDStr := chi.URLParam(r, "id")
	queueID, err := uuid.Parse(queueIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid queue ID format")
		return
	}

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	role, _ := middleware.GetUserRole(ctx)

	q, err := h.service.GetQueueByID(ctx, queueID, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrQueueNotFound):
			response.Error(w, http.StatusNotFound, "QUEUE_NOT_FOUND", "Queue entry not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to view this queue")
		default:
			h.logger.WithContext(ctx).Error("failed to get queue by id", "error", err, "queue_id", queueID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve queue")
		}
		return
	}

	response.Success(w, http.StatusOK, q)
}

// GetActiveCustomerQueue handles GET /api/v1/me/queue/active
func (h *Handler) GetActiveCustomerQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	q, err := h.service.GetActiveCustomerQueue(ctx, userID)
	if err != nil {
		switch {
		case errors.Is(err, ErrQueueNotFound):
			response.Error(w, http.StatusNotFound, "NO_ACTIVE_QUEUE", "No active queue entry for today")
		default:
			h.logger.WithContext(ctx).Error("failed to get active queue", "error", err, "user_id", userID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve active queue")
		}
		return
	}

	response.Success(w, http.StatusOK, q)
}

// ListByWorkshop handles GET /api/v1/workshops/{id}/queues
func (h *Handler) ListByWorkshop(w http.ResponseWriter, r *http.Request) {
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

	date := r.URL.Query().Get("date")
	statusParam := r.URL.Query().Get("status")
	var statusFilter *domain.QueueStatus
	if statusParam != "" {
		st := domain.QueueStatus(statusParam)
		statusFilter = &st
	}

	queues, err := h.service.GetWorkshopQueueList(ctx, wsID, date, statusFilter, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrWorkshopNotFound):
			response.Error(w, http.StatusNotFound, "WORKSHOP_NOT_FOUND", "Workshop not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not own this workshop")
		default:
			h.logger.WithContext(ctx).Error("failed to list workshop queues", "error", err, "workshop_id", wsID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to list queues")
		}
		return
	}

	if queues == nil {
		queues = []domain.Queue{}
	}

	response.Success(w, http.StatusOK, queues)
}

// GetSummary handles GET /api/v1/workshops/{id}/queues/summary
func (h *Handler) GetSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsIDStr := chi.URLParam(r, "id")
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid workshop ID format")
		return
	}

	date := r.URL.Query().Get("date")
	summary, err := h.service.GetWorkshopQueueSummary(ctx, wsID, date)
	if err != nil {
		h.logger.WithContext(ctx).Error("failed to get workshop queue summary", "error", err, "workshop_id", wsID)
		response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve queue summary")
		return
	}

	response.Success(w, http.StatusOK, summary)
}

// CallQueue handles POST /api/v1/queues/{id}/call
func (h *Handler) CallQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queueIDStr := chi.URLParam(r, "id")
	queueID, err := uuid.Parse(queueIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid queue ID format")
		return
	}

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	role, _ := middleware.GetUserRole(ctx)

	q, err := h.service.CallQueue(ctx, queueID, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrQueueNotFound):
			response.Error(w, http.StatusNotFound, "QUEUE_NOT_FOUND", "Queue entry not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to manage this queue")
		case errors.Is(err, ErrInvalidQueueStatus):
			response.Error(w, http.StatusBadRequest, "INVALID_QUEUE_STATUS", "Queue is not in a valid state to be called")
		default:
			h.logger.WithContext(ctx).Error("failed to call queue", "error", err, "queue_id", queueID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to call queue")
		}
		return
	}

	response.Success(w, http.StatusOK, q)
}

// CallNext handles POST /api/v1/workshops/{id}/queues/call-next
func (h *Handler) CallNext(w http.ResponseWriter, r *http.Request) {
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

	q, err := h.service.CallNextQueue(ctx, wsID, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoWaitingInQueue):
			response.Error(w, http.StatusNotFound, "NO_WAITING_QUEUE", "No waiting customers in queue")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not own this workshop")
		default:
			h.logger.WithContext(ctx).Error("failed to call next queue", "error", err, "workshop_id", wsID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to call next queue")
		}
		return
	}

	response.Success(w, http.StatusOK, q)
}

// StartService handles POST /api/v1/queues/{id}/start
func (h *Handler) StartService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queueIDStr := chi.URLParam(r, "id")
	queueID, err := uuid.Parse(queueIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid queue ID format")
		return
	}

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	role, _ := middleware.GetUserRole(ctx)

	q, err := h.service.StartService(ctx, queueID, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrQueueNotFound):
			response.Error(w, http.StatusNotFound, "QUEUE_NOT_FOUND", "Queue entry not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to manage this queue")
		case errors.Is(err, ErrInvalidQueueStatus):
			response.Error(w, http.StatusBadRequest, "INVALID_QUEUE_STATUS", "Queue must be in WAITING or CALLED status to start service")
		default:
			h.logger.WithContext(ctx).Error("failed to start queue service", "error", err, "queue_id", queueID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to start service")
		}
		return
	}

	response.Success(w, http.StatusOK, q)
}

// CompleteService handles POST /api/v1/queues/{id}/complete
func (h *Handler) CompleteService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queueIDStr := chi.URLParam(r, "id")
	queueID, err := uuid.Parse(queueIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid queue ID format")
		return
	}

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	role, _ := middleware.GetUserRole(ctx)

	q, err := h.service.CompleteService(ctx, queueID, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrQueueNotFound):
			response.Error(w, http.StatusNotFound, "QUEUE_NOT_FOUND", "Queue entry not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to manage this queue")
		case errors.Is(err, ErrInvalidQueueStatus):
			response.Error(w, http.StatusBadRequest, "INVALID_QUEUE_STATUS", "Queue must be in IN_SERVICE status to complete")
		default:
			h.logger.WithContext(ctx).Error("failed to complete queue service", "error", err, "queue_id", queueID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to complete service")
		}
		return
	}

	response.Success(w, http.StatusOK, q)
}

// MarkNoShow handles POST /api/v1/queues/{id}/no-show
func (h *Handler) MarkNoShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queueIDStr := chi.URLParam(r, "id")
	queueID, err := uuid.Parse(queueIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid queue ID format")
		return
	}

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	role, _ := middleware.GetUserRole(ctx)

	q, err := h.service.MarkNoShow(ctx, queueID, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrQueueNotFound):
			response.Error(w, http.StatusNotFound, "QUEUE_NOT_FOUND", "Queue entry not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to manage this queue")
		case errors.Is(err, ErrInvalidQueueStatus):
			response.Error(w, http.StatusBadRequest, "INVALID_QUEUE_STATUS", "Queue cannot be marked as no-show in its current status")
		default:
			h.logger.WithContext(ctx).Error("failed to mark queue no-show", "error", err, "queue_id", queueID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to mark no-show")
		}
		return
	}

	response.Success(w, http.StatusOK, q)
}

// CancelQueue handles POST /api/v1/queues/{id}/cancel
func (h *Handler) CancelQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queueIDStr := chi.URLParam(r, "id")
	queueID, err := uuid.Parse(queueIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid queue ID format")
		return
	}

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	role, _ := middleware.GetUserRole(ctx)

	q, err := h.service.CancelQueue(ctx, queueID, userID, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrQueueNotFound):
			response.Error(w, http.StatusNotFound, "QUEUE_NOT_FOUND", "Queue entry not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to cancel this queue")
		case errors.Is(err, ErrInvalidQueueStatus):
			response.Error(w, http.StatusBadRequest, "INVALID_QUEUE_STATUS", "Queue cannot be cancelled in its current status")
		default:
			h.logger.WithContext(ctx).Error("failed to cancel queue", "error", err, "queue_id", queueID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to cancel queue")
		}
		return
	}

	response.Success(w, http.StatusOK, q)
}
