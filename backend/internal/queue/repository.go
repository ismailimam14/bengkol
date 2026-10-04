package queue

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
	ErrQueueNotFound      = errors.New("queue entry not found")
	ErrQueueAlreadyExists = errors.New("queue entry already exists for this booking")
	ErrBookingNotFound    = errors.New("booking not found")
	ErrWorkshopNotFound   = errors.New("workshop not found")
)

// Repository defines the data operations for Queue management
type Repository interface {
	BeginTx(ctx context.Context) (*sql.Tx, error)
	GetNextQueueNumberInTx(ctx context.Context, tx *sql.Tx, workshopID uuid.UUID, queueDate string) (int, error)
	CreateQueueInTx(ctx context.Context, tx *sql.Tx, q *domain.Queue) error
	GetQueueByID(ctx context.Context, id uuid.UUID) (*domain.Queue, error)
	GetQueueByIDForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Queue, error)
	GetQueueByBookingID(ctx context.Context, bookingID uuid.UUID) (*domain.Queue, error)
	GetActiveQueueByCustomerID(ctx context.Context, customerID uuid.UUID, queueDate string) (*domain.Queue, error)
	UpdateQueueStatusInTx(ctx context.Context, tx *sql.Tx, q *domain.Queue) error
	ListQueuesByWorkshop(ctx context.Context, workshopID uuid.UUID, queueDate string, status *domain.QueueStatus) ([]domain.Queue, error)
	GetQueueSummary(ctx context.Context, workshopID uuid.UUID, queueDate string) (*domain.QueueSummary, error)
	CountAheadInQueue(ctx context.Context, workshopID uuid.UUID, queueDate string, queueNumber int) (int, error)
	GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*domain.Booking, error)
	UpdateBookingStatusInTx(ctx context.Context, tx *sql.Tx, bookingID uuid.UUID, status domain.BookingStatus) error
	GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error)
}

type postgresQueueRepository struct {
	db *sql.DB
}

// NewRepository creates a new PostgreSQL queue repository
func NewRepository(db *sql.DB) Repository {
	return &postgresQueueRepository{db: db}
}

func (r *postgresQueueRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

func (r *postgresQueueRepository) GetNextQueueNumberInTx(ctx context.Context, tx *sql.Tx, workshopID uuid.UUID, queueDate string) (int, error) {
	// Lock the workshop row to serialize queue number generation for this workshop
	queryLock := `SELECT id FROM workshops WHERE id = $1 FOR UPDATE`
	var lockedID uuid.UUID
	if tx != nil {
		if err := tx.QueryRowContext(ctx, queryLock, workshopID).Scan(&lockedID); err != nil {
			return 0, fmt.Errorf("failed to lock workshop for queue generation: %w", err)
		}
	}

	query := `
		SELECT COALESCE(MAX(queue_number), 0) + 1
		FROM queues
		WHERE workshop_id = $1 AND queue_date = $2
	`
	var nextNum int
	var err error
	if tx != nil {
		err = tx.QueryRowContext(ctx, query, workshopID, queueDate).Scan(&nextNum)
	} else {
		err = r.db.QueryRowContext(ctx, query, workshopID, queueDate).Scan(&nextNum)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to get next queue number: %w", err)
	}
	return nextNum, nil
}

func (r *postgresQueueRepository) CreateQueueInTx(ctx context.Context, tx *sql.Tx, q *domain.Queue) error {
	query := `
		INSERT INTO queues (
			id, booking_id, workshop_id, queue_date, queue_number, status,
			called_at, service_started_at, service_completed_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query,
			q.ID, q.BookingID, q.WorkshopID, q.QueueDate, q.QueueNumber, q.Status,
			q.CalledAt, q.ServiceStartedAt, q.ServiceCompletedAt, q.CreatedAt, q.UpdatedAt,
		)
	} else {
		_, err = r.db.ExecContext(ctx, query,
			q.ID, q.BookingID, q.WorkshopID, q.QueueDate, q.QueueNumber, q.Status,
			q.CalledAt, q.ServiceStartedAt, q.ServiceCompletedAt, q.CreatedAt, q.UpdatedAt,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to insert queue entry: %w", err)
	}
	return nil
}

func (r *postgresQueueRepository) GetQueueByID(ctx context.Context, id uuid.UUID) (*domain.Queue, error) {
	query := `
		SELECT 
			q.id, q.booking_id, q.workshop_id, q.queue_date, q.queue_number, q.status,
			q.called_at, q.service_started_at, q.service_completed_at, q.created_at, q.updated_at,
			b.id, b.booking_number, b.customer_id, b.service_id, b.booking_date, b.booking_time, b.status, b.customer_notes,
			u.id, u.name, u.phone, COALESCE(u.email, ''),
			s.id, s.name, s.price, s.duration_minutes,
			w.id, w.name, w.address, w.phone, w.owner_id
		FROM queues q
		JOIN bookings b ON q.booking_id = b.id
		JOIN users u ON b.customer_id = u.id
		JOIN services s ON b.service_id = s.id
		JOIN workshops w ON q.workshop_id = w.id
		WHERE q.id = $1
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanEnrichedQueue(row)
}

func (r *postgresQueueRepository) GetQueueByIDForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Queue, error) {
	query := `
		SELECT 
			id, booking_id, workshop_id, queue_date, queue_number, status,
			called_at, service_started_at, service_completed_at, created_at, updated_at
		FROM queues
		WHERE id = $1
		FOR UPDATE
	`
	var row *sql.Row
	if tx != nil {
		row = tx.QueryRowContext(ctx, query, id)
	} else {
		row = r.db.QueryRowContext(ctx, query, id)
	}

	var q domain.Queue
	var queueDate time.Time
	var calledAt, startedAt, completedAt sql.NullTime

	err := row.Scan(
		&q.ID, &q.BookingID, &q.WorkshopID, &queueDate, &q.QueueNumber, &q.Status,
		&calledAt, &startedAt, &completedAt, &q.CreatedAt, &q.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrQueueNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan queue for update: %w", err)
	}

	q.QueueDate = queueDate.Format("2006-01-02")
	if calledAt.Valid {
		t := calledAt.Time
		q.CalledAt = &t
	}
	if startedAt.Valid {
		t := startedAt.Time
		q.ServiceStartedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		q.ServiceCompletedAt = &t
	}

	return &q, nil
}

