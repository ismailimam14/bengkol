package auth_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/bengkol/backend/internal/auth"
	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/google/uuid"
)

// MockRepository implements auth.Repository for testing
type mockAuthRepo struct {
	users         map[string]*domain.User        // key: email
	usersByID     map[uuid.UUID]*domain.User     // key: id
	refreshTokens map[string]*domain.RefreshToken // key: token_hash
}

func newMockAuthRepo() *mockAuthRepo {
	return &mockAuthRepo{
		users:         make(map[string]*domain.User),
		usersByID:     make(map[uuid.UUID]*domain.User),
		refreshTokens: make(map[string]*domain.RefreshToken),
	}
}

func (m *mockAuthRepo) CreateUser(ctx context.Context, user *domain.User) error {
	if _, exists := m.users[user.Email]; exists {
		return auth.ErrUserAlreadyExists
	}
	m.users[user.Email] = user
	m.usersByID[user.ID] = user
	return nil
}

func (m *mockAuthRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return u, nil
}

func (m *mockAuthRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	u, ok := m.usersByID[id]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return u, nil
}

func (m *mockAuthRepo) SaveRefreshToken(ctx context.Context, token *domain.RefreshToken) error {
	m.refreshTokens[token.TokenHash] = token
	return nil
}

func (m *mockAuthRepo) GetRefreshToken(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	t, ok := m.refreshTokens[tokenHash]
	if !ok {
		return nil, auth.ErrRefreshTokenNotFound
	}
	return t, nil
}

func (m *mockAuthRepo) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	t, ok := m.refreshTokens[tokenHash]
	if !ok {
		return auth.ErrRefreshTokenNotFound
	}
	t.Revoked = true
	return nil
}

func (m *mockAuthRepo) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	for _, t := range m.refreshTokens {
		if t.UserID == userID {
			t.Revoked = true
		}
	}
	return nil
}

func setupAuthService() (auth.Service, *mockAuthRepo, *security.JWTManager) {
	repo := newMockAuthRepo()
	jwtMgr := security.NewJWTManager("test-secret-key-at-least-32-chars-long", "test-refresh-secret", 15, 7)
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	svc := auth.NewService(repo, jwtMgr, log)
	return svc, repo, jwtMgr
}

func TestAuthService_Register_CustomerSuccess(t *testing.T) {
	svc, _, _ := setupAuthService()

	req := auth.RegisterRequest{
		Name:     "Budi Customer",
		Email:    "budi@example.com",
		Password: "password123",
		Phone:    "08123456789",
		Role:     domain.RoleCustomer,
	}

	resp, valErrors, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error during registration: %v", err)
	}

	if valErrors != nil {
		t.Fatalf("expected no validation errors, got %+v", valErrors)
	}

	if resp.User.Email != "budi@example.com" {
		t.Errorf("expected email budi@example.com, got %s", resp.User.Email)
	}

	if resp.User.Role != domain.RoleCustomer {
		t.Errorf("expected role CUSTOMER, got %s", resp.User.Role)
	}

	if resp.Tokens.AccessToken == "" || resp.Tokens.RefreshToken == "" {
		t.Errorf("expected access and refresh tokens to be returned")
	}
}

func TestAuthService_Register_OwnerSuccess(t *testing.T) {
	svc, _, _ := setupAuthService()

	req := auth.RegisterRequest{
		Name:     "Pak Joko Workshop",
		Email:    "joko@workshop.com",
		Password: "password123",
		Phone:    "08198765432",
		Role:     domain.RoleOwner,
	}

	resp, _, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error during owner registration: %v", err)
	}

	if resp.User.Role != domain.RoleOwner {
		t.Errorf("expected role OWNER, got %s", resp.User.Role)
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	svc, _, _ := setupAuthService()

	req := auth.RegisterRequest{
		Name:     "User One",
		Email:    "duplicate@example.com",
		Password: "password123",
		Phone:    "0811111111",
		Role:     domain.RoleCustomer,
	}

	_, _, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on first registration: %v", err)
	}

	// Second registration with same email
	_, valErrors, err := svc.Register(context.Background(), req)
	if !errors.Is(err, auth.ErrUserAlreadyExists) {
		t.Fatalf("expected ErrUserAlreadyExists, got %v", err)
	}

	if _, ok := valErrors["email"]; !ok {
		t.Errorf("expected validation error on email field")
	}
}

