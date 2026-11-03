package user

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// The password rule, held where passwords are stored so that every way of
// setting one meets it. A tenant's password_min_length setting can raise the
// least length; it cannot lower it. No password is longer than the hash reads.
const (
	MinPasswordLength = 8
	MaxPasswordLength = 72 // bytes — bcrypt takes in no more
)

// ErrWrongPassword is a current password that does not match the account's.
var ErrWrongPassword = errors.New("the current password is not correct")

// PasswordError is a password the rule refuses. It is the caller's to
// correct, so its text is written to be shown.
type PasswordError struct {
	// MinLength is the least length in force for the account's tenant.
	MinLength int
	tooLong   bool
	breached  bool
}

func (e *PasswordError) Error() string {
	switch {
	case e.tooLong:
		return fmt.Sprintf("password must be at most %d bytes", MaxPasswordLength)
	case e.breached:
		return "this password appears in known data breaches; choose a different one"
	}
	return fmt.Sprintf("password must be at least %d characters", e.MinLength)
}

// Code names the refusal for an API answer.
func (e *PasswordError) Code() string {
	switch {
	case e.tooLong:
		return "password_too_long"
	case e.breached:
		return "password_breached"
	}
	return "password_too_short"
}

// BreachChecker says whether a password is in a known data breach. An error
// means it could not tell.
type BreachChecker interface {
	Breached(ctx context.Context, password string) (bool, error)
}

// breachCheckTimeout bounds the wait for a BreachChecker, whatever its own
// limits are: a person is waiting to set a password.
const breachCheckTimeout = 3 * time.Second

// PasswordBreachCheck has every password that is set also checked against
// known data breaches. When the check cannot be made the password is let
// through, and the failure is logged: a service that is down must not stop
// anyone from signing up or recovering an account, and the length rule has
// already been applied.
func PasswordBreachCheck(checker BreachChecker) Option {
	return func(s *service) { s.breach = checker }
}

// PasswordMinLength has fn asked for the least password length a tenant
// requires. An answer below MinPasswordLength — a tenant that set none —
// leaves MinPasswordLength in force.
func PasswordMinLength(fn func(tenantID uuid.UUID) int) Option {
	return func(s *service) { s.passwordMinLength = fn }
}

func (s *service) LeastPasswordLength(tenantID uuid.UUID) int {
	least := MinPasswordLength
	if s.passwordMinLength != nil {
		if asked := s.passwordMinLength(tenantID); asked > least {
			least = asked
		}
	}
	return least
}

func (s *service) checkPassword(tenantID uuid.UUID, password string) error {
	least := s.LeastPasswordLength(tenantID)
	if utf8.RuneCountInString(password) < least {
		return &PasswordError{MinLength: least}
	}
	if len(password) > MaxPasswordLength {
		return &PasswordError{MinLength: least, tooLong: true}
	}
	if s.breach != nil {
		ctx, cancel := context.WithTimeout(context.Background(), breachCheckTimeout)
		defer cancel()
		breached, err := s.breach.Breached(ctx, password)
		if err != nil {
			// Nothing here may carry the password.
			s.logger.Warn("A password was not checked against known data breaches", zap.Error(err))
			return nil
		}
		if breached {
			return &PasswordError{MinLength: least, breached: true}
		}
	}
	return nil
}
