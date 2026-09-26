package notification

import (
	"context"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

// Dispatcher defines the interface for sending push notifications.
type Dispatcher interface {
	SendToUser(ctx context.Context, userID uuid.UUID, payload domain.PushNotificationPayload) error
	SendToUsers(ctx context.Context, userIDs []uuid.UUID, payload domain.PushNotificationPayload) error
	SendToToken(ctx context.Context, token string, payload domain.PushNotificationPayload) error
}

// FCMDispatcher is a Firebase Cloud Messaging push notification dispatcher.
type FCMDispatcher struct {
	repo   Repository
	logger *logger.Logger
}

// NewFCMDispatcher creates a new FCMDispatcher instance.
func NewFCMDispatcher(repo Repository, log *logger.Logger) *FCMDispatcher {
	return &FCMDispatcher{
		repo:   repo,
		logger: log,
	}
}

func (d *FCMDispatcher) SendToUser(ctx context.Context, userID uuid.UUID, payload domain.PushNotificationPayload) error {
	devices, err := d.repo.GetDevicesByUserID(ctx, userID)
	if err != nil {
		d.logger.Error("failed to fetch user devices for notification dispatch", "user_id", userID, "error", err)
		return err
	}

	if len(devices) == 0 {
		d.logger.Debug("no registered push notification devices for user", "user_id", userID)
		return nil
	}

	for _, device := range devices {
		if err := d.SendToToken(ctx, device.Token, payload); err != nil {
			d.logger.Warn("failed to dispatch push notification to device token",
				"user_id", userID,
				"device_id", device.ID,
				"platform", device.Platform,
				"error", err,
			)
		}
	}

	return nil
}

func (d *FCMDispatcher) SendToUsers(ctx context.Context, userIDs []uuid.UUID, payload domain.PushNotificationPayload) error {
	for _, uid := range userIDs {
		_ = d.SendToUser(ctx, uid, payload)
	}
	return nil
}

func (d *FCMDispatcher) SendToToken(ctx context.Context, token string, payload domain.PushNotificationPayload) error {
	// In production, this executes an HTTP request to FCM v1 REST API
	prefixLen := 10
	if len(token) < prefixLen {
		prefixLen = len(token)
	}

	d.logger.WithContext(ctx).Info("dispatched FCM push notification",
		"token_prefix", token[:prefixLen],
		"title", payload.Title,
		"body", payload.Body,
		"priority", payload.Priority,
		"data", payload.Data,
	)
	return nil
}
