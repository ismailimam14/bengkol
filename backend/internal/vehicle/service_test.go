package vehicle_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/vehicle"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

type mockVehicleRepo struct {
	mu       sync.Mutex
	vehicles map[uuid.UUID]*domain.Vehicle
}

func newMockVehicleRepo() *mockVehicleRepo {
	return &mockVehicleRepo{
		vehicles: make(map[uuid.UUID]*domain.Vehicle),
	}
}

func (m *mockVehicleRepo) Create(ctx context.Context, v *domain.Vehicle) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	copied := *v
	m.vehicles[v.ID] = &copied
	return nil
}

func (m *mockVehicleRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Vehicle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.vehicles[id]
	if !ok {
		return nil, vehicle.ErrVehicleNotFound
	}
	copied := *v
	return &copied, nil
}

func (m *mockVehicleRepo) ListByUserID(ctx context.Context, userID uuid.UUID, pagination domain.PaginationParams) ([]domain.Vehicle, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []domain.Vehicle
	for _, v := range m.vehicles {
		if v.UserID == userID {
			list = append(list, *v)
		}
	}
	return list, int64(len(list)), nil
}

func (m *mockVehicleRepo) Update(ctx context.Context, v *domain.Vehicle) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.vehicles[v.ID]; !ok {
		return vehicle.ErrVehicleNotFound
	}
	copied := *v
	m.vehicles[v.ID] = &copied
	return nil
}

func (m *mockVehicleRepo) Delete(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.vehicles[id]; !ok {
		return vehicle.ErrVehicleNotFound
	}
	delete(m.vehicles, id)
	return nil
}

func setupVehicleService() (vehicle.Service, *mockVehicleRepo) {
	repo := newMockVehicleRepo()
	log := logger.New("testing", "error")
	svc := vehicle.NewService(repo, log)
	return svc, repo
}

func TestVehicleService_CreateVehicle_Success(t *testing.T) {
	svc, repo := setupVehicleService()
	userID := uuid.New()

	req := vehicle.CreateVehicleRequest{
		LicensePlate: "b 1234 cd",
		Brand:        "Honda",
		Model:        "Vario 160",
		Year:         2023,
		VehicleType:  domain.VehicleTypeMotorcycle,
		Color:        "Matte Black",
		Notes:        "Daily commuter",
	}

	veh, valErrors, err := svc.CreateVehicle(context.Background(), userID, req)
	if err != nil {
		t.Fatalf("expected vehicle creation to succeed, got %v (%v)", err, valErrors)
	}

	if veh.LicensePlate != "B 1234 CD" {
		t.Errorf("expected uppercase license plate 'B 1234 CD', got '%s'", veh.LicensePlate)
	}
	if veh.UserID != userID {
		t.Errorf("expected userID %s, got %s", userID, veh.UserID)
	}
	if veh.VehicleType != domain.VehicleTypeMotorcycle {
		t.Errorf("expected vehicle_type MOTORCYCLE, got %s", veh.VehicleType)
	}

	// Verify in repository
	if _, ok := repo.vehicles[veh.ID]; !ok {
		t.Errorf("expected vehicle %s to be saved in repository", veh.ID)
	}
}

func TestVehicleService_CreateVehicle_ValidationErrors(t *testing.T) {
	svc, _ := setupVehicleService()
	userID := uuid.New()

	// Missing required fields
	req := vehicle.CreateVehicleRequest{
		LicensePlate: "",
		Brand:        "",
		Model:        "",
		Year:         1800, // Invalid year
		VehicleType:  "INVALID_TYPE",
	}

	_, valErrors, err := svc.CreateVehicle(context.Background(), userID, req)
	if !errors.Is(err, vehicle.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed, got %v", err)
	}

	if valErrors["license_plate"] == "" {
		t.Errorf("expected error for license_plate")
	}
	if valErrors["brand"] == "" {
		t.Errorf("expected error for brand")
	}
	if valErrors["model"] == "" {
		t.Errorf("expected error for model")
	}
	if valErrors["year"] == "" {
		t.Errorf("expected error for year")
	}
	if valErrors["vehicle_type"] == "" {
		t.Errorf("expected error for vehicle_type")
	}
}

