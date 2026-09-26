package sparepart_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/sparepart"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

type mockSparePartRepo struct {
	spareParts     map[uuid.UUID]*domain.SparePart
	workshopOwners map[uuid.UUID]uuid.UUID // workshopID -> ownerID
}

func newMockSparePartRepo() *mockSparePartRepo {
	return &mockSparePartRepo{
		spareParts:     make(map[uuid.UUID]*domain.SparePart),
		workshopOwners: make(map[uuid.UUID]uuid.UUID),
	}
}

func (m *mockSparePartRepo) Create(ctx context.Context, sp *domain.SparePart) error {
	m.spareParts[sp.ID] = sp
	return nil
}

func (m *mockSparePartRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.SparePart, error) {
	sp, ok := m.spareParts[id]
	if !ok {
		return nil, sparepart.ErrSparePartNotFound
	}
	copied := *sp
	return &copied, nil
}

func (m *mockSparePartRepo) ListByWorkshopID(ctx context.Context, workshopID uuid.UUID, onlyActive bool) ([]domain.SparePart, error) {
	var list []domain.SparePart
	for _, sp := range m.spareParts {
		if sp.WorkshopID == workshopID {
			if onlyActive && (!sp.IsActive || sp.Stock <= 0) {
				continue
			}
			list = append(list, *sp)
		}
	}
	return list, nil
}

func (m *mockSparePartRepo) Update(ctx context.Context, sp *domain.SparePart) error {
	if _, ok := m.spareParts[sp.ID]; !ok {
		return sparepart.ErrSparePartNotFound
	}
	m.spareParts[sp.ID] = sp
	return nil
}

func (m *mockSparePartRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := m.spareParts[id]; !ok {
		return sparepart.ErrSparePartNotFound
	}
	delete(m.spareParts, id)
	return nil
}

func (m *mockSparePartRepo) GetWorkshopOwnerID(ctx context.Context, workshopID uuid.UUID) (uuid.UUID, error) {
	ownerID, ok := m.workshopOwners[workshopID]
	if !ok {
		return uuid.Nil, errors.New("workshop not found")
	}
	return ownerID, nil
}

func (m *mockSparePartRepo) GetSparePartOwnerID(ctx context.Context, sparePartID uuid.UUID) (uuid.UUID, error) {
	sp, ok := m.spareParts[sparePartID]
	if !ok {
		return uuid.Nil, sparepart.ErrSparePartNotFound
	}
	ownerID, ok := m.workshopOwners[sp.WorkshopID]
	if !ok {
		return uuid.Nil, errors.New("workshop owner not found")
	}
	return ownerID, nil
}

func setupSparePartUseCase() (sparepart.Service, *mockSparePartRepo) {
	repo := newMockSparePartRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	svc := sparepart.NewService(repo, log)
	return svc, repo
}

func TestSparePartUseCase_Create_Success(t *testing.T) {
	svc, repo := setupSparePartUseCase()
	ownerID := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerID

	req := sparepart.CreateSparePartRequest{
		Name:          "Kampas Rem Depan",
		Description:   "Original OEM brake pad",
		PurchasePrice: 80000,
		SellingPrice:  120000,
		Stock:         15,
	}

	sp, valErrors, err := svc.CreateSparePart(context.Background(), wsID, ownerID, domain.RoleOwner, req)
	if err != nil {
		t.Fatalf("unexpected error creating spare part: %v", err)
	}

	if valErrors != nil {
		t.Fatalf("expected no validation errors, got %+v", valErrors)
	}

	if sp.Name != "Kampas Rem Depan" || sp.Stock != 15 || sp.SellingPrice != 120000 {
		t.Errorf("spare part fields mismatch: %+v", sp)
	}
}