func TestAuthService_Register_ValidationErrors(t *testing.T) {
	svc, _, _ := setupAuthService()

	req := auth.RegisterRequest{
		Name:     "",
		Email:    "invalid-email",
		Password: "short",
		Phone:    "",
		Role:     "INVALID_ROLE",
	}

	_, valErrors, err := svc.Register(context.Background(), req)
	if !errors.Is(err, auth.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed, got %v", err)
	}

	expectedFields := []string{"name", "email", "password", "phone", "role"}
	for _, field := range expectedFields {
		if _, ok := valErrors[field]; !ok {
			t.Errorf("expected validation error on field %s", field)
		}
	}
}

func TestAuthService_Login_Success(t *testing.T) {
	svc, _, _ := setupAuthService()

	regReq := auth.RegisterRequest{
		Name:     "Login User",
		Email:    "login@example.com",
		Password: "password123",
		Phone:    "0812345678",
		Role:     domain.RoleCustomer,
	}
	_, _, err := svc.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("registration error: %v", err)
	}

	loginResp, err := svc.Login(context.Background(), auth.LoginRequest{
		Email:    "login@example.com",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("unexpected error on login: %v", err)
	}

	if loginResp.User.Email != "login@example.com" {
		t.Errorf("expected user email login@example.com, got %s", loginResp.User.Email)
	}

	if loginResp.Tokens.AccessToken == "" {
		t.Errorf("expected non-empty access token")
	}
}

func TestAuthService_Login_InvalidPassword(t *testing.T) {
	svc, _, _ := setupAuthService()

	regReq := auth.RegisterRequest{
		Name:     "Login User",
		Email:    "user@example.com",
		Password: "correct-password",
		Phone:    "0812345678",
		Role:     domain.RoleCustomer,
	}
	_, _, _ = svc.Register(context.Background(), regReq)

	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Email:    "user@example.com",
		Password: "wrong-password",
	})
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthService_RefreshToken_Rotation(t *testing.T) {
	svc, repo, _ := setupAuthService()

	regResp, _, err := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Refresh User",
		Email:    "refresh@example.com",
		Password: "password123",
		Phone:    "0812345678",
		Role:     domain.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("registration error: %v", err)
	}

	oldRefreshToken := regResp.Tokens.RefreshToken

	// Perform token refresh
	refreshResp, err := svc.RefreshToken(context.Background(), auth.RefreshTokenRequest{
		RefreshToken: oldRefreshToken,
	})
	if err != nil {
		t.Fatalf("unexpected error on refresh: %v", err)
	}

	if refreshResp.Tokens.AccessToken == "" || refreshResp.Tokens.RefreshToken == "" {
		t.Errorf("expected new token pair")
	}

	if refreshResp.Tokens.RefreshToken == oldRefreshToken {
		t.Errorf("expected rotated new refresh token, but got same token")
	}

	// Verify old refresh token is now revoked in repo
	oldHash := security.HashToken(oldRefreshToken)
	storedOldToken, err := repo.GetRefreshToken(context.Background(), oldHash)
	if err != nil || !storedOldToken.Revoked {
		t.Errorf("expected old refresh token to be marked as revoked")
	}

	// Attempting to reuse the revoked old refresh token should be rejected
	_, err = svc.RefreshToken(context.Background(), auth.RefreshTokenRequest{
		RefreshToken: oldRefreshToken,
	})
	if !errors.Is(err, auth.ErrTokenRevoked) {
		t.Fatalf("expected ErrTokenRevoked on token reuse, got %v", err)
	}
}

func TestAuthService_Logout(t *testing.T) {
	svc, repo, _ := setupAuthService()

	regResp, _, _ := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Logout User",
		Email:    "logout@example.com",
		Password: "password123",
		Phone:    "0812345678",
		Role:     domain.RoleCustomer,
	})

	rfToken := regResp.Tokens.RefreshToken
	if err := svc.Logout(context.Background(), rfToken); err != nil {
		t.Fatalf("unexpected error on logout: %v", err)
	}

	stored, _ := repo.GetRefreshToken(context.Background(), security.HashToken(rfToken))
	if !stored.Revoked {
		t.Errorf("expected refresh token to be revoked after logout")
	}
}
