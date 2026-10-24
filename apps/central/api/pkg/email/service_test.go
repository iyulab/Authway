package email

import "testing"

// TestValidateEmail covers the hand-rolled validator's boundary conditions —
// it is not using validator/v10's `email` tag, so its accept/reject behavior
// has no other source of truth than this.
func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"valid simple", "user@example.com", true},
		{"valid with subdomain", "user@mail.example.com", true},
		{"valid with plus tag", "user+tag@example.com", true},
		{"missing at sign", "userexample.com", false},
		{"missing domain dot", "user@examplecom", false},
		{"empty local part", "@example.com", false},
		{"two at signs", "user@@example.com", false},
		{"domain too short", "user@a.b", true}, // len("a.b") == 3, the validator's own floor
		{"domain below floor", "user@ab", false},
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"trims surrounding whitespace", "  user@example.com  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateEmail(tt.email); got != tt.want {
				t.Errorf("ValidateEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}
