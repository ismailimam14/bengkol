package employee_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/employee"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

type mockEmployeeRepo struct {
	workshopOwners map[uuid.UUID]uuid.UUID
	employees      map[uuid.UUID]map[uuid.UUID]*domain.WorkshopEmployee
	users          map[string]*domain.User
	usersByPhone   map[string]*domain.User
	usersByID      map[uuid.UUID]*domain.User
}

func newMockEmployeeRepo() *mockEmployeeRepo {
	return &mockEmployeeRepo{
		workshopOwners: make(map[uuid.UUID]uuid.UUID),
		employees:      make(map[uuid.UUID]map[uuid.UUID]*domain.WorkshopEmployee),
		users:          make(map[string]*domain.User),
		usersByPhone:   make(map[string]*domain.User),
		usersByID:      make(map[uuid.UUID]*domain.User),
	}
}

func (m *mockEmployeeRepo) GetWorkshopOwnerID(ctx context.Context, workshopID uuid.UUID) (uuid.UUID, error) {
	ownerID, ok := m.workshopOwners[workshopID]
	if !ok {
		return uuid.Nil, employee.ErrWorkshopNotFound
	}
	return ownerID, nil
}

func (m *mockEmployeeRepo) Create(ctx context.Context, emp *domain.WorkshopEmployee) error {
	if _, ok := m.employees[emp.WorkshopID]; !ok {
		m.employees[emp.WorkshopID] = make(map[uuid.UUID]*domain.WorkshopEmployee)
	}
	copied := *emp
	m.employees[emp.WorkshopID][emp.ID] = &copied
	return nil
}

func (m *mockEmployeeRepo) GetByID(ctx context.Context, workshopID, employeeID uuid.UUID) (*domain.WorkshopEmployee, error) {
	wsEmps, ok := m.employees[workshopID]
	if !ok {
		return nil, employee.ErrEmployeeNotFound
	}
	emp, ok := wsEmps[employeeID]
	if !ok {
		return nil, employee.ErrEmployeeNotFound
	}
	copied := *emp
	return &copied, nil
}

func (m *mockEmployeeRepo) GetByUserID(ctx context.Context, workshopID, userID uuid.UUID) (*domain.WorkshopEmployee, error) {
	wsEmps, ok := m.employees[workshopID]
	if !ok {
		return nil, employee.ErrEmployeeNotFound
	}
	for _, emp := range wsEmps {
		if emp.UserID != nil && *emp.UserID == userID {
			copied := *emp
			return &copied, nil
		}
	}
	return nil, employee.ErrEmployeeNotFound
}

func (m *mockEmployeeRepo) List(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams, filter employee.EmployeeFilter) ([]domain.WorkshopEmployee, int64, error) {
	wsEmps, ok := m.employees[workshopID]
	if !ok {
		return []domain.WorkshopEmployee{}, 0, nil
	}
	var list []domain.WorkshopEmployee
	for _, emp := range wsEmps {
		if filter.Role != nil && emp.Role != *filter.Role {
			continue
		}
		if filter.Status != nil && emp.Status != *filter.Status {
			continue
		}
		list = append(list, *emp)
	}
	total := int64(len(list))
	start := pagination.Offset()
	if start >= len(list) {
		return []domain.WorkshopEmployee{}, total, nil
	}
	end := start + pagination.PageSize
	if end > len(list) {
		end = len(list)
	}
	return list[start:end], total, nil
}

func (m *mockEmployeeRepo) Update(ctx context.Context, emp *domain.WorkshopEmployee) error {
	wsEmps, ok := m.employees[emp.WorkshopID]
	if !ok {
		return employee.ErrEmployeeNotFound
	}
	if _, ok := wsEmps[emp.ID]; !ok {
		return employee.ErrEmployeeNotFound
	}
	copied := *emp
	wsEmps[emp.ID] = &copied
	return nil
}

func (m *mockEmployeeRepo) Delete(ctx context.Context, workshopID, employeeID uuid.UUID) error {
	wsEmps, ok := m.employees[workshopID]
	if !ok {
		return employee.ErrEmployeeNotFound
	}
	if _, ok := wsEmps[employeeID]; !ok {
		return employee.ErrEmployeeNotFound
	}
	delete(wsEmps, employeeID)
	return nil
}

