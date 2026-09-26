package notification

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/bengkol/backend/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrDeviceNotFound = errors.New("device token not found")
)

// Repository defines data access operations for user device tokens.
type Repository interface {
	UpsertDevice(ctx context.Context, device *domain.UserDevice) error
	DeleteDevice(ctx context.Context, userID uuid.UUID, token string) error
	GetDevicesByUserID(ctx context.Context, userID uuid.UUID) ([]domain.UserDevice, error)
	GetDevicesByUserIDs(ctx context.Context, userIDs []uuid.UUID) ([]domain.UserDevice, error)
}

type postgresRepository struct {
	db *sql.DB
}

// NewRepository creates a new PostgreSQL device notification repository.
func NewRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) UpsertDevice(ctx context.Context, device *domain.UserDevice) error {
	query := `
		INSERT INTO user_devices (id, user_id, token, platform, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (token) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			platform = EXCLUDED.platform,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRowContext(ctx, query,
		device.ID,
		device.UserID,
		device.Token,
		device.Platform,
		device.CreatedAt,
		device.UpdatedAt,
	).Scan(&device.ID, &device.CreatedAt, &device.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert user device token: %w", err)
	}

	return nil
}

func (r *postgresRepository) DeleteDevice(ctx context.Context, userID uuid.UUID, token string) error {
	query := `
		DELETE FROM user_devices
		WHERE user_id = $1 AND token = $2
	`
	res, err := r.db.ExecContext(ctx, query, userID, token)
	if err != nil {
		return fmt.Errorf("failed to delete user device token: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrDeviceNotFound
	}

	return nil
}

func (r *postgresRepository) GetDevicesByUserID(ctx context.Context, userID uuid.UUID) ([]domain.UserDevice, error) {
	query := `
		SELECT id, user_id, token, platform, created_at, updated_at
		FROM user_devices
		WHERE user_id = $1
		ORDER BY updated_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user devices: %w", err)
	}
	defer rows.Close()

	var devices []domain.UserDevice
	for rows.Next() {
		var d domain.UserDevice
		var platformStr string
		if err := rows.Scan(&d.ID, &d.UserID, &d.Token, &platformStr, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user device: %w", err)
		}
		d.Platform = domain.DevicePlatform(platformStr)
		devices = append(devices, d)
	}

	return devices, rows.Err()
}

func (r *postgresRepository) GetDevicesByUserIDs(ctx context.Context, userIDs []uuid.UUID) ([]domain.UserDevice, error) {
	if len(userIDs) == 0 {
		return []domain.UserDevice{}, nil
	}

	placeholders := make([]string, len(userIDs))
	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, user_id, token, platform, created_at, updated_at
		FROM user_devices
		WHERE user_id IN (%s)
		ORDER BY updated_at DESC
	`, strings.Join(placeholders, ", "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query devices by user ids: %w", err)
	}
	defer rows.Close()

	var devices []domain.UserDevice
	for rows.Next() {
		var d domain.UserDevice
		var platformStr string
		if err := rows.Scan(&d.ID, &d.UserID, &d.Token, &platformStr, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user device: %w", err)
		}
		d.Platform = domain.DevicePlatform(platformStr)
		devices = append(devices, d)
	}

	return devices, rows.Err()
}
