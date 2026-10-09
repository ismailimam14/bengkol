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
	if user.Email != "" {
		if _, exists := m.users[user.Email]; exists {
			return auth.ErrUserAlreadyExists
		}
		m.users[user.Email] = user
	}
	for _, u := range m.usersByID {
		if u.Phone == user.Phone {
			return auth.ErrUserAlreadyExists
		}
	}
	m.usersByID[user.ID] = user
	return nil
}

func (m *mockAuthRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	if email == "" {
		return nil, auth.ErrUserNotFound
	}
	u, ok := m.users[email]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return u, nil
}

func (m *mockAuthRepo) GetUserByPhone(ctx context.Context, phone string) (*domain.User, error) {
	if phone == "" {
		return nil, auth.ErrUserNotFound
	}
	for _, u := range m.usersByID {
		if u.Phone == phone {
			return u, nil
		}
	}
	return nil, auth.ErrUserNotFound
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

func (m *mockAuthRepo) UpdatePassword(ctx context.Context, userID uuid.UUID, newPasswordHash string) error {
	u, ok := m.usersByID[userID]
	if !ok {
		return auth.ErrUserNotFound
	}
	u.PasswordHash = newPasswordHash
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

func TestAuthService_Register_Success_WithoutEmail(t *testing.T) {
	svc, _, _ := setupAuthService()

	req := auth.RegisterRequest{
		Name:     "No Email Customer",
		Password: "password123",
		Phone:    "08555444333",
		Role:     domain.RoleCustomer,
	}

	resp, valErrors, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error during registration without email: %v", err)
	}

	if valErrors != nil {
		t.Fatalf("expected no validation errors, got %+v", valErrors)
	}

	if resp.User.Email != "" {
		t.Errorf("expected empty email, got %s", resp.User.Email)
	}

	if resp.User.Phone != "08555444333" {
		t.Errorf("expected phone 08555444333, got %s", resp.User.Phone)
	}

	if resp.Tokens.AccessToken == "" || resp.Tokens.RefreshToken == "" {
		t.Errorf("expected access and refresh tokens to be returned")
	}
}

func TestAuthService_Register_Success_WithPhoneNumberField(t *testing.T) {
	svc, _, _ := setupAuthService()

	req := auth.RegisterRequest{
		Name:        "Alias Phone Customer",
		Password:    "password123",
		PhoneNumber: "08777666555",
		Role:        domain.RoleCustomer,
	}

	resp, valErrors, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error during registration with phone_number field: %v", err)
	}

	if valErrors != nil {
		t.Fatalf("expected no validation errors, got %+v", valErrors)
	}

	if resp.User.Phone != "08777666555" {
		t.Errorf("expected phone 08777666555, got %s", resp.User.Phone)
	}
}

func TestAuthService_Register_MissingPhone(t *testing.T) {
	svc, _, _ := setupAuthService()

	req := auth.RegisterRequest{
		Name:     "Missing Phone",
		Email:    "test@example.com",
		Password: "password123",
		Phone:    "",
		Role:     domain.RoleCustomer,
	}

	_, valErrors, err := svc.Register(context.Background(), req)
	if !errors.Is(err, auth.ErrValidationFailed) {
		t.Fatalf("expected ErrValidationFailed, got %v", err)
	}

	if _, ok := valErrors["phone"]; !ok {
		t.Errorf("expected validation error on phone field")
	}
}

func TestAuthService_Register_DuplicatePhone(t *testing.T) {
	svc, _, _ := setupAuthService()

	req1 := auth.RegisterRequest{
		Name:     "User One",
		Email:    "user1@example.com",
		Password: "password123",
		Phone:    "08999888777",
		Role:     domain.RoleCustomer,
	}
	_, _, err := svc.Register(context.Background(), req1)
	if err != nil {
		t.Fatalf("unexpected error on first registration: %v", err)
	}

	// Second registration with different email but SAME phone
	req2 := auth.RegisterRequest{
		Name:     "User Two",
		Email:    "user2@example.com",
		Password: "password456",
		Phone:    "08999888777",
		Role:     domain.RoleCustomer,
	}
	_, valErrors, err := svc.Register(context.Background(), req2)
	if !errors.Is(err, auth.ErrUserAlreadyExists) {
		t.Fatalf("expected ErrUserAlreadyExists on duplicate phone, got %v", err)
	}

	if _, ok := valErrors["phone"]; !ok {
		t.Errorf("expected validation error on phone field")
	}
}

