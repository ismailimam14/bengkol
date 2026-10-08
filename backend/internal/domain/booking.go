package domain

import (
	"time"

	"github.com/google/uuid"
)

// BookingStatus represents the lifecycle of a booking reservation
type BookingStatus string

const (
	BookingStatusPending    BookingStatus = "PENDING"
	BookingStatusConfirmed  BookingStatus = "CONFIRMED"
	BookingStatusCancelled  BookingStatus = "CANCELLED"
	BookingStatusNoShow     BookingStatus = "NO_SHOW"
	BookingStatusInService  BookingStatus = "IN_SERVICE"
	BookingStatusCompleted  BookingStatus = "COMPLETED"
)

// BookingSlot represents a pre-computed or on-demand time slot capacity
type BookingSlot struct {
	ID          uuid.UUID `json:"id"`
	WorkshopID  uuid.UUID `json:"workshop_id"`
	SlotDate    string    `json:"slot_date"`  // Format "2026-09-27"
	StartTime   string    `json:"start_time"` // Format "10:00:00"
	EndTime     string    `json:"end_time"`   // Format "11:00:00"
	MaxCapacity int       `json:"max_capacity"`
	BookedCount int       `json:"booked_count"`
	IsAvailable bool      `json:"is_available"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BookingSparePart represents a spare part reserved as part of a customer's booking
type BookingSparePart struct {
	ID           uuid.UUID  `json:"id"`
	BookingID    uuid.UUID  `json:"booking_id"`
	SparePartID  uuid.UUID  `json:"spare_part_id"`
	Quantity     int        `json:"quantity"`
	PricePerUnit float64    `json:"price_per_unit"`
	Subtotal     float64    `json:"subtotal"`
	CreatedAt    time.Time  `json:"created_at"`

	SparePart *SparePart `json:"spare_part,omitempty"`
}

// Booking represents a customer's appointment reservation
type Booking struct {
	ID            uuid.UUID     `json:"id"`
	BookingNumber string        `json:"booking_number"`
	CustomerID    uuid.UUID     `json:"customer_id"`
	WorkshopID    uuid.UUID     `json:"workshop_id"`
	ServiceID     uuid.UUID     `json:"service_id"`
	SlotID        uuid.UUID     `json:"slot_id"`
	BookingDate   string        `json:"booking_date"`
	BookingTime   string        `json:"booking_time"`
	Status        BookingStatus `json:"status"`
	CustomerNotes string        `json:"customer_notes,omitempty"`
	TotalPrice    float64       `json:"total_price"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`

	// Relational references
	Customer   *User              `json:"customer,omitempty"`
	Workshop   *Workshop          `json:"workshop,omitempty"`
	Service    *Service           `json:"service,omitempty"`
	SpareParts []BookingSparePart `json:"spare_parts,omitempty"`
	Queue      *Queue             `json:"queue,omitempty"`
}
