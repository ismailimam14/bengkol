package review_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/review"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

type mockReviewRepo struct {
	mu        sync.Mutex
	reviews   map[uuid.UUID]*domain.Review
	bookings  map[uuid.UUID]*domain.Booking
	workshops map[uuid.UUID]*domain.Workshop
}

func newMockReviewRepo() *mockReviewRepo {
	return &mockReviewRepo{
		reviews:   make(map[uuid.UUID]*domain.Review),
		bookings:  make(map[uuid.UUID]*domain.Booking),
		workshops: make(map[uuid.UUID]*domain.Workshop),
	}
}

func (m *mockReviewRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return nil, nil
}

func (m *mockReviewRepo) CreateReviewInTx(ctx context.Context, tx *sql.Tx, rev *domain.Review) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reviews[rev.ID] = rev
	return nil
}

func (m *mockReviewRepo) UpdateReviewInTx(ctx context.Context, tx *sql.Tx, rev *domain.Review) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.reviews[rev.ID]
	if !ok {
		return review.ErrReviewNotFound
	}
	existing.Rating = rev.Rating
	existing.Comment = rev.Comment
	existing.UpdatedAt = rev.UpdatedAt
	return nil
}

func (m *mockReviewRepo) RecalculateWorkshopRatingInTx(ctx context.Context, tx *sql.Tx, workshopID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ws, ok := m.workshops[workshopID]
	if !ok {
		return nil
	}

	total := 0
	count := 0
	for _, r := range m.reviews {
		if r.WorkshopID == workshopID {
			total += r.Rating
			count++
		}
	}

	if count > 0 {
		ws.Rating = float64(total) / float64(count)
		ws.ReviewCount = count
	} else {
		ws.Rating = 0
		ws.ReviewCount = 0
	}
	return nil
}

func (m *mockReviewRepo) GetReviewByID(ctx context.Context, id uuid.UUID) (*domain.Review, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.reviews[id]
	if !ok {
		return nil, review.ErrReviewNotFound
	}
	copied := *r
	return &copied, nil
}

func (m *mockReviewRepo) GetReviewByBookingID(ctx context.Context, bookingID uuid.UUID) (*domain.Review, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, r := range m.reviews {
		if r.BookingID == bookingID {
			copied := *r
			return &copied, nil
		}
	}
	return nil, review.ErrReviewNotFound
}

func (m *mockReviewRepo) ListReviewsByWorkshop(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams) ([]domain.Review, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []domain.Review
	for _, r := range m.reviews {
		if r.WorkshopID == workshopID {
			list = append(list, *r)
		}
	}
	return list, int64(len(list)), nil
}

func (m *mockReviewRepo) GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*domain.Booking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.bookings[bookingID]
	if !ok {
		return nil, review.ErrBookingNotFound
	}
	copied := *b
	return &copied, nil
}

func (m *mockReviewRepo) GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.workshops[workshopID]
	if !ok {
		return nil, review.ErrWorkshopNotFound
	}
	copied := *w
	return &copied, nil
}

func setupReviewService() (review.Service, *mockReviewRepo) {
	repo := newMockReviewRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	svc := review.NewService(repo, log)
	return svc, repo
}

func TestReviewService_Create_Success(t *testing.T) {
	svc, repo := setupReviewService()

	wsID := uuid.New()
	custID := uuid.New()
	bookingID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, Rating: 0, ReviewCount: 0}
	repo.bookings[bookingID] = &domain.Booking{
		ID:          bookingID,
		CustomerID:  custID,
		WorkshopID:  wsID,
		BookingDate: "2026-09-28",
		Status:      domain.BookingStatusCompleted, // Must be completed
	}

	req := review.CreateReviewRequest{
		BookingID: bookingID,
		Rating:    5,
		Comment:   "Pelayanan cepat dan mekanik sangat ramah!",
	}

	rev, valErrors, err := svc.CreateReview(context.Background(), custID, req)
	if err != nil {
		t.Fatalf("unexpected error creating review: %v", err)
	}
	if valErrors != nil {
		t.Fatalf("expected no validation errors, got %+v", valErrors)
	}

	if rev.Rating != 5 {
		t.Errorf("expected rating 5, got %d", rev.Rating)
	}

	// Verify workshop rating recalculation
	if repo.workshops[wsID].Rating != 5.0 || repo.workshops[wsID].ReviewCount != 1 {
		t.Errorf("expected workshop rating 5.0 and count 1, got rating %f, count %d", repo.workshops[wsID].Rating, repo.workshops[wsID].ReviewCount)
	}
}

func TestReviewService_Create_NotCompleted(t *testing.T) {
	svc, repo := setupReviewService()

	wsID := uuid.New()
	custID := uuid.New()
	bookingID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID}
	repo.bookings[bookingID] = &domain.Booking{
		ID:         bookingID,
		CustomerID: custID,
		WorkshopID: wsID,
		Status:     domain.BookingStatusConfirmed, // Not completed!
	}

	req := review.CreateReviewRequest{
		BookingID: bookingID,
		Rating:    4,
	}

	_, _, err := svc.CreateReview(context.Background(), custID, req)
	if !errors.Is(err, review.ErrBookingNotCompleted) {
		t.Fatalf("expected ErrBookingNotCompleted, got %v", err)
	}
}

func TestReviewService_Create_UnauthorizedCustomer(t *testing.T) {
	svc, repo := setupReviewService()

	wsID := uuid.New()
	custA := uuid.New()
	custB := uuid.New()
	bookingID := uuid.New()

	repo.bookings[bookingID] = &domain.Booking{
		ID:         bookingID,
		CustomerID: custA,
		WorkshopID: wsID,
		Status:     domain.BookingStatusCompleted,
	}

	// CustB attempts to review CustA's booking
	_, _, err := svc.CreateReview(context.Background(), custB, review.CreateReviewRequest{
		BookingID: bookingID,
		Rating:    5,
	})
	if !errors.Is(err, review.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestReviewService_Update_Success(t *testing.T) {
	svc, repo := setupReviewService()

	wsID := uuid.New()
	custID := uuid.New()
	bookingID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID}
	repo.bookings[bookingID] = &domain.Booking{
		ID:         bookingID,
		CustomerID: custID,
		WorkshopID: wsID,
		Status:     domain.BookingStatusCompleted,
	}

	rev, _, _ := svc.CreateReview(context.Background(), custID, review.CreateReviewRequest{
		BookingID: bookingID,
		Rating:    3,
		Comment:   "Cukup baik",
	})

	newRating := 5
	newComment := "Revisi: ternyata masalah tuntas dan garansi berlaku!"
	updatedRev, _, err := svc.UpdateReview(context.Background(), rev.ID, custID, domain.RoleCustomer, review.UpdateReviewRequest{
		Rating:  &newRating,
		Comment: &newComment,
	})
	if err != nil {
		t.Fatalf("unexpected error updating review: %v", err)
	}

	if updatedRev.Rating != 5 {
		t.Errorf("expected updated rating 5, got %d", updatedRev.Rating)
	}
	if repo.workshops[wsID].Rating != 5.0 {
		t.Errorf("expected recalculated workshop rating 5.0, got %f", repo.workshops[wsID].Rating)
	}
}