func (m *mockEmployeeRepo) FindUserByPhone(ctx context.Context, phone string) (*domain.User, error) {
	u, ok := m.usersByPhone[phone]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockEmployeeRepo) FindUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockEmployeeRepo) CreateUser(ctx context.Context, user *domain.User) error {
	m.usersByID[user.ID] = user
	if user.Phone != "" {
		m.usersByPhone[user.Phone] = user
	}
	if user.Email != "" {
		m.users[user.Email] = user
	}
	return nil
}

func (m *mockEmployeeRepo) IsPhoneRegisteredAsOwner(ctx context.Context, phone string) (bool, error) {
	norm := domain.NormalizePhone(phone)
	for _, u := range m.usersByPhone {
		if u.Role == domain.RoleOwner && domain.NormalizePhone(u.Phone) == norm {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockEmployeeRepo) IsPhoneRegisteredAsEmployee(ctx context.Context, phone string) (bool, error) {
	norm := domain.NormalizePhone(phone)
	for _, wsMap := range m.employees {
		for _, e := range wsMap {
			if domain.NormalizePhone(e.Phone) == norm {
				return true, nil
			}
		}
	}
	return false, nil
}

func setupEmployeeService() (employee.Service, *mockEmployeeRepo) {
	repo := newMockEmployeeRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	svc := employee.NewService(repo, log)
	return svc, repo
}

func TestEmployeeService_CreateEmployee(t *testing.T) {
	svc, repo := setupEmployeeService()
	ctx := context.Background()

	workshopID := uuid.New()
	ownerUserID := uuid.New()
	repo.workshopOwners[workshopID] = ownerUserID

	// 1. Owner creates a Mechanic
	req := employee.CreateEmployeeRequest{
		Name:           "Budi Montir",
		Phone:          "08123456789",
		Role:           "MECHANIC",
		Specialization: "Brakes & Suspension",
	}
	emp, valErrors, err := svc.CreateEmployee(ctx, workshopID, ownerUserID, domain.RoleOwner, req)
	if err != nil {
		t.Fatalf("unexpected error creating mechanic: %v (valErrors: %v)", err, valErrors)
	}
	if emp.Role != domain.EmployeeRoleMechanic {
		t.Errorf("expected role MECHANIC, got %s", emp.Role)
	}
	if emp.Permissions == nil || !emp.Permissions.CanAccessRepairJobs || emp.Permissions.CanManageInventory {
		t.Errorf("incorrect permissions for mechanic: %+v", emp.Permissions)
	}
	if emp.InitialPassword == "" {
		t.Errorf("expected initial password to be generated")
	}
	if emp.UserID == nil {
		t.Errorf("expected user ID to be assigned to new employee")
	} else if repo.usersByPhone[emp.Phone] == nil {
		t.Errorf("expected user account to be created for employee")
	}

	// 2. Setup a Manager employee with a user ID
	managerUserID := uuid.New()
	managerEmp := &domain.WorkshopEmployee{
		ID:         uuid.New(),
		WorkshopID: workshopID,
		UserID:     &managerUserID,
		Name:       "Siti Manager",
		Phone:      "08987654321",
		Role:       domain.EmployeeRoleManager,
		Status:     domain.EmployeeStatusActive,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_ = repo.Create(ctx, managerEmp)

	// 3. Manager creates an Admin Cashier
	reqCashier := employee.CreateEmployeeRequest{
		Name:  "Dewi Kasir",
		Phone: "0811223344",
		Role:  "ADMIN_CASHIER",
	}
	cashierEmp, _, err := svc.CreateEmployee(ctx, workshopID, managerUserID, domain.RoleCustomer, reqCashier)
	if err != nil {
		t.Fatalf("manager failed to create cashier: %v", err)
	}
	if cashierEmp.Role != domain.EmployeeRoleAdminCashier {
		t.Errorf("expected role ADMIN_CASHIER, got %s", cashierEmp.Role)
	}
	if !cashierEmp.Permissions.CanAccessCashier || cashierEmp.Permissions.CanManageInventory {
		t.Errorf("incorrect permissions for cashier: %+v", cashierEmp.Permissions)
	}

	// 4. Manager tries to create an Owner role -> should fail
	reqOwner := employee.CreateEmployeeRequest{
		Name:  "Fake Owner",
		Phone: "0822334455",
		Role:  "OWNER",
	}
	_, _, err = svc.CreateEmployee(ctx, workshopID, managerUserID, domain.RoleCustomer, reqOwner)
	if !errors.Is(err, employee.ErrValidationFailed) {
		t.Errorf("expected validation failure when manager creates OWNER role, got %v", err)
	}

	// 5. Unauthorized random customer tries to create employee -> ErrForbidden
	randomUser := uuid.New()
	_, _, err = svc.CreateEmployee(ctx, workshopID, randomUser, domain.RoleCustomer, req)
	if !errors.Is(err, employee.ErrForbidden) {
		t.Errorf("expected ErrForbidden for random user, got %v", err)
	}
}

func TestEmployeeService_UpdateAndDelete(t *testing.T) {
	svc, repo := setupEmployeeService()
	ctx := context.Background()

	workshopID := uuid.New()
	ownerUserID := uuid.New()
	repo.workshopOwners[workshopID] = ownerUserID

	// Create an Owner employee record
	ownerEmp := &domain.WorkshopEmployee{
		ID:         uuid.New(),
		WorkshopID: workshopID,
		UserID:     &ownerUserID,
		Name:       "Workshop Owner Person",
		Phone:      "0811111111",
		Role:       domain.EmployeeRoleOwner,
		Status:     domain.EmployeeStatusActive,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_ = repo.Create(ctx, ownerEmp)

	// Create Manager
	managerUserID := uuid.New()
	managerEmp := &domain.WorkshopEmployee{
		ID:         uuid.New(),
		WorkshopID: workshopID,
		UserID:     &managerUserID,
		Name:       "Workshop Manager Person",
		Phone:      "0822222222",
		Role:       domain.EmployeeRoleManager,
		Status:     domain.EmployeeStatusActive,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_ = repo.Create(ctx, managerEmp)

	// Create Mechanic
	mechanicEmp := &domain.WorkshopEmployee{
		ID:         uuid.New(),
		WorkshopID: workshopID,
		Name:       "Mechanic Person",
		Phone:      "0833333333",
		Role:       domain.EmployeeRoleMechanic,
		Status:     domain.EmployeeStatusActive,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_ = repo.Create(ctx, mechanicEmp)

	// 1. Manager tries to modify Owner -> ErrCannotModifyOwner
	newPhone := "0899999999"
	_, _, err := svc.UpdateEmployee(ctx, workshopID, ownerEmp.ID, managerUserID, domain.RoleCustomer, employee.UpdateEmployeeRequest{
		Phone: &newPhone,
	})
	if !errors.Is(err, employee.ErrCannotModifyOwner) {
		t.Errorf("expected ErrCannotModifyOwner, got %v", err)
	}

	// 2. Manager tries to delete Owner -> ErrCannotModifyOwner
	err = svc.DeleteEmployee(ctx, workshopID, ownerEmp.ID, managerUserID, domain.RoleCustomer)
	if !errors.Is(err, employee.ErrCannotModifyOwner) {
		t.Errorf("expected ErrCannotModifyOwner, got %v", err)
	}

	// 3. Manager tries to delete themselves -> ErrCannotDeleteSelf
	err = svc.DeleteEmployee(ctx, workshopID, managerEmp.ID, managerUserID, domain.RoleCustomer)
	if !errors.Is(err, employee.ErrCannotDeleteSelf) {
		t.Errorf("expected ErrCannotDeleteSelf, got %v", err)
	}

	// 4. Manager updates Mechanic to ADMIN_BOTH
	newRole := "ADMIN_BOTH"
	updated, _, err := svc.UpdateEmployee(ctx, workshopID, mechanicEmp.ID, managerUserID, domain.RoleCustomer, employee.UpdateEmployeeRequest{
		Role: &newRole,
	})
	if err != nil {
		t.Fatalf("unexpected error updating mechanic: %v", err)
	}
	if updated.Role != domain.EmployeeRoleAdminBoth {
		t.Errorf("expected role ADMIN_BOTH, got %s", updated.Role)
	}
	if !updated.Permissions.CanAccessCashier || !updated.Permissions.CanManageInventory {
		t.Errorf("expected permissions for ADMIN_BOTH, got %+v", updated.Permissions)
	}

	// 5. Owner deletes Mechanic -> success
	err = svc.DeleteEmployee(ctx, workshopID, mechanicEmp.ID, ownerUserID, domain.RoleOwner)
	if err != nil {
		t.Fatalf("unexpected error deleting employee: %v", err)
	}

	// Verify deleted
	_, err = svc.GetEmployeeByID(ctx, workshopID, mechanicEmp.ID, ownerUserID, domain.RoleOwner)
	if !errors.Is(err, employee.ErrEmployeeNotFound) {
		t.Errorf("expected ErrEmployeeNotFound after deletion, got %v", err)
	}
}

func TestEmployeeService_GetCurrentEmployee(t *testing.T) {
	svc, repo := setupEmployeeService()
	ctx := context.Background()

	workshopID := uuid.New()
	ownerUserID := uuid.New()
	repo.workshopOwners[workshopID] = ownerUserID

	// 1. Owner requests their own employee record when no row exists -> returns synthetic owner
	emp, err := svc.GetCurrentEmployee(ctx, workshopID, ownerUserID)
	if err != nil {
		t.Fatalf("unexpected error getting current employee for owner: %v", err)
	}
	if emp.Role != domain.EmployeeRoleOwner {
		t.Errorf("expected OWNER, got %s", emp.Role)
	}
	if !emp.Permissions.CanManageEmployees || !emp.Permissions.CanManageWorkshopOperations {
		t.Errorf("expected full owner permissions, got %+v", emp.Permissions)
	}

	// 2. Non-employee requests -> ErrEmployeeNotFound
	unknownUser := uuid.New()
	_, err = svc.GetCurrentEmployee(ctx, workshopID, unknownUser)
	if !errors.Is(err, employee.ErrEmployeeNotFound) {
		t.Errorf("expected ErrEmployeeNotFound, got %v", err)
	}
}

func TestEmployeeService_PhoneSeparation(t *testing.T) {
	svc, repo := setupEmployeeService()
	ctx := context.Background()

	workshopID := uuid.New()
	ownerUserID := uuid.New()
	repo.workshopOwners[workshopID] = ownerUserID

	// Register an existing owner user with phone 081299990001
	ownerPhone := "081299990001"
	repo.usersByPhone[ownerPhone] = &domain.User{
		ID:    ownerUserID,
		Phone: ownerPhone,
		Role:  domain.RoleOwner,
	}

	// Another workshop owner with phone 081299990002
	otherOwnerPhone := "081299990002"
	repo.usersByPhone[otherOwnerPhone] = &domain.User{
		ID:    uuid.New(),
		Phone: otherOwnerPhone,
		Role:  domain.RoleOwner,
	}

	// 1. Try to create employee with phone matching current workshop owner
	reqSameAsOwner := employee.CreateEmployeeRequest{
		Name:  "Employee Same As Owner",
		Phone: ownerPhone,
		Role:  "MECHANIC",
	}
	_, valErrors, err := svc.CreateEmployee(ctx, workshopID, ownerUserID, domain.RoleOwner, reqSameAsOwner)
	if !errors.Is(err, employee.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed, got %v", err)
	}
	if valErrors["phone"] == "" {
		t.Errorf("expected phone validation error for employee matching owner, got %v", valErrors)
	}

	// 2. Try to create employee with phone registered as owner in another workshop (+62 format)
	reqOtherOwner := employee.CreateEmployeeRequest{
		Name:  "Employee Other Owner",
		Phone: "+62 812 9999 0002",
		Role:  "MECHANIC",
	}
	_, valErrors, err = svc.CreateEmployee(ctx, workshopID, ownerUserID, domain.RoleOwner, reqOtherOwner)
	if !errors.Is(err, employee.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed, got %v", err)
	}
	if valErrors["phone"] == "" {
		t.Errorf("expected phone validation error for employee registered as owner, got %v", valErrors)
	}

	// 3. Create valid employee, then try to update phone to owner's phone
	reqValid := employee.CreateEmployeeRequest{
		Name:  "Valid Employee",
		Phone: "081233334444",
		Role:  "MECHANIC",
	}
	emp, _, err := svc.CreateEmployee(ctx, workshopID, ownerUserID, domain.RoleOwner, reqValid)
	if err != nil {
		t.Fatalf("failed to create valid employee: %v", err)
	}

	updatePhone := ownerPhone
	_, valErrors, err = svc.UpdateEmployee(ctx, workshopID, emp.ID, ownerUserID, domain.RoleOwner, employee.UpdateEmployeeRequest{
		Phone: &updatePhone,
	})
	if !errors.Is(err, employee.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed on phone update, got %v", err)
	}
	if valErrors["phone"] == "" {
		t.Errorf("expected phone error on update, got %v", valErrors)
	}
}

func TestEmployeeService_CustomAdminPermissions(t *testing.T) {
	svc, repo := setupEmployeeService()
	ctx := context.Background()

	workshopID := uuid.New()
	ownerUserID := uuid.New()
	repo.workshopOwners[workshopID] = ownerUserID

	// Owner creates an ADMIN employee with custom granular permissions
	customPerms := domain.EmployeePermissions{
		CanAccessCashier:    true,
		CanIssueRefunds:     true,
		CanManageDiscounts:  true,
		CanManageInventory:  false, // customized to false
		CanManageCustomers:  true,
	}
	req := employee.CreateEmployeeRequest{
		Name:        "Custom Admin",
		Phone:       "08155555555",
		Role:        "ADMIN",
		Permissions: &customPerms,
	}

	emp, valErrors, err := svc.CreateEmployee(ctx, workshopID, ownerUserID, domain.RoleOwner, req)
	if err != nil {
		t.Fatalf("unexpected error creating custom admin: %v (valErrors: %v)", err, valErrors)
	}

	if emp.Role != domain.EmployeeRoleAdmin {
		t.Errorf("expected role ADMIN, got %s", emp.Role)
	}
	if emp.Permissions == nil {
		t.Fatalf("expected non-nil permissions")
	}
	if !emp.Permissions.CanAccessCashier || emp.Permissions.CanManageInventory {
		t.Errorf("expected customized permissions: CanAccessCashier=true, CanManageInventory=false, got %+v", emp.Permissions)
	}

	// Owner updates custom permissions
	updatedPerms := customPerms
	updatedPerms.CanManageInventory = true
	updated, _, err := svc.UpdateEmployee(ctx, workshopID, emp.ID, ownerUserID, domain.RoleOwner, employee.UpdateEmployeeRequest{
		Permissions: &updatedPerms,
	})
	if err != nil {
		t.Fatalf("unexpected error updating permissions: %v", err)
	}
	if !updated.Permissions.CanManageInventory {
		t.Errorf("expected CanManageInventory to be updated to true")
	}
}

func TestEmployeeService_UnknownPermissionKeys(t *testing.T) {
	svc, repo := setupEmployeeService()
	ctx := context.Background()

	workshopID := uuid.New()
	ownerUserID := uuid.New()
	repo.workshopOwners[workshopID] = ownerUserID

	// 1. JSON unmarshal with unknown permission key for CreateEmployee
	bodyCreate := []byte(`{
		"name": "Test User",
		"phone": "08166666666",
		"role": "ADMIN",
		"permissions": {
			"can_access_cashier": true,
			"can_fly": true
		}
	}`)
	var reqCreate employee.CreateEmployeeRequest
	if err := json.Unmarshal(bodyCreate, &reqCreate); err != nil {
		t.Fatalf("unexpected JSON unmarshal error: %v", err)
	}

	_, valErrors, err := svc.CreateEmployee(ctx, workshopID, ownerUserID, domain.RoleOwner, reqCreate)
	if !errors.Is(err, employee.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed for unknown permission key, got %v", err)
	}
	if valErrors["permissions.can_fly"] == "" {
		t.Errorf("expected validation error for permissions.can_fly, got %v", valErrors)
	}

	// 2. JSON unmarshal with unknown permission key for UpdateEmployee
	bodyUpdate := []byte(`{
		"permissions": {
			"can_access_cashier": true,
			"super_admin_mode": true
		}
	}`)
	var reqUpdate employee.UpdateEmployeeRequest
	if err := json.Unmarshal(bodyUpdate, &reqUpdate); err != nil {
		t.Fatalf("unexpected JSON unmarshal error: %v", err)
	}

	// Create valid employee first
	validEmp, _, _ := svc.CreateEmployee(ctx, workshopID, ownerUserID, domain.RoleOwner, employee.CreateEmployeeRequest{
		Name:  "Valid Admin",
		Phone: "08177777777",
		Role:  "ADMIN",
	})

	_, valErrors, err = svc.UpdateEmployee(ctx, workshopID, validEmp.ID, ownerUserID, domain.RoleOwner, reqUpdate)
	if !errors.Is(err, employee.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed for unknown permission key on update, got %v", err)
	}
	if valErrors["permissions.super_admin_mode"] == "" {
		t.Errorf("expected validation error for permissions.super_admin_mode, got %v", valErrors)
	}
}

func TestEmployeeService_AntiEscalation(t *testing.T) {
	svc, repo := setupEmployeeService()
	ctx := context.Background()

	workshopID := uuid.New()
	ownerUserID := uuid.New()
	repo.workshopOwners[workshopID] = ownerUserID

	// Setup Manager 1 without CanManageRolesAndPermissions
	mgrUser1 := uuid.New()
	mgrPerms1 := domain.DefaultPermissionsForRole(domain.EmployeeRoleManager)
	mgrPerms1.CanManageRolesAndPermissions = false
	mgrEmp1 := &domain.WorkshopEmployee{
		ID:          uuid.New(),
		WorkshopID:  workshopID,
		UserID:      &mgrUser1,
		Name:        "Manager Without Perms Mgmt",
		Phone:       "08188888001",
		Role:        domain.EmployeeRoleManager,
		Status:      domain.EmployeeStatusActive,
		Permissions: &mgrPerms1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_ = repo.Create(ctx, mgrEmp1)

	// Manager 1 tries to set custom permissions on employee creation -> fails
	perms := domain.DefaultPermissionsForRole(domain.EmployeeRoleAdmin)
	req1 := employee.CreateEmployeeRequest{
		Name:        "Test Employee",
		Phone:       "08188888002",
		Role:        "ADMIN",
		Permissions: &perms,
	}
	_, valErrors, err := svc.CreateEmployee(ctx, workshopID, mgrUser1, domain.RoleCustomer, req1)
	if !errors.Is(err, employee.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed when manager lacks CanManageRolesAndPermissions, got %v", err)
	}
	if valErrors["permissions"] == "" {
		t.Errorf("expected error on permissions, got %v", valErrors)
	}

	// Setup Manager 2 WITH CanManageRolesAndPermissions, but without CanTransferOwnership
	mgrUser2 := uuid.New()
	mgrPerms2 := domain.DefaultPermissionsForRole(domain.EmployeeRoleManager)
	mgrPerms2.CanManageRolesAndPermissions = true
	mgrPerms2.CanTransferOwnership = false
	mgrEmp2 := &domain.WorkshopEmployee{
		ID:          uuid.New(),
		WorkshopID:  workshopID,
		UserID:      &mgrUser2,
		Name:        "Manager With Perms Mgmt",
		Phone:       "08188888003",
		Role:        domain.EmployeeRoleManager,
		Status:      domain.EmployeeStatusActive,
		Permissions: &mgrPerms2,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_ = repo.Create(ctx, mgrEmp2)

	// Manager 2 tries to grant CanTransferOwnership which exceeds their own authority -> fails
	escalatedPerms := domain.DefaultPermissionsForRole(domain.EmployeeRoleAdmin)
	escalatedPerms.CanTransferOwnership = true
	req2 := employee.CreateEmployeeRequest{
		Name:        "Escalated Employee",
		Phone:       "08188888004",
		Role:        "ADMIN",
		Permissions: &escalatedPerms,
	}
	_, valErrors, err = svc.CreateEmployee(ctx, workshopID, mgrUser2, domain.RoleCustomer, req2)
	if !errors.Is(err, employee.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed on privilege escalation, got %v", err)
	}
	if valErrors["permissions.can_transfer_ownership"] == "" {
		t.Errorf("expected error on permissions.can_transfer_ownership, got %v", valErrors)
	}
}

func TestEmployeeService_OwnerPermissionImmutability(t *testing.T) {
	svc, repo := setupEmployeeService()
	ctx := context.Background()

	workshopID := uuid.New()
	ownerUserID := uuid.New()
	repo.workshopOwners[workshopID] = ownerUserID

	// Create Owner employee record
	ownerEmp := &domain.WorkshopEmployee{
		ID:         uuid.New(),
		WorkshopID: workshopID,
		UserID:     &ownerUserID,
		Name:       "Owner Employee",
		Phone:      "08199999001",
		Role:       domain.EmployeeRoleOwner,
		Status:     domain.EmployeeStatusActive,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_ = repo.Create(ctx, ownerEmp)

	// Attempt to customize or downgrade Owner permissions -> rejected
	downgradedPerms := domain.DefaultPermissionsForRole(domain.EmployeeRoleMechanic)
	_, valErrors, err := svc.UpdateEmployee(ctx, workshopID, ownerEmp.ID, ownerUserID, domain.RoleOwner, employee.UpdateEmployeeRequest{
		Permissions: &downgradedPerms,
	})
	if !errors.Is(err, employee.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed when modifying owner permissions, got %v", err)
	}
	if valErrors["permissions"] == "" {
		t.Errorf("expected validation error for immutable owner permissions, got %v", valErrors)
	}
}

