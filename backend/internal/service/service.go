package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/validator"
	"github.com/google/uuid"
)

var (
	ErrForbidden        = errors.New("you do not have permission to manage this service")
	ErrValidationFailed = errors.New("validation failed")
)

// CreateServiceRequest DTO
type CreateServiceRequest struct {
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Price           float64 `json:"price"`
	DurationMinutes int     `json:"duration_minutes"`
	IsActive        *bool   `json:"is_active"`
}

// UpdateServiceRequest DTO
type UpdateServiceRequest struct {
	Name            *string  `json:"name,omitempty"`
	Description     *string  `json:"description,omitempty"`
	Price           *float64 `json:"price,omitempty"`
	DurationMinutes *int     `json:"duration_minutes,omitempty"`
	IsActive        *bool    `json:"is_active,omitempty"`
}

// Service defines business logic for workshop services.
type Service interface {
	CreateService(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req CreateServiceRequest) (*domain.Service, map[string]string, error)
	GetServicesByWorkshop(ctx context.Context, workshopID uuid.UUID, requestingRole *domain.UserRole, requestingUserID *uuid.UUID) ([]domain.Service, error)
	GetServiceByID(ctx context.Context, id uuid.UUID) (*domain.Service, error)
	UpdateService(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req UpdateServiceRequest) (*domain.Service, map[string]string, error)
	DeleteService(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) error
}

type serviceUseCase struct {
	repo   Repository
	logger *logger.Logger
}

// NewService creates a new Service use case instance.
func NewService(repo Repository, log *logger.Logger) Service {
	return &serviceUseCase{
		repo:   repo,
		logger: log,
	}
}

func (s *serviceUseCase) CreateService(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req CreateServiceRequest) (*domain.Service, map[string]string, error) {
	// Ownership verification
	ownerID, err := s.repo.GetWorkshopOwnerID(ctx, workshopID)
	if err != nil {
		return nil, nil, err
	}
	if ownerID != requestingUserID && requestingRole != domain.RoleAdmin {
		return nil, nil, ErrForbidden
	}

	v := validator.New()
	req.Name = strings.TrimSpace(req.Name)
	v.Required("name", req.Name)

	if req.Price < 0 {
		v.AddError("price", "price cannot be negative")
	}
	if req.DurationMinutes <= 0 {
		v.AddError("duration_minutes", "duration must be at least 1 minute")
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	now := time.Now().UTC()
	srv := &domain.Service{
		ID:              uuid.New(),
		WorkshopID:      workshopID,
		Name:            req.Name,
		Description:     req.Description,
		Price:           req.Price,
		DurationMinutes: req.DurationMinutes,
		IsActive:        isActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.repo.Create(ctx, srv); err != nil {
		s.logger.WithContext(ctx).Error("failed to create service", "error", err)
		return nil, nil, err
	}

	s.logger.WithContext(ctx).Info("service created", "service_id", srv.ID, "workshop_id", workshopID)
	return srv, nil, nil
}

func (s *serviceUseCase) GetServicesByWorkshop(ctx context.Context, workshopID uuid.UUID, requestingRole *domain.UserRole, requestingUserID *uuid.UUID) ([]domain.Service, error) {
	onlyActive := true

	// If requesting user is workshop owner or admin, show all services (including inactive)
	if requestingRole != nil && requestingUserID != nil {
		if *requestingRole == domain.RoleAdmin {
			onlyActive = false
		} else if *requestingRole == domain.RoleOwner {
			ownerID, err := s.repo.GetWorkshopOwnerID(ctx, workshopID)
			if err == nil && ownerID == *requestingUserID {
				onlyActive = false
			}
		}
	}

	return s.repo.ListByWorkshopID(ctx, workshopID, onlyActive)
}

func (s *serviceUseCase) GetServiceByID(ctx context.Context, id uuid.UUID) (*domain.Service, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *serviceUseCase) UpdateService(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req UpdateServiceRequest) (*domain.Service, map[string]string, error) {
	ownerID, err := s.repo.GetServiceOwnerID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if ownerID != requestingUserID && requestingRole != domain.RoleAdmin {
		return nil, nil, ErrForbidden
	}

	srv, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	v := validator.New()

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		v.Required("name", trimmed)
		srv.Name = trimmed
	}
	if req.Description != nil {
		srv.Description = *req.Description
	}
	if req.Price != nil {
		if *req.Price < 0 {
			v.AddError("price", "price cannot be negative")
		}
		srv.Price = *req.Price
	}
	if req.DurationMinutes != nil {
		if *req.DurationMinutes <= 0 {
			v.AddError("duration_minutes", "duration must be at least 1 minute")
		}
		srv.DurationMinutes = *req.DurationMinutes
	}
	if req.IsActive != nil {
		srv.IsActive = *req.IsActive
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	srv.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, srv); err != nil {
		return nil, nil, err
	}

	return srv, nil, nil
}

func (s *serviceUseCase) DeleteService(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) error {
	ownerID, err := s.repo.GetServiceOwnerID(ctx, id)
	if err != nil {
		return err
	}
	if ownerID != requestingUserID && requestingRole != domain.RoleAdmin {
		return ErrForbidden
	}

	return s.repo.Delete(ctx, id)
}
