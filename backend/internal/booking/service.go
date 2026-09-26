package booking

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/validator"
	"github.com/google/uuid"
)

var (
	ErrForbidden             = errors.New("you do not have permission to access this booking")
	ErrValidationFailed      = errors.New("validation failed")
	ErrWorkshopClosed        = errors.New("workshop is closed on the selected date or time")
	ErrPastDateNotAllowed    = errors.New("cannot book a date in the past")
	ErrServiceInactive       = errors.New("the selected service is currently inactive")
	ErrServiceWorkshopMismatch = errors.New("the selected service does not belong to this workshop")
)

// AvailableSlotDTO represents calculated slot capacity for clients
type AvailableSlotDTO struct {
	StartTime   string `json:"start_time"` // "09:00:00"
	EndTime     string `json:"end_time"`   // "10:00:00"
	MaxCapacity int    `json:"max_capacity"`
	BookedCount int    `json:"booked_count"`
	IsAvailable bool   `json:"is_available"`
}

// AvailableSlotsResponse contains the day schedule and available slots
type AvailableSlotsResponse struct {
	WorkshopID uuid.UUID          `json:"workshop_id"`
	Date       string             `json:"date"`
	DayOfWeek  int                `json:"day_of_week"`
	IsClosed   bool               `json:"is_closed"`
	OpenTime   string             `json:"open_time,omitempty"`
	CloseTime  string             `json:"close_time,omitempty"`
	Slots      []AvailableSlotDTO `json:"slots"`
}

// CreateBookingRequest DTO
type CreateBookingRequest struct {
	WorkshopID    uuid.UUID `json:"workshop_id"`
	ServiceID     uuid.UUID `json:"service_id"`
	BookingDate   string    `json:"booking_date"`   // "2026-09-27"
	BookingTime   string    `json:"booking_time"`   // "10:00:00" or "10:00"
	CustomerNotes string    `json:"customer_notes"`
}

// Service defines booking use case operations.
type Service interface {
	GetAvailableSlots(ctx context.Context, workshopID uuid.UUID, dateStr string) (*AvailableSlotsResponse, error)
	CreateBooking(ctx context.Context, customerID uuid.UUID, req CreateBookingRequest) (*domain.Booking, map[string]string, error)
	CancelBooking(ctx context.Context, bookingID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) error
	GetBookingByID(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Booking, error)
	ListCustomerBookings(ctx context.Context, customerID uuid.UUID, pagination domain.PaginationParams) ([]domain.Booking, *response.Meta, error)
	ListWorkshopBookings(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, pagination domain.PaginationParams, date *string) ([]domain.Booking, *response.Meta, error)
}

type bookingService struct {
	repo   Repository
	logger *logger.Logger
}

// NewService creates a new Booking Service instance.
func NewService(repo Repository, log *logger.Logger) Service {
	return &bookingService{
		repo:   repo,
		logger: log,
	}
}

func (s *bookingService) GetAvailableSlots(ctx context.Context, workshopID uuid.UUID, dateStr string) (*AvailableSlotsResponse, error) {
	targetDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil, fmt.Errorf("invalid date format; use YYYY-MM-DD: %w", err)
	}

	dayOfWeek := int(targetDate.Weekday()) // 0=Sunday, 1=Monday, ..., 6=Saturday

	opHour, err := s.repo.GetWorkshopOperatingHour(ctx, workshopID, dayOfWeek)
	if err != nil {
		return nil, err
	}

	resp := &AvailableSlotsResponse{
		WorkshopID: workshopID,
		Date:       dateStr,
		DayOfWeek:  dayOfWeek,
		IsClosed:   true,
		Slots:      []AvailableSlotDTO{},
	}

	// If no operating hours configured or workshop closed on this day
	if opHour == nil || opHour.IsClosed {
		return resp, nil
	}

	resp.IsClosed = false
	resp.OpenTime = normalizeTime(opHour.OpenTime)
	resp.CloseTime = normalizeTime(opHour.CloseTime)

	openT, err := time.Parse("15:04", resp.OpenTime)
	if err != nil {
		openT, _ = time.Parse("15:04:05", resp.OpenTime)
	}
	closeT, err := time.Parse("15:04", resp.CloseTime)
	if err != nil {
		closeT, _ = time.Parse("15:04:05", resp.CloseTime)
	}

	// Fetch existing recorded booking slots for that day
	existingSlots, err := s.repo.GetSlotsByDate(ctx, workshopID, dateStr)
	if err != nil {
		return nil, err
	}

	slotMap := make(map[string]domain.BookingSlot)
	for _, sl := range existingSlots {
		slotMap[normalizeTime(sl.StartTime)] = sl
	}

	// Generate 1-hour interval slots
	currentT := openT
	now := time.Now()
	isToday := targetDate.Format("2006-01-02") == now.Format("2006-01-02")

	for currentT.Before(closeT) {
		nextT := currentT.Add(1 * time.Hour)
		if nextT.After(closeT) {
			break
		}

		startStr := currentT.Format("15:04:00")
		endStr := nextT.Format("15:04:00")
		normStart := normalizeTime(startStr)

		maxCapacity := 2 // default 2 service bays
		bookedCount := 0
		isAvailable := true

		if sl, exists := slotMap[normStart]; exists {
			maxCapacity = sl.MaxCapacity
			bookedCount = sl.BookedCount
			isAvailable = sl.IsAvailable && (bookedCount < maxCapacity)
		}

		// If today, check if time has already passed
		if isToday {
			slotStartToday := time.Date(now.Year(), now.Month(), now.Day(), currentT.Hour(), currentT.Minute(), 0, 0, now.Location())
			if slotStartToday.Before(now) {
				isAvailable = false
			}
		}

		resp.Slots = append(resp.Slots, AvailableSlotDTO{
			StartTime:   startStr,
			EndTime:     endStr,
			MaxCapacity: maxCapacity,
			BookedCount: bookedCount,
			IsAvailable: isAvailable,
		})

		currentT = nextT
	}

	return resp, nil
}

