package booking_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/bengkol/backend/internal/booking"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

// MockBookingRepository implements booking.Repository with concurrency-safe in-memory storage
type mockBookingRepo struct {
	mu                sync.Mutex
	slots             map[string]*domain.BookingSlot // key: "wsID:date:time"
	bookings          map[uuid.UUID]*domain.Booking
	operatingHours    map[string]*domain.OperatingHour // key: "wsID:dayOfWeek"
	workshops         map[uuid.UUID]*domain.Workshop
	services          map[uuid.UUID]*domain.Service
	spareParts        map[uuid.UUID]*domain.SparePart
	bookingSpareParts map[uuid.UUID][]domain.BookingSparePart
}

func newMockBookingRepo() *mockBookingRepo {
	return &mockBookingRepo{
		slots:             make(map[string]*domain.BookingSlot),
		bookings:          make(map[uuid.UUID]*domain.Booking),
		operatingHours:    make(map[string]*domain.OperatingHour),
		workshops:         make(map[uuid.UUID]*domain.Workshop),
		services:          make(map[uuid.UUID]*domain.Service),
		spareParts:        make(map[uuid.UUID]*domain.SparePart),
		bookingSpareParts: make(map[uuid.UUID][]domain.BookingSparePart),
	}
}

func (m *mockBookingRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	// In unit testing mock, we synchronize using m.mu
	return nil, nil
}

func (m *mockBookingRepo) GetSlotForUpdate(ctx context.Context, tx *sql.Tx, workshopID uuid.UUID, slotDate, startTime string) (*domain.BookingSlot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s", workshopID, slotDate, startTime)
	slot, ok := m.slots[key]
	if !ok {
		return nil, booking.ErrSlotNotFound
	}
	copied := *slot
	return &copied, nil
}

func (m *mockBookingRepo) CreateSlotInTx(ctx context.Context, tx *sql.Tx, slot *domain.BookingSlot) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s", slot.WorkshopID, slot.SlotDate, slot.StartTime)
	m.slots[key] = slot
	return nil
}

func (m *mockBookingRepo) IncrementSlotBookingInTx(ctx context.Context, tx *sql.Tx, slotID uuid.UUID, newBookedCount int, isAvailable bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.slots {
		if s.ID == slotID {
			s.BookedCount = newBookedCount
			s.IsAvailable = isAvailable
			return nil
		}
	}
	return booking.ErrSlotNotFound
}

func (m *mockBookingRepo) DecrementSlotBookingInTx(ctx context.Context, tx *sql.Tx, slotID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.slots {
		if s.ID == slotID {
			if s.BookedCount > 0 {
				s.BookedCount--
			}
			if s.BookedCount < s.MaxCapacity {
				s.IsAvailable = true
			}
			return nil
		}
	}
	return nil
}

func (m *mockBookingRepo) GetSlotsByDate(ctx context.Context, workshopID uuid.UUID, slotDate string) ([]domain.BookingSlot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []domain.BookingSlot
	prefix := fmt.Sprintf("%s:%s:", workshopID, slotDate)
	for k, s := range m.slots {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			list = append(list, *s)
		}
	}
	return list, nil
}

func (m *mockBookingRepo) CreateBookingInTx(ctx context.Context, tx *sql.Tx, b *domain.Booking) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.bookings[b.ID] = b
	return nil
}

func (m *mockBookingRepo) GetBookingByID(ctx context.Context, id uuid.UUID) (*domain.Booking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.bookings[id]
	if !ok {
		return nil, booking.ErrBookingNotFound
	}
	copied := *b
	if parts, ok := m.bookingSpareParts[id]; ok {
		copied.SpareParts = parts
	}
	return &copied, nil
}

func (m *mockBookingRepo) GetBookingByIDForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Booking, error) {
	return m.GetBookingByID(ctx, id)
}

func (m *mockBookingRepo) UpdateBookingStatusInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.BookingStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.bookings[id]
	if !ok {
		return booking.ErrBookingNotFound
	}
	b.Status = status
	return nil
}

