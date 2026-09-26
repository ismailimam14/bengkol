package history

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
	ErrHistoryNotFound    = errors.New("service history record not found")
	ErrHistoryExists      = errors.New("service history already recorded for this booking")
	ErrBookingNotFound    = errors.New("booking not found")
	ErrSparePartNotFound  = errors.New("spare part not found")
	ErrWorkshopNotFound   = errors.New("workshop not found")
	ErrInsufficientStock  = errors.New("insufficient stock for requested spare part")
)

// Repository defines data access operations for service histories
type Repository interface {
	BeginTx(ctx context.Context) (*sql.Tx, error)
	CreateServiceHistoryInTx(ctx context.Context, tx *sql.Tx, h *domain.ServiceHistory) error
	CreateServiceHistorySparePartInTx(ctx context.Context, tx *sql.Tx, item *domain.ServiceHistorySparePart) error
	GetSparePartForUpdateInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.SparePart, error)
	DecrementSparePartStockInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, quantity int) error
	GetHistoryByID(ctx context.Context, id uuid.UUID) (*domain.ServiceHistory, error)
	GetHistoryByBookingID(ctx context.Context, bookingID uuid.UUID) (*domain.ServiceHistory, error)
	ListHistoriesByCustomer(ctx context.Context, customerID uuid.UUID, pagination domain.PaginationParams) ([]domain.ServiceHistory, int64, error)
	ListHistoriesByWorkshop(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams, startDate, endDate *string) ([]domain.ServiceHistory, int64, error)
	GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*domain.Booking, error)
	UpdateBookingStatusInTx(ctx context.Context, tx *sql.Tx, bookingID uuid.UUID, status domain.BookingStatus) error
	CompleteQueueForBookingInTx(ctx context.Context, tx *sql.Tx, bookingID uuid.UUID) error
	GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error)
}

type postgresHistoryRepository struct {
	db *sql.DB
}

// NewRepository creates a new instance of PostgreSQL history repository
func NewRepository(db *sql.DB) Repository {
	return &postgresHistoryRepository{db: db}
}

func (r *postgresHistoryRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

func (r *postgresHistoryRepository) CreateServiceHistoryInTx(ctx context.Context, tx *sql.Tx, h *domain.ServiceHistory) error {
	query := `
		INSERT INTO service_histories (
			id, booking_id, customer_id, workshop_id, service_id,
			service_date, service_price, total_price, notes, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query,
			h.ID, h.BookingID, h.CustomerID, h.WorkshopID, h.ServiceID,
			h.ServiceDate, h.ServicePrice, h.TotalPrice, h.Notes, h.CreatedAt,
		)
	} else {
		_, err = r.db.ExecContext(ctx, query,
			h.ID, h.BookingID, h.CustomerID, h.WorkshopID, h.ServiceID,
			h.ServiceDate, h.ServicePrice, h.TotalPrice, h.Notes, h.CreatedAt,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to insert service history: %w", err)
	}
	return nil
}

func (r *postgresHistoryRepository) CreateServiceHistorySparePartInTx(ctx context.Context, tx *sql.Tx, item *domain.ServiceHistorySparePart) error {
	query := `
		INSERT INTO service_history_spare_parts (
			id, service_history_id, spare_part_id, quantity, price_per_unit, subtotal, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query,
			item.ID, item.ServiceHistoryID, item.SparePartID, item.Quantity, item.PricePerUnit, item.Subtotal, item.CreatedAt,
		)
	} else {
		_, err = r.db.ExecContext(ctx, query,
			item.ID, item.ServiceHistoryID, item.SparePartID, item.Quantity, item.PricePerUnit, item.Subtotal, item.CreatedAt,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to insert service history spare part: %w", err)
	}
	return nil
}

func (r *postgresHistoryRepository) GetSparePartForUpdateInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.SparePart, error) {
	query := `
		SELECT id, workshop_id, name, description, purchase_price, selling_price, stock, is_active, created_at, updated_at
		FROM spare_parts
		WHERE id = $1
		FOR UPDATE
	`
	var row *sql.Row
	if tx != nil {
		row = tx.QueryRowContext(ctx, query, id)
	} else {
		row = r.db.QueryRowContext(ctx, query, id)
	}

	var sp domain.SparePart
	var desc sql.NullString
	err := row.Scan(
		&sp.ID, &sp.WorkshopID, &sp.Name, &desc,
		&sp.PurchasePrice, &sp.SellingPrice, &sp.Stock, &sp.IsActive,
		&sp.CreatedAt, &sp.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSparePartNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan spare part for update: %w", err)
	}

	if desc.Valid {
		sp.Description = desc.String
	}
	return &sp, nil
}

