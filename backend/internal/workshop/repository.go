package workshop

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/security"
	"github.com/google/uuid"
)

var (
	ErrWorkshopNotFound = errors.New("workshop not found")
	ErrPhotoNotFound    = errors.New("photo not found")
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

// Repository defines data access operations for Workshops, Photos, and Operating Hours.
type Repository interface {
	Create(ctx context.Context, w *domain.Workshop) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Workshop, error)
	GetByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]domain.Workshop, error)
	List(ctx context.Context, pagination domain.PaginationParams, filter WorkshopFilter) ([]domain.Workshop, int64, error)
	FindNearby(ctx context.Context, params NearbyParams) ([]domain.Workshop, error)
	Update(ctx context.Context, w *domain.Workshop) error
	GetOperatingHours(ctx context.Context, workshopID uuid.UUID) ([]domain.OperatingHour, error)
	UpsertOperatingHours(ctx context.Context, workshopID uuid.UUID, hours []domain.OperatingHour) error
	SavePhoto(ctx context.Context, photo *domain.WorkshopPhoto) error
	GetPhotoByID(ctx context.Context, id uuid.UUID) (*domain.WorkshopPhoto, error)
	GetPhotosByWorkshopID(ctx context.Context, workshopID uuid.UUID) ([]domain.WorkshopPhoto, error)
	DeletePhotosByWorkshopID(ctx context.Context, workshopID uuid.UUID) error
	CreateEmployees(ctx context.Context, workshopID uuid.UUID, employees []domain.WorkshopEmployee) error
	GetEmployees(ctx context.Context, workshopID uuid.UUID) ([]domain.WorkshopEmployee, error)
}

type postgresRepository struct {
	db *sql.DB
}

