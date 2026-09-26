package domain

import (
	"time"

	"github.com/google/uuid"
)

// Service represents a repair or maintenance service offered by a workshop
type Service struct {
	ID              uuid.UUID `json:"id"`
	WorkshopID      uuid.UUID `json:"workshop_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	Price           float64   `json:"price"`
	DurationMinutes int       `json:"duration_minutes"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
