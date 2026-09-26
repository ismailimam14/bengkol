package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

var (
	ErrForbidden            = errors.New("you do not have permission to perform this action")
	ErrInvalidQueueStatus   = errors.New("invalid queue status transition")
	ErrInvalidBookingStatus = errors.New("booking is not eligible for check-in")
	ErrInvalidCheckInDate   = errors.New("check-in is only allowed on the scheduled booking date")
	ErrNoWaitingInQueue     = errors.New("no waiting tickets in queue")
)

// Broadcaster allows emitting real-time notifications across WebSocket topics.
type Broadcaster interface {
	Broadcast(topic string, event string, data interface{})
}

// PushDispatcher allows sending native mobile push notifications to users.
type PushDispatcher interface {
	SendToUser(ctx context.Context, userID uuid.UUID, payload domain.PushNotificationPayload) error
}

// Service defines the business logic for queue management
type Service interface {
	CheckInBooking(ctx context.Context, bookingID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error)
	GetQueueByID(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error)
	GetActiveCustomerQueue(ctx context.Context, customerID uuid.UUID) (*domain.Queue, error)
	GetWorkshopQueueList(ctx context.Context, workshopID uuid.UUID, date string, status *domain.QueueStatus, requestingUserID uuid.UUID, requestingRole domain.UserRole) ([]domain.Queue, error)
	GetWorkshopQueueSummary(ctx context.Context, workshopID uuid.UUID, date string) (*domain.QueueSummary, error)
	CallQueue(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error)
	CallNextQueue(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error)
	StartService(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error)
	CompleteService(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error)
	MarkNoShow(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error)
	CancelQueue(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error)
}

type queueService struct {
	repo        Repository
	broadcaster Broadcaster
	dispatcher  PushDispatcher
	logger      *logger.Logger
}

// NewService creates a new instance of queue service
func NewService(repo Repository, log *logger.Logger, broadcaster Broadcaster, dispatcher PushDispatcher) Service {
	return &queueService{
		repo:        repo,
		broadcaster: broadcaster,
		dispatcher:  dispatcher,
		logger:      log,
	}
}

func (s *queueService) broadcast(topic string, event string, data interface{}) {
	if s.broadcaster != nil {
		s.broadcaster.Broadcast(topic, event, data)
	}
}

func (s *queueService) dispatchPush(ctx context.Context, userID uuid.UUID, payload domain.PushNotificationPayload) {
	if s.dispatcher != nil {
		_ = s.dispatcher.SendToUser(ctx, userID, payload)
	}
}

func (s *queueService) CheckInBooking(ctx context.Context, bookingID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error) {
	b, err := s.repo.GetBookingByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}

	// Authorization: Customer of booking, Owner of workshop, or Admin
	if requestingRole != domain.RoleAdmin && b.CustomerID != requestingUserID {
		ws, err := s.repo.GetWorkshopByID(ctx, b.WorkshopID)
		if err != nil || ws.OwnerID != requestingUserID {
			return nil, ErrForbidden
		}
	}

	// Status eligibility
	if b.Status != domain.BookingStatusConfirmed {
		return nil, ErrInvalidBookingStatus
	}

	// Date check: BookingDate must be today
	today := time.Now().Format("2006-01-02")
	if b.BookingDate != today {
		return nil, ErrInvalidCheckInDate
	}

	// Check if already checked in
	existing, err := s.repo.GetQueueByBookingID(ctx, bookingID)
	if err == nil && existing != nil {
		// Populate ahead and wait estimation
		ahead, _ := s.repo.CountAheadInQueue(ctx, existing.WorkshopID, existing.QueueDate, existing.QueueNumber)
		existing.CustomersAhead = ahead
		existing.EstimatedWaitMin = ahead * 20
		return existing, nil
	}

	now := time.Now()
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	if tx != nil {
		defer tx.Rollback()
	}

	// Generate next queue number atomically
	nextNum, err := s.repo.GetNextQueueNumberInTx(ctx, tx, b.WorkshopID, today)
	if err != nil {
		return nil, err
	}

	queueID := uuid.New()
	q := &domain.Queue{
		ID:          queueID,
		BookingID:   bookingID,
		WorkshopID:  b.WorkshopID,
		QueueDate:   today,
		QueueNumber: nextNum,
		Status:      domain.QueueStatusWaiting,
		CreatedAt:   now,
		UpdatedAt:   now,
		Booking:     b,
	}

	if err := s.repo.CreateQueueInTx(ctx, tx, q); err != nil {
		return nil, err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit check-in transaction: %w", err)
		}
	}

	ahead, _ := s.repo.CountAheadInQueue(ctx, b.WorkshopID, today, nextNum)
	q.CustomersAhead = ahead
	q.EstimatedWaitMin = ahead * 20

	s.logger.WithContext(ctx).Info("queue check-in successful",
		"queue_id", q.ID,
		"queue_number", q.QueueNumber,
		"booking_id", bookingID,
		"workshop_id", b.WorkshopID,
	)

	s.broadcast(fmt.Sprintf("workshop:%s", b.WorkshopID), "QUEUE_CHECKED_IN", q)
	s.broadcast(fmt.Sprintf("user:%s", b.CustomerID), "QUEUE_CHECKED_IN", q)

	s.dispatchPush(ctx, b.CustomerID, domain.PushNotificationPayload{
		Title: "Check-in Antrean Berhasil ✅",
		Body:  fmt.Sprintf("Nomor antrean Anda adalah #%d. Estimasi waktu tunggu: %d menit.", q.QueueNumber, q.EstimatedWaitMin),
		Data: map[string]string{
			"type":        "QUEUE_CHECKED_IN",
			"queue_id":    q.ID.String(),
			"workshop_id": b.WorkshopID.String(),
			"queue_num":   fmt.Sprintf("%d", q.QueueNumber),
		},
	})

	return q, nil
}

