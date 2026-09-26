package validator_test

import (
	"testing"

	"github.com/bengkol/backend/pkg/validator"
	"github.com/google/uuid"
)

func TestValidator_Required(t *testing.T) {
	v := validator.New()
	v.Required("name", "")
	v.Required("title", "   ")
	v.Required("valid_field", "Workshop Name")

	if v.IsValid() {
		t.Fatalf("expected validator to be invalid")
	}

	if len(v.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(v.Errors))
	}
}

func TestValidator_Email(t *testing.T) {
	v := validator.New()
	v.Email("email1", "invalid-email")
	v.Email("email2", "valid@domain.com")

	if v.IsValid() {
		t.Fatalf("expected validator to fail for invalid email")
	}

	if _, ok := v.Errors["email1"]; !ok {
		t.Errorf("expected error on email1")
	}

	if _, ok := v.Errors["email2"]; ok {
		t.Errorf("expected email2 to be valid")
	}
}

func TestValidator_UUID(t *testing.T) {
	v := validator.New()
	v.UUID("id1", "invalid-uuid-string")
	v.UUID("id2", uuid.New().String())

	if v.IsValid() {
		t.Fatalf("expected validator to fail on invalid uuid")
	}

	if _, ok := v.Errors["id1"]; !ok {
		t.Errorf("expected error on id1")
	}

	if _, ok := v.Errors["id2"]; ok {
		t.Errorf("expected id2 to be valid")
	}
}
