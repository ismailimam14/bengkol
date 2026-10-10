package domain

import (
	"time"

	"github.com/google/uuid"
)

// UserRole defines access level
type UserRole string

const (
	RoleCustomer UserRole = "CUSTOMER"
	RoleOwner    UserRole = "OWNER"
	RoleAdmin    UserRole = "ADMIN"
	RoleMechanic UserRole = "MECHANIC"
	RoleManager  UserRole = "MANAGER"
)

// User represents a system user
type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Phone        string    `json:"phone"`
	Role         UserRole  `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// RefreshToken represents a stored refresh token
type RefreshToken struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"-"`
	Revoked   bool      `json:"revoked"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}
