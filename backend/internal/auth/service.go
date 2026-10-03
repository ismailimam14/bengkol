package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/bengkol/backend/pkg/validator"
	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidRole        = errors.New("invalid role; must be CUSTOMER or OWNER")
	ErrValidationFailed   = errors.New("validation failed")
	ErrTokenExpired       = errors.New("refresh token has expired")
	ErrTokenRevoked       = errors.New("refresh token has been revoked")
)

// RegisterRequest DTO
type RegisterRequest struct {
	Name     string          `json:"name"`
	Email    string          `json:"email"`
	Password string          `json:"password"`
	Phone    string          `json:"phone"`
	Role     domain.UserRole `json:"role"`
}

// LoginRequest DTO
type LoginRequest struct {
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
	Password    string `json:"password"`
}

// RefreshTokenRequest DTO
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// AuthResponse returns authenticated user and active token pair
type AuthResponse struct {
	User   *domain.User        `json:"user"`
	Tokens *security.TokenPair `json:"tokens"`
}

// Service defines auth business logic methods.
type Service interface {
	Register(ctx context.Context, req RegisterRequest) (*AuthResponse, map[string]string, error)
	Login(ctx context.Context, req LoginRequest) (*AuthResponse, error)
	RefreshToken(ctx context.Context, req RefreshTokenRequest) (*AuthResponse, error)
	Logout(ctx context.Context, refreshToken string) error
	GetMe(ctx context.Context, userID uuid.UUID) (*domain.User, error)
}

type authService struct {
	repo   Repository
	jwt    *security.JWTManager
	logger *logger.Logger
}

// NewService creates a new Auth Service instance.
func NewService(repo Repository, jwtMgr *security.JWTManager, log *logger.Logger) Service {
	return &authService{
		repo:   repo,
		jwt:    jwtMgr,
		logger: log,
	}
}

func (s *authService) Register(ctx context.Context, req RegisterRequest) (*AuthResponse, map[string]string, error) {
	v := validator.New()
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Phone = strings.TrimSpace(req.Phone)

	v.Required("name", req.Name)
	v.Required("email", req.Email)
	v.Email("email", req.Email)
	v.Required("password", req.Password)
	v.MinLength("password", req.Password, 8)
	v.Required("phone", req.Phone)

	if req.Role == "" {
		req.Role = domain.RoleCustomer
	}

	if req.Role != domain.RoleCustomer && req.Role != domain.RoleOwner {
		v.AddError("role", "role must be either CUSTOMER or OWNER")
	}

	if !v.IsValid() {
		return nil, v.Errors, ErrValidationFailed
	}

	// Check if user email already exists
	existing, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err == nil && existing != nil {
		v.AddError("email", "email is already registered")
		return nil, v.Errors, ErrUserAlreadyExists
	}

	// Hash password
	passwordHash, err := security.HashPassword(req.Password)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	user := &domain.User{
		ID:           uuid.New(),
		Email:        req.Email,
		PasswordHash: passwordHash,
		Name:         req.Name,
		Phone:        req.Phone,
		Role:         req.Role,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, nil, err
	}

	// Generate tokens
	tokens, tokenHash, err := s.jwt.GenerateTokenPair(user)
	if err != nil {
		return nil, nil, err
	}

	// Store refresh token
	rfToken := &domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		Revoked:   false,
		ExpiresAt: tokens.RefreshExpiresAt,
		CreatedAt: now,
	}
	if err := s.repo.SaveRefreshToken(ctx, rfToken); err != nil {
		return nil, nil, err
	}

	s.logger.Info("user registered successfully", "user_id", user.ID, "role", user.Role)

	return &AuthResponse{
		User:   user,
		Tokens: tokens,
	}, nil, nil
}

func (s *authService) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	if req.Phone == "" && req.PhoneNumber != "" {
		req.Phone = req.PhoneNumber
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Phone = strings.TrimSpace(req.Phone)

	if (req.Email == "" && req.Phone == "") || req.Password == "" {
		return nil, ErrInvalidCredentials
	}

	var user *domain.User
	var err error

	if req.Email != "" && req.Phone != "" {
		user, err = s.repo.GetUserByEmail(ctx, req.Email)
		if err != nil || user.Phone != req.Phone {
			return nil, ErrInvalidCredentials
		}
	} else if req.Email != "" {
		user, err = s.repo.GetUserByEmail(ctx, req.Email)
		if err != nil {
			return nil, ErrInvalidCredentials
		}
	} else {
		user, err = s.repo.GetUserByPhone(ctx, req.Phone)
		if err != nil {
			return nil, ErrInvalidCredentials
		}
	}

	if !security.CheckPassword(req.Password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	// Generate tokens
	tokens, tokenHash, err := s.jwt.GenerateTokenPair(user)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	rfToken := &domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		Revoked:   false,
		ExpiresAt: tokens.RefreshExpiresAt,
		CreatedAt: now,
	}
	if err := s.repo.SaveRefreshToken(ctx, rfToken); err != nil {
		return nil, err
	}

	s.logger.Info("user logged in", "user_id", user.ID, "role", user.Role)

	return &AuthResponse{
		User:   user,
		Tokens: tokens,
	}, nil
}

func (s *authService) RefreshToken(ctx context.Context, req RefreshTokenRequest) (*AuthResponse, error) {
	if strings.TrimSpace(req.RefreshToken) == "" {
		return nil, security.ErrInvalidToken
	}

	tokenHash := security.HashToken(req.RefreshToken)
	storedToken, err := s.repo.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return nil, security.ErrInvalidToken
	}

	if storedToken.Revoked {
		// Possible token reuse attack; revoke all tokens for this user for security
		_ = s.repo.RevokeAllUserRefreshTokens(ctx, storedToken.UserID)
		s.logger.Warn("detected reuse of revoked refresh token; revoked all tokens", "user_id", storedToken.UserID)
		return nil, ErrTokenRevoked
	}

	if time.Now().UTC().After(storedToken.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	// Rotate refresh token by revoking old one
	if err := s.repo.RevokeRefreshToken(ctx, tokenHash); err != nil {
		return nil, err
	}

	user, err := s.repo.GetUserByID(ctx, storedToken.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Issue new token pair
	tokens, newTokenHash, err := s.jwt.GenerateTokenPair(user)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	newRfToken := &domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: newTokenHash,
		Revoked:   false,
		ExpiresAt: tokens.RefreshExpiresAt,
		CreatedAt: now,
	}
	if err := s.repo.SaveRefreshToken(ctx, newRfToken); err != nil {
		return nil, err
	}

	return &AuthResponse{
		User:   user,
		Tokens: tokens,
	}, nil
}

func (s *authService) Logout(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return nil
	}
	tokenHash := security.HashToken(refreshToken)
	return s.repo.RevokeRefreshToken(ctx, tokenHash)
}

func (s *authService) GetMe(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	return s.repo.GetUserByID(ctx, userID)
}