func TestAuthService_Login_AfterRegistrationWithoutEmail(t *testing.T) {
	svc, _, _ := setupAuthService()

	regReq := auth.RegisterRequest{
		Name:     "Phone Only User",
		Password: "secretPassword123",
		Phone:    "081299990000",
		Role:     domain.RoleCustomer,
	}
	_, _, err := svc.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("registration error: %v", err)
	}

	// Login with phone and password
	loginResp, err := svc.Login(context.Background(), auth.LoginRequest{
		Phone:    "081299990000",
		Password: "secretPassword123",
	})
	if err != nil {
		t.Fatalf("login error: %v", err)
	}

	if loginResp.User.Phone != "081299990000" {
		t.Errorf("expected user phone 081299990000, got %s", loginResp.User.Phone)
	}
	if loginResp.User.Email != "" {
		t.Errorf("expected empty user email, got %s", loginResp.User.Email)
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

func TestAuthService_Login_Success_WithPhone(t *testing.T) {
	svc, _, _ := setupAuthService()

	regReq := auth.RegisterRequest{
		Name:     "Phone User",
		Email:    "phoneuser@example.com",
		Password: "password123",
		Phone:    "081298765432",
		Role:     domain.RoleCustomer,
	}
	_, _, err := svc.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("registration error: %v", err)
	}

	// Login with Phone only (Email empty)
	loginResp, err := svc.Login(context.Background(), auth.LoginRequest{
		Phone:    "081298765432",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("unexpected error on login with phone: %v", err)
	}

	if loginResp.User.Phone != "081298765432" {
		t.Errorf("expected user phone 081298765432, got %s", loginResp.User.Phone)
	}
	if loginResp.Tokens.AccessToken == "" {
		t.Errorf("expected non-empty access token")
	}
}

func TestAuthService_Login_Success_WithPhoneNumberField(t *testing.T) {
	svc, _, _ := setupAuthService()

	regReq := auth.RegisterRequest{
		Name:     "Phone User",
		Email:    "phoneuser2@example.com",
		Password: "password123",
		Phone:    "081233445566",
		Role:     domain.RoleCustomer,
	}
	_, _, err := svc.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("registration error: %v", err)
	}

	// Login with PhoneNumber field
	loginResp, err := svc.Login(context.Background(), auth.LoginRequest{
		PhoneNumber: "081233445566",
		Password:    "password123",
	})
	if err != nil {
		t.Fatalf("unexpected error on login with phone_number: %v", err)
	}

	if loginResp.User.Phone != "081233445566" {
		t.Errorf("expected user phone 081233445566, got %s", loginResp.User.Phone)
	}
}

func TestAuthService_Login_Success_WithBothEmailAndPhone(t *testing.T) {
	svc, _, _ := setupAuthService()

	regReq := auth.RegisterRequest{
		Name:     "Both User",
		Email:    "both@example.com",
		Password: "password123",
		Phone:    "081211112222",
		Role:     domain.RoleCustomer,
	}
	_, _, err := svc.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("registration error: %v", err)
	}

	// Login with both matching email and phone
	loginResp, err := svc.Login(context.Background(), auth.LoginRequest{
		Email:    "both@example.com",
		Phone:    "081211112222",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("unexpected error on login with both: %v", err)
	}

	if loginResp.User.Email != "both@example.com" {
		t.Errorf("expected user email both@example.com, got %s", loginResp.User.Email)
	}
}

func TestAuthService_Login_MismatchedEmailAndPhone(t *testing.T) {
	svc, _, _ := setupAuthService()

	regReq := auth.RegisterRequest{
		Name:     "Mismatch User",
		Email:    "user1@example.com",
		Password: "password123",
		Phone:    "081211112222",
		Role:     domain.RoleCustomer,
	}
	_, _, err := svc.Register(context.Background(), regReq)
	if err != nil {
		t.Fatalf("registration error: %v", err)
	}

	// Login with email belonging to user1 but different phone
	_, err = svc.Login(context.Background(), auth.LoginRequest{
		Email:    "user1@example.com",
		Phone:    "089999999999",
		Password: "password123",
	})
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials on mismatched email/phone, got %v", err)
	}
}

