package employee

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bengkol/backend/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrEmployeeNotFound = errors.New("employee not found")
	ErrWorkshopNotFound = errors.New("workshop not found")
)

// EmployeeFilter holds criteria for searching employees.
type EmployeeFilter struct {
	Role   *domain.EmployeeRole
	Status *domain.EmployeeStatus
	Search string
}

// Repository defines data access operations for workshop employees.
type Repository interface {
	Create(ctx context.Context, emp *domain.WorkshopEmployee) error
	GetByID(ctx context.Context, workshopID, employeeID uuid.UUID) (*domain.WorkshopEmployee, error)
	GetByUserID(ctx context.Context, workshopID, userID uuid.UUID) (*domain.WorkshopEmployee, error)
	List(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams, filter EmployeeFilter) ([]domain.WorkshopEmployee, int64, error)
	Update(ctx context.Context, emp *domain.WorkshopEmployee) error
	Delete(ctx context.Context, workshopID, employeeID uuid.UUID) error
	GetWorkshopOwnerID(ctx context.Context, workshopID uuid.UUID) (uuid.UUID, error)
	FindUserByPhone(ctx context.Context, phone string) (*domain.User, error)
	FindUserByEmail(ctx context.Context, email string) (*domain.User, error)
	CreateUser(ctx context.Context, user *domain.User) error
	IsPhoneRegisteredAsOwner(ctx context.Context, phone string) (bool, error)
	IsPhoneRegisteredAsEmployee(ctx context.Context, phone string) (bool, error)
}

type postgresRepository struct {
	db *sql.DB
}

// NewRepository creates a new PostgreSQL employee repository.
func NewRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) GetWorkshopOwnerID(ctx context.Context, workshopID uuid.UUID) (uuid.UUID, error) {
	query := `SELECT owner_id FROM workshops WHERE id = $1`
	var ownerID uuid.UUID
	err := r.db.QueryRowContext(ctx, query, workshopID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, ErrWorkshopNotFound
		}
		return uuid.Nil, fmt.Errorf("failed to get workshop owner: %w", err)
	}
	return ownerID, nil
}

