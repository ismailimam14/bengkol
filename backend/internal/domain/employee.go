package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// EmployeeRole represents specific access roles within a workshop
type EmployeeRole string

const (
	EmployeeRoleAdmin          EmployeeRole = "ADMIN"
	EmployeeRoleCustomer       EmployeeRole = "CUSTOMER"
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

// Permission keys constants
const (
	PermCanManageEmployees          = "can_manage_employees"
	PermCanManageInventory          = "can_manage_inventory"
	PermCanAccessCashier            = "can_access_cashier"
	PermCanAccessRepairJobs         = "can_access_repair_jobs"
	PermCanManageWorkshopOperations = "can_manage_workshop_operations"
	PermCanIssueRefunds             = "can_issue_refunds"
	PermCanManageDiscounts          = "can_manage_discounts"
	PermCanViewSalesReports         = "can_view_sales_reports"
	PermCanViewInventoryReports     = "can_view_inventory_reports"
	PermCanViewEmployeePerformance  = "can_view_employee_performance"
	PermCanManageCustomers          = "can_manage_customers"
	PermCanAssignJobsToMechanics     = "can_assign_jobs_to_mechanics"
	PermCanUpdateJobStatus          = "can_update_job_status"
	PermCanManageSystemSettings      = "can_manage_system_settings"
	PermCanManageRolesAndPermissions = "can_manage_roles_and_permissions"
	PermCanAccessFinancialReports    = "can_access_financial_reports"
	PermCanManagePricing             = "can_manage_pricing"
	PermCanTransferOwnership         = "can_transfer_ownership"
	PermCanOverrideTransactions      = "can_override_transactions"
	PermCanRevokeManagerActions      = "can_revoke_manager_actions"
)

// EmployeePermissions defines granular action permissions across 20 server-enforced keys
type EmployeePermissions struct {
	// Standard ADMIN permissions (1-11)
	CanManageEmployees          bool `json:"can_manage_employees"`
	CanManageInventory          bool `json:"can_manage_inventory"`
	CanAccessCashier            bool `json:"can_access_cashier"`
	CanAccessRepairJobs         bool `json:"can_access_repair_jobs"`
	CanManageWorkshopOperations bool `json:"can_manage_workshop_operations"`
	CanIssueRefunds             bool `json:"can_issue_refunds"`
	CanManageDiscounts          bool `json:"can_manage_discounts"`
	CanViewSalesReports         bool `json:"can_view_sales_reports"`
	CanViewInventoryReports     bool `json:"can_view_inventory_reports"`
	CanViewEmployeePerformance  bool `json:"can_view_employee_performance"`
	CanManageCustomers          bool `json:"can_manage_customers"`

	// MANAGER additional permissions (12-13)
	CanAssignJobsToMechanics bool `json:"can_assign_jobs_to_mechanics"`
	CanUpdateJobStatus       bool `json:"can_update_job_status"`

	// OWNER additional permissions (14-20)
	CanManageSystemSettings       bool `json:"can_manage_system_settings"`
	CanManageRolesAndPermissions bool `json:"can_manage_roles_and_permissions"`
	CanAccessFinancialReports     bool `json:"can_access_financial_reports"`
	CanManagePricing              bool `json:"can_manage_pricing"`
	CanTransferOwnership          bool `json:"can_transfer_ownership"`
	CanOverrideTransactions       bool `json:"can_override_transactions"`
	CanRevokeManagerActions       bool `json:"can_revoke_manager_actions"`
}

// HasPermission checks if the permission struct grants the specified permission key
func (p EmployeePermissions) HasPermission(key string) bool {
	switch key {
	case PermCanManageEmployees:
		return p.CanManageEmployees
	case PermCanManageInventory:
		return p.CanManageInventory
	case PermCanAccessCashier:
		return p.CanAccessCashier
	case PermCanAccessRepairJobs:
		return p.CanAccessRepairJobs
	case PermCanManageWorkshopOperations:
		return p.CanManageWorkshopOperations
	case PermCanIssueRefunds:
		return p.CanIssueRefunds
	case PermCanManageDiscounts:
		return p.CanManageDiscounts
	case PermCanViewSalesReports:
		return p.CanViewSalesReports
	case PermCanViewInventoryReports:
		return p.CanViewInventoryReports
	case PermCanViewEmployeePerformance:
		return p.CanViewEmployeePerformance
	case PermCanManageCustomers:
		return p.CanManageCustomers
	case PermCanAssignJobsToMechanics:
		return p.CanAssignJobsToMechanics
	case PermCanUpdateJobStatus:
		return p.CanUpdateJobStatus
	case PermCanManageSystemSettings:
		return p.CanManageSystemSettings
	case PermCanManageRolesAndPermissions:
		return p.CanManageRolesAndPermissions
	case PermCanAccessFinancialReports:
		return p.CanAccessFinancialReports
	case PermCanManagePricing:
		return p.CanManagePricing
	case PermCanTransferOwnership:
		return p.CanTransferOwnership
	case PermCanOverrideTransactions:
		return p.CanOverrideTransactions
	case PermCanRevokeManagerActions:
		return p.CanRevokeManagerActions
	default:
		return false
	}
}

// DefaultPermissionsForRole returns the default permission matrix for a given role
func DefaultPermissionsForRole(r EmployeeRole) EmployeePermissions {
	switch r {
	case EmployeeRoleOwner:
		return EmployeePermissions{
			CanManageEmployees:           true,
			CanManageInventory:           true,
			CanAccessCashier:             true,
			CanAccessRepairJobs:          true,
			CanManageWorkshopOperations:  true,
			CanIssueRefunds:              true,
			CanManageDiscounts:           true,
			CanViewSalesReports:          true,
			CanViewInventoryReports:      true,
			CanViewEmployeePerformance:   true,
			CanManageCustomers:           true,
			CanAssignJobsToMechanics:     true,
			CanUpdateJobStatus:           true,
			CanManageSystemSettings:      true,
			CanManageRolesAndPermissions: true,
			CanAccessFinancialReports:    true,
			CanManagePricing:             true,
			CanTransferOwnership:         true,
			CanOverrideTransactions:      true,
			CanRevokeManagerActions:      true,
		}
	case EmployeeRoleManager:
		return EmployeePermissions{
			CanManageEmployees:          true,
			CanManageInventory:          true,
			CanAccessCashier:            true,
			CanAccessRepairJobs:         true,
			CanManageWorkshopOperations: true,
			CanIssueRefunds:             true,
			CanManageDiscounts:          true,
			CanViewSalesReports:         true,
			CanViewInventoryReports:     true,
			CanViewEmployeePerformance:  true,
			CanManageCustomers:          true,
			CanAssignJobsToMechanics:    true,
			CanUpdateJobStatus:          true,
			CanManageSystemSettings:      false,
			CanManageRolesAndPermissions: false,
			CanAccessFinancialReports:    false,
			CanManagePricing:             false,
			CanTransferOwnership:         false,
			CanOverrideTransactions:      false,
			CanRevokeManagerActions:      false,
		}
	case EmployeeRoleMechanic:
		return EmployeePermissions{
			CanAccessRepairJobs: true,
			CanUpdateJobStatus:  true,
		}
	case EmployeeRoleAdmin, EmployeeRoleAdminBoth:
		return EmployeePermissions{
			CanManageEmployees:          false,
			CanManageInventory:          true,
			CanAccessCashier:            true,
			CanAccessRepairJobs:         false,
			CanManageWorkshopOperations: false,
			CanIssueRefunds:             true,
			CanManageDiscounts:          true,
			CanViewSalesReports:         true,
			CanViewInventoryReports:     true,
			CanViewEmployeePerformance:  false,
			CanManageCustomers:          true,
			CanAssignJobsToMechanics:    false,
			CanUpdateJobStatus:          false,
			CanManageSystemSettings:      false,
			CanManageRolesAndPermissions: false,
			CanAccessFinancialReports:    false,
			CanManagePricing:             false,
			CanTransferOwnership:         false,
			CanOverrideTransactions:      false,
			CanRevokeManagerActions:      false,
		}
	case EmployeeRoleAdminCashier:
		return EmployeePermissions{
			CanManageEmployees:          false,
			CanManageInventory:          false,
			CanAccessCashier:            true,
			CanAccessRepairJobs:         false,
			CanManageWorkshopOperations: false,
			CanIssueRefunds:             true,
			CanManageDiscounts:          true,
			CanViewSalesReports:         true,
			CanViewInventoryReports:     true,
			CanViewEmployeePerformance:  false,
			CanManageCustomers:          true,
			CanAssignJobsToMechanics:    false,
			CanUpdateJobStatus:          false,
			CanManageSystemSettings:      false,
			CanManageRolesAndPermissions: false,
			CanAccessFinancialReports:    false,
			CanManagePricing:             false,
			CanTransferOwnership:         false,
			CanOverrideTransactions:      false,
			CanRevokeManagerActions:      false,
		}
	case EmployeeRoleAdminInventory:
		return EmployeePermissions{
			CanManageEmployees:          false,
			CanManageInventory:          true,
			CanAccessCashier:            false,
			CanAccessRepairJobs:         false,
			CanManageWorkshopOperations: false,
			CanIssueRefunds:             true,
			CanManageDiscounts:          true,
			CanViewSalesReports:         true,
			CanViewInventoryReports:     true,
			CanViewEmployeePerformance:  false,
			CanManageCustomers:          true,
			CanAssignJobsToMechanics:    false,
			CanUpdateJobStatus:          false,
			CanManageSystemSettings:      false,
			CanManageRolesAndPermissions: false,
			CanAccessFinancialReports:    false,
			CanManagePricing:             false,
			CanTransferOwnership:         false,
			CanOverrideTransactions:      false,
			CanRevokeManagerActions:      false,
		}
	default:
		return EmployeePermissions{}
	}
}

// ValidPermissionKeys returns list of all 20 valid permission key strings
func ValidPermissionKeys() []string {
	return []string{
		PermCanManageEmployees,
		PermCanManageInventory,
		PermCanAccessCashier,
		PermCanAccessRepairJobs,
		PermCanManageWorkshopOperations,
		PermCanIssueRefunds,
		PermCanManageDiscounts,
		PermCanViewSalesReports,
		PermCanViewInventoryReports,
		PermCanViewEmployeePerformance,
		PermCanManageCustomers,
		PermCanAssignJobsToMechanics,
		PermCanUpdateJobStatus,
		PermCanManageSystemSettings,
		PermCanManageRolesAndPermissions,
		PermCanAccessFinancialReports,
		PermCanManagePricing,
		PermCanTransferOwnership,
		PermCanOverrideTransactions,
		PermCanRevokeManagerActions,
	}
}

// IsValidPermissionKey checks if given key is one of the 20 supported permission keys
func IsValidPermissionKey(key string) bool {
	switch key {
	case PermCanManageEmployees, PermCanManageInventory, PermCanAccessCashier,
		PermCanAccessRepairJobs, PermCanManageWorkshopOperations, PermCanIssueRefunds,
		PermCanManageDiscounts, PermCanViewSalesReports, PermCanViewInventoryReports,
		PermCanViewEmployeePerformance, PermCanManageCustomers, PermCanAssignJobsToMechanics,
		PermCanUpdateJobStatus, PermCanManageSystemSettings, PermCanManageRolesAndPermissions,
		PermCanAccessFinancialReports, PermCanManagePricing, PermCanTransferOwnership,
		PermCanOverrideTransactions, PermCanRevokeManagerActions:
		return true
	default:
		return false
	}
}

// WorkshopEmployee represents an employee hired or assigned to a workshop
type WorkshopEmployee struct {
	ID              uuid.UUID            `json:"id"`
	WorkshopID      uuid.UUID            `json:"workshop_id"`
	UserID          *uuid.UUID           `json:"user_id,omitempty"`
	Name            string               `json:"name"`
	Email           string               `json:"email,omitempty"`
	Phone           string               `json:"phone"`
	Role            EmployeeRole         `json:"role"`
	Status          EmployeeStatus       `json:"status"`
	Specialization  string               `json:"specialization,omitempty"`
	Notes           string               `json:"notes,omitempty"`
	InitialPassword string               `json:"initial_password,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
	Permissions     *EmployeePermissions `json:"permissions,omitempty"`

	// Relational references
	User     *User     `json:"user,omitempty"`
	Workshop *Workshop `json:"workshop,omitempty"`
}

// CalculatePermissions returns the effective permissions struct for this employee.
// For OWNER, permissions are immutable (always full access).
// For active employees with custom configured permissions, those are applied.
// Inactive employees receive zero permissions.
func (e *WorkshopEmployee) CalculatePermissions() EmployeePermissions {
	if e.Status != EmployeeStatusActive {
		return EmployeePermissions{}
	}
	if e.Role == EmployeeRoleOwner {
		return DefaultPermissionsForRole(EmployeeRoleOwner)
	}
	if e.Permissions != nil {
		return *e.Permissions
	}
	return DefaultPermissionsForRole(e.Role)
}

// CanManageEmployees checks if role/permissions allow employee CRUD
func (e *WorkshopEmployee) CanManageEmployees() bool {
	return e.CalculatePermissions().CanManageEmployees
}

// CanManageInventory checks if role/permissions allow inventory / spare parts management
func (e *WorkshopEmployee) CanManageInventory() bool {
	return e.CalculatePermissions().CanManageInventory
}

// CanAccessCashier checks if role/permissions allow cashier, payments, invoices
func (e *WorkshopEmployee) CanAccessCashier() bool {
	return e.CalculatePermissions().CanAccessCashier
}

// CanAccessRepairJobs checks if role/permissions allow repair jobs, queue inspection
func (e *WorkshopEmployee) CanAccessRepairJobs() bool {
	return e.CalculatePermissions().CanAccessRepairJobs
}

// CanManageWorkshopOperations checks if role/permissions allow general operational features
func (e *WorkshopEmployee) CanManageWorkshopOperations() bool {
	return e.CalculatePermissions().CanManageWorkshopOperations
}

// CanIssueRefunds checks refund permissions
func (e *WorkshopEmployee) CanIssueRefunds() bool {
	return e.CalculatePermissions().CanIssueRefunds
}

// CanManageDiscounts checks discount management permissions
func (e *WorkshopEmployee) CanManageDiscounts() bool {
	return e.CalculatePermissions().CanManageDiscounts
}

// CanViewSalesReports checks sales reporting permissions
func (e *WorkshopEmployee) CanViewSalesReports() bool {
	return e.CalculatePermissions().CanViewSalesReports
}

// CanViewInventoryReports checks inventory reporting permissions
func (e *WorkshopEmployee) CanViewInventoryReports() bool {
	return e.CalculatePermissions().CanViewInventoryReports
}

// CanViewEmployeePerformance checks employee performance report permissions
func (e *WorkshopEmployee) CanViewEmployeePerformance() bool {
	return e.CalculatePermissions().CanViewEmployeePerformance
}

// CanManageCustomers checks customer management permissions
func (e *WorkshopEmployee) CanManageCustomers() bool {
	return e.CalculatePermissions().CanManageCustomers
}

// CanAssignJobsToMechanics checks job assignment permissions
func (e *WorkshopEmployee) CanAssignJobsToMechanics() bool {
	return e.CalculatePermissions().CanAssignJobsToMechanics
}

// CanUpdateJobStatus checks job status progression permissions
func (e *WorkshopEmployee) CanUpdateJobStatus() bool {
	return e.CalculatePermissions().CanUpdateJobStatus
}

// CanManageSystemSettings checks workshop system settings permissions
func (e *WorkshopEmployee) CanManageSystemSettings() bool {
	return e.CalculatePermissions().CanManageSystemSettings
}

// CanManageRolesAndPermissions checks permissions modification authority
func (e *WorkshopEmployee) CanManageRolesAndPermissions() bool {
	return e.CalculatePermissions().CanManageRolesAndPermissions
}

// CanAccessFinancialReports checks financial reporting permissions
func (e *WorkshopEmployee) CanAccessFinancialReports() bool {
	return e.CalculatePermissions().CanAccessFinancialReports
}

// CanManagePricing checks pricing and catalog pricing management permissions
func (e *WorkshopEmployee) CanManagePricing() bool {
	return e.CalculatePermissions().CanManagePricing
}

// CanTransferOwnership checks workshop transfer authority
func (e *WorkshopEmployee) CanTransferOwnership() bool {
	return e.CalculatePermissions().CanTransferOwnership
}

// CanOverrideTransactions checks transaction override authority
func (e *WorkshopEmployee) CanOverrideTransactions() bool {
	return e.CalculatePermissions().CanOverrideTransactions
}

// CanRevokeManagerActions checks manager action revocation authority
func (e *WorkshopEmployee) CanRevokeManagerActions() bool {
	return e.CalculatePermissions().CanRevokeManagerActions
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
	case "ADMIN_BOTH", "ADMIN_ACCESS_BOTH":
		return EmployeeRoleAdminBoth, true
	case "ADMIN":
		return EmployeeRoleAdmin, true
	case "CUSTOMER":
		return EmployeeRoleCustomer, true
	case "MANAGER":
		return EmployeeRoleManager, true
	case "OWNER":
		return EmployeeRoleOwner, true
	default:
		return "", false
	}
}

// ToUserRole converts an EmployeeRole to corresponding UserRole.
func (r EmployeeRole) ToUserRole() UserRole {
	switch r {
	case EmployeeRoleOwner:
		return RoleOwner
	case EmployeeRoleManager:
		return RoleManager
	case EmployeeRoleMechanic:
		return RoleMechanic
	case EmployeeRoleCustomer:
		return RoleCustomer
	case EmployeeRoleAdmin, EmployeeRoleAdminBoth, EmployeeRoleAdminCashier, EmployeeRoleAdminInventory:
		return RoleAdmin
	default:
		return RoleCustomer
	}
}

// EmployeeRoleToUserRole converts an EmployeeRole to UserRole.
func EmployeeRoleToUserRole(er EmployeeRole) UserRole {
	return er.ToUserRole()
}