func (r *postgresQueueRepository) GetQueueByBookingID(ctx context.Context, bookingID uuid.UUID) (*domain.Queue, error) {
	query := `
		SELECT 
			id, booking_id, workshop_id, queue_date, queue_number, status,
			called_at, service_started_at, service_completed_at, created_at, updated_at
		FROM queues
		WHERE booking_id = $1
	`
	row := r.db.QueryRowContext(ctx, query, bookingID)
	var q domain.Queue
	var queueDate time.Time
	var calledAt, startedAt, completedAt sql.NullTime

	err := row.Scan(
		&q.ID, &q.BookingID, &q.WorkshopID, &queueDate, &q.QueueNumber, &q.Status,
		&calledAt, &startedAt, &completedAt, &q.CreatedAt, &q.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrQueueNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan queue by booking id: %w", err)
	}

	q.QueueDate = queueDate.Format("2006-01-02")
	if calledAt.Valid {
		t := calledAt.Time
		q.CalledAt = &t
	}
	if startedAt.Valid {
		t := startedAt.Time
		q.ServiceStartedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		q.ServiceCompletedAt = &t
	}

	return &q, nil
}

func (r *postgresQueueRepository) GetActiveQueueByCustomerID(ctx context.Context, customerID uuid.UUID, queueDate string) (*domain.Queue, error) {
	query := `
		SELECT 
			q.id, q.booking_id, q.workshop_id, q.queue_date, q.queue_number, q.status,
			q.called_at, q.service_started_at, q.service_completed_at, q.created_at, q.updated_at,
			b.id, b.booking_number, b.customer_id, b.service_id, b.booking_date, b.booking_time, b.status, b.customer_notes,
			u.id, u.name, u.phone, COALESCE(u.email, ''),
			s.id, s.name, s.price, s.duration_minutes,
			w.id, w.name, w.address, w.phone, w.owner_id
		FROM queues q
		JOIN bookings b ON q.booking_id = b.id
		JOIN users u ON b.customer_id = u.id
		JOIN services s ON b.service_id = s.id
		JOIN workshops w ON q.workshop_id = w.id
		WHERE b.customer_id = $1 AND q.queue_date = $2 AND q.status IN ('WAITING', 'CALLED', 'IN_SERVICE')
		ORDER BY q.created_at DESC
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, customerID, queueDate)
	return r.scanEnrichedQueue(row)
}

func (r *postgresQueueRepository) UpdateQueueStatusInTx(ctx context.Context, tx *sql.Tx, q *domain.Queue) error {
	query := `
		UPDATE queues
		SET status = $1, called_at = $2, service_started_at = $3, service_completed_at = $4, updated_at = $5
		WHERE id = $6
	`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, q.Status, q.CalledAt, q.ServiceStartedAt, q.ServiceCompletedAt, q.UpdatedAt, q.ID)
	} else {
		_, err = r.db.ExecContext(ctx, query, q.Status, q.CalledAt, q.ServiceStartedAt, q.ServiceCompletedAt, q.UpdatedAt, q.ID)
	}

	if err != nil {
		return fmt.Errorf("failed to update queue status: %w", err)
	}
	return nil
}

func (r *postgresQueueRepository) ListQueuesByWorkshop(ctx context.Context, workshopID uuid.UUID, queueDate string, status *domain.QueueStatus) ([]domain.Queue, error) {
	baseQuery := `
		SELECT 
			q.id, q.booking_id, q.workshop_id, q.queue_date, q.queue_number, q.status,
			q.called_at, q.service_started_at, q.service_completed_at, q.created_at, q.updated_at,
			b.id, b.booking_number, b.customer_id, b.service_id, b.booking_date, b.booking_time, b.status, b.customer_notes,
			u.id, u.name, u.phone, COALESCE(u.email, ''),
			s.id, s.name, s.price, s.duration_minutes,
			w.id, w.name, w.address, w.phone, w.owner_id
		FROM queues q
		JOIN bookings b ON q.booking_id = b.id
		JOIN users u ON b.customer_id = u.id
		JOIN services s ON b.service_id = s.id
		JOIN workshops w ON q.workshop_id = w.id
		WHERE q.workshop_id = $1 AND q.queue_date = $2
	`
	var args []interface{}
	args = append(args, workshopID, queueDate)

	if status != nil && *status != "" {
		baseQuery += " AND q.status = $3"
		args = append(args, *status)
	}

	baseQuery += " ORDER BY q.queue_number ASC"

	rows, err := r.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query queues: %w", err)
	}
	defer rows.Close()

	var list []domain.Queue
	for rows.Next() {
		var q domain.Queue
		var queueDateVal, bookingDateVal time.Time
		var calledAt, startedAt, completedAt sql.NullTime
		var customerNotes sql.NullString
		var b domain.Booking
		var u domain.User
		var s domain.Service
		var w domain.Workshop

		err := rows.Scan(
			&q.ID, &q.BookingID, &q.WorkshopID, &queueDateVal, &q.QueueNumber, &q.Status,
			&calledAt, &startedAt, &completedAt, &q.CreatedAt, &q.UpdatedAt,
			&b.ID, &b.BookingNumber, &b.CustomerID, &b.ServiceID, &bookingDateVal, &b.BookingTime, &b.Status, &customerNotes,
			&u.ID, &u.Name, &u.Phone, &u.Email,
			&s.ID, &s.Name, &s.Price, &s.DurationMinutes,
			&w.ID, &w.Name, &w.Address, &w.Phone, &w.OwnerID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue row: %w", err)
		}

		q.QueueDate = queueDateVal.Format("2006-01-02")
		if calledAt.Valid {
			t := calledAt.Time
			q.CalledAt = &t
		}
		if startedAt.Valid {
			t := startedAt.Time
			q.ServiceStartedAt = &t
		}
		if completedAt.Valid {
			t := completedAt.Time
			q.ServiceCompletedAt = &t
		}

		b.BookingDate = bookingDateVal.Format("2006-01-02")
		if customerNotes.Valid {
			b.CustomerNotes = customerNotes.String
		}
		b.Customer = &u
		b.Service = &s
		b.Workshop = &w
		q.Booking = &b

		list = append(list, q)
	}

	return list, nil
}

func (r *postgresQueueRepository) GetQueueSummary(ctx context.Context, workshopID uuid.UUID, queueDate string) (*domain.QueueSummary, error) {
	query := `
		SELECT 
			COUNT(*) FILTER (WHERE status = 'WAITING') AS total_waiting,
			COUNT(*) FILTER (WHERE status = 'IN_SERVICE') AS total_in_service,
			COUNT(*) FILTER (WHERE status = 'COMPLETED') AS total_completed,
			MIN(queue_number) FILTER (WHERE status = 'IN_SERVICE') AS current_serving_no,
			MIN(queue_number) FILTER (WHERE status = 'CALLED') AS current_called_no
		FROM queues
		WHERE workshop_id = $1 AND queue_date = $2
	`
	var summary domain.QueueSummary
	summary.WorkshopID = workshopID
	summary.QueueDate = queueDate

	var servingNo, calledNo sql.NullInt32
	err := r.db.QueryRowContext(ctx, query, workshopID, queueDate).Scan(
		&summary.TotalWaiting,
		&summary.TotalInService,
		&summary.TotalCompleted,
		&servingNo,
		&calledNo,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue summary: %w", err)
	}

	if servingNo.Valid {
		num := int(servingNo.Int32)
		summary.CurrentServingNo = &num
	}
	if calledNo.Valid {
		num := int(calledNo.Int32)
		summary.CurrentCalledNo = &num
	}

	return &summary, nil
}

func (r *postgresQueueRepository) CountAheadInQueue(ctx context.Context, workshopID uuid.UUID, queueDate string, queueNumber int) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM queues
		WHERE workshop_id = $1 AND queue_date = $2 AND queue_number < $3 AND status IN ('WAITING', 'CALLED')
	`
	var count int
	err := r.db.QueryRowContext(ctx, query, workshopID, queueDate, queueNumber).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count ahead queues: %w", err)
	}
	return count, nil
}