func (r *postgresRepository) Create(ctx context.Context, emp *domain.WorkshopEmployee) error {
	var permsJSON []byte
	if emp.Permissions != nil {
		var err error
		permsJSON, err = json.Marshal(emp.Permissions)
		if err != nil {
			return fmt.Errorf("failed to marshal employee permissions: %w", err)
		}
	}

	query := `
		INSERT INTO workshop_employees (
			id, workshop_id, user_id, name, email, phone, role, status, specialization, notes, permissions, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
	`
	_, err := r.db.ExecContext(
		ctx,
		query,
		emp.ID,
		emp.WorkshopID,
		emp.UserID,
		emp.Name,
		emp.Email,
		emp.Phone,
		string(emp.Role),
		string(emp.Status),
		emp.Specialization,
		emp.Notes,
		permsJSON,
		emp.CreatedAt,
		emp.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert workshop employee: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetByID(ctx context.Context, workshopID, employeeID uuid.UUID) (*domain.WorkshopEmployee, error) {
	query := `
		SELECT id, workshop_id, user_id, name, email, phone, role, status, specialization, notes, permissions, created_at, updated_at
		FROM workshop_employees
		WHERE workshop_id = $1 AND id = $2
	`
	var emp domain.WorkshopEmployee
	var email, specialization, notes sql.NullString
	var permsJSON []byte
	err := r.db.QueryRowContext(ctx, query, workshopID, employeeID).Scan(
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
		&permsJSON,
		&emp.CreatedAt,
		&emp.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrEmployeeNotFound
		}
		return nil, fmt.Errorf("failed to get workshop employee: %w", err)
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
	if len(permsJSON) > 0 {
		var customPerms domain.EmployeePermissions
		if err := json.Unmarshal(permsJSON, &customPerms); err == nil {
			emp.Permissions = &customPerms
		}
	}
	perms := emp.CalculatePermissions()
	emp.Permissions = &perms
	return &emp, nil
}

func (r *postgresRepository) GetByUserID(ctx context.Context, workshopID, userID uuid.UUID) (*domain.WorkshopEmployee, error) {
	query := `
		SELECT id, workshop_id, user_id, name, email, phone, role, status, specialization, notes, permissions, created_at, updated_at
		FROM workshop_employees
		WHERE workshop_id = $1 AND user_id = $2
		LIMIT 1
	`
	var emp domain.WorkshopEmployee
	var email, specialization, notes sql.NullString
	var permsJSON []byte
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
		&permsJSON,
		&emp.CreatedAt,
		&emp.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrEmployeeNotFound
		}
		return nil, fmt.Errorf("failed to get workshop employee by user id: %w", err)
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
	if len(permsJSON) > 0 {
		var customPerms domain.EmployeePermissions
		if err := json.Unmarshal(permsJSON, &customPerms); err == nil {
			emp.Permissions = &customPerms
		}
	}
	perms := emp.CalculatePermissions()
	emp.Permissions = &perms
	return &emp, nil
}

func (r *postgresRepository) List(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams, filter EmployeeFilter) ([]domain.WorkshopEmployee, int64, error) {
	whereClauses := []string{"workshop_id = $1"}
	args := []interface{}{workshopID}
	argIdx := 2

	if filter.Role != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("role = $%d", argIdx))
		args = append(args, string(*filter.Role))
		argIdx++
	}

	if filter.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(*filter.Status))
		argIdx++
	}

	if strings.TrimSpace(filter.Search) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR email ILIKE $%d OR phone ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+strings.TrimSpace(filter.Search)+"%")
		argIdx++
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM workshop_employees WHERE %s", whereSQL)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count workshop employees: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, workshop_id, user_id, name, email, phone, role, status, specialization, notes, permissions, created_at, updated_at
		FROM workshop_employees
		WHERE %s
		ORDER BY created_at ASC
		LIMIT $%d OFFSET $%d
	`, whereSQL, argIdx, argIdx+1)

	args = append(args, pagination.PageSize, pagination.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list workshop employees: %w", err)
	}
	defer rows.Close()

	employees := []domain.WorkshopEmployee{}
	for rows.Next() {
		var emp domain.WorkshopEmployee
		var email, specialization, notes sql.NullString
		var permsJSON []byte
		if err := rows.Scan(
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
			&permsJSON,
			&emp.CreatedAt,
			&emp.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan workshop employee: %w", err)
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
		if len(permsJSON) > 0 {
			var customPerms domain.EmployeePermissions
			if err := json.Unmarshal(permsJSON, &customPerms); err == nil {
				emp.Permissions = &customPerms
			}
		}
		perms := emp.CalculatePermissions()
		emp.Permissions = &perms
		employees = append(employees, emp)
	}

	return employees, total, rows.Err()
}

func (r *postgresRepository) Update(ctx context.Context, emp *domain.WorkshopEmployee) error {
	var permsJSON []byte
	if emp.Permissions != nil {
		var err error
		permsJSON, err = json.Marshal(emp.Permissions)
		if err != nil {
			return fmt.Errorf("failed to marshal employee permissions: %w", err)
		}
	}

	query := `
		UPDATE workshop_employees
		SET user_id = $1, name = $2, email = $3, phone = $4, role = $5, status = $6, specialization = $7, notes = $8, permissions = $9, updated_at = $10
		WHERE workshop_id = $11 AND id = $12
	`
	res, err := r.db.ExecContext(
		ctx,
		query,
		emp.UserID,
		emp.Name,
		emp.Email,
		emp.Phone,
		string(emp.Role),
		string(emp.Status),
		emp.Specialization,
		emp.Notes,
		permsJSON,
		emp.UpdatedAt,
		emp.WorkshopID,
		emp.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update workshop employee: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrEmployeeNotFound
	}
	return nil
}

func (r *postgresRepository) Delete(ctx context.Context, workshopID, employeeID uuid.UUID) error {
	query := `DELETE FROM workshop_employees WHERE workshop_id = $1 AND id = $2`
	res, err := r.db.ExecContext(ctx, query, workshopID, employeeID)
	if err != nil {
		return fmt.Errorf("failed to delete workshop employee: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrEmployeeNotFound
	}
	return nil
}

func (r *postgresRepository) FindUserByPhone(ctx context.Context, phone string) (*domain.User, error) {
	trimmed := strings.TrimSpace(phone)
	if trimmed == "" {
		return nil, nil
	}
	norm := domain.NormalizePhone(trimmed)
	query := `
		SELECT id, COALESCE(email, ''), password_hash, name, phone, role, created_at, updated_at
		FROM users
		WHERE phone = $1 OR phone = $2
		LIMIT 1
	`
	var u domain.User
	err := r.db.QueryRowContext(ctx, query, trimmed, norm).Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.Name,
		&u.Phone,
		&u.Role,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query user by phone: %w", err)
	}
	return &u, nil
}

func (r *postgresRepository) FindUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	trimmed := strings.ToLower(strings.TrimSpace(email))
	if trimmed == "" {
		return nil, nil
	}
	query := `
		SELECT id, COALESCE(email, ''), password_hash, name, phone, role, created_at, updated_at
		FROM users
		WHERE email IS NOT NULL AND LOWER(email) = LOWER($1)
		LIMIT 1
	`
	var u domain.User
	err := r.db.QueryRowContext(ctx, query, trimmed).Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.Name,
		&u.Phone,
		&u.Role,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}
	return &u, nil
}

func (r *postgresRepository) CreateUser(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (id, email, password_hash, name, phone, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	var email *string
	if strings.TrimSpace(user.Email) != "" {
		trimmed := strings.ToLower(strings.TrimSpace(user.Email))
		email = &trimmed
	}

	_, err := r.db.ExecContext(ctx, query,
		user.ID,
		email,
		user.PasswordHash,
		user.Name,
		user.Phone,
		user.Role,
		user.CreatedAt,
		user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert user for employee: %w", err)
	}
	return nil
}

func (r *postgresRepository) IsPhoneRegisteredAsOwner(ctx context.Context, phone string) (bool, error) {
	trimmed := strings.TrimSpace(phone)
	if trimmed == "" {
		return false, nil
	}
	norm := domain.NormalizePhone(trimmed)

	query := `
		SELECT EXISTS (
			SELECT 1 FROM users u
			WHERE (u.role = 'OWNER' OR EXISTS (SELECT 1 FROM workshops w WHERE w.owner_id = u.id))
			  AND (u.phone = $1 OR u.phone = $2)
		)
	`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, trimmed, norm).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if phone is registered as owner: %w", err)
	}
	return exists, nil
}

func (r *postgresRepository) IsPhoneRegisteredAsEmployee(ctx context.Context, phone string) (bool, error) {
	trimmed := strings.TrimSpace(phone)
	if trimmed == "" {
		return false, nil
	}
	norm := domain.NormalizePhone(trimmed)

	query := `
		SELECT EXISTS (
			SELECT 1 FROM workshop_employees
			WHERE phone = $1 OR phone = $2
		)
	`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, trimmed, norm).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if phone is registered as employee: %w", err)
	}
	return exists, nil
}
