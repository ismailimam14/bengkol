package vehicle

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/validator"
	"github.com/google/uuid"
)

var (
	ErrForbidden           = errors.New("you do not have permission to access or manage this vehicle")
	ErrValidationFailed    = errors.New("validation failed")
	ErrInvalidVehicleType  = errors.New("invalid vehicle type; must be MOTORCYCLE, CAR, or OTHER")
)

// CreateVehicleRequest DTO
type CreateVehicleRequest struct {
	LicensePlate string             `json:"license_plate"`
	Brand        string             `json:"brand"`
	Model        string             `json:"model"`
	Year         int                `json:"year,omitempty"`
	VehicleType  domain.VehicleType `json:"vehicle_type,omitempty"`
	Color        string             `json:"color,omitempty"`
	Notes        string             `json:"notes,omitempty"`
}

// UpdateVehicleRequest DTO
type UpdateVehicleRequest struct {
	LicensePlate *string             `json:"license_plate,omitempty"`
	Brand        *string             `json:"brand,omitempty"`
	Model        *string             `json:"model,omitempty"`
	Year         *int                `json:"year,omitempty"`
	VehicleType  *domain.VehicleType `json:"vehicle_type,omitempty"`
	Color        *string             `json:"color,omitempty"`
	Notes        *string             `json:"notes,omitempty"`
}

// Service defines vehicle business use cases.
type Service interface {
	CreateVehicle(ctx context.Context, userID uuid.UUID, req CreateVehicleRequest) (*domain.Vehicle, map[string]string, error)
	GetVehicleByID(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Vehicle, error)
	ListMyVehicles(ctx context.Context, userID uuid.UUID, pagination domain.PaginationParams) ([]domain.Vehicle, *response.Meta, error)
	UpdateVehicle(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req UpdateVehicleRequest) (*domain.Vehicle, map[string]string, error)
	DeleteVehicle(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) error
}

type vehicleService struct {
	repo   Repository
	logger *logger.Logger
}

// NewService creates a new Vehicle Service instance.
func NewService(repo Repository, log *logger.Logger) Service {
	return &vehicleService{
		repo:   repo,
		logger: log,
	}
}

func isValidVehicleType(vt domain.VehicleType) bool {
	switch vt {
	case domain.VehicleTypeCar, domain.VehicleTypeMotorcycle, domain.VehicleTypeOther:
		return true
	default:
		return false
	}
}

