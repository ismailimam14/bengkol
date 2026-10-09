package workshop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/security"
	"github.com/bengkol/backend/pkg/validator"
	"github.com/google/uuid"
)

var (
	ErrForbidden         = errors.New("you do not have permission to modify this workshop")
	ErrValidationFailed  = errors.New("validation failed")
	ErrInvalidCoordinates = errors.New("invalid latitude or longitude coordinates")
)

// RawPhotoInput holds binary photo data uploaded by clients.
type RawPhotoInput struct {
	Data        []byte
	ContentType string
	Filename    string
}

// CreateEmployeeInput DTO for optionally adding employees when creating a workshop
type CreateEmployeeInput struct {
	UserID         *uuid.UUID          `json:"user_id,omitempty"`
	Name           string              `json:"name"`
	Email          string              `json:"email,omitempty"`
	Phone          string              `json:"phone"`
	Role           domain.EmployeeRole `json:"role"`
	Specialization string              `json:"specialization,omitempty"`
	Notes          string              `json:"notes,omitempty"`
}

// CreateWorkshopRequest DTO
type CreateWorkshopRequest struct {
	Name           string                `json:"name"`
	Description    string                `json:"description"`
	Address        string                `json:"address"`
	Latitude       float64               `json:"latitude"`
	Longitude      float64               `json:"longitude"`
	Phone          string                `json:"phone"`
	Photos         []string              `json:"photos"`
	RawPhotos      []RawPhotoInput       `json:"-"`
	OperatingHours []OperatingHourInput  `json:"operating_hours,omitempty"`
	Employees      []CreateEmployeeInput `json:"employees,omitempty"`
}

// UpdateWorkshopRequest DTO
type UpdateWorkshopRequest struct {
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	Address     *string                `json:"address,omitempty"`
	Latitude    *float64               `json:"latitude,omitempty"`
	Longitude   *float64               `json:"longitude,omitempty"`
	Phone       *string                `json:"phone,omitempty"`
	Photos      *[]string              `json:"photos,omitempty"`
	RawPhotos   []RawPhotoInput        `json:"-"`
	Status      *domain.WorkshopStatus `json:"status,omitempty"`
}

// OperatingHourInput DTO
type OperatingHourInput struct {
	DayOfWeek int    `json:"day_of_week"` // 0=Sunday, 1=Monday, ..., 6=Saturday
	OpenTime  string `json:"open_time"`   // "08:00"
	CloseTime string `json:"close_time"`  // "17:00"
	IsClosed  bool   `json:"is_closed"`
}

// Service defines workshop domain business logic.
type Service interface {
	CreateWorkshop(ctx context.Context, ownerID uuid.UUID, req CreateWorkshopRequest) (*domain.Workshop, map[string]string, error)
	GetWorkshopByID(ctx context.Context, id uuid.UUID) (*domain.Workshop, error)
	ListWorkshops(ctx context.Context, pagination domain.PaginationParams, filter WorkshopFilter) ([]domain.Workshop, *response.Meta, error)
	FindNearbyWorkshops(ctx context.Context, params NearbyParams) ([]domain.Workshop, error)
	GetMyWorkshops(ctx context.Context, ownerID uuid.UUID) ([]domain.Workshop, error)
	UpdateWorkshop(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req UpdateWorkshopRequest) (*domain.Workshop, map[string]string, error)
	GetOperatingHours(ctx context.Context, workshopID uuid.UUID) ([]domain.OperatingHour, error)
	UpdateOperatingHours(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, hours []OperatingHourInput) ([]domain.OperatingHour, map[string]string, error)
	GetPhoto(ctx context.Context, photoID uuid.UUID) (*domain.WorkshopPhoto, error)
}

type workshopService struct {
	repo   Repository
	logger *logger.Logger
}

// NewService creates a new Workshop Service instance.
func NewService(repo Repository, log *logger.Logger) Service {
	return &workshopService{
		repo:   repo,
		logger: log,
	}
}