func (m *mockBookingRepo) ListByCustomer(ctx context.Context, customerID uuid.UUID, pagination domain.PaginationParams) ([]domain.Booking, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []domain.Booking
	for _, b := range m.bookings {
		if b.CustomerID == customerID {
			list = append(list, *b)
		}
	}
	return list, int64(len(list)), nil
}

func (m *mockBookingRepo) ListByWorkshop(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams, date *string) ([]domain.Booking, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []domain.Booking
	for _, b := range m.bookings {
		if b.WorkshopID == workshopID {
			if date != nil && *date != "" && b.BookingDate != *date {
				continue
			}
			list = append(list, *b)
		}
	}
	return list, int64(len(list)), nil
}

func (m *mockBookingRepo) GetWorkshopOperatingHour(ctx context.Context, workshopID uuid.UUID, dayOfWeek int) (*domain.OperatingHour, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%d", workshopID, dayOfWeek)
	oh, ok := m.operatingHours[key]
	if !ok {
		return nil, nil
	}
	copied := *oh
	return &copied, nil
}

func (m *mockBookingRepo) GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.workshops[workshopID]
	if !ok {
		return nil, errors.New("workshop not found")
	}
	copied := *w
	return &copied, nil
}

func (m *mockBookingRepo) GetServiceByID(ctx context.Context, serviceID uuid.UUID) (*domain.Service, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.services[serviceID]
	if !ok {
		return nil, errors.New("service not found")
	}
	copied := *s
	return &copied, nil
}

func (m *mockBookingRepo) GetSparePartForUpdateInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.SparePart, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sp, ok := m.spareParts[id]
	if !ok {
		return nil, booking.ErrSparePartNotFound
	}
	copied := *sp
	return &copied, nil
}

func (m *mockBookingRepo) DecrementSparePartStockInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, quantity int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sp, ok := m.spareParts[id]
	if !ok {
		return booking.ErrSparePartNotFound
	}
	if sp.Stock < quantity {
		return booking.ErrInsufficientStock
	}
	sp.Stock -= quantity
	return nil
}

func (m *mockBookingRepo) IncrementSparePartStockInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, quantity int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sp, ok := m.spareParts[id]
	if !ok {
		return booking.ErrSparePartNotFound
	}
	sp.Stock += quantity
	return nil
}

func (m *mockBookingRepo) CreateBookingSparePartInTx(ctx context.Context, tx *sql.Tx, item *domain.BookingSparePart) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.bookingSpareParts[item.BookingID] = append(m.bookingSpareParts[item.BookingID], *item)
	return nil
}

func (m *mockBookingRepo) GetBookingSpareParts(ctx context.Context, bookingID uuid.UUID) ([]domain.BookingSparePart, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	parts := m.bookingSpareParts[bookingID]
	var list []domain.BookingSparePart
	for _, p := range parts {
		copied := p
		if sp, ok := m.spareParts[p.SparePartID]; ok {
			spCopy := *sp
			copied.SparePart = &spCopy
		}
		list = append(list, copied)
	}
	return list, nil
}

func (m *mockBookingRepo) GetBookingSparePartsInTx(ctx context.Context, tx *sql.Tx, bookingID uuid.UUID) ([]domain.BookingSparePart, error) {
	return m.GetBookingSpareParts(ctx, bookingID)
}

func setupBookingService() (booking.Service, *mockBookingRepo) {
	repo := newMockBookingRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	svc := booking.NewService(repo, log)
	return svc, repo
}

func TestBookingService_GetAvailableSlots(t *testing.T) {
	svc, repo := setupBookingService()
	wsID := uuid.New()

	// Monday (dayOfWeek = 1) open 08:00 to 12:00
	repo.operatingHours[fmt.Sprintf("%s:1", wsID)] = &domain.OperatingHour{
		WorkshopID: wsID,
		DayOfWeek:  1,
		OpenTime:   "08:00:00",
		CloseTime:  "12:00:00",
		IsClosed:   false,
	}

	// Target date: upcoming Monday
	targetDate := getNextMonday()

	resp, err := svc.GetAvailableSlots(context.Background(), wsID, targetDate)
	if err != nil {
		t.Fatalf("unexpected error getting available slots: %v", err)
	}

	if resp.IsClosed {
		t.Errorf("expected workshop to be open on Monday")
	}

	// 08:00 to 12:00 -> 4 hourly slots: 08-09, 09-10, 10-11, 11-12
	if len(resp.Slots) != 4 {
		t.Fatalf("expected 4 slots, got %d", len(resp.Slots))
	}

	for _, slot := range resp.Slots {
		if !slot.IsAvailable {
			t.Errorf("expected slot %s to be available", slot.StartTime)
		}
	}
}

