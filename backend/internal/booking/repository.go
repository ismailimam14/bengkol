package booking

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
	ErrBookingNotFound   = errors.New("booking not found")
	ErrSlotUnavailable   = errors.New("the selected slot is no longer available")
	ErrSlotNotFound      = errors.New("booking slot not found")
	ErrInvalidOperation  = errors.New("cannot cancel a booking that is completed or in service")
)

// Repository defines data operations for bookings and slots.
type Repository interface {
	GetSlotForUpdate(ctx context.Context, tx *sql.Tx, workshopID uuid.UUID, slotDate, startTime string) (*domain.BookingSlot, error)
	CreateSlotInTx(ctx context.Context, tx *sql.Tx, slot *domain.BookingSlot) error
	IncrementSlotBookingInTx(ctx context.Context, tx *sql.Tx, slotID uuid.UUID, newBookedCount int, isAvailable bool) error
	DecrementSlotBookingInTx(ctx context.Context, tx *sql.Tx, slotID uuid.UUID) error
	GetSlotsByDate(ctx context.Context, workshopID uuid.UUID, slotDate string) ([]domain.BookingSlot, error)
	
	CreateBookingInTx(ctx context.Context, tx *sql.Tx, b *domain.Booking) error
	GetBookingByID(ctx context.Context, id uuid.UUID) (*domain.Booking, error)
	GetBookingByIDForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Booking, error)
	UpdateBookingStatusInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.BookingStatus) error
	ListByCustomer(ctx context.Context, customerID uuid.UUID, pagination domain.PaginationParams) ([]domain.Booking, int64, error)
	ListByWorkshop(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams, date *string) ([]domain.Booking, int64, error)
	
	GetWorkshopOperatingHour(ctx context.Context, workshopID uuid.UUID, dayOfWeek int) (*domain.OperatingHour, error)
	GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error)
	GetServiceByID(ctx context.Context, serviceID uuid.UUID) (*domain.Service, error)

	BeginTx(ctx context.Context) (*sql.Tx, error)
}

type postgresRepository struct {
	db *sql.DB
}

// NewRepository creates a new PostgreSQL Booking repository.
func NewRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

func (r *postgresRepository) GetSlotForUpdate(ctx context.Context, tx *sql.Tx, workshopID uuid.UUID, slotDate, startTime string) (*domain.BookingSlot, error) {
	query := `
		SELECT id, workshop_id, slot_date, start_time, end_time, max_capacity, booked_count, is_available, created_at, updated_at
		FROM booking_slots
		WHERE workshop_id = $1 AND slot_date = $2 AND start_time = $3
		FOR UPDATE
	`
	var slot domain.BookingSlot
	var slotDateVal time.Time

	err := tx.QueryRowContext(ctx, query, workshopID, slotDate, startTime).Scan(
		&slot.ID,
		&slot.WorkshopID,
		&slotDateVal,
		&slot.StartTime,
		&slot.EndTime,
		&slot.MaxCapacity,
		&slot.BookedCount,
		&slot.IsAvailable,
		&slot.CreatedAt,
		&slot.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSlotNotFound
		}
		return nil, fmt.Errorf("failed to query slot for update: %w", err)
	}
	slot.SlotDate = slotDateVal.Format("2006-01-02")
	return &slot, nil
}

func (r *postgresRepository) CreateSlotInTx(ctx context.Context, tx *sql.Tx, slot *domain.BookingSlot) error {
	query := `
		INSERT INTO booking_slots (
			id, workshop_id, slot_date, start_time, end_time, max_capacity, booked_count, is_available, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`
	_, err := tx.ExecContext(ctx, query,
		slot.ID,
		slot.WorkshopID,
		slot.SlotDate,
		slot.StartTime,
		slot.EndTime,
		slot.MaxCapacity,
		slot.BookedCount,
		slot.IsAvailable,
		slot.CreatedAt,
		slot.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert booking slot: %w", err)
	}
	return nil
}

func (r *postgresRepository) IncrementSlotBookingInTx(ctx context.Context, tx *sql.Tx, slotID uuid.UUID, newBookedCount int, isAvailable bool) error {
	query := `
		UPDATE booking_slots
		SET booked_count = $1, is_available = $2, updated_at = $3
		WHERE id = $4
	`
	_, err := tx.ExecContext(ctx, query, newBookedCount, isAvailable, time.Now().UTC(), slotID)
	if err != nil {
		return fmt.Errorf("failed to increment slot booked count: %w", err)
	}
	return nil
}