func (r *postgresQueueRepository) GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*domain.Booking, error) {
	query := `
		SELECT 
			b.id, b.booking_number, b.customer_id, b.workshop_id, b.service_id, b.slot_id,
			b.booking_date, b.booking_time, b.status, b.customer_notes, b.created_at, b.updated_at,
			u.id, u.name, u.phone, COALESCE(u.email, ''),
			s.id, s.name, s.price, s.duration_minutes,
			w.id, w.name, w.address, w.phone, w.owner_id
		FROM bookings b
		JOIN users u ON b.customer_id = u.id
		JOIN services s ON b.service_id = s.id
		JOIN workshops w ON b.workshop_id = w.id
		WHERE b.id = $1
	`
	row := r.db.QueryRowContext(ctx, query, bookingID)

	var b domain.Booking
	var u domain.User
	var s domain.Service
	var w domain.Workshop
	var bookingDateVal time.Time
	var customerNotes sql.NullString

	err := row.Scan(
		&b.ID, &b.BookingNumber, &b.CustomerID, &b.WorkshopID, &b.ServiceID, &b.SlotID,
		&bookingDateVal, &b.BookingTime, &b.Status, &customerNotes, &b.CreatedAt, &b.UpdatedAt,
		&u.ID, &u.Name, &u.Phone, &u.Email,
		&s.ID, &s.Name, &s.Price, &s.DurationMinutes,
		&w.ID, &w.Name, &w.Address, &w.Phone, &w.OwnerID,
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
	b.Customer = &u
	b.Service = &s
	b.Workshop = &w

	return &b, nil
}

func (r *postgresQueueRepository) UpdateBookingStatusInTx(ctx context.Context, tx *sql.Tx, bookingID uuid.UUID, status domain.BookingStatus) error {
	query := `UPDATE bookings SET status = $1, updated_at = NOW() WHERE id = $2`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, status, bookingID)
	} else {
		_, err = r.db.ExecContext(ctx, query, status, bookingID)
	}
	if err != nil {
		return fmt.Errorf("failed to update booking status: %w", err)
	}
	return nil
}

