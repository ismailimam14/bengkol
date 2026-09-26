package history_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/history"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

type mockHistoryRepo struct {
	mu         sync.Mutex
	histories  map[uuid.UUID]*domain.ServiceHistory
	spareParts map[uuid.UUID]*domain.SparePart
	bookings   map[uuid.UUID]*domain.Booking
	workshops  map[uuid.UUID]*domain.Workshop
	queues     map[uuid.UUID]*domain.Queue
}

func newMockHistoryRepo() *mockHistoryRepo {
	return &mockHistoryRepo{
		histories:  make(map[uuid.UUID]*domain.ServiceHistory),
		spareParts: make(map[uuid.UUID]*domain.SparePart),
		bookings:   make(map[uuid.UUID]*domain.Booking),
		workshops:  make(map[uuid.UUID]*domain.Workshop),
		queues:     make(map[uuid.UUID]*domain.Queue),
	}
}

func (m *mockHistoryRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return nil, nil
}

func (m *mockHistoryRepo) CreateServiceHistoryInTx(ctx context.Context, tx *sql.Tx, h *domain.ServiceHistory) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.histories[h.ID] = h
	return nil
}

func (m *mockHistoryRepo) CreateServiceHistorySparePartInTx(ctx context.Context, tx *sql.Tx, item *domain.ServiceHistorySparePart) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	h, ok := m.histories[item.ServiceHistoryID]
	if ok {
		h.SpareParts = append(h.SpareParts, *item)
	}
	return nil
}

func (m *mockHistoryRepo) GetSparePartForUpdateInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.SparePart, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sp, ok := m.spareParts[id]
	if !ok {
		return nil, history.ErrSparePartNotFound
	}
	copied := *sp
	return &copied, nil
}

func (m *mockHistoryRepo) DecrementSparePartStockInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, quantity int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sp, ok := m.spareParts[id]
	if !ok {
		return history.ErrSparePartNotFound
	}
	if sp.Stock < quantity {
		return history.ErrInsufficientStock
	}
	sp.Stock -= quantity
	return nil
}

func (m *mockHistoryRepo) GetHistoryByID(ctx context.Context, id uuid.UUID) (*domain.ServiceHistory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	h, ok := m.histories[id]
	if !ok {
		return nil, history.ErrHistoryNotFound
	}
	copied := *h
	return &copied, nil
}

func (m *mockHistoryRepo) GetHistoryByBookingID(ctx context.Context, bookingID uuid.UUID) (*domain.ServiceHistory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, h := range m.histories {
		if h.BookingID == bookingID {
			copied := *h
			return &copied, nil
		}
	}
	return nil, history.ErrHistoryNotFound
}

func (m *mockHistoryRepo) ListHistoriesByCustomer(ctx context.Context, customerID uuid.UUID, pagination domain.PaginationParams) ([]domain.ServiceHistory, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []domain.ServiceHistory
	for _, h := range m.histories {
		if h.CustomerID == customerID {
			list = append(list, *h)
		}
	}
	return list, int64(len(list)), nil
}

func (m *mockHistoryRepo) ListHistoriesByWorkshop(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams, startDate, endDate *string) ([]domain.ServiceHistory, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []domain.ServiceHistory
	for _, h := range m.histories {
		if h.WorkshopID == workshopID {
			list = append(list, *h)
		}
	}
	return list, int64(len(list)), nil
}

func (m *mockHistoryRepo) GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*domain.Booking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.bookings[bookingID]
	if !ok {
		return nil, history.ErrBookingNotFound
	}
	copied := *b
	return &copied, nil
}

func (m *mockHistoryRepo) UpdateBookingStatusInTx(ctx context.Context, tx *sql.Tx, bookingID uuid.UUID, status domain.BookingStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.bookings[bookingID]
	if !ok {
		return history.ErrBookingNotFound
	}
	b.Status = status
	return nil
}

func (m *mockHistoryRepo) CompleteQueueForBookingInTx(ctx context.Context, tx *sql.Tx, bookingID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, q := range m.queues {
		if q.BookingID == bookingID {
			q.Status = domain.QueueStatusCompleted
		}
	}
	return nil
}

func (m *mockHistoryRepo) GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.workshops[workshopID]
	if !ok {
		return nil, history.ErrWorkshopNotFound
	}
	copied := *w
	return &copied, nil
}

func setupHistoryService() (history.Service, *mockHistoryRepo) {
	repo := newMockHistoryRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	svc := history.NewService(repo, log)
	return svc, repo
}

