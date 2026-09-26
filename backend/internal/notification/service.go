package notification

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/validator"
	"github.com/google/uuid"
)

var (
	ErrValidationFailed = errors.New("validation failed")
)

// RegisterDeviceRequest DTO
type RegisterDeviceRequest struct {
	Token    string                `json:"token"`
	Platform domain.DevicePlatform `json:"platform"`
}

// Service defines business operations for device tokens and notifications.
type Service interface {
	RegisterDevice(ctx context.Context, userID uuid.UUID, req RegisterDeviceRequest) (*domain.UserDevice, map[string]string, error)
	UnregisterDevice(ctx context.Context, userID uuid.UUID, token string) error
	GetMyDevices(ctx context.Context, userID uuid.UUID) ([]domain.UserDevice, error)
}

type service struct {
	repo   Repository
	logger *logger.Logger
}

// NewService creates a new notification and device service instance.
func NewService(repo Repository, log *logger.Logger) Service {
	return &service{
		repo:   repo,
		logger: log,
	}
}

func (s *service) RegisterDevice(ctx context.Context, userID uuid.UUID, req RegisterDeviceRequest) (*domain.UserDevice, map[string]string, error) {
	v := validator.New()
	v.Required("token", req.Token)

	req.Token = strings.TrimSpace(req.Token)
	if len(req.Token) < 10 {
		v.AddError("token", "token must be at least 10 characters")
	}

	platform := req.Platform
	if platform == "" {
		platform = domain.PlatformAndroid
	}

	switch platform {
	case domain.PlatformAndroid, domain.PlatformIOS, domain.PlatformWeb:
		// Valid
	default:
		v.AddError("platform", "platform must be ANDROID, IOS, or WEB")
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	now := time.Now().UTC()
	device := &domain.UserDevice{
		ID:        uuid.New(),
		UserID:    userID,
		Token:     req.Token,
		Platform:  platform,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.UpsertDevice(ctx, device); err != nil {
		return nil, nil, err
	}

	s.logger.WithContext(ctx).Info("user device registered for push notifications",
		"user_id", userID,
		"device_id", device.ID,
		"platform", platform,
	)

	return device, nil, nil
}

func (s *service) UnregisterDevice(ctx context.Context, userID uuid.UUID, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("device token is required")
	}

	if err := s.repo.DeleteDevice(ctx, userID, token); err != nil {
		return err
	}

	s.logger.WithContext(ctx).Info("user device unregistered", "user_id", userID)
	return nil
}

func (s *service) GetMyDevices(ctx context.Context, userID uuid.UUID) ([]domain.UserDevice, error) {
	return s.repo.GetDevicesByUserID(ctx, userID)
}
