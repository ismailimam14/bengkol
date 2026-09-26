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

type mockDeviceRepo struct {
	devices map[string]*domain.UserDevice // keyed by token
}

func newMockDeviceRepo() *mockDeviceRepo {
	return &mockDeviceRepo{
		devices: make(map[string]*domain.UserDevice),
	}
}

func (m *mockDeviceRepo) UpsertDevice(ctx context.Context, device *domain.UserDevice) error {
	m.devices[device.Token] = device
	return nil
}

func (m *mockDeviceRepo) DeleteDevice(ctx context.Context, userID uuid.UUID, token string) error {
	dev, exists := m.devices[token]
	if !exists || dev.UserID != userID {
		return notification.ErrDeviceNotFound
	}
	delete(m.devices, token)
	return nil
}

func (m *mockDeviceRepo) GetDevicesByUserID(ctx context.Context, userID uuid.UUID) ([]domain.UserDevice, error) {
	var result []domain.UserDevice
	for _, dev := range m.devices {
		if dev.UserID == userID {
			result = append(result, *dev)
		}
	}
	return result, nil
}

func (m *mockDeviceRepo) GetDevicesByUserIDs(ctx context.Context, userIDs []uuid.UUID) ([]domain.UserDevice, error) {
	var result []domain.UserDevice
	lookup := make(map[uuid.UUID]bool)
	for _, id := range userIDs {
		lookup[id] = true
	}

	for _, dev := range m.devices {
		if lookup[dev.UserID] {
			result = append(result, *dev)
		}
	}
	return result, nil
}

func setupDeviceService() (notification.Service, *mockDeviceRepo) {
	repo := newMockDeviceRepo()
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	svc := notification.NewService(repo, log)
	return svc, repo
}

func TestDeviceService_RegisterDevice_Success(t *testing.T) {
	svc, repo := setupDeviceService()
	userID := uuid.New()

	req := notification.RegisterDeviceRequest{
		Token:    "fcm_token_sample_1234567890_abcdef",
		Platform: domain.PlatformAndroid,
	}

	device, valErrors, err := svc.RegisterDevice(context.Background(), userID, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(valErrors) > 0 {
		t.Fatalf("expected no validation errors, got: %v", valErrors)
	}
	if device.Token != req.Token {
		t.Errorf("expected token %s, got %s", req.Token, device.Token)
	}
	if device.Platform != domain.PlatformAndroid {
		t.Errorf("expected platform ANDROID, got %s", device.Platform)
	}
	if len(repo.devices) != 1 {
		t.Errorf("expected 1 device in repo, got %d", len(repo.devices))
	}
}

func TestDeviceService_RegisterDevice_ValidationErrors(t *testing.T) {
	svc, _ := setupDeviceService()
	userID := uuid.New()

	// Short token and invalid platform
	req := notification.RegisterDeviceRequest{
		Token:    "short",
		Platform: "WINDOWS_PHONE",
	}

	device, valErrors, err := svc.RegisterDevice(context.Background(), userID, req)
	if err == nil || device != nil {
		t.Fatalf("expected error and nil device, got device: %v", device)
	}
	if valErrors["token"] == "" {
		t.Errorf("expected token validation error")
	}
	if valErrors["platform"] == "" {
		t.Errorf("expected platform validation error")
	}
}

func TestDeviceService_UnregisterDevice(t *testing.T) {
	svc, repo := setupDeviceService()
	userID := uuid.New()
	token := "fcm_token_to_delete_1234567890"

	repo.devices[token] = &domain.UserDevice{
		ID:        uuid.New(),
		UserID:    userID,
		Token:     token,
		Platform:  domain.PlatformAndroid,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := svc.UnregisterDevice(context.Background(), userID, token)
	if err != nil {
		t.Fatalf("unexpected error unregistering device: %v", err)
	}
	if len(repo.devices) != 0 {
		t.Errorf("expected 0 devices in repo, got %d", len(repo.devices))
	}

	// Try deleting again -> ErrDeviceNotFound
	err = svc.UnregisterDevice(context.Background(), userID, token)
	if err == nil {
		t.Fatalf("expected error on non-existing token, got nil")
	}
}

func TestDeviceService_GetMyDevices(t *testing.T) {
	svc, repo := setupDeviceService()
	userID := uuid.New()

	repo.devices["tok_1"] = &domain.UserDevice{ID: uuid.New(), UserID: userID, Token: "tok_1", Platform: domain.PlatformAndroid}
	repo.devices["tok_2"] = &domain.UserDevice{ID: uuid.New(), UserID: userID, Token: "tok_2", Platform: domain.PlatformIOS}
	repo.devices["tok_other"] = &domain.UserDevice{ID: uuid.New(), UserID: uuid.New(), Token: "tok_other", Platform: domain.PlatformWeb}

	devices, err := svc.GetMyDevices(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices for user, got %d", len(devices))
	}
}
