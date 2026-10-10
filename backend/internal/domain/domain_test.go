package domain_test

import (
	"testing"

	"github.com/bengkol/backend/internal/domain"
	"github.com/google/uuid"
)

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"whitespace string", "   ", ""},
		{"standard indonesian 08", "081234567890", "081234567890"},
		{"plus 62 prefix", "+6281234567890", "081234567890"},
		{"62 prefix without plus", "6281234567890", "081234567890"},
		{"with dashes and spaces", "+62 812-3456-7890", "081234567890"},
		{"with parentheses and dots", "(0812) 3456.7890", "081234567890"},
		{"international number", "+1 555 123 4567", "+15551234567"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := domain.NormalizePhone(tc.input)
			if actual != tc.expected {
				t.Errorf("NormalizePhone(%q) = %q; expected %q", tc.input, actual, tc.expected)
			}
		})
	}
}

func TestNormalizeEmployeeRole(t *testing.T) {
	tests := []struct {
		input    string
		expected domain.EmployeeRole
		valid    bool
	}{
		{"ADMIN", domain.EmployeeRoleAdmin, true},
		{"admin", domain.EmployeeRoleAdmin, true},
		{"CUSTOMER", domain.EmployeeRoleCustomer, true},
		{"customer", domain.EmployeeRoleCustomer, true},
		{"MECHANIC", domain.EmployeeRoleMechanic, true},
		{"MANAGER", domain.EmployeeRoleManager, true},
		{"OWNER", domain.EmployeeRoleOwner, true},
		{"ADMIN_CASHIER", domain.EmployeeRoleAdminCashier, true},
		{"ADMIN_INVENTORY", domain.EmployeeRoleAdminInventory, true},
		{"ADMIN_BOTH", domain.EmployeeRoleAdminBoth, true},
		{"INVALID_ROLE", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			r, ok := domain.NormalizeEmployeeRole(tc.input)
			if ok != tc.valid {
				t.Fatalf("expected valid=%v, got %v", tc.valid, ok)
			}
			if ok && r != tc.expected {
				t.Errorf("expected role=%q, got %q", tc.expected, r)
			}
		})
	}
}

func TestDefaultPermissionsForRole(t *testing.T) {
	// OWNER check: all 20 true
	ownerPerms := domain.DefaultPermissionsForRole(domain.EmployeeRoleOwner)
	if !ownerPerms.CanManageEmployees || !ownerPerms.CanManageInventory || !ownerPerms.CanAccessCashier ||
		!ownerPerms.CanAccessRepairJobs || !ownerPerms.CanManageWorkshopOperations || !ownerPerms.CanIssueRefunds ||
		!ownerPerms.CanManageDiscounts || !ownerPerms.CanViewSalesReports || !ownerPerms.CanViewInventoryReports ||
		!ownerPerms.CanViewEmployeePerformance || !ownerPerms.CanManageCustomers || !ownerPerms.CanAssignJobsToMechanics ||
		!ownerPerms.CanUpdateJobStatus || !ownerPerms.CanManageSystemSettings || !ownerPerms.CanManageRolesAndPermissions ||
		!ownerPerms.CanAccessFinancialReports || !ownerPerms.CanManagePricing || !ownerPerms.CanTransferOwnership ||
		!ownerPerms.CanOverrideTransactions || !ownerPerms.CanRevokeManagerActions {
		t.Errorf("expected all 20 permissions true for OWNER, got %+v", ownerPerms)
	}

	// MANAGER check
	mgrPerms := domain.DefaultPermissionsForRole(domain.EmployeeRoleManager)
	if !mgrPerms.CanManageEmployees || !mgrPerms.CanManageInventory || !mgrPerms.CanAccessCashier ||
		!mgrPerms.CanAccessRepairJobs || !mgrPerms.CanManageWorkshopOperations || !mgrPerms.CanAssignJobsToMechanics ||
		!mgrPerms.CanUpdateJobStatus {
		t.Errorf("expected manager true permissions to be set, got %+v", mgrPerms)
	}
	if mgrPerms.CanManageSystemSettings || mgrPerms.CanManageRolesAndPermissions || mgrPerms.CanTransferOwnership {
		t.Errorf("expected owner-only permissions false for manager, got %+v", mgrPerms)
	}

	// MECHANIC check: only repair jobs and job status
	mechPerms := domain.DefaultPermissionsForRole(domain.EmployeeRoleMechanic)
	if !mechPerms.CanAccessRepairJobs || !mechPerms.CanUpdateJobStatus {
		t.Errorf("expected mechanic CanAccessRepairJobs and CanUpdateJobStatus true, got %+v", mechPerms)
	}
	if mechPerms.CanManageEmployees || mechPerms.CanManageInventory || mechPerms.CanAccessCashier || mechPerms.CanIssueRefunds {
		t.Errorf("expected mechanic operational/financial permissions false, got %+v", mechPerms)
	}

	// ADMIN check: inventory, cashier, refunds, discounts, sales/inv reports, customers = true; others = false
	adminPerms := domain.DefaultPermissionsForRole(domain.EmployeeRoleAdmin)
	if !adminPerms.CanManageInventory || !adminPerms.CanAccessCashier || !adminPerms.CanIssueRefunds ||
		!adminPerms.CanManageDiscounts || !adminPerms.CanViewSalesReports || !adminPerms.CanViewInventoryReports ||
		!adminPerms.CanManageCustomers {
		t.Errorf("expected admin default true permissions, got %+v", adminPerms)
	}
	if adminPerms.CanManageEmployees || adminPerms.CanAccessRepairJobs || adminPerms.CanManageWorkshopOperations ||
		adminPerms.CanAssignJobsToMechanics || adminPerms.CanManageSystemSettings {
		t.Errorf("expected admin false permissions, got %+v", adminPerms)
	}
}

