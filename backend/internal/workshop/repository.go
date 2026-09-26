package workshop

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
	ErrWorkshopNotFound = errors.New("workshop not found")
)

// WorkshopFilter contains criteria for searching workshops.
type WorkshopFilter struct {
	Status *domain.WorkshopStatus
	Search string
}

// NearbyParams holds spatial search parameters for PostGIS.
type NearbyParams struct {
	Latitude     float64
	Longitude    float64
	RadiusMeters float64
	Limit        int
	Offset       int
}

// Repository defines data access operations for Workshops and Operating Hours.
type Repository interface {
	Create(ctx context.Context, w *domain.Workshop) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Workshop, error)
	GetByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]domain.Workshop, error)
	List(ctx context.Context, pagination domain.PaginationParams, filter WorkshopFilter) ([]domain.Workshop, int64, error)
	FindNearby(ctx context.Context, params NearbyParams) ([]domain.Workshop, error)
	Update(ctx context.Context, w *domain.Workshop) error
	GetOperatingHours(ctx context.Context, workshopID uuid.UUID) ([]domain.OperatingHour, error)
	UpsertOperatingHours(ctx context.Context, workshopID uuid.UUID, hours []domain.OperatingHour) error
}

type postgresRepository struct {
	db *sql.DB
}

// NewRepository creates a new PostgreSQL workshop repository.
func NewRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Create(ctx context.Context, w *domain.Workshop) error {
	query := `
		INSERT INTO workshops (
			id, owner_id, name, description, address,
			latitude, longitude, phone, rating, review_count, status,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10, $11,
			$12, $13
		)
	`
	_, err := r.db.ExecContext(ctx, query,
		w.ID,
		w.OwnerID,
		w.Name,
		w.Description,
		w.Address,
		w.Latitude,
		w.Longitude,
		w.Phone,
		w.Rating,
		w.ReviewCount,
		w.Status,
		w.CreatedAt,
		w.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert workshop: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Workshop, error) {
	query := `
		SELECT 
			id, owner_id, name, description, address,
			latitude, longitude, phone, rating, review_count, status,
			created_at, updated_at
		FROM workshops
		WHERE id = $1
	`
	var w domain.Workshop
	var desc sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&w.ID,
		&w.OwnerID,
		&w.Name,
		&desc,
		&w.Address,
		&w.Latitude,
		&w.Longitude,
		&w.Phone,
		&w.Rating,
		&w.ReviewCount,
		&w.Status,
		&w.CreatedAt,
		&w.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkshopNotFound
		}
		return nil, fmt.Errorf("failed to query workshop by id: %w", err)
	}
	if desc.Valid {
		w.Description = desc.String
	}
	return &w, nil
}

