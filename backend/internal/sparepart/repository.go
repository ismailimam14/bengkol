package sparepart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bengkol/backend/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrSparePartNotFound = errors.New("spare part not found")
)

// Repository defines data access for workshop spare parts inventory.
type Repository interface {
	Create(ctx context.Context, sp *domain.SparePart) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.SparePart, error)
	ListByWorkshopID(ctx context.Context, workshopID uuid.UUID, onlyActive bool) ([]domain.SparePart, error)
	Update(ctx context.Context, sp *domain.SparePart) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetWorkshopOwnerID(ctx context.Context, workshopID uuid.UUID) (uuid.UUID, error)
	GetSparePartOwnerID(ctx context.Context, sparePartID uuid.UUID) (uuid.UUID, error)
	GetEmployee(ctx context.Context, workshopID, userID uuid.UUID) (*domain.WorkshopEmployee, error)
}

type postgresRepository struct {
	db *sql.DB
}

// NewRepository creates a new PostgreSQL SparePart repository.
func NewRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Create(ctx context.Context, sp *domain.SparePart) error {
	query := `
		INSERT INTO spare_parts (
			id, workshop_id, name, description, purchase_price, selling_price, stock, is_active, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`
	_, err := r.db.ExecContext(ctx, query,
		sp.ID,
		sp.WorkshopID,
		sp.Name,
		sp.Description,
		sp.PurchasePrice,
		sp.SellingPrice,
		sp.Stock,
		sp.IsActive,
		sp.CreatedAt,
		sp.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert spare part: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.SparePart, error) {
	query := `
		SELECT id, workshop_id, name, description, purchase_price, selling_price, stock, is_active, created_at, updated_at
		FROM spare_parts
		WHERE id = $1
	`
	var sp domain.SparePart
	var desc sql.NullString
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&sp.ID,
		&sp.WorkshopID,
		&sp.Name,
		&desc,
		&sp.PurchasePrice,
		&sp.SellingPrice,
		&sp.Stock,
		&sp.IsActive,
		&sp.CreatedAt,
		&sp.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSparePartNotFound
		}
		return nil, fmt.Errorf("failed to query spare part by id: %w", err)
	}
	if desc.Valid {
		sp.Description = desc.String
	}
	return &sp, nil
}

func (r *postgresRepository) ListByWorkshopID(ctx context.Context, workshopID uuid.UUID, onlyActive bool) ([]domain.SparePart, error) {
	query := `
		SELECT id, workshop_id, name, description, purchase_price, selling_price, stock, is_active, created_at, updated_at
		FROM spare_parts
		WHERE workshop_id = $1
	`
	if onlyActive {
		query += " AND is_active = TRUE AND stock > 0"
	}
	query += " ORDER BY name ASC"

	rows, err := r.db.QueryContext(ctx, query, workshopID)
	if err != nil {
		return nil, fmt.Errorf("failed to list spare parts: %w", err)
	}
	defer rows.Close()

	var list []domain.SparePart
	for rows.Next() {
		var sp domain.SparePart
		var desc sql.NullString
		if err := rows.Scan(
			&sp.ID,
			&sp.WorkshopID,
			&sp.Name,
			&desc,
			&sp.PurchasePrice,
			&sp.SellingPrice,
			&sp.Stock,
			&sp.IsActive,
			&sp.CreatedAt,
			&sp.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if desc.Valid {
			sp.Description = desc.String
		}
		list = append(list, sp)
	}
	return list, rows.Err()
}

func (r *postgresRepository) Update(ctx context.Context, sp *domain.SparePart) error {
	query := `
		UPDATE spare_parts
		SET 
			name = $1,
			description = $2,
			purchase_price = $3,
			selling_price = $4,
			stock = $5,
			is_active = $6,
			updated_at = $7
		WHERE id = $8
	`
	res, err := r.db.ExecContext(ctx, query,
		sp.Name,
		sp.Description,
		sp.PurchasePrice,
		sp.SellingPrice,
		sp.Stock,
		sp.IsActive,
		sp.UpdatedAt,
		sp.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update spare part: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrSparePartNotFound
	}
	return nil
}

func (r *postgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM spare_parts WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete spare part: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrSparePartNotFound
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

func (r *postgresRepository) GetSparePartOwnerID(ctx context.Context, sparePartID uuid.UUID) (uuid.UUID, error) {
	query := `
		SELECT w.owner_id
		FROM spare_parts sp
		JOIN workshops w ON sp.workshop_id = w.id
		WHERE sp.id = $1
	`
	var ownerID uuid.UUID
	err := r.db.QueryRowContext(ctx, query, sparePartID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, ErrSparePartNotFound
		}
		return uuid.Nil, err
	}
	return ownerID, nil
}

func (r *postgresRepository) GetEmployee(ctx context.Context, workshopID, userID uuid.UUID) (*domain.WorkshopEmployee, error) {
	query := `
		SELECT id, workshop_id, user_id, name, email, phone, role, status, specialization, notes, created_at, updated_at
		FROM workshop_employees
		WHERE workshop_id = $1 AND user_id = $2
		LIMIT 1
	`
	var emp domain.WorkshopEmployee
	var email, specialization, notes sql.NullString
	err := r.db.QueryRowContext(ctx, query, workshopID, userID).Scan(
		&emp.ID,
		&emp.WorkshopID,
		&emp.UserID,
		&emp.Name,
		&email,
		&emp.Phone,
		&emp.Role,
		&emp.Status,
		&specialization,
		&notes,
		&emp.CreatedAt,
		&emp.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query workshop employee: %w", err)
	}
	if email.Valid {
		emp.Email = email.String
	}
	if specialization.Valid {
		emp.Specialization = specialization.String
	}
	if notes.Valid {
		emp.Notes = notes.String
	}
	perms := emp.CalculatePermissions()
	emp.Permissions = &perms
	return &emp, nil
}