func TestBookingService_Create_Success(t *testing.T) {
	svc, repo := setupBookingService()

	wsID := uuid.New()
	ownerID := uuid.New()
	customerID := uuid.New()
	srvID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, OwnerID: ownerID, Status: domain.WorkshopStatusActive}
	repo.services[srvID] = &domain.Service{ID: srvID, WorkshopID: wsID, Price: 100000, DurationMinutes: 60, IsActive: true}

	// Operating hour for Monday (2026-09-28)
	repo.operatingHours[fmt.Sprintf("%s:1", wsID)] = &domain.OperatingHour{
		WorkshopID: wsID,
		DayOfWeek:  1,
		OpenTime:   "08:00:00",
		CloseTime:  "17:00:00",
		IsClosed:   false,
	}

	mondayDate := getNextMonday()
	req := booking.CreateBookingRequest{
		WorkshopID:    wsID,
		ServiceID:     srvID,
		BookingDate:   mondayDate,
		BookingTime:   "10:00:00",
		CustomerNotes: "Mohon diperiksa filter bensin",
	}

	b, valErrors, err := svc.CreateBooking(context.Background(), customerID, req)
	if err != nil {
		t.Fatalf("unexpected error creating booking: %v", err)
	}

	if valErrors != nil {
		t.Fatalf("expected no validation errors, got %+v", valErrors)
	}

	if b.Status != domain.BookingStatusConfirmed {
		t.Errorf("expected status CONFIRMED, got %s", b.Status)
	}

	if b.BookingNumber == "" {
		t.Errorf("expected generated booking number")
	}

	// Verify slot was created and count is 1
	slotKey := fmt.Sprintf("%s:%s:10:00:00", wsID, mondayDate)
	slot := repo.slots[slotKey]
	if slot == nil || slot.BookedCount != 1 {
		t.Errorf("expected slot to have booked_count = 1")
	}
}

func TestBookingService_ConcurrentBooking_Protection(t *testing.T) {
	svc, repo := setupBookingService()

	wsID := uuid.New()
	srvID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, Status: domain.WorkshopStatusActive}
	repo.services[srvID] = &domain.Service{ID: srvID, WorkshopID: wsID, Price: 50000, DurationMinutes: 30, IsActive: true}

	// Operating hour for Monday
	repo.operatingHours[fmt.Sprintf("%s:1", wsID)] = &domain.OperatingHour{
		WorkshopID: wsID,
		DayOfWeek:  1,
		OpenTime:   "08:00:00",
		CloseTime:  "17:00:00",
		IsClosed:   false,
	}

	mondayDate := getNextMonday()
	// Pre-create slot with max_capacity = 1 and booked_count = 0
	slotKey := fmt.Sprintf("%s:%s:14:00:00", wsID, mondayDate)
	slotID := uuid.New()
	repo.slots[slotKey] = &domain.BookingSlot{
		ID:          slotID,
		WorkshopID:  wsID,
		SlotDate:    mondayDate,
		StartTime:   "14:00:00",
		EndTime:     "15:00:00",
		MaxCapacity: 1, // Only 1 spot available!
		BookedCount: 0,
		IsAvailable: true,
	}

	customerA := uuid.New()
	customerB := uuid.New()

	req := booking.CreateBookingRequest{
		WorkshopID:  wsID,
		ServiceID:   srvID,
		BookingDate: mondayDate,
		BookingTime: "14:00:00",
	}

	var wg sync.WaitGroup
	var results = make(chan error, 2)

	// Simulate concurrent requests from two customers at the exact same millisecond
	for _, cust := range []uuid.UUID{customerA, customerB} {
		wg.Add(1)
		go func(cID uuid.UUID) {
			defer wg.Done()
			_, _, err := svc.CreateBooking(context.Background(), cID, req)
			results <- err
		}(cust)
	}

	wg.Wait()
	close(results)

	var successCount, failureCount int
	for err := range results {
		if err == nil {
			successCount++
		} else if errors.Is(err, booking.ErrSlotUnavailable) {
			failureCount++
		}
	}

	if successCount != 1 || failureCount != 1 {
		t.Fatalf("expected exactly 1 success and 1 ErrSlotUnavailable, got %d successes and %d failures", successCount, failureCount)
	}
}