func (s *queueService) GetQueueByID(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error) {
	q, err := s.repo.GetQueueByID(ctx, queueID)
	if err != nil {
		return nil, err
	}

	// Authorization
	if requestingRole != domain.RoleAdmin && q.Booking != nil && q.Booking.CustomerID != requestingUserID {
		if q.Booking.Workshop == nil || q.Booking.Workshop.OwnerID != requestingUserID {
			return nil, ErrForbidden
		}
	}

	if q.Status == domain.QueueStatusWaiting || q.Status == domain.QueueStatusCalled {
		ahead, _ := s.repo.CountAheadInQueue(ctx, q.WorkshopID, q.QueueDate, q.QueueNumber)
		q.CustomersAhead = ahead
		q.EstimatedWaitMin = ahead * 20
	}

	return q, nil
}

func (s *queueService) GetActiveCustomerQueue(ctx context.Context, customerID uuid.UUID) (*domain.Queue, error) {
	today := time.Now().Format("2006-01-02")
	q, err := s.repo.GetActiveQueueByCustomerID(ctx, customerID, today)
	if err != nil {
		return nil, err
	}

	if q.Status == domain.QueueStatusWaiting || q.Status == domain.QueueStatusCalled {
		ahead, _ := s.repo.CountAheadInQueue(ctx, q.WorkshopID, q.QueueDate, q.QueueNumber)
		q.CustomersAhead = ahead
		q.EstimatedWaitMin = ahead * 20
	}

	return q, nil
}

func (s *queueService) GetWorkshopQueueList(ctx context.Context, workshopID uuid.UUID, date string, status *domain.QueueStatus, requestingUserID uuid.UUID, requestingRole domain.UserRole) ([]domain.Queue, error) {
	ws, err := s.repo.GetWorkshopByID(ctx, workshopID)
	if err != nil {
		return nil, err
	}

	// Authorization: Workshop owner or Admin
	if requestingRole != domain.RoleAdmin && ws.OwnerID != requestingUserID {
		return nil, ErrForbidden
	}

	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	queues, err := s.repo.ListQueuesByWorkshop(ctx, workshopID, date, status)
	if err != nil {
		return nil, err
	}

	for i := range queues {
		if queues[i].Status == domain.QueueStatusWaiting || queues[i].Status == domain.QueueStatusCalled {
			ahead, _ := s.repo.CountAheadInQueue(ctx, workshopID, date, queues[i].QueueNumber)
			queues[i].CustomersAhead = ahead
			queues[i].EstimatedWaitMin = ahead * 20
		}
	}

	return queues, nil
}

func (s *queueService) GetWorkshopQueueSummary(ctx context.Context, workshopID uuid.UUID, date string) (*domain.QueueSummary, error) {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	return s.repo.GetQueueSummary(ctx, workshopID, date)
}

