package employee

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/validator"
	"github.com/google/uuid"
)

var (
	ErrForbidden         = errors.New("you do not have permission to perform this action")
	ErrValidationFailed  = errors.New("validation failed")
	ErrCannotModifyOwner = errors.New("managers cannot modify workshop owners")
	ErrCannotDeleteSelf  = errors.New("cannot delete your own employee record")
)

// CreateEmployeeRequest contains the payload for creating an employee.
type CreateEmployeeRequest struct {
	UserID         *uuid.UUID `json:"user_id,omitempty"`
	Name           string     `json:"name"`
	Email          string     `json:"email,omitempty"`
	Phone          string     `json:"phone"`
	Role           string     `json:"role"`
	Status         string     `json:"status,omitempty"`
	Specialization string     `json:"specialization,omitempty"`
	Notes          string     `json:"notes,omitempty"`
}

// UpdateEmployeeRequest contains the payload for updating an employee.
type UpdateEmployeeRequest struct {
	UserID         *uuid.UUID `json:"user_id,omitempty"`
	Name           *string    `json:"name,omitempty"`
	Email          *string    `json:"email,omitempty"`
	Phone          *string    `json:"phone,omitempty"`
	Role           *string    `json:"role,omitempty"`
	Status         *string    `json:"status,omitempty"`
	Specialization *string    `json:"specialization,omitempty"`
	Notes          *string    `json:"notes,omitempty"`
}

// Service defines business operations for workshop employees.
type Service interface {
	CreateEmployee(ctx context.Context, workshopID, callerUserID uuid.UUID, callerRole domain.UserRole, req CreateEmployeeRequest) (*domain.WorkshopEmployee, map[string]string, error)
	GetEmployeeByID(ctx context.Context, workshopID, employeeID, callerUserID uuid.UUID, callerRole domain.UserRole) (*domain.WorkshopEmployee, error)
	GetCurrentEmployee(ctx context.Context, workshopID, callerUserID uuid.UUID) (*domain.WorkshopEmployee, error)
	ListEmployees(ctx context.Context, workshopID, callerUserID uuid.UUID, callerRole domain.UserRole, pagination domain.PaginationParams, filter EmployeeFilter) ([]domain.WorkshopEmployee, *response.Meta, error)
	UpdateEmployee(ctx context.Context, workshopID, employeeID, callerUserID uuid.UUID, callerRole domain.UserRole, req UpdateEmployeeRequest) (*domain.WorkshopEmployee, map[string]string, error)
	DeleteEmployee(ctx context.Context, workshopID, employeeID, callerUserID uuid.UUID, callerRole domain.UserRole) error
}

type employeeService struct {
	repo   Repository
	logger *logger.Logger
}

// NewService creates a new Employee Service instance.
func NewService(repo Repository, log *logger.Logger) Service {
	return &employeeService{
		repo:   repo,
		logger: log,
	}
}

func (s *employeeService) canCallerManageEmployees(ctx context.Context, workshopID, callerUserID uuid.UUID, callerRole domain.UserRole) (isOwner bool, isManager bool, err error) {
	if callerRole == domain.RoleAdmin {
		return true, false, nil
	}
	ownerID, err := s.repo.GetWorkshopOwnerID(ctx, workshopID)
	if err != nil {
		return false, false, err
	}
	if ownerID == callerUserID {
		return true, false, nil
	}
	emp, err := s.repo.GetByUserID(ctx, workshopID, callerUserID)
	if err == nil && emp != nil && emp.CanManageEmployees() {
		return false, true, nil
	}
	return false, false, ErrForbidden
}

func (s *employeeService) canCallerViewEmployees(ctx context.Context, workshopID, callerUserID uuid.UUID, callerRole domain.UserRole) error {
	if callerRole == domain.RoleAdmin {
		return nil
	}
	ownerID, err := s.repo.GetWorkshopOwnerID(ctx, workshopID)
	if err != nil {
		return err
	}
	if ownerID == callerUserID {
		return nil
	}
	emp, err := s.repo.GetByUserID(ctx, workshopID, callerUserID)
	if err == nil && emp != nil && emp.Status == domain.EmployeeStatusActive {
		return nil
	}
	return ErrForbidden
}