func TestVehicleService_GetVehicleByID(t *testing.T) {
	svc, repo := setupVehicleService()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	vehID := uuid.New()

	repo.vehicles[vehID] = &domain.Vehicle{
		ID:           vehID,
		UserID:       ownerID,
		LicensePlate: "B 1111 AA",
		Brand:        "Toyota",
		Model:        "Avanza",
		VehicleType:  domain.VehicleTypeCar,
	}

	// 1. Owner can view
	v, err := svc.GetVehicleByID(context.Background(), vehID, ownerID, domain.RoleCustomer)
	if err != nil {
		t.Fatalf("expected owner to view vehicle, got error: %v", err)
	}
	if v.ID != vehID {
		t.Errorf("expected vehicle ID %s, got %s", vehID, v.ID)
	}

	// 2. Other user cannot view (ErrForbidden)
	_, err = svc.GetVehicleByID(context.Background(), vehID, otherUserID, domain.RoleCustomer)
	if !errors.Is(err, vehicle.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for other user, got: %v", err)
	}

	// 3. Admin can view other user's vehicle
	adminID := uuid.New()
	vAdmin, err := svc.GetVehicleByID(context.Background(), vehID, adminID, domain.RoleAdmin)
	if err != nil {
		t.Fatalf("expected admin to view vehicle, got error: %v", err)
	}
	if vAdmin.ID != vehID {
		t.Errorf("expected vehicle ID %s, got %s", vehID, vAdmin.ID)
	}

	// 4. Not found
	_, err = svc.GetVehicleByID(context.Background(), uuid.New(), ownerID, domain.RoleCustomer)
	if !errors.Is(err, vehicle.ErrVehicleNotFound) {
		t.Fatalf("expected ErrVehicleNotFound, got %v", err)
	}
}

func TestVehicleService_ListMyVehicles(t *testing.T) {
	svc, repo := setupVehicleService()
	user1 := uuid.New()
	user2 := uuid.New()

	repo.vehicles[uuid.New()] = &domain.Vehicle{ID: uuid.New(), UserID: user1, LicensePlate: "B 1 AA"}
	repo.vehicles[uuid.New()] = &domain.Vehicle{ID: uuid.New(), UserID: user1, LicensePlate: "B 2 BB"}
	repo.vehicles[uuid.New()] = &domain.Vehicle{ID: uuid.New(), UserID: user2, LicensePlate: "B 3 CC"}

	list, meta, err := svc.ListMyVehicles(context.Background(), user1, domain.PaginationParams{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error listing vehicles: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 vehicles for user1, got %d", len(list))
	}
	if meta.TotalItems != 2 {
		t.Errorf("expected meta.TotalItems 2, got %d", meta.TotalItems)
	}
}

func TestVehicleService_UpdateVehicle(t *testing.T) {
	svc, repo := setupVehicleService()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	vehID := uuid.New()

	repo.vehicles[vehID] = &domain.Vehicle{
		ID:           vehID,
		UserID:       ownerID,
		LicensePlate: "B 1234 OLD",
		Brand:        "Honda",
		Model:        "Beat",
		Year:         2020,
		VehicleType:  domain.VehicleTypeMotorcycle,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	// Forbidden if other user
	newPlate := "B 5678 NEW"
	_, _, err := svc.UpdateVehicle(context.Background(), vehID, otherUserID, domain.RoleCustomer, vehicle.UpdateVehicleRequest{
		LicensePlate: &newPlate,
	})
	if !errors.Is(err, vehicle.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for other user update, got %v", err)
	}

	// Owner update success
	newBrand := "Honda Upgraded"
	updated, _, err := svc.UpdateVehicle(context.Background(), vehID, ownerID, domain.RoleCustomer, vehicle.UpdateVehicleRequest{
		LicensePlate: &newPlate,
		Brand:        &newBrand,
	})
	if err != nil {
		t.Fatalf("expected update to succeed, got %v", err)
	}

	if updated.LicensePlate != "B 5678 NEW" {
		t.Errorf("expected updated plate 'B 5678 NEW', got %s", updated.LicensePlate)
	}
	if updated.Brand != "Honda Upgraded" {
		t.Errorf("expected updated brand 'Honda Upgraded', got %s", updated.Brand)
	}
}

func TestVehicleService_DeleteVehicle(t *testing.T) {
	svc, repo := setupVehicleService()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	vehID := uuid.New()

	repo.vehicles[vehID] = &domain.Vehicle{
		ID:           vehID,
		UserID:       ownerID,
		LicensePlate: "B 9999 ZZ",
	}

	// Forbidden for other user
	err := svc.DeleteVehicle(context.Background(), vehID, otherUserID, domain.RoleCustomer)
	if !errors.Is(err, vehicle.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	// Owner can delete
	err = svc.DeleteVehicle(context.Background(), vehID, ownerID, domain.RoleCustomer)
	if err != nil {
		t.Fatalf("expected successful delete, got %v", err)
	}

	if _, ok := repo.vehicles[vehID]; ok {
		t.Errorf("expected vehicle to be removed from repository")
	}
}