func (r *postgresRepository) DecrementSlotBookingInTx(ctx context.Context, tx *sql.Tx, slotID uuid.UUID) error {
	query := `
		UPDATE booking_slots
		SET 
			booked_count = GREATEST(0, booked_count - 1),
			is_available = CASE WHEN (booked_count - 1) < max_capacity THEN TRUE ELSE is_available END,
			updated_at = $1
		WHERE id = $2
	`
	_, err := tx.ExecContext(ctx, query, time.Now().UTC(), slotID)
	if err != nil {
		return fmt.Errorf("failed to decrement slot booking count: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetSlotsByDate(ctx context.Context, workshopID uuid.UUID, slotDate string) ([]domain.BookingSlot, error) {
	query := `
		SELECT id, workshop_id, slot_date, start_time, end_time, max_capacity, booked_count, is_available, created_at, updated_at
		FROM booking_slots
		WHERE workshop_id = $1 AND slot_date = $2
		ORDER BY start_time ASC
	`
	rows, err := r.db.QueryContext(ctx, query, workshopID, slotDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query slots by date: %w", err)
	}
	defer rows.Close()

	var list []domain.BookingSlot
	for rows.Next() {
		var slot domain.BookingSlot
		var slotDateVal time.Time
		if err := rows.Scan(
			&slot.ID,
			&slot.WorkshopID,
			&slotDateVal,
			&slot.StartTime,
			&slot.EndTime,
			&slot.MaxCapacity,
			&slot.BookedCount,
			&slot.IsAvailable,
			&slot.CreatedAt,
			&slot.UpdatedAt,
		); err != nil {
			return nil, err
		}
		slot.SlotDate = slotDateVal.Format("2006-01-02")
		list = append(list, slot)
	}
	return list, rows.Err()
}

func (r *postgresRepository) CreateBookingInTx(ctx context.Context, tx *sql.Tx, b *domain.Booking) error {
	query := `
		INSERT INTO bookings (
			id, booking_number, customer_id, workshop_id, service_id, slot_id,
			booking_date, booking_time, status, customer_notes, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
	`
	_, err := tx.ExecContext(ctx, query,
		b.ID,
		b.BookingNumber,
		b.CustomerID,
		b.WorkshopID,
		b.ServiceID,
		b.SlotID,
		b.BookingDate,
		b.BookingTime,
		b.Status,
		b.CustomerNotes,
		b.CreatedAt,
		b.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert booking: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetBookingByID(ctx context.Context, id uuid.UUID) (*domain.Booking, error) {
	query := `
		SELECT 
			b.id, b.booking_number, b.customer_id, b.workshop_id, b.service_id, b.slot_id,
			b.booking_date, b.booking_time, b.status, b.customer_notes, b.created_at, b.updated_at,
			u.id, u.name, u.email, u.phone,
			w.id, w.name, w.address, w.phone,
			s.id, s.name, s.price, s.duration_minutes
		FROM bookings b
		JOIN users u ON b.customer_id = u.id
		JOIN workshops w ON b.workshop_id = w.id
		JOIN services s ON b.service_id = s.id
		WHERE b.id = $1
	`
	var b domain.Booking
	var bDateVal time.Time
	var notes sql.NullString

	var u domain.User
	var w domain.Workshop
	var s domain.Service

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&b.ID,
		&b.BookingNumber,
		&b.CustomerID,
		&b.WorkshopID,
		&b.ServiceID,
		&b.SlotID,
		&bDateVal,
		&b.BookingTime,
		&b.Status,
		&notes,
		&b.CreatedAt,
		&b.UpdatedAt,
		&u.ID, &u.Name, &u.Email, &u.Phone,
		&w.ID, &w.Name, &w.Address, &w.Phone,
		&s.ID, &s.Name, &s.Price, &s.DurationMinutes,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("failed to query booking by id: %w", err)
	}
	b.BookingDate = bDateVal.Format("2006-01-02")
	if notes.Valid {
		b.CustomerNotes = notes.String
	}
	b.Customer = &u
	b.Workshop = &w
	b.Service = &s
	return &b, nil
}

func (r *postgresRepository) GetBookingByIDForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Booking, error) {
	query := `
		SELECT id, booking_number, customer_id, workshop_id, service_id, slot_id, booking_date, booking_time, status, customer_notes, created_at, updated_at
		FROM bookings
		WHERE id = $1
		FOR UPDATE
	`
	var b domain.Booking
	var bDateVal time.Time
	var notes sql.NullString

	err := tx.QueryRowContext(ctx, query, id).Scan(
		&b.ID,
		&b.BookingNumber,
		&b.CustomerID,
		&b.WorkshopID,
		&b.ServiceID,
		&b.SlotID,
		&bDateVal,
		&b.BookingTime,
		&b.Status,
		&notes,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("failed to query booking for update: %w", err)
	}
	b.BookingDate = bDateVal.Format("2006-01-02")
	if notes.Valid {
		b.CustomerNotes = notes.String
	}
	return &b, nil
}

func (r *postgresRepository) UpdateBookingStatusInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.BookingStatus) error {
	query := `
		UPDATE bookings
		SET status = $1, updated_at = $2
		WHERE id = $3
	`
	_, err := tx.ExecContext(ctx, query, status, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to update booking status: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListByCustomer(ctx context.Context, customerID uuid.UUID, pagination domain.PaginationParams) ([]domain.Booking, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM bookings WHERE customer_id = $1`
	if err := r.db.QueryRowContext(ctx, countQuery, customerID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT 
			b.id, b.booking_number, b.customer_id, b.workshop_id, b.service_id, b.slot_id,
			b.booking_date, b.booking_time, b.status, b.customer_notes, b.created_at, b.updated_at,
			w.id, w.name, w.address, w.phone,
			s.id, s.name, s.price, s.duration_minutes
		FROM bookings b
		JOIN workshops w ON b.workshop_id = w.id
		JOIN services s ON b.service_id = s.id
		WHERE b.customer_id = $1
		ORDER BY b.booking_date DESC, b.booking_time DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, customerID, pagination.PageSize, pagination.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []domain.Booking
	for rows.Next() {
		var b domain.Booking
		var bDateVal time.Time
		var notes sql.NullString
		var w domain.Workshop
		var s domain.Service

		if err := rows.Scan(
			&b.ID, &b.BookingNumber, &b.CustomerID, &b.WorkshopID, &b.ServiceID, &b.SlotID,
			&bDateVal, &b.BookingTime, &b.Status, &notes, &b.CreatedAt, &b.UpdatedAt,
			&w.ID, &w.Name, &w.Address, &w.Phone,
			&s.ID, &s.Name, &s.Price, &s.DurationMinutes,
		); err != nil {
			return nil, 0, err
		}
		b.BookingDate = bDateVal.Format("2006-01-02")
		if notes.Valid {
			b.CustomerNotes = notes.String
		}
		b.Workshop = &w
		b.Service = &s
		list = append(list, b)
	}
	return list, total, rows.Err()
}

func (r *postgresRepository) ListByWorkshop(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams, date *string) ([]domain.Booking, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM bookings WHERE workshop_id = $1`
	var args []interface{}
	args = append(args, workshopID)

	if date != nil && *date != "" {
		countQuery += " AND booking_date = $2"
		args = append(args, *date)
	}

	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT 
			b.id, b.booking_number, b.customer_id, b.workshop_id, b.service_id, b.slot_id,
			b.booking_date, b.booking_time, b.status, b.customer_notes, b.created_at, b.updated_at,
			u.id, u.name, u.email, u.phone,
			s.id, s.name, s.price, s.duration_minutes
		FROM bookings b
		JOIN users u ON b.customer_id = u.id
		JOIN services s ON b.service_id = s.id
		WHERE b.workshop_id = $1
	`
	var selectArgs []interface{}
	selectArgs = append(selectArgs, workshopID)
	argIdx := 2

	if date != nil && *date != "" {
		query += fmt.Sprintf(" AND b.booking_date = $%d", argIdx)
		selectArgs = append(selectArgs, *date)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY b.booking_date ASC, b.booking_time ASC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	selectArgs = append(selectArgs, pagination.PageSize, pagination.Offset())

	rows, err := r.db.QueryContext(ctx, query, selectArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []domain.Booking
	for rows.Next() {
		var b domain.Booking
		var bDateVal time.Time
		var notes sql.NullString
		var u domain.User
		var s domain.Service

		if err := rows.Scan(
			&b.ID, &b.BookingNumber, &b.CustomerID, &b.WorkshopID, &b.ServiceID, &b.SlotID,
			&bDateVal, &b.BookingTime, &b.Status, &notes, &b.CreatedAt, &b.UpdatedAt,
			&u.ID, &u.Name, &u.Email, &u.Phone,
			&s.ID, &s.Name, &s.Price, &s.DurationMinutes,
		); err != nil {
			return nil, 0, err
		}
		b.BookingDate = bDateVal.Format("2006-01-02")
		if notes.Valid {
			b.CustomerNotes = notes.String
		}
		b.Customer = &u
		b.Service = &s
		list = append(list, b)
	}
	return list, total, rows.Err()
}

func (r *postgresRepository) GetWorkshopOperatingHour(ctx context.Context, workshopID uuid.UUID, dayOfWeek int) (*domain.OperatingHour, error) {
	query := `
		SELECT id, workshop_id, day_of_week, open_time, close_time, is_closed, created_at, updated_at
		FROM operating_hours
		WHERE workshop_id = $1 AND day_of_week = $2
	`
	var oh domain.OperatingHour
	err := r.db.QueryRowContext(ctx, query, workshopID, dayOfWeek).Scan(
		&oh.ID,
		&oh.WorkshopID,
		&oh.DayOfWeek,
		&oh.OpenTime,
		&oh.CloseTime,
		&oh.IsClosed,
		&oh.CreatedAt,
		&oh.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No custom hours set
		}
		return nil, err
	}
	return &oh, nil
}

func (r *postgresRepository) GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error) {
	query := `SELECT id, owner_id, name, address, status FROM workshops WHERE id = $1`
	var w domain.Workshop
	err := r.db.QueryRowContext(ctx, query, workshopID).Scan(&w.ID, &w.OwnerID, &w.Name, &w.Address, &w.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("workshop not found")
		}
		return nil, err
	}
	return &w, nil
}

func (r *postgresRepository) GetServiceByID(ctx context.Context, serviceID uuid.UUID) (*domain.Service, error) {
	query := `SELECT id, workshop_id, name, price, duration_minutes, is_active FROM services WHERE id = $1`
	var s domain.Service
	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(&s.ID, &s.WorkshopID, &s.Name, &s.Price, &s.DurationMinutes, &s.IsActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("service not found")
		}
		return nil, err
	}
	return &s, nil
}