func TestWorkshopEmployee_CalculatePermissions(t *testing.T) {
	// Active Owner is always immutable full permissions
	owner := domain.WorkshopEmployee{
		ID:     uuid.New(),
		Role:   domain.EmployeeRoleOwner,
		Status: domain.EmployeeStatusActive,
		Permissions: &domain.EmployeePermissions{
			CanManageInventory: false, // Attempt to restrict owner
		},
	}
	perms := owner.CalculatePermissions()
	if !perms.CanManageInventory {
		t.Errorf("owner permissions must be immutable and remain true")
	}

	// Inactive Employee receives zero permissions
	inactiveMech := domain.WorkshopEmployee{
		ID:     uuid.New(),
		Role:   domain.EmployeeRoleMechanic,
		Status: domain.EmployeeStatusInactive,
	}
	inactivePerms := inactiveMech.CalculatePermissions()
	if inactivePerms.CanAccessRepairJobs || inactivePerms.CanUpdateJobStatus {
		t.Errorf("inactive employee must receive false for all permissions, got %+v", inactivePerms)
	}

	// Configurable ADMIN permissions
	customAdmin := domain.WorkshopEmployee{
		ID:     uuid.New(),
		Role:   domain.EmployeeRoleAdmin,
		Status: domain.EmployeeStatusActive,
		Permissions: &domain.EmployeePermissions{
			CanManageInventory: false, // toggled off
			CanAccessCashier:   true,  // toggled on
			CanIssueRefunds:    true,
		},
	}
	adminCalc := customAdmin.CalculatePermissions()
	if adminCalc.CanManageInventory {
		t.Errorf("expected CanManageInventory false from custom toggle, got true")
	}
	if !adminCalc.CanAccessCashier {
		t.Errorf("expected CanAccessCashier true from custom toggle, got false")
	}
}

func TestPermissionKeyValidation(t *testing.T) {
	keys := domain.ValidPermissionKeys()
	if len(keys) != 20 {
		t.Fatalf("expected exactly 20 valid permission keys, got %d", len(keys))
	}

	for _, k := range keys {
		if !domain.IsValidPermissionKey(k) {
			t.Errorf("expected %q to be recognized as valid permission key", k)
		}
	}

	if domain.IsValidPermissionKey("can_hack_system") {
		t.Errorf("expected unknown key to be invalid")
	}
}
