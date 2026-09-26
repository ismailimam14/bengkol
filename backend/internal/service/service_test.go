package service_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/service"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

type mockServiceRepo struct {
	services       map[uuid.UUID]*domain.Service
	workshopOwners map[uuid.UUID]uuid.UUID // workshopID -> ownerID
}

func newMockServiceRepo() *mockServiceRepo {
	return &mockServiceRepo{
		services:       make(map[uuid.UUID]*domain.Service),
		workshopOwners: make(map[uuid.UUID]uuid.UUID),
	}
}

func (m *mockServiceRepo) Create(ctx context.Context, s *domain.Service) error {
	m.services[s.ID] = s
	return nil
}

func (m *mockServiceRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Service, error) {
	s, ok := m.services[id]
	if !ok {
		return nil, service.ErrServiceNotFound
	}
	copied := *s
	return &copied, nil
}

func (m *mockServiceRepo) ListByWorkshopID(ctx context.Context, workshopID uuid.UUID, onlyActive bool) ([]domain.Service, error) {
	var list []domain.Service
	for _, s := range m.services {
		if s.WorkshopID == workshopID {
			if onlyActive && !s.IsActive {
				continue
			}
			list = append(list, *s)
		}
	}
	return list, nil
}

func (m *mockServiceRepo) Update(ctx context.Context, s *domain.Service) error {
	if _, ok := m.services[s.ID]; !ok {
		return service.ErrServiceNotFound
	}
	m.services[s.ID] = s
	return nil
}

func (m *mockServiceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := m.services[id]; !ok {
		return service.ErrServiceNotFound
	}
	delete(m.services, id)
	return nil
}

func (m *mockServiceRepo) GetWorkshopOwnerID(ctx context.Context, workshopID uuid.UUID) (uuid.UUID, error) {
	ownerID, ok := m.workshopOwners[workshopID]
	if !ok {
		return uuid.Nil, errors.New("workshop not found")
	}
	return ownerID, nil
}

func (m *mockServiceRepo) GetServiceOwnerID(ctx context.Context, serviceID uuid.UUID) (uuid.UUID, error) {
	s, ok := m.services[serviceID]
	if !ok {
		return uuid.Nil, service.ErrServiceNotFound
	}
	ownerID, ok := m.workshopOwners[s.WorkshopID]
	if !ok {
		return uuid.Nil, errors.New("workshop owner not found")
	}
	return ownerID, nil
}

func setupServiceUseCase() (service.Service, *mockServiceRepo) {
	repo := newMockServiceRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	svc := service.NewService(repo, log)
	return svc, repo
}

func TestServiceUseCase_Create_Success(t *testing.T) {
	svc, repo := setupServiceUseCase()
	ownerID := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerID

	req := service.CreateServiceRequest{
		Name:            "Ganti Oli Mesin",
		Description:     "Penggantian oli mesin dan filter oli",
		Price:           50000,
		DurationMinutes: 30,
	}

	srv, valErrors, err := svc.CreateService(context.Background(), wsID, ownerID, domain.RoleOwner, req)
	if err != nil {
		t.Fatalf("unexpected error creating service: %v", err)
	}

	if valErrors != nil {
		t.Fatalf("expected no validation errors, got %+v", valErrors)
	}

	if srv.Name != "Ganti Oli Mesin" || srv.Price != 50000 || srv.DurationMinutes != 30 {
		t.Errorf("service fields mismatch: %+v", srv)
	}

	if !srv.IsActive {
		t.Errorf("expected new service to be active by default")
	}
}

func TestServiceUseCase_Create_OwnershipForbidden(t *testing.T) {
	svc, repo := setupServiceUseCase()
	ownerA := uuid.New()
	ownerB := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerA

	req := service.CreateServiceRequest{
		Name:            "Tune Up",
		Price:           150000,
		DurationMinutes: 60,
	}

	// Owner B tries to create service in Owner A's workshop
	_, _, err := svc.CreateService(context.Background(), wsID, ownerB, domain.RoleOwner, req)
	if !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected ErrForbidden when creating service in non-owned workshop, got %v", err)
	}
}