func (s *bookingService) CreateBooking(ctx context.Context, customerID uuid.UUID, req CreateBookingRequest) (*domain.Booking, map[string]string, error) {
	v := validator.New()
	v.Check(req.WorkshopID != uuid.Nil, "workshop_id", "workshop_id is required")
	v.Check(req.ServiceID != uuid.Nil, "service_id", "service_id is required")
	v.Required("booking_date", req.BookingDate)
	v.Required("booking_time", req.BookingTime)

	targetDate, err := time.Parse("2006-01-02", req.BookingDate)
	if err != nil {
		v.AddError("booking_date", "invalid date format; expected YYYY-MM-DD")
	}

	now := time.Now().UTC()
	todayDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if err == nil && targetDate.Before(todayDate) {
		v.AddError("booking_date", "cannot book a date in the past")
	}

	normTime := normalizeTime(req.BookingTime)
	if len(normTime) < 5 {
		v.AddError("booking_time", "invalid time format; expected HH:MM or HH:MM:SS")
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	// 1. Verify Workshop & Service
	ws, err := s.repo.GetWorkshopByID(ctx, req.WorkshopID)
	if err != nil {
		return nil, nil, err
	}
	if ws.Status != domain.WorkshopStatusActive {
		v.AddError("workshop_id", "workshop is currently not active")
		return nil, v.Errors, ErrValidationFailed
	}

	srv, err := s.repo.GetServiceByID(ctx, req.ServiceID)
	if err != nil {
		return nil, nil, err
	}
	if srv.WorkshopID != req.WorkshopID {
		v.AddError("service_id", "service does not belong to this workshop")
		return nil, v.Errors, ErrServiceWorkshopMismatch
	}
	if !srv.IsActive {
		v.AddError("service_id", "selected service is inactive")
		return nil, v.Errors, ErrServiceInactive
	}

	// 2. Verify Operating Hours
	dayOfWeek := int(targetDate.Weekday())
	opHour, err := s.repo.GetWorkshopOperatingHour(ctx, req.WorkshopID, dayOfWeek)
	if err != nil {
		return nil, nil, err
	}
	if opHour == nil || opHour.IsClosed {
		return nil, nil, ErrWorkshopClosed
	}

	openNorm := normalizeTime(opHour.OpenTime)
	closeNorm := normalizeTime(opHour.CloseTime)
	if normTime < openNorm || normTime >= closeNorm {
		return nil, nil, ErrWorkshopClosed
	}

	formattedStartTime := normTime + ":00"
	if len(normTime) == 8 {
		formattedStartTime = normTime
	}

	// Calculate end time (1 hour default slot)
	parsedStart, _ := time.Parse("15:04", normTime[:5])
	formattedEndTime := parsedStart.Add(1 * time.Hour).Format("15:04:00")

	// 3. Begin Transaction with Row Locking
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	if tx != nil {
		defer tx.Rollback()
	}

	// Try to get slot for update
	slot, err := s.repo.GetSlotForUpdate(ctx, tx, req.WorkshopID, req.BookingDate, formattedStartTime)
	var slotID uuid.UUID

	if errors.Is(err, ErrSlotNotFound) {
		// Create new slot row
		slotID = uuid.New()
		newSlot := &domain.BookingSlot{
			ID:          slotID,
			WorkshopID:  req.WorkshopID,
			SlotDate:    req.BookingDate,
			StartTime:   formattedStartTime,
			EndTime:     formattedEndTime,
			MaxCapacity: 2, // 2 vehicles capacity
			BookedCount: 1,
			IsAvailable: true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.repo.CreateSlotInTx(ctx, tx, newSlot); err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	} else {
		// Existing slot: check capacity
		if !slot.IsAvailable || slot.BookedCount >= slot.MaxCapacity {
			return nil, nil, ErrSlotUnavailable
		}

		slotID = slot.ID
		newBookedCount := slot.BookedCount + 1
		isAvailable := newBookedCount < slot.MaxCapacity

		if err := s.repo.IncrementSlotBookingInTx(ctx, tx, slot.ID, newBookedCount, isAvailable); err != nil {
			return nil, nil, err
		}
	}

	// 4. Create Booking
	bookingNumber := generateBookingNumber(req.BookingDate)
	booking := &domain.Booking{
		ID:            uuid.New(),
		BookingNumber: bookingNumber,
		CustomerID:    customerID,
		WorkshopID:    req.WorkshopID,
		ServiceID:     req.ServiceID,
		SlotID:        slotID,
		BookingDate:   req.BookingDate,
		BookingTime:   formattedStartTime,
		Status:        domain.BookingStatusConfirmed,
		CustomerNotes: req.CustomerNotes,
		CreatedAt:     now,
		UpdatedAt:     now,
		Workshop:      ws,
		Service:       srv,
	}

	if err := s.repo.CreateBookingInTx(ctx, tx, booking); err != nil {
		return nil, nil, err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("failed to commit booking transaction: %w", err)
		}
	}

	s.logger.WithContext(ctx).Info("booking created successfully",
		"booking_id", booking.ID,
		"booking_number", booking.BookingNumber,
		"customer_id", customerID,
		"workshop_id", req.WorkshopID,
	)

	return booking, nil, nil
}

func (s *bookingService) CancelBooking(ctx context.Context, bookingID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	if tx != nil {
		defer tx.Rollback()
	}

	b, err := s.repo.GetBookingByIDForUpdate(ctx, tx, bookingID)
	if err != nil {
		return err
	}

	// Authorization: Customer who booked, or Owner of workshop, or Admin
	if requestingRole != domain.RoleAdmin && b.CustomerID != requestingUserID {
		ws, err := s.repo.GetWorkshopByID(ctx, b.WorkshopID)
		if err != nil || ws.OwnerID != requestingUserID {
			return ErrForbidden
		}
	}

	// Check status
	if b.Status == domain.BookingStatusCompleted || b.Status == domain.BookingStatusInService || b.Status == domain.BookingStatusCancelled {
		return ErrInvalidOperation
	}

	// Update booking status
	if err := s.repo.UpdateBookingStatusInTx(ctx, tx, bookingID, domain.BookingStatusCancelled); err != nil {
		return err
	}

	// Decrement slot count
	if err := s.repo.DecrementSlotBookingInTx(ctx, tx, b.SlotID); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit cancellation: %w", err)
		}
	}

	s.logger.WithContext(ctx).Info("booking cancelled", "booking_id", bookingID, "user_id", requestingUserID)
	return nil
}

