package notification_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/internal/notification"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/google/uuid"
)

func TestFCMDispatcher_SendToUser(t *testing.T) {
	repo := newMockDeviceRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	dispatcher := notification.NewFCMDispatcher(repo, log)

	userID := uuid.New()
	repo.devices["fcm_tok_1"] = &domain.UserDevice{
		ID:        uuid.New(),
		UserID:    userID,
		Token:     "fcm_tok_1",
		Platform:  domain.PlatformAndroid,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	payload := domain.PushNotificationPayload{
		Title: "Nomor Antrean Dipanggil! 📢",
		Body:  "Nomor antrean #5 Anda telah dipanggil.",
		Data: map[string]string{
			"type":      "QUEUE_CALLED",
			"queue_num": "5",
		},
		Priority: "HIGH",
	}

	err := dispatcher.SendToUser(context.Background(), userID, payload)
	if err != nil {
		t.Fatalf("unexpected error dispatching notification: %v", err)
	}

	// Test user with no devices
	errNoDevices := dispatcher.SendToUser(context.Background(), uuid.New(), payload)
	if errNoDevices != nil {
		t.Fatalf("expected nil error for user with no devices, got: %v", errNoDevices)
	}
}

func TestFCMDispatcher_SendToUsers(t *testing.T) {
	repo := newMockDeviceRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	dispatcher := notification.NewFCMDispatcher(repo, log)

	u1 := uuid.New()
	u2 := uuid.New()

	repo.devices["tok_u1"] = &domain.UserDevice{ID: uuid.New(), UserID: u1, Token: "tok_u1", Platform: domain.PlatformAndroid}
	repo.devices["tok_u2"] = &domain.UserDevice{ID: uuid.New(), UserID: u2, Token: "tok_u2", Platform: domain.PlatformIOS}

	payload := domain.PushNotificationPayload{
		Title: "Pengumuman Bengkol",
		Body:  "Pemberitahuan sistem.",
	}

	err := dispatcher.SendToUsers(context.Background(), []uuid.UUID{u1, u2}, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