func (r *postgresQueueRepository) GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error) {
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

func (r *postgresQueueRepository) scanEnrichedQueue(row *sql.Row) (*domain.Queue, error) {
	var q domain.Queue
	var queueDateVal, bookingDateVal time.Time
	var calledAt, startedAt, completedAt sql.NullTime
	var customerNotes sql.NullString
	var b domain.Booking
	var u domain.User
	var s domain.Service
	var w domain.Workshop

	err := row.Scan(
		&q.ID, &q.BookingID, &q.WorkshopID, &queueDateVal, &q.QueueNumber, &q.Status,
		&calledAt, &startedAt, &completedAt, &q.CreatedAt, &q.UpdatedAt,
		&b.ID, &b.BookingNumber, &b.CustomerID, &b.ServiceID, &bookingDateVal, &b.BookingTime, &b.Status, &customerNotes,
		&u.ID, &u.Name, &u.Phone, &u.Email,
		&s.ID, &s.Name, &s.Price, &s.DurationMinutes,
		&w.ID, &w.Name, &w.Address, &w.Phone, &w.OwnerID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrQueueNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan enriched queue: %w", err)
	}

	q.QueueDate = queueDateVal.Format("2006-01-02")
	if calledAt.Valid {
		t := calledAt.Time
		q.CalledAt = &t
	}
	if startedAt.Valid {
		t := startedAt.Time
		q.ServiceStartedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		q.ServiceCompletedAt = &t
	}

	b.BookingDate = bookingDateVal.Format("2006-01-02")
	if customerNotes.Valid {
		b.CustomerNotes = customerNotes.String
	}
	b.Customer = &u
	b.Service = &s
	b.Workshop = &w
	q.Booking = &b

	return &q, nil
}
