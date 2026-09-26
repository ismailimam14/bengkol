package domain

import (
	"time"

	"github.com/google/uuid"
)

// ServiceHistory represents the permanent completed service record
type ServiceHistory struct {
	ID           uuid.UUID                  `json:"id"`
	BookingID    uuid.UUID                  `json:"booking_id"`
	CustomerID   uuid.UUID                  `json:"customer_id"`
	WorkshopID   uuid.UUID                  `json:"workshop_id"`
	ServiceID    uuid.UUID                  `json:"service_id"`
	ServiceDate  string                     `json:"service_date"`
	ServicePrice float64                    `json:"service_price"`
	TotalPrice   float64                    `json:"total_price"`
	Notes        string                     `json:"notes,omitempty"`
	SpareParts   []ServiceHistorySparePart `json:"spare_parts,omitempty"`
	CreatedAt    time.Time                  `json:"created_at"`

	Workshop *Workshop `json:"workshop,omitempty"`
	Service  *Service  `json:"service,omitempty"`
}

// ServiceHistorySparePart records the spare parts used during service execution
type ServiceHistorySparePart struct {
	ID               uuid.UUID `json:"id"`
	ServiceHistoryID uuid.UUID `json:"service_history_id"`
	SparePartID      uuid.UUID `json:"spare_part_id"`
	Quantity         int       `json:"quantity"`
	PricePerUnit     float64   `json:"price_per_unit"`
	Subtotal         float64   `json:"subtotal"`
	CreatedAt        time.Time `json:"created_at"`

	SparePart *SparePart `json:"spare_part,omitempty"`
}