func (s *queueService) CallQueue(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		defer tx.Rollback()
	}

	q, err := s.repo.GetQueueByIDForUpdate(ctx, tx, queueID)
	if err != nil {
		return nil, err
	}

	// Authorization
	if requestingRole != domain.RoleAdmin {
		ws, err := s.repo.GetWorkshopByID(ctx, q.WorkshopID)
		if err != nil || ws.OwnerID != requestingUserID {
			return nil, ErrForbidden
		}
	}

	if q.Status != domain.QueueStatusWaiting && q.Status != domain.QueueStatusCalled {
		return nil, ErrInvalidQueueStatus
	}

	now := time.Now()
	q.Status = domain.QueueStatusCalled
	q.CalledAt = &now
	q.UpdatedAt = now

	if err := s.repo.UpdateQueueStatusInTx(ctx, tx, q); err != nil {
		return nil, err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	s.logger.WithContext(ctx).Info("queue called", "queue_id", q.ID, "queue_number", q.QueueNumber)

	s.broadcast(fmt.Sprintf("workshop:%s", q.WorkshopID), "QUEUE_CALLED", q)
	if b, err := s.repo.GetBookingByID(ctx, q.BookingID); err == nil && b != nil {
		q.Booking = b
		s.broadcast(fmt.Sprintf("user:%s", b.CustomerID), "QUEUE_CALLED", q)
		s.dispatchPush(ctx, b.CustomerID, domain.PushNotificationPayload{
			Title:    "Nomor Antrean Dipanggil! 📢",
			Body:     fmt.Sprintf("Nomor antrean #%d Anda telah dipanggil. Silakan menuju stall servis.", q.QueueNumber),
			Priority: "HIGH",
			Data: map[string]string{
				"type":        "QUEUE_CALLED",
				"queue_id":    q.ID.String(),
				"workshop_id": q.WorkshopID.String(),
				"queue_num":   fmt.Sprintf("%d", q.QueueNumber),
			},
		})
	}

	return q, nil
}

func (s *queueService) CallNextQueue(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error) {
	ws, err := s.repo.GetWorkshopByID(ctx, workshopID)
	if err != nil {
		return nil, err
	}

	if requestingRole != domain.RoleAdmin && ws.OwnerID != requestingUserID {
		return nil, ErrForbidden
	}

	today := time.Now().Format("2006-01-02")
	waitingStatus := domain.QueueStatusWaiting
	waitingList, err := s.repo.ListQueuesByWorkshop(ctx, workshopID, today, &waitingStatus)
	if err != nil {
		return nil, err
	}

	if len(waitingList) == 0 {
		return nil, ErrNoWaitingInQueue
	}

	// Call the first waiting ticket
	targetID := waitingList[0].ID
	return s.CallQueue(ctx, targetID, requestingUserID, requestingRole)
}

func (s *queueService) StartService(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		defer tx.Rollback()
	}

	q, err := s.repo.GetQueueByIDForUpdate(ctx, tx, queueID)
	if err != nil {
		return nil, err
	}

	// Authorization
	if requestingRole != domain.RoleAdmin {
		ws, err := s.repo.GetWorkshopByID(ctx, q.WorkshopID)
		if err != nil || ws.OwnerID != requestingUserID {
			return nil, ErrForbidden
		}
	}

	if q.Status != domain.QueueStatusWaiting && q.Status != domain.QueueStatusCalled {
		return nil, ErrInvalidQueueStatus
	}

	now := time.Now()
	q.Status = domain.QueueStatusInService
	q.ServiceStartedAt = &now
	q.UpdatedAt = now

	if err := s.repo.UpdateQueueStatusInTx(ctx, tx, q); err != nil {
		return nil, err
	}

	// Update associated booking to IN_SERVICE
	if err := s.repo.UpdateBookingStatusInTx(ctx, tx, q.BookingID, domain.BookingStatusInService); err != nil {
		return nil, err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	s.logger.WithContext(ctx).Info("service started for queue", "queue_id", q.ID, "booking_id", q.BookingID)

	s.broadcast(fmt.Sprintf("workshop:%s", q.WorkshopID), "QUEUE_STARTED", q)
	if b, err := s.repo.GetBookingByID(ctx, q.BookingID); err == nil && b != nil {
		q.Booking = b
		s.broadcast(fmt.Sprintf("user:%s", b.CustomerID), "QUEUE_STARTED", q)
		s.dispatchPush(ctx, b.CustomerID, domain.PushNotificationPayload{
			Title: "Pengerjaan Servis Dimulai 🔧",
			Body:  "Kendaraan Anda sedang dalam proses pengerjaan oleh teknisi bengkel.",
			Data: map[string]string{
				"type":     "QUEUE_STARTED",
				"queue_id": q.ID.String(),
			},
		})
	}

	return q, nil
}

