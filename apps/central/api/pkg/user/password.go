package user

import (
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"
)

// The password rule, held where passwords are stored so that every way of
// setting one meets it. A tenant's password_min_length setting can raise the
// least length; it cannot lower it. No password is longer than the hash reads.
const (
	MinPasswordLength = 8
	MaxPasswordLength = 72 // bytes — bcrypt takes in no more
)

// PasswordError is a password the rule refuses. It is the caller's to
// correct, so its text is written to be shown.
type PasswordError struct {
	// MinLength is the least length in force for the account's tenant.
	MinLength int
	tooLong   bool
}

func (e *PasswordError) Error() string {
	if e.tooLong {
		return fmt.Sprintf("password must be at most %d bytes", MaxPasswordLength)
	}
	return fmt.Sprintf("password must be at least %d characters", e.MinLength)
}

// Code names the refusal for an API answer.
func (e *PasswordError) Code() string {
	if e.tooLong {
		return "password_too_long"
	}
	return "password_too_short"
}

// PasswordMinLength has fn asked for the least password length a tenant
// requires. An answer below MinPasswordLength — a tenant that set none —
// leaves MinPasswordLength in force.
func PasswordMinLength(fn func(tenantID uuid.UUID) int) Option {
	return func(s *service) { s.passwordMinLength = fn }
}

func (s *service) checkPassword(tenantID uuid.UUID, password string) error {
	least := MinPasswordLength
	if s.passwordMinLength != nil {
		if asked := s.passwordMinLength(tenantID); asked > least {
			least = asked
		}
	}
	if utf8.RuneCountInString(password) < least {
		return &PasswordError{MinLength: least}
	}
	if len(password) > MaxPasswordLength {
		return &PasswordError{MinLength: least, tooLong: true}
	}
	return nil
}
