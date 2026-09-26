package domain

import (
	"time"

	"github.com/google/uuid"
)

type DevicePlatform string

const (
	PlatformAndroid DevicePlatform = "ANDROID"
	PlatformIOS     DevicePlatform = "IOS"
	PlatformWeb     DevicePlatform = "WEB"
)

// UserDevice represents an FCM / APNs registered push notification target device.
type UserDevice struct {
	ID        uuid.UUID      `json:"id"`
	UserID    uuid.UUID      `json:"user_id"`
	Token     string         `json:"token"`
	Platform  DevicePlatform `json:"platform"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// PushNotificationPayload represents a structured message to be delivered to user devices.
type PushNotificationPayload struct {
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Data     map[string]string `json:"data,omitempty"`
	Priority string            `json:"priority,omitempty"` // "HIGH", "NORMAL"
}
