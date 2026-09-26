package domain

import (
	"time"

	"github.com/google/uuid"
)

// PaginationParams represents standard request pagination.
type PaginationParams struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// EnsureValid ensures pagination numbers are within acceptable ranges.
func (p *PaginationParams) EnsureValid() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
}

// Offset calculates the database offset.
func (p PaginationParams) Offset() int {
	return (p.Page - 1) * p.PageSize
}

// BaseEntity holds common entity properties.
type BaseEntity struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