func (r *postgresHistoryRepository) DecrementSparePartStockInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, quantity int) error {
	query := `
		UPDATE spare_parts
		SET stock = stock - $1, updated_at = NOW()
		WHERE id = $2 AND stock >= $1
	`
	var res sql.Result
	var err error
	if tx != nil {
		res, err = tx.ExecContext(ctx, query, quantity, id)
	} else {
		res, err = r.db.ExecContext(ctx, query, quantity, id)
	}

	if err != nil {
		return fmt.Errorf("failed to decrement spare part stock: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrInsufficientStock
	}
	return nil
}

func (r *postgresHistoryRepository) GetHistoryByID(ctx context.Context, id uuid.UUID) (*domain.ServiceHistory, error) {
	query := `
		SELECT 
			sh.id, sh.booking_id, sh.customer_id, sh.workshop_id, sh.service_id,
			sh.service_date, sh.service_price, sh.total_price, sh.notes, sh.created_at,
			w.id, w.name, w.address, w.phone, w.owner_id,
			s.id, s.name, s.duration_minutes
		FROM service_histories sh
		JOIN workshops w ON sh.workshop_id = w.id
		JOIN services s ON sh.service_id = s.id
		WHERE sh.id = $1
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var sh domain.ServiceHistory
	var w domain.Workshop
	var s domain.Service
	var serviceDateVal time.Time
	var notes sql.NullString

	err := row.Scan(
		&sh.ID, &sh.BookingID, &sh.CustomerID, &sh.WorkshopID, &sh.ServiceID,
		&serviceDateVal, &sh.ServicePrice, &sh.TotalPrice, &notes, &sh.CreatedAt,
		&w.ID, &w.Name, &w.Address, &w.Phone, &w.OwnerID,
		&s.ID, &s.Name, &s.DurationMinutes,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrHistoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan service history: %w", err)
	}

	sh.ServiceDate = serviceDateVal.Format("2006-01-02")
	if notes.Valid {
		sh.Notes = notes.String
	}
	sh.Workshop = &w
	sh.Service = &s

	// Load spare parts used
	parts, err := r.loadHistorySpareParts(ctx, sh.ID)
	if err != nil {
		return nil, err
	}
	sh.SpareParts = parts

	return &sh, nil
}

func (r *postgresHistoryRepository) GetHistoryByBookingID(ctx context.Context, bookingID uuid.UUID) (*domain.ServiceHistory, error) {
	query := `
		SELECT 
			sh.id, sh.booking_id, sh.customer_id, sh.workshop_id, sh.service_id,
			sh.service_date, sh.service_price, sh.total_price, sh.notes, sh.created_at,
			w.id, w.name, w.address, w.phone, w.owner_id,
			s.id, s.name, s.duration_minutes
		FROM service_histories sh
		JOIN workshops w ON sh.workshop_id = w.id
		JOIN services s ON sh.service_id = s.id
		WHERE sh.booking_id = $1
	`
	row := r.db.QueryRowContext(ctx, query, bookingID)

	var sh domain.ServiceHistory
	var w domain.Workshop
	var s domain.Service
	var serviceDateVal time.Time
	var notes sql.NullString

	err := row.Scan(
		&sh.ID, &sh.BookingID, &sh.CustomerID, &sh.WorkshopID, &sh.ServiceID,
		&serviceDateVal, &sh.ServicePrice, &sh.TotalPrice, &notes, &sh.CreatedAt,
		&w.ID, &w.Name, &w.Address, &w.Phone, &w.OwnerID,
		&s.ID, &s.Name, &s.DurationMinutes,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrHistoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan service history by booking id: %w", err)
	}

	sh.ServiceDate = serviceDateVal.Format("2006-01-02")
	if notes.Valid {
		sh.Notes = notes.String
	}
	sh.Workshop = &w
	sh.Service = &s

	parts, err := r.loadHistorySpareParts(ctx, sh.ID)
	if err != nil {
		return nil, err
	}
	sh.SpareParts = parts

	return &sh, nil
}

func (r *postgresHistoryRepository) ListHistoriesByCustomer(ctx context.Context, customerID uuid.UUID, pagination domain.PaginationParams) ([]domain.ServiceHistory, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM service_histories WHERE customer_id = $1`
	if err := r.db.QueryRowContext(ctx, countQuery, customerID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count customer histories: %w", err)
	}

	query := `
		SELECT 
			sh.id, sh.booking_id, sh.customer_id, sh.workshop_id, sh.service_id,
			sh.service_date, sh.service_price, sh.total_price, sh.notes, sh.created_at,
			w.id, w.name, w.address, w.phone, w.owner_id,
			s.id, s.name, s.duration_minutes
		FROM service_histories sh
		JOIN workshops w ON sh.workshop_id = w.id
		JOIN services s ON sh.service_id = s.id
		WHERE sh.customer_id = $1
		ORDER BY sh.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, customerID, pagination.PageSize, pagination.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query customer histories: %w", err)
	}
	defer rows.Close()

	var list []domain.ServiceHistory
	for rows.Next() {
		var sh domain.ServiceHistory
		var w domain.Workshop
		var s domain.Service
		var serviceDateVal time.Time
		var notes sql.NullString

		err := rows.Scan(
			&sh.ID, &sh.BookingID, &sh.CustomerID, &sh.WorkshopID, &sh.ServiceID,
			&serviceDateVal, &sh.ServicePrice, &sh.TotalPrice, &notes, &sh.CreatedAt,
			&w.ID, &w.Name, &w.Address, &w.Phone, &w.OwnerID,
			&s.ID, &s.Name, &s.DurationMinutes,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan history row: %w", err)
		}

		sh.ServiceDate = serviceDateVal.Format("2006-01-02")
		if notes.Valid {
			sh.Notes = notes.String
		}
		sh.Workshop = &w
		sh.Service = &s

		parts, _ := r.loadHistorySpareParts(ctx, sh.ID)
		sh.SpareParts = parts

		list = append(list, sh)
	}

	return list, total, nil
}

func (r *postgresHistoryRepository) ListHistoriesByWorkshop(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams, startDate, endDate *string) ([]domain.ServiceHistory, int64, error) {
	baseWhere := "WHERE sh.workshop_id = $1"
	args := []interface{}{workshopID}
	argIdx := 2

	if startDate != nil && *startDate != "" {
		baseWhere += fmt.Sprintf(" AND sh.service_date >= $%d", argIdx)
		args = append(args, *startDate)
		argIdx++
	}
	if endDate != nil && *endDate != "" {
		baseWhere += fmt.Sprintf(" AND sh.service_date <= $%d", argIdx)
		args = append(args, *endDate)
		argIdx++
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM service_histories sh %s", baseWhere)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count workshop histories: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT 
			sh.id, sh.booking_id, sh.customer_id, sh.workshop_id, sh.service_id,
			sh.service_date, sh.service_price, sh.total_price, sh.notes, sh.created_at,
			w.id, w.name, w.address, w.phone, w.owner_id,
			s.id, s.name, s.duration_minutes
		FROM service_histories sh
		JOIN workshops w ON sh.workshop_id = w.id
		JOIN services s ON sh.service_id = s.id
		%s
		ORDER BY sh.created_at DESC
		LIMIT $%d OFFSET $%d
	`, baseWhere, argIdx, argIdx+1)

	args = append(args, pagination.PageSize, pagination.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query workshop histories: %w", err)
	}
	defer rows.Close()

	var list []domain.ServiceHistory
	for rows.Next() {
		var sh domain.ServiceHistory
		var w domain.Workshop
		var s domain.Service
		var serviceDateVal time.Time
		var notes sql.NullString

		err := rows.Scan(
			&sh.ID, &sh.BookingID, &sh.CustomerID, &sh.WorkshopID, &sh.ServiceID,
			&serviceDateVal, &sh.ServicePrice, &sh.TotalPrice, &notes, &sh.CreatedAt,
			&w.ID, &w.Name, &w.Address, &w.Phone, &w.OwnerID,
			&s.ID, &s.Name, &s.DurationMinutes,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan history row: %w", err)
		}

		sh.ServiceDate = serviceDateVal.Format("2006-01-02")
		if notes.Valid {
			sh.Notes = notes.String
		}
		sh.Workshop = &w
		sh.Service = &s

		parts, _ := r.loadHistorySpareParts(ctx, sh.ID)
		sh.SpareParts = parts

		list = append(list, sh)
	}

	return list, total, nil
}

