package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrUserNotFound         = errors.New("user not found")
	ErrUserAlreadyExists    = errors.New("user already exists")
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
)

// Repository defines data access operations for users and authentication tokens.
type Repository interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByPhone(ctx context.Context, phone string) (*domain.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	SaveRefreshToken(ctx context.Context, token *domain.RefreshToken) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error
	UpdatePassword(ctx context.Context, userID uuid.UUID, newPasswordHash string) error
	IsPhoneRegisteredAsEmployee(ctx context.Context, phone string) (bool, error)
	GetWorkshopsByUserID(ctx context.Context, userID uuid.UUID) ([]domain.Workshop, error)
	GetWorkshopEmployeeMembership(ctx context.Context, workshopID, userID uuid.UUID) (*domain.WorkshopEmployee, error)
	GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error)
}

type postgresRepository struct {
	db *sql.DB
}

// NewRepository creates a new PostgreSQL auth repository.
func NewRepository(db *sql.DB) Repository {
	return &postgresRepository{
		db: db,
	}
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
		return fmt.Errorf("failed to insert user: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	trimmed := strings.ToLower(strings.TrimSpace(email))
	if trimmed == "" {
		return nil, ErrUserNotFound
	}

	query := `
		SELECT id, COALESCE(email, ''), password_hash, name, phone, role, created_at, updated_at
		FROM users
		WHERE email IS NOT NULL AND LOWER(email) = LOWER($1)
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
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}
	return &u, nil
}

func (r *postgresRepository) GetUserByPhone(ctx context.Context, phone string) (*domain.User, error) {
	trimmed := strings.TrimSpace(phone)
	if trimmed == "" {
		return nil, ErrUserNotFound
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
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user by phone: %w", err)
	}
	return &u, nil
}

func (r *postgresRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `
		SELECT id, COALESCE(email, ''), password_hash, name, phone, role, created_at, updated_at
		FROM users
		WHERE id = $1
	`
	var u domain.User
	err := r.db.QueryRowContext(ctx, query, id).Scan(
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
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user by id: %w", err)
	}
	return &u, nil
}

func (r *postgresRepository) SaveRefreshToken(ctx context.Context, token *domain.RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (id, user_id, token_hash, revoked, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.ExecContext(ctx, query,
		token.ID,
		token.UserID,
		token.TokenHash,
		token.Revoked,
		token.ExpiresAt,
		token.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save refresh token: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetRefreshToken(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, revoked, expires_at, created_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`
	var t domain.RefreshToken
	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&t.ID,
		&t.UserID,
		&t.TokenHash,
		&t.Revoked,
		&t.ExpiresAt,
		&t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRefreshTokenNotFound
		}
		return nil, fmt.Errorf("failed to query refresh token: %w", err)
	}
	return &t, nil
}

func (r *postgresRepository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	query := `
		UPDATE refresh_tokens
		SET revoked = TRUE
		WHERE token_hash = $1
	`
	res, err := r.db.ExecContext(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to revoke refresh token: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrRefreshTokenNotFound
	}
	return nil
}

func (r *postgresRepository) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE refresh_tokens
		SET revoked = TRUE
		WHERE user_id = $1 AND revoked = FALSE
	`
	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke user refresh tokens: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, newPasswordHash string) error {
	query := `
		UPDATE users
		SET password_hash = $1, updated_at = $2
		WHERE id = $3
	`
	res, err := r.db.ExecContext(ctx, query, newPasswordHash, time.Now().UTC(), userID)
	if err != nil {
		return fmt.Errorf("failed to update user password: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
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
		return false, fmt.Errorf("failed to check if phone is employee: %w", err)
	}
	return exists, nil
}

func (r *postgresRepository) GetWorkshopsByUserID(ctx context.Context, userID uuid.UUID) ([]domain.Workshop, error) {
	query := `
		SELECT DISTINCT
			w.id, w.owner_id, w.name, COALESCE(w.description, ''), w.address,
			w.latitude, w.longitude, w.phone, w.photos, w.rating, w.review_count, w.status,
			w.created_at, w.updated_at
		FROM workshops w
		LEFT JOIN workshop_employees we ON we.workshop_id = w.id AND we.user_id = $1 AND we.status = 'ACTIVE'
		WHERE w.owner_id = $1 OR we.id IS NOT NULL
		ORDER BY w.created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query workshops by user: %w", err)
	}
	defer rows.Close()

	list := make([]domain.Workshop, 0)
	for rows.Next() {
		var w domain.Workshop
		if err := rows.Scan(
			&w.ID,
			&w.OwnerID,
			&w.Name,
			&w.Description,
			&w.Address,
			&w.Latitude,
			&w.Longitude,
			&w.Phone,
			&w.Photos,
			&w.Rating,
			&w.ReviewCount,
			&w.Status,
			&w.CreatedAt,
			&w.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if w.Photos == nil {
			w.Photos = []string{}
		}
		list = append(list, w)
	}
	return list, rows.Err()
}

func (r *postgresRepository) GetWorkshopEmployeeMembership(ctx context.Context, workshopID, userID uuid.UUID) (*domain.WorkshopEmployee, error) {
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
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query workshop employee membership: %w", err)
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

func (r *postgresRepository) GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error) {
	query := `
		SELECT id, owner_id, name, COALESCE(description, ''), address,
			latitude, longitude, phone, photos, rating, review_count, status,
			created_at, updated_at
		FROM workshops
		WHERE id = $1
	`
	var w domain.Workshop
	err := r.db.QueryRowContext(ctx, query, workshopID).Scan(
		&w.ID,
		&w.OwnerID,
		&w.Name,
		&w.Description,
		&w.Address,
		&w.Latitude,
		&w.Longitude,
		&w.Phone,
		&w.Photos,
		&w.Rating,
		&w.ReviewCount,
		&w.Status,
		&w.CreatedAt,
		&w.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query workshop: %w", err)
	}
	if w.Photos == nil {
		w.Photos = []string{}
	}
	return &w, nil
}
