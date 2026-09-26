package domain

import (
	"time"

	"github.com/google/uuid"
)

// Review represents customer ratings and feedback for a completed booking
type Review struct {
	ID         uuid.UUID `json:"id"`
	BookingID  uuid.UUID `json:"booking_id"`
	CustomerID uuid.UUID `json:"customer_id"`
	WorkshopID uuid.UUID `json:"workshop_id"`
	Rating     int       `json:"rating"` // 1 to 5
	Comment    string    `json:"comment,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Customer *User `json:"customer,omitempty"`
}
