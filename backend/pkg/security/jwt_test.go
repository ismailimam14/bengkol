package security_test

import (
	"testing"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/bengkol/backend/pkg/security"
	"github.com/google/uuid"
)

func TestJWTManager_GenerateAndValidateToken(t *testing.T) {
	jwtMgr := security.NewJWTManager("test-access-secret-key-at-least-32-chars", "test-refresh-secret", 15, 7)

	user := &domain.User{
		ID:    uuid.New(),
		Email: "budi@example.com",
		Name:  "Budi Santoso",
		Role:  domain.RoleCustomer,
	}

	tokenPair, tokenHash, err := jwtMgr.GenerateTokenPair(user)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	if tokenPair.AccessToken == "" || tokenPair.RefreshToken == "" {
		t.Fatalf("tokens must not be empty")
	}

	if tokenHash == "" {
		t.Fatalf("token hash must not be empty")
	}

	claims, err := jwtMgr.ValidateAccessToken(tokenPair.AccessToken)
	if err != nil {
		t.Fatalf("unexpected error validating token: %v", err)
	}

	if claims.UserID != user.ID {
		t.Errorf("expected user ID %s, got %s", user.ID, claims.UserID)
	}

	if claims.Email != user.Email {
		t.Errorf("expected email %s, got %s", user.Email, claims.Email)
	}

	if claims.Role != user.Role {
		t.Errorf("expected role %s, got %s", user.Role, claims.Role)
	}
}

func TestJWTManager_InvalidSecret(t *testing.T) {
	mgr1 := security.NewJWTManager("secret-key-one-123456789012345678", "refresh-1", 15, 7)
	mgr2 := security.NewJWTManager("secret-key-two-123456789012345678", "refresh-2", 15, 7)

	user := &domain.User{
		ID:    uuid.New(),
		Email: "owner@bengkol.com",
		Name:  "Pak Joko",
		Role:  domain.RoleOwner,
	}

	tokenPair, _, _ := mgr1.GenerateTokenPair(user)

	_, err := mgr2.ValidateAccessToken(tokenPair.AccessToken)
	if err == nil {
		t.Fatalf("expected validation error with mismatched secret")
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	rawToken := "sample-raw-refresh-token"
	hash1 := security.HashToken(rawToken)
	hash2 := security.HashToken(rawToken)

	if hash1 != hash2 {
		t.Errorf("expected deterministic hash output, got %s and %s", hash1, hash2)
	}
}

func TestJWTManager_ExpiredToken(t *testing.T) {
	// 0 minute expiry to trigger immediate expiry
	mgr := security.NewJWTManager("test-access-secret-key-at-least-32-chars", "test-refresh-secret", 0, 7)

	user := &domain.User{
		ID:    uuid.New(),
		Email: "expired@example.com",
		Name:  "Expired User",
		Role:  domain.RoleCustomer,
	}

	tokenPair, _, _ := mgr.GenerateTokenPair(user)

	// Wait 10ms
	time.Sleep(10 * time.Millisecond)

	_, err := mgr.ValidateAccessToken(tokenPair.AccessToken)
	if err == nil {
		t.Fatalf("expected token validation to fail for expired token")
	}
}
