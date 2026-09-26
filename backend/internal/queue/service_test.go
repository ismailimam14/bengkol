package queue_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/queue"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

type mockQueueRepo struct {
	mu        sync.Mutex
	queues    map[uuid.UUID]*domain.Queue
	bookings  map[uuid.UUID]*domain.Booking
	workshops map[uuid.UUID]*domain.Workshop
}

func newMockQueueRepo() *mockQueueRepo {
	return &mockQueueRepo{
		queues:    make(map[uuid.UUID]*domain.Queue),
		bookings:  make(map[uuid.UUID]*domain.Booking),
		workshops: make(map[uuid.UUID]*domain.Workshop),
	}
}

func (m *mockQueueRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return nil, nil
}

func (m *mockQueueRepo) GetNextQueueNumberInTx(ctx context.Context, tx *sql.Tx, workshopID uuid.UUID, queueDate string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	maxNum := 0
	for _, q := range m.queues {
		if q.WorkshopID == workshopID && q.QueueDate == queueDate {
			if q.QueueNumber > maxNum {
				maxNum = q.QueueNumber
			}
		}
	}
	return maxNum + 1, nil
}

func (m *mockQueueRepo) CreateQueueInTx(ctx context.Context, tx *sql.Tx, q *domain.Queue) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queues[q.ID] = q
	return nil
}

func (m *mockQueueRepo) GetQueueByID(ctx context.Context, id uuid.UUID) (*domain.Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	q, ok := m.queues[id]
	if !ok {
		return nil, queue.ErrQueueNotFound
	}
	copied := *q
	return &copied, nil
}

func (m *mockQueueRepo) GetQueueByIDForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Queue, error) {
	return m.GetQueueByID(ctx, id)
}

func (m *mockQueueRepo) GetQueueByBookingID(ctx context.Context, bookingID uuid.UUID) (*domain.Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, q := range m.queues {
		if q.BookingID == bookingID {
			copied := *q
			return &copied, nil
		}
	}
	return nil, queue.ErrQueueNotFound
}

func (m *mockQueueRepo) GetActiveQueueByCustomerID(ctx context.Context, customerID uuid.UUID, queueDate string) (*domain.Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, q := range m.queues {
		if q.Booking != nil && q.Booking.CustomerID == customerID && q.QueueDate == queueDate {
			if q.Status == domain.QueueStatusWaiting || q.Status == domain.QueueStatusCalled || q.Status == domain.QueueStatusInService {
				copied := *q
				return &copied, nil
			}
		}
	}
	return nil, queue.ErrQueueNotFound
}

func (m *mockQueueRepo) UpdateQueueStatusInTx(ctx context.Context, tx *sql.Tx, q *domain.Queue) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.queues[q.ID]
	if !ok {
		return queue.ErrQueueNotFound
	}
	existing.Status = q.Status
	existing.CalledAt = q.CalledAt
	existing.ServiceStartedAt = q.ServiceStartedAt
	existing.ServiceCompletedAt = q.ServiceCompletedAt
	existing.UpdatedAt = q.UpdatedAt
	return nil
}

func (m *mockQueueRepo) ListQueuesByWorkshop(ctx context.Context, workshopID uuid.UUID, queueDate string, status *domain.QueueStatus) ([]domain.Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []domain.Queue
	for _, q := range m.queues {
		if q.WorkshopID == workshopID && q.QueueDate == queueDate {
			if status != nil && *status != "" && q.Status != *status {
				continue
			}
			list = append(list, *q)
		}
	}
	return list, nil
}

func (m *mockQueueRepo) GetQueueSummary(ctx context.Context, workshopID uuid.UUID, queueDate string) (*domain.QueueSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	summary := &domain.QueueSummary{
		WorkshopID: workshopID,
		QueueDate:  queueDate,
	}

	for _, q := range m.queues {
		if q.WorkshopID == workshopID && q.QueueDate == queueDate {
			switch q.Status {
			case domain.QueueStatusWaiting:
				summary.TotalWaiting++
			case domain.QueueStatusInService:
				summary.TotalInService++
				if summary.CurrentServingNo == nil || q.QueueNumber < *summary.CurrentServingNo {
					num := q.QueueNumber
					summary.CurrentServingNo = &num
				}
			case domain.QueueStatusCalled:
				if summary.CurrentCalledNo == nil || q.QueueNumber < *summary.CurrentCalledNo {
					num := q.QueueNumber
					summary.CurrentCalledNo = &num
				}
			case domain.QueueStatusCompleted:
				summary.TotalCompleted++
			}
		}
	}

	return summary, nil
}

