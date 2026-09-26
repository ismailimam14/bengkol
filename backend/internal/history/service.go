package history

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/validator"
	"github.com/google/uuid"
)

var (
	ErrForbidden                 = errors.New("you do not have permission to perform this action")
	ErrInvalidBookingStatus      = errors.New("booking is not in a valid state to be completed")
	ErrSparePartInactive         = errors.New("selected spare part is inactive")
	ErrSparePartWorkshopMismatch = errors.New("spare part does not belong to this workshop")
	ErrValidationFailed          = errors.New("validation failed")
)

// CreateHistoryItemRequest represents a spare part utilized during service
type CreateHistoryItemRequest struct {
	SparePartID uuid.UUID `json:"spare_part_id"`
	Quantity    int       `json:"quantity"`
}

// CreateHistoryRequest payload to finalize a service and record its history
type CreateHistoryRequest struct {
	BookingID  uuid.UUID                  `json:"booking_id"`
	Notes      string                     `json:"notes"`
	SpareParts []CreateHistoryItemRequest `json:"spare_parts"`
}

// Service defines business logic for service history records
type Service interface {
	CreateHistory(ctx context.Context, requestingUserID uuid.UUID, requestingRole domain.UserRole, req CreateHistoryRequest) (*domain.ServiceHistory, map[string]string, error)
	GetHistoryByID(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.ServiceHistory, error)
	GetHistoryByBookingID(ctx context.Context, bookingID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.ServiceHistory, error)
	ListCustomerHistories(ctx context.Context, customerID uuid.UUID, pagination domain.PaginationParams) ([]domain.ServiceHistory, *response.Meta, error)
	ListWorkshopHistories(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams, startDate, endDate *string, requestingUserID uuid.UUID, requestingRole domain.UserRole) ([]domain.ServiceHistory, *response.Meta, error)
}

type historyService struct {
	repo   Repository
	logger *logger.Logger
}

// NewService creates a new instance of History service
func NewService(repo Repository, log *logger.Logger) Service {
	return &historyService{
		repo:   repo,
		logger: log,
	}
}

func (s *historyService) CreateHistory(ctx context.Context, requestingUserID uuid.UUID, requestingRole domain.UserRole, req CreateHistoryRequest) (*domain.ServiceHistory, map[string]string, error) {
	v := validator.New()
	v.Check(req.BookingID != uuid.Nil, "booking_id", "booking ID is required")

	for i, p := range req.SpareParts {
		v.Check(p.SparePartID != uuid.Nil, fmt.Sprintf("spare_parts[%d].spare_part_id", i), "spare part ID is required")
		v.Check(p.Quantity > 0, fmt.Sprintf("spare_parts[%d].quantity", i), "quantity must be greater than zero")
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	// 1. Fetch Booking and verify Workshop ownership
	b, err := s.repo.GetBookingByID(ctx, req.BookingID)
	if err != nil {
		return nil, nil, err
	}

	if requestingRole != domain.RoleAdmin {
		ws, err := s.repo.GetWorkshopByID(ctx, b.WorkshopID)
		if err != nil || ws.OwnerID != requestingUserID {
			return nil, nil, ErrForbidden
		}
	}

	// Check if history already exists
	existing, err := s.repo.GetHistoryByBookingID(ctx, req.BookingID)
	if err == nil && existing != nil {
		return nil, nil, ErrHistoryExists
	}

	// Booking status check: cannot complete cancelled or no-show bookings
	if b.Status == domain.BookingStatusCancelled || b.Status == domain.BookingStatusNoShow {
		return nil, nil, ErrInvalidBookingStatus
	}

	now := time.Now()
	serviceDate := b.BookingDate
	if serviceDate == "" {
		serviceDate = now.Format("2006-01-02")
	}

	servicePrice := 0.0
	if b.Service != nil {
		servicePrice = b.Service.Price
	}

	// 2. Begin Transaction
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	if tx != nil {
		defer tx.Rollback()
	}

	historyID := uuid.New()
	var sparePartRecords []domain.ServiceHistorySparePart
	totalSparePartsPrice := 0.0

	// 3. Process Spare Parts Inventory & Snapshot Pricing
	for _, item := range req.SpareParts {
		sp, err := s.repo.GetSparePartForUpdateInTx(ctx, tx, item.SparePartID)
		if err != nil {
			return nil, nil, err
		}

		if sp.WorkshopID != b.WorkshopID {
			return nil, nil, ErrSparePartWorkshopMismatch
		}
		if !sp.IsActive {
			return nil, nil, ErrSparePartInactive
		}
		if sp.Stock < item.Quantity {
			return nil, nil, ErrInsufficientStock
		}

		// Decrement inventory stock atomically
		if err := s.repo.DecrementSparePartStockInTx(ctx, tx, sp.ID, item.Quantity); err != nil {
			return nil, nil, err
		}

		subtotal := sp.SellingPrice * float64(item.Quantity)
		totalSparePartsPrice += subtotal

		spRecord := domain.ServiceHistorySparePart{
			ID:               uuid.New(),
			ServiceHistoryID: historyID,
			SparePartID:      sp.ID,
			Quantity:         item.Quantity,
			PricePerUnit:     sp.SellingPrice,
			Subtotal:         subtotal,
			CreatedAt:        now,
			SparePart:        sp,
		}

		if err := s.repo.CreateServiceHistorySparePartInTx(ctx, tx, &spRecord); err != nil {
			return nil, nil, err
		}

		sparePartRecords = append(sparePartRecords, spRecord)
	}

	totalPrice := servicePrice + totalSparePartsPrice

	// 4. Create Service History Record
	historyRecord := &domain.ServiceHistory{
		ID:           historyID,
		BookingID:    req.BookingID,
		CustomerID:   b.CustomerID,
		WorkshopID:   b.WorkshopID,
		ServiceID:    b.ServiceID,
		ServiceDate:  serviceDate,
		ServicePrice: servicePrice,
		TotalPrice:   totalPrice,
		Notes:        req.Notes,
		SpareParts:   sparePartRecords,
		CreatedAt:    now,
		Workshop:     b.Workshop,
		Service:      b.Service,
	}

	if err := s.repo.CreateServiceHistoryInTx(ctx, tx, historyRecord); err != nil {
		return nil, nil, err
	}

	// 5. Update Booking status to COMPLETED
	if err := s.repo.UpdateBookingStatusInTx(ctx, tx, b.ID, domain.BookingStatusCompleted); err != nil {
		return nil, nil, err
	}

	// 6. Update associated queue ticket (if any) to COMPLETED
	if err := s.repo.CompleteQueueForBookingInTx(ctx, tx, b.ID); err != nil {
		return nil, nil, err
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("failed to commit service history transaction: %w", err)
		}
	}

	s.logger.WithContext(ctx).Info("service history recorded successfully",
		"history_id", historyRecord.ID,
		"booking_id", historyRecord.BookingID,
		"workshop_id", historyRecord.WorkshopID,
		"total_price", historyRecord.TotalPrice,
	)

	return historyRecord, nil, nil
}