func TestBookingService_CancelBooking_Success(t *testing.T) {
	svc, repo := setupBookingService()

	wsID := uuid.New()
	customerID := uuid.New()
	srvID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, Status: domain.WorkshopStatusActive}
	repo.services[srvID] = &domain.Service{ID: srvID, WorkshopID: wsID, Price: 50000, DurationMinutes: 30, IsActive: true}
	repo.operatingHours[fmt.Sprintf("%s:1", wsID)] = &domain.OperatingHour{
		WorkshopID: wsID,
		DayOfWeek:  1,
		OpenTime:   "08:00:00",
		CloseTime:  "17:00:00",
	}

	mondayDate := getNextMonday()
	b, _, err := svc.CreateBooking(context.Background(), customerID, booking.CreateBookingRequest{
		WorkshopID:  wsID,
		ServiceID:   srvID,
		BookingDate: mondayDate,
		BookingTime: "09:00:00",
	})
	if err != nil {
		t.Fatalf("error creating booking: %v", err)
	}

	// Customer cancels own booking
	err = svc.CancelBooking(context.Background(), b.ID, customerID, domain.RoleCustomer)
	if err != nil {
		t.Fatalf("unexpected error cancelling booking: %v", err)
	}

	// Verify status is CANCELLED
	cancelledBooking, _ := repo.GetBookingByID(context.Background(), b.ID)
	if cancelledBooking.Status != domain.BookingStatusCancelled {
		t.Errorf("expected status CANCELLED, got %s", cancelledBooking.Status)
	}

	// Verify slot count was decremented back to 0
	slotKey := fmt.Sprintf("%s:%s:09:00:00", wsID, mondayDate)
	slot := repo.slots[slotKey]
	if slot.BookedCount != 0 {
		t.Errorf("expected booked_count to be 0 after cancellation, got %d", slot.BookedCount)
	}
}

func TestBookingService_Create_WithSpareParts_Success(t *testing.T) {
	svc, repo := setupBookingService()

	wsID := uuid.New()
	customerID := uuid.New()
	srvID := uuid.New()
	sp1ID := uuid.New()
	sp2ID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, Status: domain.WorkshopStatusActive}
	repo.services[srvID] = &domain.Service{ID: srvID, WorkshopID: wsID, Price: 100000, DurationMinutes: 60, IsActive: true}
	repo.spareParts[sp1ID] = &domain.SparePart{
		ID:           sp1ID,
		WorkshopID:   wsID,
		Name:         "Oli Mesin 1L",
		SellingPrice: 65000,
		Stock:        5,
		IsActive:     true,
	}
	repo.spareParts[sp2ID] = &domain.SparePart{
		ID:           sp2ID,
		WorkshopID:   wsID,
		Name:         "Busi Iridium",
		SellingPrice: 45000,
		Stock:        10,
		IsActive:     true,
	}
	repo.operatingHours[fmt.Sprintf("%s:1", wsID)] = &domain.OperatingHour{
		WorkshopID: wsID,
		DayOfWeek:  1,
		OpenTime:   "08:00:00",
		CloseTime:  "17:00:00",
	}

	mondayDate := getNextMonday()
	req := booking.CreateBookingRequest{
		WorkshopID:  wsID,
		ServiceID:   srvID,
		BookingDate: mondayDate,
		BookingTime: "10:00:00",
		SpareParts: []booking.BookingSparePartItemRequest{
			{SparePartID: sp1ID, Quantity: 2},
			{SparePartID: sp2ID, Quantity: 1},
		},
	}

	b, valErrors, err := svc.CreateBooking(context.Background(), customerID, req)
	if err != nil {
		t.Fatalf("unexpected error creating booking with spare parts: %v", err)
	}
	if valErrors != nil {
		t.Fatalf("expected no validation errors, got %+v", valErrors)
	}

	// Total price should be: service 100,000 + (65,000 * 2) + (45,000 * 1) = 275,000
	expectedTotal := 100000.0 + (65000.0 * 2) + 45000.0
	if b.TotalPrice != expectedTotal {
		t.Errorf("expected total price %.2f, got %.2f", expectedTotal, b.TotalPrice)
	}

	if len(b.SpareParts) != 2 {
		t.Fatalf("expected 2 booked spare parts, got %d", len(b.SpareParts))
	}

	// Verify inventory decrement
	if repo.spareParts[sp1ID].Stock != 3 {
		t.Errorf("expected sp1 stock to be 3, got %d", repo.spareParts[sp1ID].Stock)
	}
	if repo.spareParts[sp2ID].Stock != 9 {
		t.Errorf("expected sp2 stock to be 9, got %d", repo.spareParts[sp2ID].Stock)
	}
}