func (r *postgresHistoryRepository) loadHistorySpareParts(ctx context.Context, historyID uuid.UUID) ([]domain.ServiceHistorySparePart, error) {
	query := `
		SELECT 
			shsp.id, shsp.service_history_id, shsp.spare_part_id, shsp.quantity, shsp.price_per_unit, shsp.subtotal, shsp.created_at,
			sp.id, sp.name, sp.description
		FROM service_history_spare_parts shsp
		JOIN spare_parts sp ON shsp.spare_part_id = sp.id
		WHERE shsp.service_history_id = $1
		ORDER BY shsp.created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, historyID)
	if err != nil {
		return nil, fmt.Errorf("failed to query history spare parts: %w", err)
	}
	defer rows.Close()

	var list []domain.ServiceHistorySparePart
	for rows.Next() {
		var item domain.ServiceHistorySparePart
		var sp domain.SparePart
		var desc sql.NullString

		err := rows.Scan(
			&item.ID, &item.ServiceHistoryID, &item.SparePartID, &item.Quantity, &item.PricePerUnit, &item.Subtotal, &item.CreatedAt,
			&sp.ID, &sp.Name, &desc,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan spare part row: %w", err)
		}

		if desc.Valid {
			sp.Description = desc.String
		}
		item.SparePart = &sp
		list = append(list, item)
	}

	return list, nil
}

func (r *postgresHistoryRepository) GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*domain.Booking, error) {
	query := `
		SELECT 
			b.id, b.booking_number, b.customer_id, b.workshop_id, b.service_id, b.slot_id,
			b.booking_date, b.booking_time, b.status, b.customer_notes, b.created_at, b.updated_at,
			u.id, u.name, u.phone, u.email,
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

func (r *postgresHistoryRepository) UpdateBookingStatusInTx(ctx context.Context, tx *sql.Tx, bookingID uuid.UUID, status domain.BookingStatus) error {
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

func (r *postgresHistoryRepository) CompleteQueueForBookingInTx(ctx context.Context, tx *sql.Tx, bookingID uuid.UUID) error {
	query := `
		UPDATE queues
		SET status = 'COMPLETED', service_completed_at = NOW(), updated_at = NOW()
		WHERE booking_id = $1 AND status != 'COMPLETED' AND status != 'CANCELLED'
	`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, bookingID)
	} else {
		_, err = r.db.ExecContext(ctx, query, bookingID)
	}
	if err != nil {
		return fmt.Errorf("failed to complete queue for booking: %w", err)
	}
	return nil
}

func (r *postgresHistoryRepository) GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error) {
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
