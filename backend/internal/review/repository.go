package review

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrReviewNotFound    = errors.New("review not found")
	ErrReviewExists      = errors.New("review already submitted for this booking")
	ErrBookingNotFound   = errors.New("booking not found")
	ErrWorkshopNotFound  = errors.New("workshop not found")
)

// Repository defines data access operations for reviews
type Repository interface {
	BeginTx(ctx context.Context) (*sql.Tx, error)
	CreateReviewInTx(ctx context.Context, tx *sql.Tx, r *domain.Review) error
	UpdateReviewInTx(ctx context.Context, tx *sql.Tx, r *domain.Review) error
	RecalculateWorkshopRatingInTx(ctx context.Context, tx *sql.Tx, workshopID uuid.UUID) error
	GetReviewByID(ctx context.Context, id uuid.UUID) (*domain.Review, error)
	GetReviewByBookingID(ctx context.Context, bookingID uuid.UUID) (*domain.Review, error)
	ListReviewsByWorkshop(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams) ([]domain.Review, int64, error)
	GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*domain.Booking, error)
	GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error)
}

type postgresReviewRepository struct {
	db *sql.DB
}

// NewRepository creates a new instance of PostgreSQL review repository
func NewRepository(db *sql.DB) Repository {
	return &postgresReviewRepository{db: db}
}

func (r *postgresReviewRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

func (r *postgresReviewRepository) CreateReviewInTx(ctx context.Context, tx *sql.Tx, rev *domain.Review) error {
	query := `
		INSERT INTO reviews (
			id, booking_id, customer_id, workshop_id, rating, comment, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query,
			rev.ID, rev.BookingID, rev.CustomerID, rev.WorkshopID, rev.Rating, rev.Comment, rev.CreatedAt, rev.UpdatedAt,
		)
	} else {
		_, err = r.db.ExecContext(ctx, query,
			rev.ID, rev.BookingID, rev.CustomerID, rev.WorkshopID, rev.Rating, rev.Comment, rev.CreatedAt, rev.UpdatedAt,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to insert review: %w", err)
	}
	return nil
}

func (r *postgresReviewRepository) UpdateReviewInTx(ctx context.Context, tx *sql.Tx, rev *domain.Review) error {
	query := `
		UPDATE reviews
		SET rating = $1, comment = $2, updated_at = $3
		WHERE id = $4
	`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, rev.Rating, rev.Comment, rev.UpdatedAt, rev.ID)
	} else {
		_, err = r.db.ExecContext(ctx, query, rev.Rating, rev.Comment, rev.UpdatedAt, rev.ID)
	}

	if err != nil {
		return fmt.Errorf("failed to update review: %w", err)
	}
	return nil
}

func (r *postgresReviewRepository) RecalculateWorkshopRatingInTx(ctx context.Context, tx *sql.Tx, workshopID uuid.UUID) error {
	query := `
		UPDATE workshops
		SET 
			rating = COALESCE((SELECT ROUND(AVG(rating)::numeric, 2) FROM reviews WHERE workshop_id = $1), 0.00),
			review_count = COALESCE((SELECT COUNT(*) FROM reviews WHERE workshop_id = $1), 0),
			updated_at = NOW()
		WHERE id = $1
	`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, workshopID)
	} else {
		_, err = r.db.ExecContext(ctx, query, workshopID)
	}

	if err != nil {
		return fmt.Errorf("failed to recalculate workshop rating: %w", err)
	}
	return nil
}

func (r *postgresReviewRepository) GetReviewByID(ctx context.Context, id uuid.UUID) (*domain.Review, error) {
	query := `
		SELECT 
			r.id, r.booking_id, r.customer_id, r.workshop_id, r.rating, r.comment, r.created_at, r.updated_at,
			u.id, u.name, COALESCE(u.email, '')
		FROM reviews r
		JOIN users u ON r.customer_id = u.id
		WHERE r.id = $1
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var rev domain.Review
	var u domain.User
	var comment sql.NullString

	err := row.Scan(
		&rev.ID, &rev.BookingID, &rev.CustomerID, &rev.WorkshopID, &rev.Rating, &comment, &rev.CreatedAt, &rev.UpdatedAt,
		&u.ID, &u.Name, &u.Email,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReviewNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan review: %w", err)
	}

	if comment.Valid {
		rev.Comment = comment.String
	}
	rev.Customer = &u

	return &rev, nil
}

func (r *postgresReviewRepository) GetReviewByBookingID(ctx context.Context, bookingID uuid.UUID) (*domain.Review, error) {
	query := `
		SELECT 
			r.id, r.booking_id, r.customer_id, r.workshop_id, r.rating, r.comment, r.created_at, r.updated_at,
			u.id, u.name, COALESCE(u.email, '')
		FROM reviews r
		JOIN users u ON r.customer_id = u.id
		WHERE r.booking_id = $1
	`
	row := r.db.QueryRowContext(ctx, query, bookingID)

	var rev domain.Review
	var u domain.User
	var comment sql.NullString

	err := row.Scan(
		&rev.ID, &rev.BookingID, &rev.CustomerID, &rev.WorkshopID, &rev.Rating, &comment, &rev.CreatedAt, &rev.UpdatedAt,
		&u.ID, &u.Name, &u.Email,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReviewNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan review by booking: %w", err)
	}

	if comment.Valid {
		rev.Comment = comment.String
	}
	rev.Customer = &u

	return &rev, nil
}

func (r *postgresReviewRepository) ListReviewsByWorkshop(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams) ([]domain.Review, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM reviews WHERE workshop_id = $1`
	if err := r.db.QueryRowContext(ctx, countQuery, workshopID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count workshop reviews: %w", err)
	}

	query := `
		SELECT 
			r.id, r.booking_id, r.customer_id, r.workshop_id, r.rating, r.comment, r.created_at, r.updated_at,
			u.id, u.name, COALESCE(u.email, '')
		FROM reviews r
		JOIN users u ON r.customer_id = u.id
		WHERE r.workshop_id = $1
		ORDER BY r.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, workshopID, pagination.PageSize, pagination.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query reviews: %w", err)
	}
	defer rows.Close()

	var list []domain.Review
	for rows.Next() {
		var rev domain.Review
		var u domain.User
		var comment sql.NullString

		err := rows.Scan(
			&rev.ID, &rev.BookingID, &rev.CustomerID, &rev.WorkshopID, &rev.Rating, &comment, &rev.CreatedAt, &rev.UpdatedAt,
			&u.ID, &u.Name, &u.Email,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan review row: %w", err)
		}

		if comment.Valid {
			rev.Comment = comment.String
		}
		rev.Customer = &u

		list = append(list, rev)
	}

	return list, total, nil
}