// NewRepository creates a new PostgreSQL workshop repository.
func NewRepository(db *sql.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Create(ctx context.Context, w *domain.Workshop) error {
	if w.Photos == nil {
		w.Photos = []string{}
	}
	query := `
		INSERT INTO workshops (
			id, owner_id, name, description, address,
			latitude, longitude, phone, photos, rating, review_count, status,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10, $11, $12,
			$13, $14
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
		w.Photos,
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
			latitude, longitude, phone, photos, rating, review_count, status,
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
		&w.Photos,
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
	if w.Photos == nil {
		w.Photos = []string{}
	}
	return &w, nil
}

func (r *postgresRepository) GetByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]domain.Workshop, error) {
	query := `
		SELECT DISTINCT
			w.id, w.owner_id, w.name, w.description, w.address,
			w.latitude, w.longitude, w.phone, w.photos, w.rating, w.review_count, w.status,
			w.created_at, w.updated_at
		FROM workshops w
		LEFT JOIN workshop_employees we ON we.workshop_id = w.id AND we.user_id = $1 AND we.status = 'ACTIVE'
		WHERE w.owner_id = $1 OR we.id IS NOT NULL
		ORDER BY w.created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query workshops by owner/employee: %w", err)
	}
	defer rows.Close()

	list := make([]domain.Workshop, 0)
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
			&w.Photos,
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
		if w.Photos == nil {
			w.Photos = []string{}
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
			latitude, longitude, phone, photos, rating, review_count, status,
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
			&w.Photos,
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
		if w.Photos == nil {
			w.Photos = []string{}
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
			phone, photos, rating, review_count, status,
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
			&w.Photos,
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
		if w.Photos == nil {
			w.Photos = []string{}
		}
		w.DistanceMeters = &dist
		list = append(list, w)
	}
	return list, rows.Err()
}

func (r *postgresRepository) Update(ctx context.Context, w *domain.Workshop) error {
	if w.Photos == nil {
		w.Photos = []string{}
	}
	query := `
		UPDATE workshops
		SET 
			name = $1,
			description = $2,
			address = $3,
			latitude = $4,
			longitude = $5,
			phone = $6,
			photos = $7,
			status = $8,
			updated_at = $9
		WHERE id = $10
	`
	res, err := r.db.ExecContext(ctx, query,
		w.Name,
		w.Description,
		w.Address,
		w.Latitude,
		w.Longitude,
		w.Phone,
		w.Photos,
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

func (r *postgresRepository) SavePhoto(ctx context.Context, p *domain.WorkshopPhoto) error {
	query := `
		INSERT INTO workshop_photos (
			id, workshop_id, data, content_type, filename, byte_size, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
	`
	_, err := r.db.ExecContext(ctx, query,
		p.ID,
		p.WorkshopID,
		p.Data,
		p.ContentType,
		p.Filename,
		p.ByteSize,
		p.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save workshop photo: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetPhotoByID(ctx context.Context, id uuid.UUID) (*domain.WorkshopPhoto, error) {
	query := `
		SELECT id, workshop_id, data, content_type, filename, byte_size, created_at
		FROM workshop_photos
		WHERE id = $1
	`
	var p domain.WorkshopPhoto
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID,
		&p.WorkshopID,
		&p.Data,
		&p.ContentType,
		&p.Filename,
		&p.ByteSize,
		&p.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPhotoNotFound
		}
		return nil, fmt.Errorf("failed to get workshop photo: %w", err)
	}
	return &p, nil
}

func (r *postgresRepository) GetPhotosByWorkshopID(ctx context.Context, workshopID uuid.UUID) ([]domain.WorkshopPhoto, error) {
	query := `
		SELECT id, workshop_id, data, content_type, filename, byte_size, created_at
		FROM workshop_photos
		WHERE workshop_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, workshopID)
	if err != nil {
		return nil, fmt.Errorf("failed to query workshop photos: %w", err)
	}
	defer rows.Close()

	var photos []domain.WorkshopPhoto
	for rows.Next() {
		var p domain.WorkshopPhoto
		if err := rows.Scan(
			&p.ID,
			&p.WorkshopID,
			&p.Data,
			&p.ContentType,
			&p.Filename,
			&p.ByteSize,
			&p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan workshop photo: %w", err)
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}

func (r *postgresRepository) DeletePhotosByWorkshopID(ctx context.Context, workshopID uuid.UUID) error {
	query := `DELETE FROM workshop_photos WHERE workshop_id = $1`
	_, err := r.db.ExecContext(ctx, query, workshopID)
	return err
}

func (r *postgresRepository) CreateEmployees(ctx context.Context, workshopID uuid.UUID, employees []domain.WorkshopEmployee) error {
	if len(employees) == 0 {
		return nil
	}
	query := `
		INSERT INTO workshop_employees (
			id, workshop_id, user_id, name, email, phone, role, status, specialization, notes, permissions, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
	`
	for _, emp := range employees {
		id := emp.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		status := emp.Status
		if status == "" {
			status = domain.EmployeeStatusActive
		}
		createdAt := emp.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		updatedAt := emp.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = createdAt
		}

		userID := emp.UserID
		if userID == nil {
			var existingID uuid.UUID
			err := r.db.QueryRowContext(ctx, "SELECT id FROM users WHERE phone = $1 LIMIT 1", emp.Phone).Scan(&existingID)
			if err == nil {
				userID = &existingID
			} else if emp.Email != "" {
				err = r.db.QueryRowContext(ctx, "SELECT id FROM users WHERE email IS NOT NULL AND LOWER(email) = LOWER($1) LIMIT 1", emp.Email).Scan(&existingID)
				if err == nil {
					userID = &existingID
				}
			}
			if userID == nil {
				newID := uuid.New()
				pass := emp.InitialPassword
				if pass == "" {
					pass, _ = security.GenerateRandomPassword(10)
				}
				passHash, _ := security.HashPassword(pass)
				var email *string
				if emp.Email != "" {
					email = &emp.Email
				}
				_, _ = r.db.ExecContext(ctx, `
					INSERT INTO users (id, email, password_hash, name, phone, role, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				`, newID, email, passHash, emp.Name, emp.Phone, string(domain.RoleCustomer), createdAt, updatedAt)
				userID = &newID
			}
		}

		var permsJSON []byte
		if emp.Permissions != nil {
			permsJSON, _ = json.Marshal(emp.Permissions)
		}

		_, err := r.db.ExecContext(
			ctx,
			query,
			id,
			workshopID,
			userID,
			emp.Name,
			emp.Email,
			emp.Phone,
			string(emp.Role),
			string(status),
			emp.Specialization,
			emp.Notes,
			permsJSON,
			createdAt,
			updatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert workshop employee: %w", err)
		}
	}
	return nil
}

func (r *postgresRepository) GetEmployees(ctx context.Context, workshopID uuid.UUID) ([]domain.WorkshopEmployee, error) {
	query := `
		SELECT id, workshop_id, user_id, name, email, phone, role, status, specialization, notes, permissions, created_at, updated_at
		FROM workshop_employees
		WHERE workshop_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, workshopID)
	if err != nil {
		return nil, fmt.Errorf("failed to query workshop employees: %w", err)
	}
	defer rows.Close()

	var employees []domain.WorkshopEmployee
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
			return nil, fmt.Errorf("failed to scan workshop employee: %w", err)
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
	return employees, rows.Err()
}

