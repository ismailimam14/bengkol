package domain

import (
	"time"

	"github.com/google/uuid"
)

// SparePart represents an auto component in a workshop's inventory
type SparePart struct {
	ID            uuid.UUID `json:"id"`
	WorkshopID    uuid.UUID `json:"workshop_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	PurchasePrice float64   `json:"purchase_price"`
	SellingPrice  float64   `json:"selling_price"`
	Stock         int       `json:"stock"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