func (s *historyService) GetHistoryByID(ctx context.Context, id uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.ServiceHistory, error) {
	h, err := s.repo.GetHistoryByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if requestingRole != domain.RoleAdmin && h.CustomerID != requestingUserID {
		if h.Workshop == nil || h.Workshop.OwnerID != requestingUserID {
			return nil, ErrForbidden
		}
	}

	return h, nil
}

func (s *historyService) GetHistoryByBookingID(ctx context.Context, bookingID uuid.UUID, requestingUserID uuid.UUID, requestingRole domain.UserRole) (*domain.ServiceHistory, error) {
	h, err := s.repo.GetHistoryByBookingID(ctx, bookingID)
	if err != nil {
		return nil, err
	}

	if requestingRole != domain.RoleAdmin && h.CustomerID != requestingUserID {
		if h.Workshop == nil || h.Workshop.OwnerID != requestingUserID {
			return nil, ErrForbidden
		}
	}

	return h, nil
}

func (s *historyService) ListCustomerHistories(ctx context.Context, customerID uuid.UUID, pagination domain.PaginationParams) ([]domain.ServiceHistory, *response.Meta, error) {
	pagination.EnsureValid()

	list, total, err := s.repo.ListHistoriesByCustomer(ctx, customerID, pagination)
	if err != nil {
		return nil, nil, err
	}

	if list == nil {
		list = []domain.ServiceHistory{}
	}

	meta := &response.Meta{
		Page:       pagination.Page,
		PageSize:   pagination.PageSize,
		TotalItems: total,
		TotalPages: int((total + int64(pagination.PageSize) - 1) / int64(pagination.PageSize)),
	}

	return list, meta, nil
}

func (s *historyService) ListWorkshopHistories(ctx context.Context, workshopID uuid.UUID, pagination domain.PaginationParams, startDate, endDate *string, requestingUserID uuid.UUID, requestingRole domain.UserRole) ([]domain.ServiceHistory, *response.Meta, error) {
	ws, err := s.repo.GetWorkshopByID(ctx, workshopID)
	if err != nil {
		return nil, nil, err
	}

	if requestingRole != domain.RoleAdmin && ws.OwnerID != requestingUserID {
		return nil, nil, ErrForbidden
	}

	pagination.EnsureValid()

	list, total, err := s.repo.ListHistoriesByWorkshop(ctx, workshopID, pagination, startDate, endDate)
	if err != nil {
		return nil, nil, err
	}

	if list == nil {
		list = []domain.ServiceHistory{}
	}

	meta := &response.Meta{
		Page:       pagination.Page,
		PageSize:   pagination.PageSize,
		TotalItems: total,
		TotalPages: int((total + int64(pagination.PageSize) - 1) / int64(pagination.PageSize)),
	}

	return list, meta, nil
}
