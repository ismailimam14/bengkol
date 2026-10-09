package security

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

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

// GenerateRandomPassword generates a secure random password of specified length.
func GenerateRandomPassword(length int) (string, error) {
	if length < 8 {
		length = 10
	}
	const uppercase = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	const lowercase = "abcdefghjkmnpqrstuvwxyz"
	const digits = "23456789"
	const specials = "!@#$%^&*"
	const all = uppercase + lowercase + digits + specials

	result := make([]byte, length)

	// Ensure at least one from each character set for strength
	sets := []string{uppercase, lowercase, digits, specials}
	for i, set := range sets {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
		if err != nil {
			return "", fmt.Errorf("failed to generate random character: %w", err)
		}
		result[i] = set[idx.Int64()]
	}

	for i := len(sets); i < length; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(all))))
		if err != nil {
			return "", fmt.Errorf("failed to generate random character: %w", err)
		}
		result[i] = all[idx.Int64()]
	}

	// Shuffle
	for i := range result {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(length)))
		if err != nil {
			return "", fmt.Errorf("failed to shuffle password: %w", err)
		}
		result[i], result[j.Int64()] = result[j.Int64()], result[i]
	}

	return string(result), nil
}

