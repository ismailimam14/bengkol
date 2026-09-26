package security

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrPasswordTooLong is returned if the password exceeds bcrypt's 72-byte limit
	ErrPasswordTooLong = errors.New("password length cannot exceed 72 bytes")
)

// HashPassword creates a bcrypt hash of the given plain password.
func HashPassword(password string) (string, error) {
	if len(password) > 72 {
		return "", ErrPasswordTooLong
	}

	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	return string(bytes), nil
}

// CheckPassword compares a plaintext password with a hashed password.
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
