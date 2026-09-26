package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bengkol/backend/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrServiceNotFound = errors.New("service not found")
)

// Repository defines data access for workshop services.
type Repository interface {
	Create(ctx context.Context, s *domain.Service) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Service, error)
	ListByWorkshopID(ctx context.Context, workshopID uuid.UUID, onlyActive bool) ([]domain.Service, error)
	Update(ctx context.Context, s *domain.Service) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetWorkshopOwnerID(ctx context.Context, workshopID uuid.UUID) (uuid.UUID, error)
	GetServiceOwnerID(ctx context.Context, serviceID uuid.UUID) (uuid.UUID, error)
}

type postgresRepository struct {
	db *sql.DB
}

// NewRepository creates a new PostgreSQL Service repository.
func NewRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Create(ctx context.Context, s *domain.Service) error {
	query := `
		INSERT INTO services (
			id, workshop_id, name, description, price, duration_minutes, is_active, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`
	_, err := r.db.ExecContext(ctx, query,
		s.ID,
		s.WorkshopID,
		s.Name,
		s.Description,
		s.Price,
		s.DurationMinutes,
		s.IsActive,
		s.CreatedAt,
		s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert service: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Service, error) {
	query := `
		SELECT id, workshop_id, name, description, price, duration_minutes, is_active, created_at, updated_at
		FROM services
		WHERE id = $1
	`
	var s domain.Service
	var desc sql.NullString
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&s.ID,
		&s.WorkshopID,
		&s.Name,
		&desc,
		&s.Price,
		&s.DurationMinutes,
		&s.IsActive,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrServiceNotFound
		}
		return nil, fmt.Errorf("failed to query service by id: %w", err)
	}
	if desc.Valid {
		s.Description = desc.String
	}
	return &s, nil
}

func (r *postgresRepository) ListByWorkshopID(ctx context.Context, workshopID uuid.UUID, onlyActive bool) ([]domain.Service, error) {
	query := `
		SELECT id, workshop_id, name, description, price, duration_minutes, is_active, created_at, updated_at
		FROM services
		WHERE workshop_id = $1
	`
	if onlyActive {
		query += " AND is_active = TRUE"
	}
	query += " ORDER BY name ASC"

	rows, err := r.db.QueryContext(ctx, query, workshopID)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	defer rows.Close()

	var list []domain.Service
	for rows.Next() {
		var s domain.Service
		var desc sql.NullString
		if err := rows.Scan(
			&s.ID,
			&s.WorkshopID,
			&s.Name,
			&desc,
			&s.Price,
			&s.DurationMinutes,
			&s.IsActive,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if desc.Valid {
			s.Description = desc.String
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func (r *postgresRepository) Update(ctx context.Context, s *domain.Service) error {
	query := `
		UPDATE services
		SET 
			name = $1,
			description = $2,
			price = $3,
			duration_minutes = $4,
			is_active = $5,
			updated_at = $6
		WHERE id = $7
	`
	res, err := r.db.ExecContext(ctx, query,
		s.Name,
		s.Description,
		s.Price,
		s.DurationMinutes,
		s.IsActive,
		s.UpdatedAt,
		s.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update service: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrServiceNotFound
	}
	return nil
}

func (r *postgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM services WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrServiceNotFound
	}
	return nil
}

func (r *postgresRepository) GetWorkshopOwnerID(ctx context.Context, workshopID uuid.UUID) (uuid.UUID, error) {
	query := `SELECT owner_id FROM workshops WHERE id = $1`
	var ownerID uuid.UUID
	err := r.db.QueryRowContext(ctx, query, workshopID).Scan(&ownerID)
	if err != nil {
		return uuid.Nil, err
	}
	return ownerID, nil
}

func (r *postgresRepository) GetServiceOwnerID(ctx context.Context, serviceID uuid.UUID) (uuid.UUID, error) {
	query := `
		SELECT w.owner_id
		FROM services s
		JOIN workshops w ON s.workshop_id = w.id
		WHERE s.id = $1
	`
	var ownerID uuid.UUID
	err := r.db.QueryRowContext(ctx, query, serviceID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, ErrServiceNotFound
		}
		return uuid.Nil, err
	}
	return ownerID, nil
}