func (r *postgresReviewRepository) GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*domain.Booking, error) {
	query := `
		SELECT id, booking_number, customer_id, workshop_id, service_id, slot_id, booking_date, booking_time, status, customer_notes, created_at, updated_at
		FROM bookings
		WHERE id = $1
	`
	row := r.db.QueryRowContext(ctx, query, bookingID)

	var b domain.Booking
	var bookingDateVal time.Time
	var customerNotes sql.NullString

	err := row.Scan(
		&b.ID, &b.BookingNumber, &b.CustomerID, &b.WorkshopID, &b.ServiceID, &b.SlotID,
		&bookingDateVal, &b.BookingTime, &b.Status, &customerNotes, &b.CreatedAt, &b.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan booking: %w", err)
	}

	b.BookingDate = bookingDateVal.Format("2006-01-02")
	if customerNotes.Valid {
		b.CustomerNotes = customerNotes.String
	}

	return &b, nil
}

func (r *postgresReviewRepository) GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error) {
	query := `
		SELECT id, name, address, phone, owner_id, status, created_at, updated_at
		FROM workshops
		WHERE id = $1
	`
	row := r.db.QueryRowContext(ctx, query, workshopID)
	var w domain.Workshop
	err := row.Scan(&w.ID, &w.Name, &w.Address, &w.Phone, &w.OwnerID, &w.Status, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWorkshopNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan workshop: %w", err)
	}
	return &w, nil
}
