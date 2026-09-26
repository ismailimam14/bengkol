package domain

import (
	"time"

	"github.com/google/uuid"
)

// QueueStatus represents operational states in serving customers
type QueueStatus string

const (
	QueueStatusWaiting    QueueStatus = "WAITING"
	QueueStatusCalled     QueueStatus = "CALLED"
	QueueStatusInService  QueueStatus = "IN_SERVICE"
	QueueStatusCompleted  QueueStatus = "COMPLETED"
	QueueStatusNoShow     QueueStatus = "NO_SHOW"
	QueueStatusCancelled  QueueStatus = "CANCELLED"
)

// Queue represents the sequential queue entry for workshop operations
type Queue struct {
	ID                 uuid.UUID   `json:"id"`
	BookingID          uuid.UUID   `json:"booking_id"`
	WorkshopID         uuid.UUID   `json:"workshop_id"`
	QueueDate          string      `json:"queue_date"` // Format "YYYY-MM-DD"
	QueueNumber        int         `json:"queue_number"`
	Status             QueueStatus `json:"status"`
	CalledAt           *time.Time  `json:"called_at,omitempty"`
	ServiceStartedAt   *time.Time  `json:"service_started_at,omitempty"`
	ServiceCompletedAt *time.Time  `json:"service_completed_at,omitempty"`
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`

	// Enriched operational fields for client views
	CustomersAhead   int      `json:"customers_ahead,omitempty"`
	EstimatedWaitMin int      `json:"estimated_wait_minutes,omitempty"`
	Booking          *Booking `json:"booking,omitempty"`
}

// QueueSummary represents aggregated live stats for workshop queue board
type QueueSummary struct {
	WorkshopID       uuid.UUID `json:"workshop_id"`
	QueueDate        string    `json:"queue_date"`
	TotalWaiting     int       `json:"total_waiting"`
	TotalInService   int       `json:"total_in_service"`
	TotalCompleted   int       `json:"total_completed"`
	CurrentServingNo *int      `json:"current_serving_number,omitempty"`
	CurrentCalledNo  *int      `json:"current_called_number,omitempty"`
}
