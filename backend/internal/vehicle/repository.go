package vehicle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bengkol/backend/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrVehicleNotFound = errors.New("vehicle not found")
)

// Repository defines data access operations for vehicles.
type Repository interface {
	Create(ctx context.Context, v *domain.Vehicle) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Vehicle, error)
	ListByUserID(ctx context.Context, userID uuid.UUID, pagination domain.PaginationParams) ([]domain.Vehicle, int64, error)
	Update(ctx context.Context, v *domain.Vehicle) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type postgresRepository struct {
	db *sql.DB
}

// NewRepository creates a new PostgreSQL Vehicle repository.
func NewRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Create(ctx context.Context, v *domain.Vehicle) error {
	query := `
		INSERT INTO vehicles (
			id, user_id, license_plate, brand, model, year, vehicle_type, color, notes, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`
	var yearVal interface{}
	if v.Year > 0 {
		yearVal = v.Year
	}

	var colorVal interface{}
	if v.Color != "" {
		colorVal = v.Color
	}

	var notesVal interface{}
	if v.Notes != "" {
		notesVal = v.Notes
	}

	_, err := r.db.ExecContext(ctx, query,
		v.ID,
		v.UserID,
		v.LicensePlate,
		v.Brand,
		v.Model,
		yearVal,
		v.VehicleType,
		colorVal,
		notesVal,
		v.CreatedAt,
		v.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert vehicle: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Vehicle, error) {
	query := `
		SELECT 
			v.id, v.user_id, v.license_plate, v.brand, v.model, v.year, v.vehicle_type, v.color, v.notes, v.created_at, v.updated_at,
			u.id, u.name, COALESCE(u.email, ''), u.phone
		FROM vehicles v
		JOIN users u ON v.user_id = u.id
		WHERE v.id = $1
	`
	var v domain.Vehicle
	var u domain.User
	var yearNull sql.NullInt64
	var colorNull sql.NullString
	var notesNull sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&v.ID,
		&v.UserID,
		&v.LicensePlate,
		&v.Brand,
		&v.Model,
		&yearNull,
		&v.VehicleType,
		&colorNull,
		&notesNull,
		&v.CreatedAt,
		&v.UpdatedAt,
		&u.ID,
		&u.Name,
		&u.Email,
		&u.Phone,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrVehicleNotFound
		}
		return nil, fmt.Errorf("failed to query vehicle by id: %w", err)
	}

	if yearNull.Valid {
		v.Year = int(yearNull.Int64)
	}
	if colorNull.Valid {
		v.Color = colorNull.String
	}
	if notesNull.Valid {
		v.Notes = notesNull.String
	}
	v.User = &u

	return &v, nil
}

func (r *postgresRepository) ListByUserID(ctx context.Context, userID uuid.UUID, pagination domain.PaginationParams) ([]domain.Vehicle, int64, error) {
	pagination.EnsureValid()

	var total int64
	countQuery := `SELECT COUNT(*) FROM vehicles WHERE user_id = $1`
	if err := r.db.QueryRowContext(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count vehicles: %w", err)
	}

	query := `
		SELECT id, user_id, license_plate, brand, model, year, vehicle_type, color, notes, created_at, updated_at
		FROM vehicles
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, userID, pagination.PageSize, pagination.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list vehicles: %w", err)
	}
	defer rows.Close()

	var list []domain.Vehicle
	for rows.Next() {
		var v domain.Vehicle
		var yearNull sql.NullInt64
		var colorNull sql.NullString
		var notesNull sql.NullString

		if err := rows.Scan(
			&v.ID,
			&v.UserID,
			&v.LicensePlate,
			&v.Brand,
			&v.Model,
			&yearNull,
			&v.VehicleType,
			&colorNull,
			&notesNull,
			&v.CreatedAt,
			&v.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan vehicle row: %w", err)
		}

		if yearNull.Valid {
			v.Year = int(yearNull.Int64)
		}
		if colorNull.Valid {
			v.Color = colorNull.String
		}
		if notesNull.Valid {
			v.Notes = notesNull.String
		}

		list = append(list, v)
	}

	return list, total, rows.Err()
}

func (r *postgresRepository) Update(ctx context.Context, v *domain.Vehicle) error {
	query := `
		UPDATE vehicles
		SET license_plate = $1, brand = $2, model = $3, year = $4, vehicle_type = $5, color = $6, notes = $7, updated_at = $8
		WHERE id = $9
	`
	var yearVal interface{}
	if v.Year > 0 {
		yearVal = v.Year
	}

	var colorVal interface{}
	if v.Color != "" {
		colorVal = v.Color
	}

	var notesVal interface{}
	if v.Notes != "" {
		notesVal = v.Notes
	}

	res, err := r.db.ExecContext(ctx, query,
		v.LicensePlate,
		v.Brand,
		v.Model,
		yearVal,
		v.VehicleType,
		colorVal,
		notesVal,
		v.UpdatedAt,
		v.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update vehicle: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrVehicleNotFound
	}

	return nil
}

func (r *postgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM vehicles WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete vehicle: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrVehicleNotFound
	}

	return nil
}