func TestAuthService_Login_PhoneNotFound(t *testing.T) {
	svc, _, _ := setupAuthService()

	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Phone:    "089999999999",
		Password: "password123",
	})
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for non-existent phone, got %v", err)
	}
}

func TestAuthService_Login_MissingBothEmailAndPhone(t *testing.T) {
	svc, _, _ := setupAuthService()

	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Password: "password123",
	})
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials when both email and phone are missing, got %v", err)
	}
}

func TestAuthService_Login_MissingPassword(t *testing.T) {
	svc, _, _ := setupAuthService()

	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Phone: "0812345678",
	})
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials when password missing, got %v", err)
	}
}

func TestAuthService_Login_InvalidPassword_WithPhone(t *testing.T) {
	svc, _, _ := setupAuthService()

	regReq := auth.RegisterRequest{
		Name:     "Phone User",
		Email:    "phoneuser3@example.com",
		Password: "correctpassword",
		Phone:    "081277778888",
		Role:     domain.RoleCustomer,
	}
	_, _, _ = svc.Register(context.Background(), regReq)

	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Phone:    "081277778888",
		Password: "wrongpassword",
	})
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials on wrong password with phone, got %v", err)
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

func TestAuthService_ChangePassword(t *testing.T) {
	svc, repo, _ := setupAuthService()
	ctx := context.Background()

	regResp, _, err := svc.Register(ctx, auth.RegisterRequest{
		Name:     "Test Employee",
		Email:    "emp@example.com",
		Password: "InitialPassword123!",
		Phone:    "0899887766",
		Role:     domain.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}

	userID := regResp.User.ID

	// 1. Wrong current password -> validation error
	valErrors, err := svc.ChangePassword(ctx, userID, auth.ChangePasswordRequest{
		CurrentPassword: "WrongPassword!",
		NewPassword:     "NewSecurePassword456!",
	})
	if !errors.Is(err, auth.ErrValidationFailed) {
		t.Errorf("expected ErrValidationFailed for wrong password, got %v", err)
	}
	if valErrors["current_password"] != "incorrect current password" {
		t.Errorf("expected 'incorrect current password' error, got %v", valErrors)
	}

	// 2. Same new password as old -> validation error
	valErrors, err = svc.ChangePassword(ctx, userID, auth.ChangePasswordRequest{
		CurrentPassword: "InitialPassword123!",
		NewPassword:     "InitialPassword123!",
	})
	if !errors.Is(err, auth.ErrValidationFailed) {
		t.Errorf("expected ErrValidationFailed for identical password, got %v", err)
	}

	// 3. Successful change
	valErrors, err = svc.ChangePassword(ctx, userID, auth.ChangePasswordRequest{
		CurrentPassword: "InitialPassword123!",
		NewPassword:     "NewSecurePassword456!",
		ConfirmPassword: "NewSecurePassword456!",
	})
	if err != nil {
		t.Fatalf("unexpected error changing password: %v (valErrors: %v)", err, valErrors)
	}

	// 4. Verify login succeeds with new password and fails with old
	_, err = svc.Login(ctx, auth.LoginRequest{
		Phone:    "0899887766",
		Password: "InitialPassword123!",
	})
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("expected old password to fail login, got %v", err)
	}

	loginResp, err := svc.Login(ctx, auth.LoginRequest{
		Phone:    "0899887766",
		Password: "NewSecurePassword456!",
	})
	if err != nil {
		t.Fatalf("login failed with new password: %v", err)
	}
	if loginResp.User.ID != userID {
		t.Errorf("expected user ID %s, got %s", userID, loginResp.User.ID)
	}

	// 5. Verify refresh tokens revoked
	oldRfToken := regResp.Tokens.RefreshToken
	stored, _ := repo.GetRefreshToken(ctx, security.HashToken(oldRfToken))
	if !stored.Revoked {
		t.Errorf("expected existing refresh token to be revoked upon password change")
	}
}

