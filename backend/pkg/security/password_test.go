package security_test

import (
	"strings"
	"testing"

	"github.com/bengkol/backend/pkg/security"
)

func TestHashPassword_Success(t *testing.T) {
	password := "SecurePassword123!"
	hash, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error hashing password: %v", err)
	}

	if hash == "" || hash == password {
		t.Fatalf("hash is invalid or identical to plaintext")
	}

	if !security.CheckPassword(password, hash) {
		t.Errorf("expected password to match hash")
	}
}

func TestCheckPassword_Mismatch(t *testing.T) {
	hash, err := security.HashPassword("CorrectPassword123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if security.CheckPassword("WrongPassword123", hash) {
		t.Errorf("expected check to fail for incorrect password")
	}
}

func TestHashPassword_TooLong(t *testing.T) {
	tooLongPassword := strings.Repeat("a", 73)
	_, err := security.HashPassword(tooLongPassword)
	if err == nil {
		t.Fatalf("expected error for password exceeding 72 bytes")
	}
}