func TestBookingService_Create_WithSpareParts_InsufficientStock(t *testing.T) {
	svc, repo := setupBookingService()

	wsID := uuid.New()
	customerID := uuid.New()
	srvID := uuid.New()
	spID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, Status: domain.WorkshopStatusActive}
	repo.services[srvID] = &domain.Service{ID: srvID, WorkshopID: wsID, Price: 100000, DurationMinutes: 60, IsActive: true}
	repo.spareParts[spID] = &domain.SparePart{
		ID:           spID,
		WorkshopID:   wsID,
		Name:         "Oli Mesin 1L",
		SellingPrice: 65000,
		Stock:        1, // only 1 available!
		IsActive:     true,
	}
	repo.operatingHours[fmt.Sprintf("%s:1", wsID)] = &domain.OperatingHour{
		WorkshopID: wsID,
		DayOfWeek:  1,
		OpenTime:   "08:00:00",
		CloseTime:  "17:00:00",
	}

	mondayDate := getNextMonday()
	req := booking.CreateBookingRequest{
		WorkshopID:  wsID,
		ServiceID:   srvID,
		BookingDate: mondayDate,
		BookingTime: "10:00:00",
		SpareParts: []booking.BookingSparePartItemRequest{
			{SparePartID: spID, Quantity: 3}, // requesting 3
		},
	}

	_, _, err := svc.CreateBooking(context.Background(), customerID, req)
	if !errors.Is(err, booking.ErrInsufficientStock) {
		t.Fatalf("expected ErrInsufficientStock, got %v", err)
	}

	// Stock should not be modified
	if repo.spareParts[spID].Stock != 1 {
		t.Errorf("expected stock to remain 1, got %d", repo.spareParts[spID].Stock)
	}
}

func TestBookingService_Create_WithSpareParts_InactivePart(t *testing.T) {
	svc, repo := setupBookingService()

	wsID := uuid.New()
	customerID := uuid.New()
	srvID := uuid.New()
	spID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, Status: domain.WorkshopStatusActive}
	repo.services[srvID] = &domain.Service{ID: srvID, WorkshopID: wsID, Price: 100000, DurationMinutes: 60, IsActive: true}
	repo.spareParts[spID] = &domain.SparePart{
		ID:           spID,
		WorkshopID:   wsID,
		Name:         "Oli Lama",
		SellingPrice: 50000,
		Stock:        10,
		IsActive:     false, // inactive
	}
	repo.operatingHours[fmt.Sprintf("%s:1", wsID)] = &domain.OperatingHour{
		WorkshopID: wsID,
		DayOfWeek:  1,
		OpenTime:   "08:00:00",
		CloseTime:  "17:00:00",
	}

	mondayDate := getNextMonday()
	req := booking.CreateBookingRequest{
		WorkshopID:  wsID,
		ServiceID:   srvID,
		BookingDate: mondayDate,
		BookingTime: "10:00:00",
		SpareParts: []booking.BookingSparePartItemRequest{
			{SparePartID: spID, Quantity: 1},
		},
	}

	_, _, err := svc.CreateBooking(context.Background(), customerID, req)
	if !errors.Is(err, booking.ErrSparePartInactive) {
		t.Fatalf("expected ErrSparePartInactive, got %v", err)
	}
}