func TestServiceUseCase_Create_ValidationErrors(t *testing.T) {
	svc, repo := setupServiceUseCase()
	ownerID := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerID

	req := service.CreateServiceRequest{
		Name:            "",
		Price:           -1000,
		DurationMinutes: 0,
	}

	_, valErrors, err := svc.CreateService(context.Background(), wsID, ownerID, domain.RoleOwner, req)
	if !errors.Is(err, service.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed, got %v", err)
	}

	if _, ok := valErrors["name"]; !ok {
		t.Errorf("expected validation error on name")
	}
	if _, ok := valErrors["price"]; !ok {
		t.Errorf("expected validation error on price")
	}
	if _, ok := valErrors["duration_minutes"]; !ok {
		t.Errorf("expected validation error on duration_minutes")
	}
}

func TestServiceUseCase_ListByWorkshop_ActiveFilter(t *testing.T) {
	svc, repo := setupServiceUseCase()
	ownerID := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerID

	id1 := uuid.New()
	id2 := uuid.New()
	repo.services[id1] = &domain.Service{ID: id1, WorkshopID: wsID, Name: "Active Service", IsActive: true, CreatedAt: time.Now()}
	repo.services[id2] = &domain.Service{ID: id2, WorkshopID: wsID, Name: "Inactive Service", IsActive: false, CreatedAt: time.Now()}

	// 1. Customer (public) should only see 1 active service
	custRole := domain.RoleCustomer
	custID := uuid.New()
	custServices, err := svc.GetServicesByWorkshop(context.Background(), wsID, &custRole, &custID)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(custServices) != 1 || custServices[0].Name != "Active Service" {
		t.Errorf("expected 1 active service for customer, got %d", len(custServices))
	}

	// 2. Owner of the workshop should see all 2 services (active + inactive)
	ownerRole := domain.RoleOwner
	ownerServices, err := svc.GetServicesByWorkshop(context.Background(), wsID, &ownerRole, &ownerID)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(ownerServices) != 2 {
		t.Errorf("expected 2 services for owner, got %d", len(ownerServices))
	}
}

func TestServiceUseCase_Update_Ownership(t *testing.T) {
	svc, repo := setupServiceUseCase()
	ownerA := uuid.New()
	ownerB := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerA

	srvID := uuid.New()
	repo.services[srvID] = &domain.Service{
		ID:              srvID,
		WorkshopID:      wsID,
		Name:            "Old Service Name",
		Price:           100000,
		DurationMinutes: 45,
		IsActive:        true,
	}

	newPrice := 120000.0
	// 1. Owner A updates -> 200 OK
	updated, _, err := svc.UpdateService(context.Background(), srvID, ownerA, domain.RoleOwner, service.UpdateServiceRequest{
		Price: &newPrice,
	})
	if err != nil {
		t.Fatalf("unexpected error on owner update: %v", err)
	}
	if updated.Price != newPrice {
		t.Errorf("expected price %.2f, got %.2f", newPrice, updated.Price)
	}

	// 2. Owner B updates -> 403 Forbidden
	_, _, err = svc.UpdateService(context.Background(), srvID, ownerB, domain.RoleOwner, service.UpdateServiceRequest{
		Price: &newPrice,
	})
	if !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for wrong owner, got %v", err)
	}
}

func TestServiceUseCase_Delete(t *testing.T) {
	svc, repo := setupServiceUseCase()
	ownerA := uuid.New()
	ownerB := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerA

	srvID := uuid.New()
	repo.services[srvID] = &domain.Service{
		ID:         srvID,
		WorkshopID: wsID,
		Name:       "To Delete",
	}

	// 1. Non-owner delete -> 403 Forbidden
	err := svc.DeleteService(context.Background(), srvID, ownerB, domain.RoleOwner)
	if !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	// 2. Real owner delete -> Success
	err = svc.DeleteService(context.Background(), srvID, ownerA, domain.RoleOwner)
	if err != nil {
		t.Fatalf("unexpected error on delete: %v", err)
	}

	if _, exists := repo.services[srvID]; exists {
		t.Errorf("expected service to be deleted from repo")
	}
}
