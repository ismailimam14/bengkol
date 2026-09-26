package domain

import (
	"time"

	"github.com/google/uuid"
)

// WorkshopStatus indicates availability of the workshop
type WorkshopStatus string

const (
	WorkshopStatusActive    WorkshopStatus = "ACTIVE"
	WorkshopStatusInactive  WorkshopStatus = "INACTIVE"
	WorkshopStatusSuspended WorkshopStatus = "SUSPENDED"
)

// Workshop represents an auto repair shop
type Workshop struct {
	ID             uuid.UUID        `json:"id"`
	OwnerID        uuid.UUID        `json:"owner_id"`
	Name           string           `json:"name"`
	Description    string           `json:"description,omitempty"`
	Address        string           `json:"address"`
	Latitude       float64          `json:"latitude"`
	Longitude      float64          `json:"longitude"`
	DistanceMeters *float64         `json:"distance_meters,omitempty"`
	Phone          string           `json:"phone"`
	Rating         float64          `json:"rating"`
	ReviewCount    int              `json:"review_count"`
	Status         WorkshopStatus   `json:"status"`
	OperatingHours []OperatingHour  `json:"operating_hours,omitempty"`
	Services       []Service        `json:"services,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

// OperatingHour represents daily operating schedules
type OperatingHour struct {
	ID         uuid.UUID `json:"id"`
	WorkshopID uuid.UUID `json:"workshop_id"`
	DayOfWeek  int       `json:"day_of_week"` // 0 = Sunday, 1 = Monday, ..., 6 = Saturday
	OpenTime   string    `json:"open_time"`   // Format "15:04:05" or "15:04"
	CloseTime  string    `json:"close_time"`
	IsClosed   bool      `json:"is_closed"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