func (s *vehicleService) CreateVehicle(ctx context.Context, userID uuid.UUID, req CreateVehicleRequest) (*domain.Vehicle, map[string]string, error) {
	v := validator.New()

	plate := strings.ToUpper(strings.TrimSpace(req.LicensePlate))
	brand := strings.TrimSpace(req.Brand)
	model := strings.TrimSpace(req.Model)
	color := strings.TrimSpace(req.Color)
	notes := strings.TrimSpace(req.Notes)

	v.Required("license_plate", plate)
	v.Required("brand", brand)
	v.Required("model", model)

	if len(plate) > 20 {
		v.AddError("license_plate", "license plate cannot exceed 20 characters")
	}
	if len(brand) > 50 {
		v.AddError("brand", "brand cannot exceed 50 characters")
	}
	if len(model) > 50 {
		v.AddError("model", "model cannot exceed 50 characters")
	}

	currentYear := time.Now().Year()
	if req.Year != 0 {
		if req.Year < 1900 || req.Year > currentYear+1 {
			v.AddError("year", "year must be between 1900 and next year")
		}
	}

	vType := req.VehicleType
	if vType == "" {
		vType = domain.VehicleTypeMotorcycle
	} else if !isValidVehicleType(vType) {
		v.AddError("vehicle_type", "vehicle type must be MOTORCYCLE, CAR, or OTHER")
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	now := time.Now().UTC()
	veh := &domain.Vehicle{
		ID:           uuid.New(),
		UserID:       userID,
		LicensePlate: plate,
		Brand:        brand,
		Model:        model,
		Year:         req.Year,
		VehicleType:  vType,
		Color:        color,
		Notes:        notes,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Create(ctx, veh); err != nil {
		s.logger.WithContext(ctx).Error("failed to create vehicle", "user_id", userID, "error", err)
		return nil, nil, err
	}

	s.logger.WithContext(ctx).Info("vehicle created successfully", "vehicle_id", veh.ID, "user_id", userID, "license_plate", veh.LicensePlate)
	return veh, nil, nil
}

func (s *vehicleService) GetVehicleByID(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Vehicle, error) {
	veh, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if requestingRole != domain.RoleAdmin && veh.UserID != requestingUserID {
		return nil, ErrForbidden
	}

	return veh, nil
}

func (s *vehicleService) ListMyVehicles(ctx context.Context, userID uuid.UUID, pagination domain.PaginationParams) ([]domain.Vehicle, *response.Meta, error) {
	pagination.EnsureValid()

	list, total, err := s.repo.ListByUserID(ctx, userID, pagination)
	if err != nil {
		s.logger.WithContext(ctx).Error("failed to list user vehicles", "user_id", userID, "error", err)
		return nil, nil, err
	}

	totalPages := int(math.Ceil(float64(total) / float64(pagination.PageSize)))
	if totalPages == 0 {
		totalPages = 1
	}

	meta := &response.Meta{
		Page:       pagination.Page,
		PageSize:   pagination.PageSize,
		TotalItems: total,
		TotalPages: totalPages,
	}

	return list, meta, nil
}

func (s *vehicleService) UpdateVehicle(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req UpdateVehicleRequest) (*domain.Vehicle, map[string]string, error) {
	veh, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	if requestingRole != domain.RoleAdmin && veh.UserID != requestingUserID {
		return nil, nil, ErrForbidden
	}

	v := validator.New()

	if req.LicensePlate != nil {
		plate := strings.ToUpper(strings.TrimSpace(*req.LicensePlate))
		v.Required("license_plate", plate)
		if len(plate) > 20 {
			v.AddError("license_plate", "license plate cannot exceed 20 characters")
		}
		veh.LicensePlate = plate
	}

	if req.Brand != nil {
		brand := strings.TrimSpace(*req.Brand)
		v.Required("brand", brand)
		if len(brand) > 50 {
			v.AddError("brand", "brand cannot exceed 50 characters")
		}
		veh.Brand = brand
	}

	if req.Model != nil {
		model := strings.TrimSpace(*req.Model)
		v.Required("model", model)
		if len(model) > 50 {
			v.AddError("model", "model cannot exceed 50 characters")
		}
		veh.Model = model
	}

	if req.Year != nil {
		currentYear := time.Now().Year()
		if *req.Year != 0 && (*req.Year < 1900 || *req.Year > currentYear+1) {
			v.AddError("year", "year must be between 1900 and next year")
		}
		veh.Year = *req.Year
	}

	if req.VehicleType != nil {
		if !isValidVehicleType(*req.VehicleType) {
			v.AddError("vehicle_type", "vehicle type must be MOTORCYCLE, CAR, or OTHER")
		} else {
			veh.VehicleType = *req.VehicleType
		}
	}

	if req.Color != nil {
		veh.Color = strings.TrimSpace(*req.Color)
	}

	if req.Notes != nil {
		veh.Notes = strings.TrimSpace(*req.Notes)
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	veh.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, veh); err != nil {
		s.logger.WithContext(ctx).Error("failed to update vehicle", "vehicle_id", id, "error", err)
		return nil, nil, err
	}

	s.logger.WithContext(ctx).Info("vehicle updated successfully", "vehicle_id", id, "user_id", requestingUserID)
	return veh, nil, nil
}

func (s *vehicleService) DeleteVehicle(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) error {
	veh, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if requestingRole != domain.RoleAdmin && veh.UserID != requestingUserID {
		return ErrForbidden
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		s.logger.WithContext(ctx).Error("failed to delete vehicle", "vehicle_id", id, "error", err)
		return err
	}

	s.logger.WithContext(ctx).Info("vehicle deleted successfully", "vehicle_id", id, "user_id", requestingUserID)
	return nil
}