func (s *workshopService) CreateWorkshop(ctx context.Context, ownerID uuid.UUID, req CreateWorkshopRequest) (*domain.Workshop, map[string]string, error) {
	v := validator.New()
	req.Name = strings.TrimSpace(req.Name)
	req.Address = strings.TrimSpace(req.Address)
	req.Phone = strings.TrimSpace(req.Phone)

	v.Required("name", req.Name)
	v.Required("address", req.Address)
	v.Required("phone", req.Phone)

	var validPhotos []string
	for _, p := range req.Photos {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			validPhotos = append(validPhotos, trimmed)
		}
	}
	totalPhotos := len(validPhotos) + len(req.RawPhotos)
	if totalPhotos < 3 {
		v.AddError("photos", "at least 3 photos are required")
	}

	if req.Latitude < -90 || req.Latitude > 90 {
		v.AddError("latitude", "latitude must be between -90 and 90 degrees")
	}
	if req.Longitude < -180 || req.Longitude > 180 {
		v.AddError("longitude", "longitude must be between -180 and 180 degrees")
	}

	var validEmployees []domain.WorkshopEmployee
	for i, emp := range req.Employees {
		empName := strings.TrimSpace(emp.Name)
		empPhone := strings.TrimSpace(emp.Phone)
		if empName == "" {
			v.AddError(fmt.Sprintf("employees[%d].name", i), "employee name is required")
		}
		if empPhone == "" {
			v.AddError(fmt.Sprintf("employees[%d].phone", i), "employee phone is required")
		}
		normRole, ok := domain.NormalizeEmployeeRole(string(emp.Role))
		if !ok {
			v.AddError(fmt.Sprintf("employees[%d].role", i), "invalid employee role; must be MECHANIC, ADMIN_CASHIER, ADMIN_INVENTORY, ADMIN_BOTH, MANAGER, or OWNER")
		}
		if v.IsValid() {
			var initialPassword string
			if emp.UserID == nil {
				genPass, err := security.GenerateRandomPassword(10)
				if err == nil {
					initialPassword = genPass
				}
			}
			we := domain.WorkshopEmployee{
				ID:              uuid.New(),
				UserID:          emp.UserID,
				Name:            empName,
				Email:           strings.TrimSpace(emp.Email),
				Phone:           empPhone,
				Role:            normRole,
				Status:          domain.EmployeeStatusActive,
				Specialization:  strings.TrimSpace(emp.Specialization),
				Notes:           strings.TrimSpace(emp.Notes),
				InitialPassword: initialPassword,
			}
			validEmployees = append(validEmployees, we)
		}
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	if validPhotos == nil {
		validPhotos = []string{}
	}

	now := time.Now().UTC()
	ws := &domain.Workshop{
		ID:          uuid.New(),
		OwnerID:     ownerID,
		Name:        req.Name,
		Description: req.Description,
		Address:     req.Address,
		Latitude:    req.Latitude,
		Longitude:   req.Longitude,
		Phone:       req.Phone,
		Photos:      validPhotos,
		Rating:      0.0,
		ReviewCount: 0,
		Status:      domain.WorkshopStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Create(ctx, ws); err != nil {
		s.logger.WithContext(ctx).Error("failed to create workshop", "error", err)
		return nil, nil, err
	}

	// If raw binary photos uploaded, store them directly in the database
	if len(req.RawPhotos) > 0 {
		for _, raw := range req.RawPhotos {
			photoID := uuid.New()
			p := &domain.WorkshopPhoto{
				ID:          photoID,
				WorkshopID:  ws.ID,
				Data:        raw.Data,
				ContentType: raw.ContentType,
				Filename:    raw.Filename,
				ByteSize:    len(raw.Data),
				CreatedAt:   now,
			}
			if err := s.repo.SavePhoto(ctx, p); err != nil {
				s.logger.WithContext(ctx).Error("failed to save workshop photo to database", "workshop_id", ws.ID, "error", err)
				continue
			}
			ws.Photos = append(ws.Photos, fmt.Sprintf("/api/v1/workshops/%s/photos/%s", ws.ID, photoID))
		}
		_ = s.repo.Update(ctx, ws)
	}

	// If initial operating hours provided, save them
	if len(req.OperatingHours) > 0 {
		hours := make([]domain.OperatingHour, len(req.OperatingHours))
		for i, oh := range req.OperatingHours {
			hours[i] = domain.OperatingHour{
				ID:         uuid.New(),
				WorkshopID: ws.ID,
				DayOfWeek:  oh.DayOfWeek,
				OpenTime:   oh.OpenTime,
				CloseTime:  oh.CloseTime,
				IsClosed:   oh.IsClosed,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
		}
		if err := s.repo.UpsertOperatingHours(ctx, ws.ID, hours); err != nil {
			s.logger.WithContext(ctx).Warn("failed to save initial operating hours", "workshop_id", ws.ID, "error", err)
		} else {
			ws.OperatingHours = hours
		}
	}

	// If initial employees provided, save them
	if len(validEmployees) > 0 {
		for i := range validEmployees {
			validEmployees[i].WorkshopID = ws.ID
			validEmployees[i].CreatedAt = now
			validEmployees[i].UpdatedAt = now
			perms := validEmployees[i].CalculatePermissions()
			validEmployees[i].Permissions = &perms
		}
		if err := s.repo.CreateEmployees(ctx, ws.ID, validEmployees); err != nil {
			s.logger.WithContext(ctx).Warn("failed to save initial workshop employees", "workshop_id", ws.ID, "error", err)
		} else {
			ws.Employees = validEmployees
		}
	}

	s.logger.WithContext(ctx).Info("workshop created successfully", "workshop_id", ws.ID, "owner_id", ownerID)
	return ws, nil, nil
}

func (s *workshopService) GetWorkshopByID(ctx context.Context, id uuid.UUID) (*domain.Workshop, error) {
	ws, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if ws.Photos == nil {
		ws.Photos = []string{}
	}

	hours, err := s.repo.GetOperatingHours(ctx, id)
	if err == nil {
		ws.OperatingHours = hours
	}

	employees, err := s.repo.GetEmployees(ctx, id)
	if err == nil && len(employees) > 0 {
		ws.Employees = employees
	}

	return ws, nil
}

func (s *workshopService) ListWorkshops(ctx context.Context, pagination domain.PaginationParams, filter WorkshopFilter) ([]domain.Workshop, *response.Meta, error) {
	pagination.EnsureValid()

	list, total, err := s.repo.List(ctx, pagination, filter)
	if err != nil {
		return nil, nil, err
	}

	for i := range list {
		if list[i].Photos == nil {
			list[i].Photos = []string{}
		}
	}

	totalPages := int(total) / pagination.PageSize
	if int(total)%pagination.PageSize != 0 {
		totalPages++
	}

	meta := &response.Meta{
		Page:       pagination.Page,
		PageSize:   pagination.PageSize,
		TotalItems: total,
		TotalPages: totalPages,
	}

	return list, meta, nil
}

func (s *workshopService) FindNearbyWorkshops(ctx context.Context, params NearbyParams) ([]domain.Workshop, error) {
	if params.Latitude < -90 || params.Latitude > 90 || params.Longitude < -180 || params.Longitude > 180 {
		return nil, ErrInvalidCoordinates
	}

	if params.RadiusMeters <= 0 {
		params.RadiusMeters = 5000 // Default 5 km
	}
	if params.RadiusMeters > 50000 {
		params.RadiusMeters = 50000 // Max 50 km
	}
	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Limit > 50 {
		params.Limit = 50
	}

	list, err := s.repo.FindNearby(ctx, params)
	if err != nil {
		return nil, err
	}

	for i := range list {
		if list[i].Photos == nil {
			list[i].Photos = []string{}
		}
	}

	return list, nil
}

func (s *workshopService) GetMyWorkshops(ctx context.Context, ownerID uuid.UUID) ([]domain.Workshop, error) {
	list, err := s.repo.GetByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, err
	}

	for i := range list {
		if list[i].Photos == nil {
			list[i].Photos = []string{}
		}
	}

	return list, nil
}

func (s *workshopService) UpdateWorkshop(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req UpdateWorkshopRequest) (*domain.Workshop, map[string]string, error) {
	ws, err := s.repo.GetByID(ctx, workshopID)
	if err != nil {
		return nil, nil, err
	}

	// Ownership check
	if ws.OwnerID != requestingUserID && requestingRole != domain.RoleAdmin {
		return nil, nil, ErrForbidden
	}

	v := validator.New()

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		v.Required("name", trimmed)
		ws.Name = trimmed
	}
	if req.Description != nil {
		ws.Description = *req.Description
	}
	if req.Address != nil {
		trimmed := strings.TrimSpace(*req.Address)
		v.Required("address", trimmed)
		ws.Address = trimmed
	}
	if req.Phone != nil {
		trimmed := strings.TrimSpace(*req.Phone)
		v.Required("phone", trimmed)
		ws.Phone = trimmed
	}
	if req.Latitude != nil {
		if *req.Latitude < -90 || *req.Latitude > 90 {
			v.AddError("latitude", "must be between -90 and 90")
		}
		ws.Latitude = *req.Latitude
	}
	if req.Longitude != nil {
		if *req.Longitude < -180 || *req.Longitude > 180 {
			v.AddError("longitude", "must be between -180 and 180")
		}
		ws.Longitude = *req.Longitude
	}
	if req.Photos != nil || len(req.RawPhotos) > 0 {
		var validPhotos []string
		if req.Photos != nil {
			for _, p := range *req.Photos {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					validPhotos = append(validPhotos, trimmed)
				}
			}
		}
		totalPhotos := len(validPhotos) + len(req.RawPhotos)
		if totalPhotos < 3 {
			v.AddError("photos", "at least 3 photos are required")
		} else {
			if len(req.RawPhotos) > 0 {
				now := time.Now().UTC()
				for _, raw := range req.RawPhotos {
					photoID := uuid.New()
					p := &domain.WorkshopPhoto{
						ID:          photoID,
						WorkshopID:  ws.ID,
						Data:        raw.Data,
						ContentType: raw.ContentType,
						Filename:    raw.Filename,
						ByteSize:    len(raw.Data),
						CreatedAt:   now,
					}
					if err := s.repo.SavePhoto(ctx, p); err != nil {
						s.logger.WithContext(ctx).Error("failed to save workshop photo to database", "workshop_id", ws.ID, "error", err)
						continue
					}
					validPhotos = append(validPhotos, fmt.Sprintf("/api/v1/workshops/%s/photos/%s", ws.ID, photoID))
				}
			}
			ws.Photos = validPhotos
		}
	}
	if req.Status != nil {
		ws.Status = *req.Status
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	if ws.Photos == nil {
		ws.Photos = []string{}
	}

	ws.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, ws); err != nil {
		return nil, nil, err
	}

	return ws, nil, nil
}

