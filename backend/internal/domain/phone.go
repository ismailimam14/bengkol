package domain

import (
	"strings"
)

// NormalizePhone converts phone numbers to canonical representation.
// It strips non-digit characters (preserving leading '+'),
// and normalizes Indonesian national prefixes (+62 or 62) to standard 0-prefixed format.
// Example:
//
//	"+6281234567890" -> "081234567890"
//	"6281234567890"  -> "081234567890"
//	"0812-3456-7890" -> "081234567890"
//	"+1 555 123 4567" -> "+15551234567"
func NormalizePhone(phone string) string {
	raw := strings.TrimSpace(phone)
	if raw == "" {
		return ""
	}

	var b strings.Builder
	for i, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if r == '+' && i == 0 {
			b.WriteRune(r)
		}
	}
	cleaned := b.String()

	if strings.HasPrefix(cleaned, "+62") {
		return "0" + cleaned[3:]
	}
	if strings.HasPrefix(cleaned, "62") && len(cleaned) > 9 {
		return "0" + cleaned[2:]
	}
	return cleaned
}