func (s *queueService) CompleteService(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		defer tx.Rollback()
	}

	q, err := s.repo.GetQueueByIDForUpdate(ctx, tx, queueID)
	if err != nil {
		return nil, err
	}

	// Authorization
	if requestingRole != domain.RoleAdmin {
		ws, err := s.repo.GetWorkshopByID(ctx, q.WorkshopID)
		if err != nil || ws.OwnerID != requestingUserID {
			return nil, ErrForbidden
		}
	}

	if q.Status != domain.QueueStatusInService {
		return nil, ErrInvalidQueueStatus
	}

	now := time.Now()
	q.Status = domain.QueueStatusCompleted
	q.ServiceCompletedAt = &now
	q.UpdatedAt = now

	if err := s.repo.UpdateQueueStatusInTx(ctx, tx, q); err != nil {
		return nil, err
	}

	// Update associated booking to COMPLETED
	if err := s.repo.UpdateBookingStatusInTx(ctx, tx, q.BookingID, domain.BookingStatusCompleted); err != nil {
		return nil, err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	s.logger.WithContext(ctx).Info("service completed for queue", "queue_id", q.ID, "booking_id", q.BookingID)

	s.broadcast(fmt.Sprintf("workshop:%s", q.WorkshopID), "QUEUE_COMPLETED", q)
	if b, err := s.repo.GetBookingByID(ctx, q.BookingID); err == nil && b != nil {
		q.Booking = b
		s.broadcast(fmt.Sprintf("user:%s", b.CustomerID), "QUEUE_COMPLETED", q)
		s.dispatchPush(ctx, b.CustomerID, domain.PushNotificationPayload{
			Title: "Servis Kendaraan Selesai! 🎉",
			Body:  "Pengerjaan kendaraan Anda telah selesai. Silakan lakukan penyelesaian pembayaran dan pengambilan.",
			Data: map[string]string{
				"type":        "QUEUE_COMPLETED",
				"queue_id":    q.ID.String(),
				"booking_id":  b.ID.String(),
				"workshop_id": q.WorkshopID.String(),
			},
		})
	}

	return q, nil
}

func (s *queueService) MarkNoShow(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		defer tx.Rollback()
	}

	q, err := s.repo.GetQueueByIDForUpdate(ctx, tx, queueID)
	if err != nil {
		return nil, err
	}

	// Authorization
	if requestingRole != domain.RoleAdmin {
		ws, err := s.repo.GetWorkshopByID(ctx, q.WorkshopID)
		if err != nil || ws.OwnerID != requestingUserID {
			return nil, ErrForbidden
		}
	}

	if q.Status != domain.QueueStatusWaiting && q.Status != domain.QueueStatusCalled {
		return nil, ErrInvalidQueueStatus
	}

	now := time.Now()
	q.Status = domain.QueueStatusNoShow
	q.UpdatedAt = now

	if err := s.repo.UpdateQueueStatusInTx(ctx, tx, q); err != nil {
		return nil, err
	}

	if err := s.repo.UpdateBookingStatusInTx(ctx, tx, q.BookingID, domain.BookingStatusNoShow); err != nil {
		return nil, err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	s.logger.WithContext(ctx).Info("marked queue as no-show", "queue_id", q.ID, "booking_id", q.BookingID)

	s.broadcast(fmt.Sprintf("workshop:%s", q.WorkshopID), "QUEUE_NO_SHOW", q)
	if b, err := s.repo.GetBookingByID(ctx, q.BookingID); err == nil && b != nil {
		q.Booking = b
		s.broadcast(fmt.Sprintf("user:%s", b.CustomerID), "QUEUE_NO_SHOW", q)
	}

	return q, nil
}

func (s *queueService) CancelQueue(ctx context.Context, queueID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Queue, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		defer tx.Rollback()
	}

	q, err := s.repo.GetQueueByIDForUpdate(ctx, tx, queueID)
	if err != nil {
		return nil, err
	}

	// Authorization: Customer of booking, Owner, or Admin
	b, err := s.repo.GetBookingByID(ctx, q.BookingID)
	if err != nil {
		return nil, err
	}
	if requestingRole != domain.RoleAdmin {
		if b.CustomerID != requestingUserID {
			ws, err := s.repo.GetWorkshopByID(ctx, q.WorkshopID)
			if err != nil || ws.OwnerID != requestingUserID {
				return nil, ErrForbidden
			}
		}
	}

	if q.Status == domain.QueueStatusCompleted || q.Status == domain.QueueStatusInService || q.Status == domain.QueueStatusCancelled {
		return nil, ErrInvalidQueueStatus
	}

	now := time.Now()
	q.Status = domain.QueueStatusCancelled
	q.UpdatedAt = now

	if err := s.repo.UpdateQueueStatusInTx(ctx, tx, q); err != nil {
		return nil, err
	}

	if err := s.repo.UpdateBookingStatusInTx(ctx, tx, q.BookingID, domain.BookingStatusCancelled); err != nil {
		return nil, err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	s.logger.WithContext(ctx).Info("queue cancelled", "queue_id", q.ID, "booking_id", q.BookingID)

	s.broadcast(fmt.Sprintf("workshop:%s", q.WorkshopID), "QUEUE_CANCELLED", q)
	s.broadcast(fmt.Sprintf("user:%s", b.CustomerID), "QUEUE_CANCELLED", q)

	return q, nil
}