func TestSparePartUseCase_Create_ValidationErrors(t *testing.T) {
	svc, repo := setupSparePartUseCase()
	ownerID := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerID

	req := sparepart.CreateSparePartRequest{
		Name:          "",
		PurchasePrice: -100,
		SellingPrice:  -50,
		Stock:         -2,
	}

	_, valErrors, err := svc.CreateSparePart(context.Background(), wsID, ownerID, domain.RoleOwner, req)
	if !errors.Is(err, sparepart.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed, got %v", err)
	}

	if _, ok := valErrors["name"]; !ok {
		t.Errorf("expected error on name")
	}
	if _, ok := valErrors["purchase_price"]; !ok {
		t.Errorf("expected error on purchase_price")
	}
	if _, ok := valErrors["selling_price"]; !ok {
		t.Errorf("expected error on selling_price")
	}
	if _, ok := valErrors["stock"]; !ok {
		t.Errorf("expected error on stock")
	}
}

func TestSparePartUseCase_List_ActiveAndStockFilter(t *testing.T) {
	svc, repo := setupSparePartUseCase()
	ownerID := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerID

	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()
	repo.spareParts[id1] = &domain.SparePart{ID: id1, WorkshopID: wsID, Name: "Available Part", IsActive: true, Stock: 5, CreatedAt: time.Now()}
	repo.spareParts[id2] = &domain.SparePart{ID: id2, WorkshopID: wsID, Name: "Out of Stock Part", IsActive: true, Stock: 0, CreatedAt: time.Now()}
	repo.spareParts[id3] = &domain.SparePart{ID: id3, WorkshopID: wsID, Name: "Inactive Part", IsActive: false, Stock: 10, CreatedAt: time.Now()}

	// 1. Customer should only see 1 available part with stock > 0
	custRole := domain.RoleCustomer
	custID := uuid.New()
	custParts, err := svc.GetSparePartsByWorkshop(context.Background(), wsID, &custRole, &custID)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(custParts) != 1 || custParts[0].Name != "Available Part" {
		t.Errorf("expected 1 available spare part for customer, got %d", len(custParts))
	}

	// 2. Owner sees all 3 spare parts
	ownerRole := domain.RoleOwner
	ownerParts, err := svc.GetSparePartsByWorkshop(context.Background(), wsID, &ownerRole, &ownerID)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(ownerParts) != 3 {
		t.Errorf("expected 3 spare parts for owner, got %d", len(ownerParts))
	}
}

func TestSparePartUseCase_Update_Ownership(t *testing.T) {
	svc, repo := setupSparePartUseCase()
	ownerA := uuid.New()
	ownerB := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerA

	spID := uuid.New()
	repo.spareParts[spID] = &domain.SparePart{
		ID:           spID,
		WorkshopID:   wsID,
		Name:         "Original Filter",
		SellingPrice: 40000,
		Stock:        10,
	}

	newStock := 25
	// 1. Owner A updates -> Success
	updated, _, err := svc.UpdateSparePart(context.Background(), spID, ownerA, domain.RoleOwner, sparepart.UpdateSparePartRequest{
		Stock: &newStock,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if updated.Stock != 25 {
		t.Errorf("expected stock 25, got %d", updated.Stock)
	}

	// 2. Owner B updates -> 403 Forbidden
	_, _, err = svc.UpdateSparePart(context.Background(), spID, ownerB, domain.RoleOwner, sparepart.UpdateSparePartRequest{
		Stock: &newStock,
	})
	if !errors.Is(err, sparepart.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestSparePartUseCase_Delete(t *testing.T) {
	svc, repo := setupSparePartUseCase()
	ownerA := uuid.New()
	ownerB := uuid.New()
	wsID := uuid.New()
	repo.workshopOwners[wsID] = ownerA

	spID := uuid.New()
	repo.spareParts[spID] = &domain.SparePart{
		ID:         spID,
		WorkshopID: wsID,
		Name:       "To Delete",
	}

	// 1. Owner B attempts delete -> 403 Forbidden
	err := svc.DeleteSparePart(context.Background(), spID, ownerB, domain.RoleOwner)
	if !errors.Is(err, sparepart.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	// 2. Owner A deletes -> Success
	err = svc.DeleteSparePart(context.Background(), spID, ownerA, domain.RoleOwner)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
}
