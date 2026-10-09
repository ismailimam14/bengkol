package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// EmployeeRole represents specific access roles within a workshop
type EmployeeRole string

const (
	EmployeeRoleMechanic       EmployeeRole = "MECHANIC"
	EmployeeRoleAdminCashier   EmployeeRole = "ADMIN_CASHIER"
	EmployeeRoleAdminInventory EmployeeRole = "ADMIN_INVENTORY"
	EmployeeRoleAdminBoth      EmployeeRole = "ADMIN_BOTH"
	EmployeeRoleManager        EmployeeRole = "MANAGER"
	EmployeeRoleOwner          EmployeeRole = "OWNER"
)

// EmployeeStatus represents employment state
type EmployeeStatus string

const (
	EmployeeStatusActive   EmployeeStatus = "ACTIVE"
	EmployeeStatusInactive EmployeeStatus = "INACTIVE"
)

// EmployeePermissions defines granular action permissions for an employee
type EmployeePermissions struct {
	CanManageEmployees          bool `json:"can_manage_employees"`
	CanManageInventory          bool `json:"can_manage_inventory"`
	CanAccessCashier            bool `json:"can_access_cashier"`
	CanAccessRepairJobs         bool `json:"can_access_repair_jobs"`
	CanManageWorkshopOperations bool `json:"can_manage_workshop_operations"`
}

// WorkshopEmployee represents an employee hired or assigned to a workshop
type WorkshopEmployee struct {
	ID             uuid.UUID            `json:"id"`
	WorkshopID     uuid.UUID            `json:"workshop_id"`
	UserID         *uuid.UUID           `json:"user_id,omitempty"`
	Name           string               `json:"name"`
	Email          string               `json:"email,omitempty"`
	Phone          string               `json:"phone"`
	Role           EmployeeRole         `json:"role"`
	Status         EmployeeStatus       `json:"status"`
	Specialization string               `json:"specialization,omitempty"`
	Notes          string               `json:"notes,omitempty"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
	Permissions    *EmployeePermissions `json:"permissions,omitempty"`

	// Relational references
	User     *User     `json:"user,omitempty"`
	Workshop *Workshop `json:"workshop,omitempty"`
}

// CanManageEmployees checks if role allows employee CRUD (Owner, Manager)
func (e *WorkshopEmployee) CanManageEmployees() bool {
	return e.Status == EmployeeStatusActive && (e.Role == EmployeeRoleOwner || e.Role == EmployeeRoleManager)
}

// CanManageInventory checks if role allows inventory / spare parts management (Owner, Manager, AdminInventory, AdminBoth)
func (e *WorkshopEmployee) CanManageInventory() bool {
	return e.Status == EmployeeStatusActive && (e.Role == EmployeeRoleOwner || e.Role == EmployeeRoleManager || e.Role == EmployeeRoleAdminInventory || e.Role == EmployeeRoleAdminBoth)
}

// CanAccessCashier checks if role allows payments, invoices, transactions (Owner, Manager, AdminCashier, AdminBoth)
func (e *WorkshopEmployee) CanAccessCashier() bool {
	return e.Status == EmployeeStatusActive && (e.Role == EmployeeRoleOwner || e.Role == EmployeeRoleManager || e.Role == EmployeeRoleAdminCashier || e.Role == EmployeeRoleAdminBoth)
}

// CanAccessRepairJobs checks if role allows repair jobs, assigned services (Owner, Manager, Mechanic)
func (e *WorkshopEmployee) CanAccessRepairJobs() bool {
	return e.Status == EmployeeStatusActive && (e.Role == EmployeeRoleOwner || e.Role == EmployeeRoleManager || e.Role == EmployeeRoleMechanic)
}

// CanManageWorkshopOperations checks if role allows general operational features (Owner, Manager)
func (e *WorkshopEmployee) CanManageWorkshopOperations() bool {
	return e.Status == EmployeeStatusActive && (e.Role == EmployeeRoleOwner || e.Role == EmployeeRoleManager)
}

// CalculatePermissions returns the permissions struct for this employee
func (e *WorkshopEmployee) CalculatePermissions() EmployeePermissions {
	return EmployeePermissions{
		CanManageEmployees:          e.CanManageEmployees(),
		CanManageInventory:          e.CanManageInventory(),
		CanAccessCashier:            e.CanAccessCashier(),
		CanAccessRepairJobs:         e.CanAccessRepairJobs(),
		CanManageWorkshopOperations: e.CanManageWorkshopOperations(),
	}
}

// NormalizeEmployeeRole parses input strings to valid EmployeeRole enum values
func NormalizeEmployeeRole(r string) (EmployeeRole, bool) {
	norm := strings.ToUpper(strings.TrimSpace(r))
	switch norm {
	case "MECHANIC":
		return EmployeeRoleMechanic, true
	case "ADMIN_CASHIER", "ADMIN_ONLY_ACCESS_CASHIER", "CASHIER":
		return EmployeeRoleAdminCashier, true
	case "ADMIN_INVENTORY", "ADMIN_ONLY_ACCESS_INVENTORY", "INVENTORY":
		return EmployeeRoleAdminInventory, true
	case "ADMIN_BOTH", "ADMIN_ACCESS_BOTH", "ADMIN":
		return EmployeeRoleAdminBoth, true
	case "MANAGER":
		return EmployeeRoleManager, true
	case "OWNER":
		return EmployeeRoleOwner, true
	default:
		return "", false
	}
}