func (m *mockQueueRepo) CountAheadInQueue(ctx context.Context, workshopID uuid.UUID, queueDate string, queueNumber int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, q := range m.queues {
		if q.WorkshopID == workshopID && q.QueueDate == queueDate && q.QueueNumber < queueNumber {
			if q.Status == domain.QueueStatusWaiting || q.Status == domain.QueueStatusCalled {
				count++
			}
		}
	}
	return count, nil
}

func (m *mockQueueRepo) GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*domain.Booking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.bookings[bookingID]
	if !ok {
		return nil, queue.ErrBookingNotFound
	}
	copied := *b
	return &copied, nil
}

func (m *mockQueueRepo) UpdateBookingStatusInTx(ctx context.Context, tx *sql.Tx, bookingID uuid.UUID, status domain.BookingStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.bookings[bookingID]
	if !ok {
		return queue.ErrBookingNotFound
	}
	b.Status = status
	return nil
}

func (m *mockQueueRepo) GetWorkshopByID(ctx context.Context, workshopID uuid.UUID) (*domain.Workshop, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.workshops[workshopID]
	if !ok {
		return nil, queue.ErrWorkshopNotFound
	}
	copied := *w
	return &copied, nil
}

type mockBroadcaster struct {
	events []struct {
		Topic string
		Event string
		Data  interface{}
	}
}

func (m *mockBroadcaster) Broadcast(topic string, event string, data interface{}) {
	m.events = append(m.events, struct {
		Topic string
		Event string
		Data  interface{}
	}{Topic: topic, Event: event, Data: data})
}

type mockPushDispatcher struct {
	dispatched []struct {
		UserID  uuid.UUID
		Payload domain.PushNotificationPayload
	}
}

func (m *mockPushDispatcher) SendToUser(ctx context.Context, userID uuid.UUID, payload domain.PushNotificationPayload) error {
	m.dispatched = append(m.dispatched, struct {
		UserID  uuid.UUID
		Payload domain.PushNotificationPayload
	}{UserID: userID, Payload: payload})
	return nil
}

func setupQueueService() (queue.Service, *mockQueueRepo) {
	repo := newMockQueueRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	broadcaster := &mockBroadcaster{}
	dispatcher := &mockPushDispatcher{}
	svc := queue.NewService(repo, log, broadcaster, dispatcher)
	return svc, repo
}

func TestQueueService_CheckInBooking_Success(t *testing.T) {
	svc, repo := setupQueueService()

	wsID := uuid.New()
	ownerID := uuid.New()
	custID := uuid.New()
	bookingID := uuid.New()
	today := time.Now().Format("2006-01-02")

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, OwnerID: ownerID, Status: domain.WorkshopStatusActive}
	repo.bookings[bookingID] = &domain.Booking{
		ID:            bookingID,
		BookingNumber: "BK-20260928-1234",
		CustomerID:    custID,
		WorkshopID:    wsID,
		BookingDate:   today,
		BookingTime:   "10:00:00",
		Status:        domain.BookingStatusConfirmed,
	}

	q, err := svc.CheckInBooking(context.Background(), bookingID, custID, domain.RoleCustomer)
	if err != nil {
		t.Fatalf("unexpected check-in error: %v", err)
	}

	if q.QueueNumber != 1 {
		t.Errorf("expected queue number 1, got %d", q.QueueNumber)
	}
	if q.Status != domain.QueueStatusWaiting {
		t.Errorf("expected status WAITING, got %s", q.Status)
	}
	if q.CustomersAhead != 0 {
		t.Errorf("expected 0 customers ahead, got %d", q.CustomersAhead)
	}
}