func (s *bookingService) GetBookingByID(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.Booking, error) {
	b, err := s.repo.GetBookingByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Authorization
	if requestingRole != domain.RoleAdmin && b.CustomerID != requestingUserID {
		ws, err := s.repo.GetWorkshopByID(ctx, b.WorkshopID)
		if err != nil || ws.OwnerID != requestingUserID {
			return nil, ErrForbidden
		}
	}

	return b, nil
}

func (s *bookingService) ListCustomerBookings(ctx context.Context, customerID uuid.UUID, pagination domain.PaginationParams) ([]domain.Booking, *response.Meta, error) {
	pagination.EnsureValid()

	list, total, err := s.repo.ListByCustomer(ctx, customerID, pagination)
	if err != nil {
		return nil, nil, err
	}

	totalPages := int(total) / pagination.PageSize
	if int(total)%pagination.PageSize != 0 {
		totalPages++
	}

	meta := &response.Meta{
		Page:       pagination.Page,
		PageSize:   pagination.PageSize,
		TotalItems: total,
		TotalPages: totalPages,
	}

	return list, meta, nil
}

func (s *bookingService) ListWorkshopBookings(ctx context.Context, workshopID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole, pagination domain.PaginationParams, date *string) ([]domain.Booking, *response.Meta, error) {
	// Verify workshop owner
	if requestingRole != domain.RoleAdmin {
		ws, err := s.repo.GetWorkshopByID(ctx, workshopID)
		if err != nil {
			return nil, nil, err
		}
		if ws.OwnerID != requestingUserID {
			return nil, nil, ErrForbidden
		}
	}

	pagination.EnsureValid()

	list, total, err := s.repo.ListByWorkshop(ctx, workshopID, pagination, date)
	if err != nil {
		return nil, nil, err
	}

	totalPages := int(total) / pagination.PageSize
	if int(total)%pagination.PageSize != 0 {
		totalPages++
	}

	meta := &response.Meta{
		Page:       pagination.Page,
		PageSize:   pagination.PageSize,
		TotalItems: total,
		TotalPages: totalPages,
	}

	return list, meta, nil
}

func normalizeTime(t string) string {
	parts := strings.Split(t, ":")
	if len(parts) >= 2 {
		return fmt.Sprintf("%02s:%02s", parts[0], parts[1])
	}
	return t
}

func generateBookingNumber(dateStr string) string {
	cleanDate := strings.ReplaceAll(dateStr, "-", "")
	bytes := make([]byte, 2)
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("BK-%s-%04X", cleanDate, bytes)
}