func (s *employeeService) CreateEmployee(ctx context.Context, workshopID, callerUserID uuid.UUID, callerRole domain.UserRole, req CreateEmployeeRequest) (*domain.WorkshopEmployee, map[string]string, error) {
	isOwner, _, err := s.canCallerManageEmployees(ctx, workshopID, callerUserID, callerRole)
	if err != nil {
		return nil, nil, err
	}

	v := validator.New()
	req.Name = strings.TrimSpace(req.Name)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Email = strings.TrimSpace(req.Email)
	req.Role = strings.TrimSpace(req.Role)

	v.Required("name", req.Name)
	v.Required("phone", req.Phone)
	v.Required("role", req.Role)

	normRole, ok := domain.NormalizeEmployeeRole(req.Role)
	if !ok {
		v.AddError("role", "invalid employee role; must be MECHANIC, ADMIN_CASHIER, ADMIN_INVENTORY, ADMIN_BOTH, MANAGER, or OWNER")
	} else if !isOwner && normRole == domain.EmployeeRoleOwner {
		v.AddError("role", "managers cannot assign the owner role")
	}

	status := domain.EmployeeStatusActive
	if strings.TrimSpace(req.Status) != "" {
		st := strings.ToUpper(strings.TrimSpace(req.Status))
		if st != string(domain.EmployeeStatusActive) && st != string(domain.EmployeeStatusInactive) {
			v.AddError("status", "status must be ACTIVE or INACTIVE")
		} else {
			status = domain.EmployeeStatus(st)
		}
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	now := time.Now().UTC()
	emp := &domain.WorkshopEmployee{
		ID:             uuid.New(),
		WorkshopID:     workshopID,
		UserID:         req.UserID,
		Name:           req.Name,
		Email:          req.Email,
		Phone:          req.Phone,
		Role:           normRole,
		Status:         status,
		Specialization: strings.TrimSpace(req.Specialization),
		Notes:          strings.TrimSpace(req.Notes),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.Create(ctx, emp); err != nil {
		s.logger.WithContext(ctx).Error("failed to create employee", "workshop_id", workshopID, "error", err)
		return nil, nil, err
	}

	perms := emp.CalculatePermissions()
	emp.Permissions = &perms

	s.logger.WithContext(ctx).Info("employee created successfully", "workshop_id", workshopID, "employee_id", emp.ID, "role", emp.Role)
	return emp, nil, nil
}

func (s *employeeService) GetEmployeeByID(ctx context.Context, workshopID, employeeID, callerUserID uuid.UUID, callerRole domain.UserRole) (*domain.WorkshopEmployee, error) {
	if err := s.canCallerViewEmployees(ctx, workshopID, callerUserID, callerRole); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, workshopID, employeeID)
}

func (s *employeeService) GetCurrentEmployee(ctx context.Context, workshopID, callerUserID uuid.UUID) (*domain.WorkshopEmployee, error) {
	emp, err := s.repo.GetByUserID(ctx, workshopID, callerUserID)
	if err == nil && emp != nil {
		return emp, nil
	}

	ownerID, err := s.repo.GetWorkshopOwnerID(ctx, workshopID)
	if err == nil && ownerID == callerUserID {
		perms := domain.EmployeePermissions{
			CanManageEmployees:          true,
			CanManageInventory:          true,
			CanAccessCashier:            true,
			CanAccessRepairJobs:         true,
			CanManageWorkshopOperations: true,
		}
		now := time.Now().UTC()
		return &domain.WorkshopEmployee{
			ID:          uuid.Nil,
			WorkshopID:  workshopID,
			UserID:      &callerUserID,
			Name:        "Workshop Owner",
			Role:        domain.EmployeeRoleOwner,
			Status:      domain.EmployeeStatusActive,
			CreatedAt:   now,
			UpdatedAt:   now,
			Permissions: &perms,
		}, nil
	}

	return nil, ErrEmployeeNotFound
}

func (s *employeeService) ListEmployees(ctx context.Context, workshopID, callerUserID uuid.UUID, callerRole domain.UserRole, pagination domain.PaginationParams, filter EmployeeFilter) ([]domain.WorkshopEmployee, *response.Meta, error) {
	if err := s.canCallerViewEmployees(ctx, workshopID, callerUserID, callerRole); err != nil {
		return nil, nil, err
	}
	pagination.EnsureValid()

	list, total, err := s.repo.List(ctx, workshopID, pagination, filter)
	if err != nil {
		s.logger.WithContext(ctx).Error("failed to list workshop employees", "workshop_id", workshopID, "error", err)
		return nil, nil, err
	}

	totalPages := int((total + int64(pagination.PageSize) - 1) / int64(pagination.PageSize))
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

func (s *employeeService) UpdateEmployee(ctx context.Context, workshopID, employeeID, callerUserID uuid.UUID, callerRole domain.UserRole, req UpdateEmployeeRequest) (*domain.WorkshopEmployee, map[string]string, error) {
	isOwner, _, err := s.canCallerManageEmployees(ctx, workshopID, callerUserID, callerRole)
	if err != nil {
		return nil, nil, err
	}

	emp, err := s.repo.GetByID(ctx, workshopID, employeeID)
	if err != nil {
		return nil, nil, err
	}

	// Managers cannot modify Owner
	if !isOwner && emp.Role == domain.EmployeeRoleOwner {
		return nil, nil, ErrCannotModifyOwner
	}

	v := validator.New()

	if req.UserID != nil {
		emp.UserID = req.UserID
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		v.Required("name", name)
		emp.Name = name
	}
	if req.Phone != nil {
		phone := strings.TrimSpace(*req.Phone)
		v.Required("phone", phone)
		emp.Phone = phone
	}
	if req.Email != nil {
		emp.Email = strings.TrimSpace(*req.Email)
	}
	if req.Specialization != nil {
		emp.Specialization = strings.TrimSpace(*req.Specialization)
	}
	if req.Notes != nil {
		emp.Notes = strings.TrimSpace(*req.Notes)
	}
	if req.Role != nil {
		normRole, ok := domain.NormalizeEmployeeRole(*req.Role)
		if !ok {
			v.AddError("role", "invalid employee role; must be MECHANIC, ADMIN_CASHIER, ADMIN_INVENTORY, ADMIN_BOTH, MANAGER, or OWNER")
		} else if !isOwner && normRole == domain.EmployeeRoleOwner {
			v.AddError("role", "managers cannot assign the owner role")
		} else {
			emp.Role = normRole
		}
	}
	if req.Status != nil {
		st := strings.ToUpper(strings.TrimSpace(*req.Status))
		if st != string(domain.EmployeeStatusActive) && st != string(domain.EmployeeStatusInactive) {
			v.AddError("status", "status must be ACTIVE or INACTIVE")
		} else {
			emp.Status = domain.EmployeeStatus(st)
		}
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	emp.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, emp); err != nil {
		s.logger.WithContext(ctx).Error("failed to update employee", "workshop_id", workshopID, "employee_id", employeeID, "error", err)
		return nil, nil, err
	}

	perms := emp.CalculatePermissions()
	emp.Permissions = &perms

	s.logger.WithContext(ctx).Info("employee updated successfully", "workshop_id", workshopID, "employee_id", employeeID)
	return emp, nil, nil
}

func (s *employeeService) DeleteEmployee(ctx context.Context, workshopID, employeeID, callerUserID uuid.UUID, callerRole domain.UserRole) error {
	isOwner, _, err := s.canCallerManageEmployees(ctx, workshopID, callerUserID, callerRole)
	if err != nil {
		return err
	}

	emp, err := s.repo.GetByID(ctx, workshopID, employeeID)
	if err != nil {
		return err
	}

	// Managers cannot delete Owner
	if !isOwner && emp.Role == domain.EmployeeRoleOwner {
		return ErrCannotModifyOwner
	}

	// Caller cannot delete their own employee record
	if emp.UserID != nil && *emp.UserID == callerUserID {
		return ErrCannotDeleteSelf
	}

	if err := s.repo.Delete(ctx, workshopID, employeeID); err != nil {
		s.logger.WithContext(ctx).Error("failed to delete employee", "workshop_id", workshopID, "employee_id", employeeID, "error", err)
		return err
	}

	s.logger.WithContext(ctx).Info("employee deleted successfully", "workshop_id", workshopID, "employee_id", employeeID)
	return nil
}