func (r *postgresRepository) GetByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]domain.Workshop, error) {
	query := `
		SELECT 
			id, owner_id, name, description, address,
			latitude, longitude, phone, rating, review_count, status,
			created_at, updated_at
		FROM workshops
		WHERE owner_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query workshops by owner: %w", err)
	}
	defer rows.Close()

	var list []domain.Workshop
	for rows.Next() {
		var w domain.Workshop
		var desc sql.NullString
		if err := rows.Scan(
			&w.ID,
			&w.OwnerID,
			&w.Name,
			&desc,
			&w.Address,
			&w.Latitude,
			&w.Longitude,
			&w.Phone,
			&w.Rating,
			&w.ReviewCount,
			&w.Status,
			&w.CreatedAt,
			&w.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if desc.Valid {
			w.Description = desc.String
		}
		list = append(list, w)
	}
	return list, rows.Err()
}

func (r *postgresRepository) List(ctx context.Context, pagination domain.PaginationParams, filter WorkshopFilter) ([]domain.Workshop, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM workshops WHERE 1=1`
	var args []interface{}
	argIdx := 1

	if filter.Status != nil {
		countQuery += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.Search != "" {
		countQuery += fmt.Sprintf(" AND (name ILIKE $%d OR address ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}

	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count workshops: %w", err)
	}

	selectQuery := `
		SELECT 
			id, owner_id, name, description, address,
			latitude, longitude, phone, rating, review_count, status,
			created_at, updated_at
		FROM workshops
		WHERE 1=1
	`
	var selectArgs []interface{}
	argIdx = 1

	if filter.Status != nil {
		selectQuery += fmt.Sprintf(" AND status = $%d", argIdx)
		selectArgs = append(selectArgs, *filter.Status)
		argIdx++
	}
	if filter.Search != "" {
		selectQuery += fmt.Sprintf(" AND (name ILIKE $%d OR address ILIKE $%d)", argIdx, argIdx)
		selectArgs = append(selectArgs, "%"+filter.Search+"%")
		argIdx++
	}

	selectQuery += fmt.Sprintf(" ORDER BY rating DESC, review_count DESC, created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	selectArgs = append(selectArgs, pagination.PageSize, pagination.Offset())

	rows, err := r.db.QueryContext(ctx, selectQuery, selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list workshops: %w", err)
	}
	defer rows.Close()

	var list []domain.Workshop
	for rows.Next() {
		var w domain.Workshop
		var desc sql.NullString
		if err := rows.Scan(
			&w.ID,
			&w.OwnerID,
			&w.Name,
			&desc,
			&w.Address,
			&w.Latitude,
			&w.Longitude,
			&w.Phone,
			&w.Rating,
			&w.ReviewCount,
			&w.Status,
			&w.CreatedAt,
			&w.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		if desc.Valid {
			w.Description = desc.String
		}
		list = append(list, w)
	}
	return list, total, rows.Err()
}

func (r *postgresRepository) FindNearby(ctx context.Context, params NearbyParams) ([]domain.Workshop, error) {
	// PostGIS spatial query calculating distance in meters and filtering by radius
	query := `
		SELECT 
			id, owner_id, name, description, address,
			latitude, longitude,
			ST_Distance(location, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography) AS distance_meters,
			phone, rating, review_count, status,
			created_at, updated_at
		FROM workshops
		WHERE status = 'ACTIVE'
		  AND ST_DWithin(location, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)
		ORDER BY distance_meters ASC
		LIMIT $4 OFFSET $5
	`
	rows, err := r.db.QueryContext(ctx, query,
		params.Longitude,
		params.Latitude,
		params.RadiusMeters,
		params.Limit,
		params.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query nearby workshops with PostGIS: %w", err)
	}
	defer rows.Close()

	var list []domain.Workshop
	for rows.Next() {
		var w domain.Workshop
		var desc sql.NullString
		var dist float64
		if err := rows.Scan(
			&w.ID,
			&w.OwnerID,
			&w.Name,
			&desc,
			&w.Address,
			&w.Latitude,
			&w.Longitude,
			&dist,
			&w.Phone,
			&w.Rating,
			&w.ReviewCount,
			&w.Status,
			&w.CreatedAt,
			&w.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if desc.Valid {
			w.Description = desc.String
		}
		w.DistanceMeters = &dist
		list = append(list, w)
	}
	return list, rows.Err()
}

func (r *postgresRepository) Update(ctx context.Context, w *domain.Workshop) error {
	query := `
		UPDATE workshops
		SET 
			name = $1,
			description = $2,
			address = $3,
			latitude = $4,
			longitude = $5,
			phone = $6,
			status = $7,
			updated_at = $8
		WHERE id = $9
	`
	res, err := r.db.ExecContext(ctx, query,
		w.Name,
		w.Description,
		w.Address,
		w.Latitude,
		w.Longitude,
		w.Phone,
		w.Status,
		w.UpdatedAt,
		w.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update workshop: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrWorkshopNotFound
	}
	return nil
}

func (r *postgresRepository) GetOperatingHours(ctx context.Context, workshopID uuid.UUID) ([]domain.OperatingHour, error) {
	query := `
		SELECT id, workshop_id, day_of_week, open_time, close_time, is_closed, created_at, updated_at
		FROM operating_hours
		WHERE workshop_id = $1
		ORDER BY day_of_week ASC
	`
	rows, err := r.db.QueryContext(ctx, query, workshopID)
	if err != nil {
		return nil, fmt.Errorf("failed to query operating hours: %w", err)
	}
	defer rows.Close()

	var list []domain.OperatingHour
	for rows.Next() {
		var oh domain.OperatingHour
		if err := rows.Scan(
			&oh.ID,
			&oh.WorkshopID,
			&oh.DayOfWeek,
			&oh.OpenTime,
			&oh.CloseTime,
			&oh.IsClosed,
			&oh.CreatedAt,
			&oh.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, oh)
	}
	return list, rows.Err()
}

func (r *postgresRepository) UpsertOperatingHours(ctx context.Context, workshopID uuid.UUID, hours []domain.OperatingHour) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO operating_hours (id, workshop_id, day_of_week, open_time, close_time, is_closed, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (workshop_id, day_of_week)
		DO UPDATE SET
			open_time = EXCLUDED.open_time,
			close_time = EXCLUDED.close_time,
			is_closed = EXCLUDED.is_closed,
			updated_at = EXCLUDED.updated_at
	`
	now := time.Now().UTC()
	for _, h := range hours {
		id := h.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		if _, err := tx.ExecContext(ctx, query,
			id,
			workshopID,
			h.DayOfWeek,
			h.OpenTime,
			h.CloseTime,
			h.IsClosed,
			now,
			now,
		); err != nil {
			return fmt.Errorf("failed to upsert operating hour day %d: %w", h.DayOfWeek, err)
		}
	}

	return tx.Commit()
}
