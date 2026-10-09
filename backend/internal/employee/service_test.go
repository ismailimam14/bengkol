package employee_test

import (
	"bytes"
	"context"
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
