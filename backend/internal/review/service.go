package review

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/validator"
	"github.com/google/uuid"
)

var (
	ErrForbidden           = errors.New("you do not have permission to perform this action")
	ErrBookingNotCompleted = errors.New("reviews can only be submitted for completed bookings")
	ErrValidationFailed    = errors.New("validation failed")
)

// CreateReviewRequest payload for submitting a rating and review
type CreateReviewRequest struct {
	BookingID uuid.UUID `json:"booking_id"`
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment"`
}

// UpdateReviewRequest payload for updating an existing review
type UpdateReviewRequest struct {
	Rating  *int    `json:"rating"`
	Comment *string `json:"comment"`
}

// Service defines the business logic for customer reviews and workshop ratings
type Service interface {
	CreateReview(ctx context.Context, customerID uuid.UUID, req CreateReviewRequest) (*domain.Review, map[string]string, error)
	UpdateReview(ctx context.Context, reviewID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req UpdateReviewRequest) (*domain.Review, map[string]string, error)
	GetReviewByID(ctx context.Context, id uuid.UUID) (*domain.Review, error)
	GetReviewByBookingID(ctx context.Context, bookingID uuid.UUID) (*domain.Review, error)
	ListWorkshopReviews(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams) ([]domain.Review, *response.Meta, error)
}

type reviewService struct {
	repo   Repository
	logger *logger.Logger
}

// NewService creates a new review service instance
func NewService(repo Repository, log *logger.Logger) Service {
	return &reviewService{
		repo:   repo,
		logger: log,
	}
}

func (s *reviewService) CreateReview(ctx context.Context, customerID uuid.UUID, req CreateReviewRequest) (*domain.Review, map[string]string, error) {
	v := validator.New()
	v.Check(req.BookingID != uuid.Nil, "booking_id", "booking ID is required")
	v.Check(req.Rating >= 1 && req.Rating <= 5, "rating", "rating must be between 1 and 5")

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	// 1. Verify booking ownership and completion status
	b, err := s.repo.GetBookingByID(ctx, req.BookingID)
	if err != nil {
		return nil, nil, err
	}

	if b.CustomerID != customerID {
		return nil, nil, ErrForbidden
	}

	if b.Status != domain.BookingStatusCompleted {
		return nil, nil, ErrBookingNotCompleted
	}

	// 2. Verify duplicate review
	existing, err := s.repo.GetReviewByBookingID(ctx, req.BookingID)
	if err == nil && existing != nil {
		return nil, nil, ErrReviewExists
	}

	now := time.Now()
	rev := &domain.Review{
		ID:         uuid.New(),
		BookingID:  b.ID,
		CustomerID: customerID,
		WorkshopID: b.WorkshopID,
		Rating:     req.Rating,
		Comment:    req.Comment,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// 3. Begin Transaction
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	if tx != nil {
		defer tx.Rollback()
	}

	if err := s.repo.CreateReviewInTx(ctx, tx, rev); err != nil {
		return nil, nil, err
	}

	// Recalculate workshop overall rating & review count
	if err := s.repo.RecalculateWorkshopRatingInTx(ctx, tx, b.WorkshopID); err != nil {
		return nil, nil, err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("failed to commit review transaction: %w", err)
		}
	}

	s.logger.WithContext(ctx).Info("review submitted successfully",
		"review_id", rev.ID,
		"workshop_id", rev.WorkshopID,
		"rating", rev.Rating,
	)

	return rev, nil, nil
}

func (s *reviewService) UpdateReview(ctx context.Context, reviewID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req UpdateReviewRequest) (*domain.Review, map[string]string, error) {
	v := validator.New()
	if req.Rating != nil {
		v.Check(*req.Rating >= 1 && *req.Rating <= 5, "rating", "rating must be between 1 and 5")
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	rev, err := s.repo.GetReviewByID(ctx, reviewID)
	if err != nil {
		return nil, nil, err
	}

	if requestingRole != domain.RoleAdmin && rev.CustomerID != requestingUserID {
		return nil, nil, ErrForbidden
	}

	if req.Rating != nil {
		rev.Rating = *req.Rating
	}
	if req.Comment != nil {
		rev.Comment = *req.Comment
	}
	rev.UpdatedAt = time.Now()

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, nil, err
	}
	if tx != nil {
		defer tx.Rollback()
	}

	if err := s.repo.UpdateReviewInTx(ctx, tx, rev); err != nil {
		return nil, nil, err
	}

	if err := s.repo.RecalculateWorkshopRatingInTx(ctx, tx, rev.WorkshopID); err != nil {
		return nil, nil, err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, nil, err
		}
	}

	return rev, nil, nil
}

func (s *reviewService) GetReviewByID(ctx context.Context, id uuid.UUID) (*domain.Review, error) {
	return s.repo.GetReviewByID(ctx, id)
}

func (s *reviewService) GetReviewByBookingID(ctx context.Context, bookingID uuid.UUID) (*domain.Review, error) {
	return s.repo.GetReviewByBookingID(ctx, bookingID)
}

func (s *reviewService) ListWorkshopReviews(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams) ([]domain.Review, *response.Meta, error) {
	_, err := s.repo.GetWorkshopByID(ctx, workshopID)
	if err != nil {
		return nil, nil, err
	}

	pagination.EnsureValid()

	list, total, err := s.repo.ListReviewsByWorkshop(ctx, workshopID, pagination)
	if err != nil {
		return nil, nil, err
	}

	if list == nil {
		list = []domain.Review{}
	}

	totalPages := int(total) / pagination.PageSize
	if int(total)%pagination.PageSize != 0 {
		totalPages++
	}

	meta := &response.Meta{
		Page:       pagination.Page,
		PageSize:   pagination.PageSize,
		TotalItems: total,
		TotalPages: totalPages,
	}

	return list, meta, nil
}