func TestQueueService_CheckInBooking_UnauthorizedCustomer(t *testing.T) {
	svc, repo := setupQueueService()

	wsID := uuid.New()
	custA := uuid.New()
	custB := uuid.New()
	bookingID := uuid.New()
	today := time.Now().Format("2006-01-02")

	repo.bookings[bookingID] = &domain.Booking{
		ID:          bookingID,
		CustomerID:  custA,
		WorkshopID:  wsID,
		BookingDate: today,
		Status:      domain.BookingStatusConfirmed,
	}

	// CustB attempts to check in CustA's booking
	_, err := svc.CheckInBooking(context.Background(), bookingID, custB, domain.RoleCustomer)
	if !errors.Is(err, queue.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestQueueService_QueueLifecycleTransitions(t *testing.T) {
	svc, repo := setupQueueService()

	wsID := uuid.New()
	ownerID := uuid.New()
	custID := uuid.New()
	bookingID := uuid.New()
	today := time.Now().Format("2006-01-02")

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, OwnerID: ownerID, Status: domain.WorkshopStatusActive}
	repo.bookings[bookingID] = &domain.Booking{
		ID:          bookingID,
		CustomerID:  custID,
		WorkshopID:  wsID,
		BookingDate: today,
		Status:      domain.BookingStatusConfirmed,
	}

	// 1. Check in -> WAITING
	q, err := svc.CheckInBooking(context.Background(), bookingID, custID, domain.RoleCustomer)
	if err != nil {
		t.Fatalf("error checking in: %v", err)
	}

	// 2. Call ticket -> CALLED
	qCalled, err := svc.CallQueue(context.Background(), q.ID, ownerID, domain.RoleOwner)
	if err != nil {
		t.Fatalf("error calling queue: %v", err)
	}
	if qCalled.Status != domain.QueueStatusCalled {
		t.Errorf("expected status CALLED, got %s", qCalled.Status)
	}
	if qCalled.CalledAt == nil {
		t.Errorf("expected called_at timestamp")
	}

	// 3. Start service -> IN_SERVICE (also updates booking status)
	qStarted, err := svc.StartService(context.Background(), q.ID, ownerID, domain.RoleOwner)
	if err != nil {
		t.Fatalf("error starting service: %v", err)
	}
	if qStarted.Status != domain.QueueStatusInService {
		t.Errorf("expected status IN_SERVICE, got %s", qStarted.Status)
	}
	if repo.bookings[bookingID].Status != domain.BookingStatusInService {
		t.Errorf("expected booking status IN_SERVICE, got %s", repo.bookings[bookingID].Status)
	}

	// 4. Complete service -> COMPLETED (also updates booking status)
	qDone, err := svc.CompleteService(context.Background(), q.ID, ownerID, domain.RoleOwner)
	if err != nil {
		t.Fatalf("error completing service: %v", err)
	}
	if qDone.Status != domain.QueueStatusCompleted {
		t.Errorf("expected status COMPLETED, got %s", qDone.Status)
	}
	if repo.bookings[bookingID].Status != domain.BookingStatusCompleted {
		t.Errorf("expected booking status COMPLETED, got %s", repo.bookings[bookingID].Status)
	}
}

func TestQueueService_NoShow_Transition(t *testing.T) {
	svc, repo := setupQueueService()

	wsID := uuid.New()
	ownerID := uuid.New()
	custID := uuid.New()
	bookingID := uuid.New()
	today := time.Now().Format("2006-01-02")

	repo.workshops[wsID] = &domain.Workshop{ID: wsID, OwnerID: ownerID, Status: domain.WorkshopStatusActive}
	repo.bookings[bookingID] = &domain.Booking{
		ID:          bookingID,
		CustomerID:  custID,
		WorkshopID:  wsID,
		BookingDate: today,
		Status:      domain.BookingStatusConfirmed,
	}

	q, _ := svc.CheckInBooking(context.Background(), bookingID, custID, domain.RoleCustomer)
	qNoShow, err := svc.MarkNoShow(context.Background(), q.ID, ownerID, domain.RoleOwner)
	if err != nil {
		t.Fatalf("error marking no show: %v", err)
	}

	if qNoShow.Status != domain.QueueStatusNoShow {
		t.Errorf("expected status NO_SHOW, got %s", qNoShow.Status)
	}
	if repo.bookings[bookingID].Status != domain.BookingStatusNoShow {
		t.Errorf("expected booking status NO_SHOW, got %s", repo.bookings[bookingID].Status)
	}
}

func TestQueueService_GetWorkshopQueueSummary(t *testing.T) {
	svc, repo := setupQueueService()

	wsID := uuid.New()
	today := time.Now().Format("2006-01-02")

	repo.queues[uuid.New()] = &domain.Queue{ID: uuid.New(), WorkshopID: wsID, QueueDate: today, QueueNumber: 1, Status: domain.QueueStatusInService}
	repo.queues[uuid.New()] = &domain.Queue{ID: uuid.New(), WorkshopID: wsID, QueueDate: today, QueueNumber: 2, Status: domain.QueueStatusWaiting}
	repo.queues[uuid.New()] = &domain.Queue{ID: uuid.New(), WorkshopID: wsID, QueueDate: today, QueueNumber: 3, Status: domain.QueueStatusWaiting}

	summary, err := svc.GetWorkshopQueueSummary(context.Background(), wsID, today)
	if err != nil {
		t.Fatalf("error getting summary: %v", err)
	}

	if summary.TotalWaiting != 2 {
		t.Errorf("expected 2 waiting, got %d", summary.TotalWaiting)
	}
	if summary.TotalInService != 1 {
		t.Errorf("expected 1 in service, got %d", summary.TotalInService)
	}
	if summary.CurrentServingNo == nil || *summary.CurrentServingNo != 1 {
		t.Errorf("expected current serving number 1")
	}
}