func TestHistoryService_Create_Success(t *testing.T) {
	svc, repo := setupHistoryService()

	wsID := uuid.New()
	ownerID := uuid.New()
	custID := uuid.New()
	srvID := uuid.New()
	spID := uuid.New()
	bookingID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, OwnerID: ownerID, Status: domain.WorkshopStatusActive}
	service := &domain.Service{ID: srvID, WorkshopID: wsID, Name: "Tune Up", Price: 150000}
	repo.spareParts[spID] = &domain.SparePart{
		ID:           spID,
		WorkshopID:   wsID,
		Name:         "Oli Mesin 1L",
		SellingPrice: 65000,
		Stock:        10,
		IsActive:     true,
	}
	repo.bookings[bookingID] = &domain.Booking{
		ID:          bookingID,
		CustomerID:  custID,
		WorkshopID:  wsID,
		ServiceID:   srvID,
		BookingDate: "2026-09-28",
		Status:      domain.BookingStatusInService,
		Service:     service,
	}

	req := history.CreateHistoryRequest{
		BookingID: bookingID,
		Notes:     "Penggantian oli mesin & servis rutin",
		SpareParts: []history.CreateHistoryItemRequest{
			{
				SparePartID: spID,
				Quantity:    2,
			},
		},
	}

	h, valErrors, err := svc.CreateHistory(context.Background(), ownerID, domain.RoleOwner, req)
	if err != nil {
		t.Fatalf("unexpected error creating history: %v", err)
	}
	if valErrors != nil {
		t.Fatalf("expected no validation errors, got %+v", valErrors)
	}

	// Verify Pricing: Service 150000 + (65000 * 2) = 280000
	expectedTotal := 150000.0 + (65000.0 * 2)
	if h.TotalPrice != expectedTotal {
		t.Errorf("expected total price %f, got %f", expectedTotal, h.TotalPrice)
	}

	// Verify Inventory Decrement: 10 - 2 = 8
	if repo.spareParts[spID].Stock != 8 {
		t.Errorf("expected stock to be 8, got %d", repo.spareParts[spID].Stock)
	}

	// Verify Booking status updated to COMPLETED
	if repo.bookings[bookingID].Status != domain.BookingStatusCompleted {
		t.Errorf("expected booking status COMPLETED, got %s", repo.bookings[bookingID].Status)
	}
}

func TestHistoryService_Create_InsufficientStock(t *testing.T) {
	svc, repo := setupHistoryService()

	wsID := uuid.New()
	ownerID := uuid.New()
	custID := uuid.New()
	srvID := uuid.New()
	spID := uuid.New()
	bookingID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, OwnerID: ownerID, Status: domain.WorkshopStatusActive}
	service := &domain.Service{ID: srvID, WorkshopID: wsID, Name: "Tune Up", Price: 150000}
	repo.spareParts[spID] = &domain.SparePart{
		ID:           spID,
		WorkshopID:   wsID,
		Name:         "Oli Mesin 1L",
		SellingPrice: 65000,
		Stock:        1, // only 1 in stock!
		IsActive:     true,
	}
	repo.bookings[bookingID] = &domain.Booking{
		ID:          bookingID,
		CustomerID:  custID,
		WorkshopID:  wsID,
		ServiceID:   srvID,
		BookingDate: "2026-09-28",
		Status:      domain.BookingStatusInService,
		Service:     service,
	}

	req := history.CreateHistoryRequest{
		BookingID: bookingID,
		SpareParts: []history.CreateHistoryItemRequest{
			{
				SparePartID: spID,
				Quantity:    5, // requires 5
			},
		},
	}

	_, _, err := svc.CreateHistory(context.Background(), ownerID, domain.RoleOwner, req)
	if !errors.Is(err, history.ErrInsufficientStock) {
		t.Fatalf("expected ErrInsufficientStock, got %v", err)
	}
}

func TestHistoryService_Create_Duplicate(t *testing.T) {
	svc, repo := setupHistoryService()

	wsID := uuid.New()
	ownerID := uuid.New()
	custID := uuid.New()
	srvID := uuid.New()
	bookingID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, OwnerID: ownerID, Status: domain.WorkshopStatusActive}
	repo.bookings[bookingID] = &domain.Booking{
		ID:          bookingID,
		CustomerID:  custID,
		WorkshopID:  wsID,
		ServiceID:   srvID,
		BookingDate: "2026-09-28",
		Status:      domain.BookingStatusInService,
		Service:     &domain.Service{ID: srvID, Price: 50000},
	}

	req := history.CreateHistoryRequest{
		BookingID: bookingID,
	}

	// 1st time succeeds
	_, _, err := svc.CreateHistory(context.Background(), ownerID, domain.RoleOwner, req)
	if err != nil {
		t.Fatalf("unexpected error on 1st create: %v", err)
	}

	// 2nd time fails with duplicate
	_, _, err = svc.CreateHistory(context.Background(), ownerID, domain.RoleOwner, req)
	if !errors.Is(err, history.ErrHistoryExists) {
		t.Fatalf("expected ErrHistoryExists, got %v", err)
	}
}

func TestHistoryService_GetByID_AccessControl(t *testing.T) {
	svc, repo := setupHistoryService()

	wsID := uuid.New()
	ownerID := uuid.New()
	custA := uuid.New()
	custB := uuid.New()
	hID := uuid.New()

	repo.histories[hID] = &domain.ServiceHistory{
		ID:          hID,
		CustomerID:  custA,
		WorkshopID:  wsID,
		TotalPrice:  100000,
		ServiceDate: "2026-09-28",
		Workshop:    &domain.Workshop{ID: wsID, OwnerID: ownerID},
		CreatedAt:   time.Now(),
	}

	// CustA (owner of service record) -> OK
	_, err := svc.GetHistoryByID(context.Background(), hID, custA, domain.RoleCustomer)
	if err != nil {
		t.Errorf("expected customer to view own history, got %v", err)
	}

	// Workshop Owner -> OK
	_, err = svc.GetHistoryByID(context.Background(), hID, ownerID, domain.RoleOwner)
	if err != nil {
		t.Errorf("expected workshop owner to view history, got %v", err)
	}

	// CustB (unrelated customer) -> Forbidden
	_, err = svc.GetHistoryByID(context.Background(), hID, custB, domain.RoleCustomer)
	if !errors.Is(err, history.ErrForbidden) {
		t.Errorf("expected ErrForbidden for unrelated customer, got %v", err)
	}
}
