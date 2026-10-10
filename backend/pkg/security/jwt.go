package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/bengkol/backend/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("token is invalid or expired")
	ErrExpiredToken = errors.New("token has expired")
)

// CustomClaims represents the JWT payload claims for Bengkol.
type CustomClaims struct {
	UserID      uuid.UUID                   `json:"user_id"`
	Email       string                      `json:"email"`
	Name        string                      `json:"name"`
	Role        domain.UserRole             `json:"role"`
	WorkshopID  *uuid.UUID                  `json:"workshop_id,omitempty"`
	Permissions *domain.EmployeePermissions `json:"permissions,omitempty"`
	jwt.RegisteredClaims
}

// TokenPair contains generated access and refresh tokens.
type TokenPair struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	ExpiresInSeconds int64     `json:"expires_in"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

// JWTManager handles JWT access token creation, validation, and refresh token hashing.
type JWTManager struct {
	accessSecret      []byte
	refreshSecret     []byte
	accessExpiryMin   time.Duration
	refreshExpiryDays time.Duration
	issuer            string
}

// NewJWTManager creates a new JWTManager.
func NewJWTManager(accessSecret, refreshSecret string, accessExpiryMinutes, refreshExpiryDays int) *JWTManager {
	return &JWTManager{
		accessSecret:      []byte(accessSecret),
		refreshSecret:     []byte(refreshSecret),
		accessExpiryMin:   time.Duration(accessExpiryMinutes) * time.Minute,
		refreshExpiryDays: time.Duration(refreshExpiryDays) * 24 * time.Hour,
		issuer:            "bengkol-api",
	}
}

// GenerateTokenPair generates an access token and a cryptographically secure random refresh token.
func (m *JWTManager) GenerateTokenPair(user *domain.User) (*TokenPair, string, error) {
	return m.GenerateScopedTokenPair(user, nil, user.Role, nil)
}

// GenerateScopedTokenPair generates an access token and refresh token scoped to a workshop with role and permissions.
func (m *JWTManager) GenerateScopedTokenPair(user *domain.User, workshopID *uuid.UUID, role domain.UserRole, perms *domain.EmployeePermissions) (*TokenPair, string, error) {
	now := time.Now().UTC()
	accessExpiresAt := now.Add(m.accessExpiryMin)
	refreshExpiresAt := now.Add(m.refreshExpiryDays)

	claims := CustomClaims{
		UserID:      user.ID,
		Email:       user.Email,
		Name:        user.Name,
		Role:        role,
		WorkshopID:  workshopID,
		Permissions: perms,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExpiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedAccessToken, err := token.SignedString(m.accessSecret)
	if err != nil {
		return nil, "", fmt.Errorf("failed to sign access token: %w", err)
	}

	rawRefreshToken, refreshTokenHash, err := m.GenerateRandomRefreshToken()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:      signedAccessToken,
		RefreshToken:     rawRefreshToken,
		TokenType:        "Bearer",
		ExpiresInSeconds: int64(m.accessExpiryMin.Seconds()),
		AccessExpiresAt:  accessExpiresAt,
		RefreshExpiresAt: refreshExpiresAt,
	}, refreshTokenHash, nil
}

// ValidateAccessToken parses and validates a signed JWT access token.
func (m *JWTManager) ValidateAccessToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.accessSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// GenerateRandomRefreshToken creates a cryptographically secure random token and its SHA256 hash.
func (m *JWTManager) GenerateRandomRefreshToken() (rawToken string, tokenHash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	rawToken = hex.EncodeToString(bytes)
	tokenHash = HashToken(rawToken)
	return rawToken, tokenHash, nil
}

// HashToken generates a deterministic SHA256 hex string of a raw token.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
