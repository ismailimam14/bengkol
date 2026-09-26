package validator

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Validator accumulates field-level validation errors.
type Validator struct {
	Errors map[string]string
}

// New creates a new Validator instance.
func New() *Validator {
	return &Validator{
		Errors: make(map[string]string),
	}
}

// IsValid returns true if there are no validation errors.
func (v *Validator) IsValid() bool {
	return len(v.Errors) == 0
}

// AddError records an error for a given field if not already present.
func (v *Validator) AddError(field, message string) {
	if _, exists := v.Errors[field]; !exists {
		v.Errors[field] = message
	}
}

// Check evaluates a boolean condition and adds an error message if false.
func (v *Validator) Check(ok bool, field, message string) {
	if !ok {
		v.AddError(field, message)
	}
}

// Required checks that a string is not empty or pure whitespace.
func (v *Validator) Required(field, value string) {
	v.Check(strings.TrimSpace(value) != "", field, field+" is required")
}

// Matches checks string against regex pattern.
func (v *Validator) Matches(field, value string, rx *regexp.Regexp, message string) {
	if value == "" {
		return
	}
	v.Check(rx.MatchString(value), field, message)
}

// Email checks if string is a valid email format.
func (v *Validator) Email(field, value string) {
	if value == "" {
		return
	}
	v.Check(emailRegex.MatchString(value), field, "must be a valid email address")
}

// MinLength checks minimum string length.
func (v *Validator) MinLength(field, value string, min int) {
	v.Check(len(strings.TrimSpace(value)) >= min, field, "must be at least "+string(rune('0'+min))+" characters")
}

// UUID checks if a string is a valid UUID.
func (v *Validator) UUID(field, value string) {
	if value == "" {
		return
	}
	_, err := uuid.Parse(value)
	v.Check(err == nil, field, "must be a valid UUID")
}
