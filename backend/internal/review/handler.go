package review

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

// Handler handles HTTP requests for Reviews
type Handler struct {
	service Service
	logger  *logger.Logger
}

// NewHandler creates a new Review HTTP handler
func NewHandler(service Service, log *logger.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  log,
	}
}

// Create handles POST /api/v1/reviews
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	customerID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	var req CreateReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "MALFORMED_JSON", "Invalid request body payload")
		return
	}

	rev, valErrors, err := h.service.CreateReview(ctx, customerID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrValidationFailed):
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid review parameters", valErrors)
		case errors.Is(err, ErrBookingNotFound):
			response.Error(w, http.StatusNotFound, "BOOKING_NOT_FOUND", "Booking not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You can only submit reviews for your own bookings")
		case errors.Is(err, ErrBookingNotCompleted):
			response.Error(w, http.StatusBadRequest, "BOOKING_NOT_COMPLETED", "Reviews can only be submitted for completed bookings")
		case errors.Is(err, ErrReviewExists):
			response.Error(w, http.StatusConflict, "REVIEW_ALREADY_EXISTS", "A review has already been submitted for this booking")
		default:
			h.logger.WithContext(ctx).Error("failed to create review", "error", err, "booking_id", req.BookingID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to submit review")
		}
		return
	}

	response.Success(w, http.StatusCreated, rev)
}

// GetByID handles GET /api/v1/reviews/{id}
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reviewIDStr := chi.URLParam(r, "id")
	reviewID, err := uuid.Parse(reviewIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid review ID format")
		return
	}

	rev, err := h.service.GetReviewByID(ctx, reviewID)
	if err != nil {
		switch {
		case errors.Is(err, ErrReviewNotFound):
			response.Error(w, http.StatusNotFound, "REVIEW_NOT_FOUND", "Review not found")
		default:
			h.logger.WithContext(ctx).Error("failed to get review by id", "error", err, "review_id", reviewID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve review")
		}
		return
	}

	response.Success(w, http.StatusOK, rev)
}

// GetByBookingID handles GET /api/v1/bookings/{id}/review
func (h *Handler) GetByBookingID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bookingIDStr := chi.URLParam(r, "id")
	bookingID, err := uuid.Parse(bookingIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid booking ID format")
		return
	}

	rev, err := h.service.GetReviewByBookingID(ctx, bookingID)
	if err != nil {
		switch {
		case errors.Is(err, ErrReviewNotFound):
			response.Error(w, http.StatusNotFound, "REVIEW_NOT_FOUND", "No review submitted for this booking")
		default:
			h.logger.WithContext(ctx).Error("failed to get review by booking id", "error", err, "booking_id", bookingID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to retrieve review")
		}
		return
	}

	response.Success(w, http.StatusOK, rev)
}

// ListByWorkshop handles GET /api/v1/workshops/{id}/reviews
func (h *Handler) ListByWorkshop(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsIDStr := chi.URLParam(r, "id")
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid workshop ID format")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	pagination := domain.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}

	list, meta, err := h.service.ListWorkshopReviews(ctx, wsID, pagination)
	if err != nil {
		switch {
		case errors.Is(err, ErrWorkshopNotFound):
			response.Error(w, http.StatusNotFound, "WORKSHOP_NOT_FOUND", "Workshop not found")
		default:
			h.logger.WithContext(ctx).Error("failed to list workshop reviews", "error", err, "workshop_id", wsID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to list reviews")
		}
		return
	}

	response.SuccessWithMeta(w, http.StatusOK, list, meta)
}

// Update handles PATCH /api/v1/reviews/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reviewIDStr := chi.URLParam(r, "id")
	reviewID, err := uuid.Parse(reviewIDStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_UUID", "Invalid review ID format")
		return
	}

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		response.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}
	role, _ := middleware.GetUserRole(ctx)

	var req UpdateReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "MALFORMED_JSON", "Invalid request body payload")
		return
	}

	rev, valErrors, err := h.service.UpdateReview(ctx, reviewID, userID, role, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrValidationFailed):
			response.ErrorWithDetails(w, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid review update parameters", valErrors)
		case errors.Is(err, ErrReviewNotFound):
			response.Error(w, http.StatusNotFound, "REVIEW_NOT_FOUND", "Review not found")
		case errors.Is(err, ErrForbidden):
			response.Error(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to update this review")
		default:
			h.logger.WithContext(ctx).Error("failed to update review", "error", err, "review_id", reviewID)
			response.Error(w, http.StatusInternalServerError, response.ErrCodeInternalServerError, "Failed to update review")
		}
		return
	}

	response.Success(w, http.StatusOK, rev)
}
