package sparepart

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
	ErrForbidden        = errors.New("you do not have permission to manage this spare part")
	ErrValidationFailed = errors.New("validation failed")
)

// CreateSparePartRequest DTO
type CreateSparePartRequest struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	PurchasePrice float64 `json:"purchase_price"`
	SellingPrice  float64 `json:"selling_price"`
	Stock         int     `json:"stock"`
	IsActive      *bool   `json:"is_active"`
}

// UpdateSparePartRequest DTO
type UpdateSparePartRequest struct {
	Name          *string  `json:"name,omitempty"`
	Description   *string  `json:"description,omitempty"`
	PurchasePrice *float64 `json:"purchase_price,omitempty"`
	SellingPrice  *float64 `json:"selling_price,omitempty"`
	Stock         *int     `json:"stock,omitempty"`
	IsActive      *bool    `json:"is_active,omitempty"`
}

// Service defines business logic for workshop spare parts inventory.
type Service interface {
	CreateSparePart(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req CreateSparePartRequest) (*domain.SparePart, map[string]string, error)
	GetSparePartsByWorkshop(ctx context.Context, workshopID uuid.UUID, requestingRole *domain.UserRole, requestingUserID *uuid.UUID) ([]domain.SparePart, error)
	GetSparePartByID(ctx context.Context, id uuid.UUID) (*domain.SparePart, error)
	UpdateSparePart(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req UpdateSparePartRequest) (*domain.SparePart, map[string]string, error)
	DeleteSparePart(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) error
}

type sparePartUseCase struct {
	repo   Repository
	logger *logger.Logger
}

// NewService creates a new SparePart Service instance.
func NewService(repo Repository, log *logger.Logger) Service {
	return &sparePartUseCase{
		repo:   repo,
		logger: log,
	}
}

func (s *sparePartUseCase) checkInventoryPermission(ctx context.Context, workshopID, userID uuid.UUID, role domain.UserRole) (bool, error) {
	if role == domain.RoleAdmin {
		return true, nil
	}
	ownerID, err := s.repo.GetWorkshopOwnerID(ctx, workshopID)
	if err != nil {
		return false, err
	}
	if ownerID == userID {
		return true, nil
	}
	emp, err := s.repo.GetEmployee(ctx, workshopID, userID)
	if err == nil && emp != nil && emp.CanManageInventory() {
		return true, nil
	}
	return false, nil
}

func (s *sparePartUseCase) CreateSparePart(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req CreateSparePartRequest) (*domain.SparePart, map[string]string, error) {
	allowed, err := s.checkInventoryPermission(ctx, workshopID, requestingUserID, requestingRole)
	if err != nil {
		return nil, nil, err
	}
	if !allowed {
		return nil, nil, ErrForbidden
	}

	v := validator.New()
	req.Name = strings.TrimSpace(req.Name)
	v.Required("name", req.Name)

	if req.PurchasePrice < 0 {
		v.AddError("purchase_price", "purchase price cannot be negative")
	}
	if req.SellingPrice < 0 {
		v.AddError("selling_price", "selling price cannot be negative")
	}
	if req.Stock < 0 {
		v.AddError("stock", "stock cannot be negative")
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	now := time.Now().UTC()
	sp := &domain.SparePart{
		ID:            uuid.New(),
		WorkshopID:    workshopID,
		Name:          req.Name,
		Description:   req.Description,
		PurchasePrice: req.PurchasePrice,
		SellingPrice:  req.SellingPrice,
		Stock:         req.Stock,
		IsActive:      isActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.repo.Create(ctx, sp); err != nil {
		s.logger.WithContext(ctx).Error("failed to create spare part", "error", err)
		return nil, nil, err
	}

	s.logger.WithContext(ctx).Info("spare part created", "spare_part_id", sp.ID, "workshop_id", workshopID)
	return sp, nil, nil
}

func (s *sparePartUseCase) GetSparePartsByWorkshop(ctx context.Context, workshopID uuid.UUID, requestingRole *domain.UserRole, requestingUserID *uuid.UUID) ([]domain.SparePart, error) {
	onlyActive := true

	if requestingRole != nil && requestingUserID != nil {
		allowed, _ := s.checkInventoryPermission(ctx, workshopID, *requestingUserID, *requestingRole)
		if allowed {
			onlyActive = false
		}
	}

	return s.repo.ListByWorkshopID(ctx, workshopID, onlyActive)
}

func (s *sparePartUseCase) GetSparePartByID(ctx context.Context, id uuid.UUID) (*domain.SparePart, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *sparePartUseCase) UpdateSparePart(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, req UpdateSparePartRequest) (*domain.SparePart, map[string]string, error) {
	sp, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	allowed, err := s.checkInventoryPermission(ctx, sp.WorkshopID, requestingUserID, requestingRole)
	if err != nil {
		return nil, nil, err
	}
	if !allowed {
		return nil, nil, ErrForbidden
	}

	v := validator.New()

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		v.Required("name", trimmed)
		sp.Name = trimmed
	}
	if req.Description != nil {
		sp.Description = *req.Description
	}
	if req.PurchasePrice != nil {
		if *req.PurchasePrice < 0 {
			v.AddError("purchase_price", "purchase price cannot be negative")
		}
		sp.PurchasePrice = *req.PurchasePrice
	}
	if req.SellingPrice != nil {
		if *req.SellingPrice < 0 {
			v.AddError("selling_price", "selling price cannot be negative")
		}
		sp.SellingPrice = *req.SellingPrice
	}
	if req.Stock != nil {
		if *req.Stock < 0 {
			v.AddError("stock", "stock cannot be negative")
		}
		sp.Stock = *req.Stock
	}
	if req.IsActive != nil {
		sp.IsActive = *req.IsActive
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	sp.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, sp); err != nil {
		return nil, nil, err
	}

	return sp, nil, nil
}

func (s *sparePartUseCase) DeleteSparePart(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) error {
	sp, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	allowed, err := s.checkInventoryPermission(ctx, sp.WorkshopID, requestingUserID, requestingRole)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}

	return s.repo.Delete(ctx, id)
}