func TestBookingService_Create_WithSpareParts_WorkshopMismatch(t *testing.T) {
	svc, repo := setupBookingService()

	wsID1 := uuid.New()
	wsID2 := uuid.New()
	customerID := uuid.New()
	srvID := uuid.New()
	spID := uuid.New()

	repo.workshops[wsID1] = &domain.Workshop{ID: wsID1, Status: domain.WorkshopStatusActive}
	repo.services[srvID] = &domain.Service{ID: srvID, WorkshopID: wsID1, Price: 100000, DurationMinutes: 60, IsActive: true}
	repo.spareParts[spID] = &domain.SparePart{
		ID:           spID,
		WorkshopID:   wsID2, // belongs to other workshop!
		Name:         "Oli Luar",
		SellingPrice: 50000,
		Stock:        10,
		IsActive:     true,
	}
	repo.operatingHours[fmt.Sprintf("%s:1", wsID1)] = &domain.OperatingHour{
		WorkshopID: wsID1,
		DayOfWeek:  1,
		OpenTime:   "08:00:00",
		CloseTime:  "17:00:00",
	}

	mondayDate := getNextMonday()
	req := booking.CreateBookingRequest{
		WorkshopID:  wsID1,
		ServiceID:   srvID,
		BookingDate: mondayDate,
		BookingTime: "10:00:00",
		SpareParts: []booking.BookingSparePartItemRequest{
			{SparePartID: spID, Quantity: 1},
		},
	}

	_, _, err := svc.CreateBooking(context.Background(), customerID, req)
	if !errors.Is(err, booking.ErrSparePartWorkshopMismatch) {
		t.Fatalf("expected ErrSparePartWorkshopMismatch, got %v", err)
	}
}

func TestBookingService_CancelBooking_RestoresSparePartsStock(t *testing.T) {
	svc, repo := setupBookingService()

	wsID := uuid.New()
	customerID := uuid.New()
	srvID := uuid.New()
	spID := uuid.New()

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, Status: domain.WorkshopStatusActive}
	repo.services[srvID] = &domain.Service{ID: srvID, WorkshopID: wsID, Price: 100000, DurationMinutes: 60, IsActive: true}
	repo.spareParts[spID] = &domain.SparePart{
		ID:           spID,
		WorkshopID:   wsID,
		Name:         "Oli Mesin 1L",
		SellingPrice: 65000,
		Stock:        5,
		IsActive:     true,
	}
	repo.operatingHours[fmt.Sprintf("%s:1", wsID)] = &domain.OperatingHour{
		WorkshopID: wsID,
		DayOfWeek:  1,
		OpenTime:   "08:00:00",
		CloseTime:  "17:00:00",
	}

	mondayDate := getNextMonday()
	b, _, err := svc.CreateBooking(context.Background(), customerID, booking.CreateBookingRequest{
		WorkshopID:  wsID,
		ServiceID:   srvID,
		BookingDate: mondayDate,
		BookingTime: "10:00:00",
		SpareParts: []booking.BookingSparePartItemRequest{
			{SparePartID: spID, Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error creating booking: %v", err)
	}

	// Stock decremented from 5 to 3
	if repo.spareParts[spID].Stock != 3 {
		t.Fatalf("expected stock to be 3, got %d", repo.spareParts[spID].Stock)
	}

	// Cancel booking
	err = svc.CancelBooking(context.Background(), b.ID, customerID, domain.RoleCustomer)
	if err != nil {
		t.Fatalf("unexpected error cancelling booking: %v", err)
	}

	// Verify inventory stock restored to 5!
	if repo.spareParts[spID].Stock != 5 {
		t.Errorf("expected stock to be restored to 5, got %d", repo.spareParts[spID].Stock)
	}
}
