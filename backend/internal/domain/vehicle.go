package domain

import (
	"time"

	"github.com/google/uuid"
)

// VehicleType represents the category of the vehicle
type VehicleType string

const (
	VehicleTypeMotorcycle VehicleType = "MOTORCYCLE"
	VehicleTypeCar        VehicleType = "CAR"
	VehicleTypeOther      VehicleType = "OTHER"
)

// Vehicle represents a customer-owned vehicle
type Vehicle struct {
	ID           uuid.UUID   `json:"id"`
	UserID       uuid.UUID   `json:"user_id"`
	LicensePlate string      `json:"license_plate"`
	Brand        string      `json:"brand"`
	Model        string      `json:"model"`
	Year         int         `json:"year,omitempty"`
	VehicleType  VehicleType `json:"vehicle_type"`
	Color        string      `json:"color,omitempty"`
	Notes        string      `json:"notes,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`

	// Relational references
	User *User `json:"user,omitempty"`
}