func (s *workshopService) GetPhoto(ctx context.Context, photoID uuid.UUID) (*domain.WorkshopPhoto, error) {
	return s.repo.GetPhotoByID(ctx, photoID)
}

func (s *workshopService) GetOperatingHours(ctx context.Context, workshopID uuid.UUID) ([]domain.OperatingHour, error) {
	return s.repo.GetOperatingHours(ctx, workshopID)
}

func (s *workshopService) UpdateOperatingHours(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, inputs []OperatingHourInput) ([]domain.OperatingHour, map[string]string, error) {
	ws, err := s.repo.GetByID(ctx, workshopID)
	if err != nil {
		return nil, nil, err
	}

	// Ownership check
	if ws.OwnerID != requestingUserID && requestingRole != domain.RoleAdmin {
		return nil, nil, ErrForbidden
	}

	v := validator.New()
	hours := make([]domain.OperatingHour, len(inputs))
	now := time.Now().UTC()

	for i, in := range inputs {
		if in.DayOfWeek < 0 || in.DayOfWeek > 6 {
			v.AddError("day_of_week", "must be between 0 (Sunday) and 6 (Saturday)")
		}
		if !in.IsClosed {
			if in.OpenTime == "" || in.CloseTime == "" {
				v.AddError("operating_hours", "open_time and close_time required when not closed")
			}
		}
		hours[i] = domain.OperatingHour{
			WorkshopID: workshopID,
			DayOfWeek:  in.DayOfWeek,
			OpenTime:   in.OpenTime,
			CloseTime:  in.CloseTime,
			IsClosed:   in.IsClosed,
			UpdatedAt:  now,
		}
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	if err := s.repo.UpsertOperatingHours(ctx, workshopID, hours); err != nil {
		return nil, nil, err
	}

	updatedHours, err := s.repo.GetOperatingHours(ctx, workshopID)
	if err != nil {
		return nil, nil, err
	}

	return updatedHours, nil, nil
}
