package user

import (
	"errors"
	"strings"
	"testing"

	"authway/apps/central/api/pkg/tenant"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// The rule is held where passwords are stored: every way of setting one
// meets it, including ones whose request is not validated on the way in.
func TestPasswordRule_HoldsWhereverAPasswordIsSet(t *testing.T) {
	db := setupPostgres(t)
	ts := tenant.NewService(db)
	plain := freshTenant(t, db, ts)
	strict := freshTenant(t, db, ts)
	lax := freshTenant(t, db, ts)
	asks := map[uuid.UUID]int{strict.ID: 12, lax.ID: 4}
	svc := NewService(db, zap.NewNop(), PasswordMinLength(func(id uuid.UUID) int { return asks[id] }))

	refusal := func(err error) *PasswordError {
		var refused *PasswordError
		errors.As(err, &refused)
		return refused
	}
	create := func(tenantID uuid.UUID, password string) (*User, error) {
		u, err := svc.Create(tenantID, &CreateUserRequest{Email: "pw-" + uuid.NewString()[:8] + "@example.com", Name: "P", Password: password})
		if err == nil {
			t.Cleanup(func() { db.Unscoped().Delete(&User{}, "id = ?", u.ID) })
		}
		return u, err
	}

	// Every tenant: at least 8, whatever less a tenant asks for.
	for _, tenantID := range []uuid.UUID{plain.ID, lax.ID} {
		if _, err := create(tenantID, "seven77"); refusal(err) == nil || refusal(err).MinLength != 8 || refusal(err).Code() != "password_too_short" {
			t.Errorf("7 characters: got %v, want password_too_short naming 8", err)
		}
		if _, err := create(tenantID, "eight888"); err != nil {
			t.Errorf("8 characters: %v", err)
		}
	}

	// A tenant that asks for more.
	if _, err := create(strict.ID, "elevenchars"); refusal(err) == nil || refusal(err).MinLength != 12 {
		t.Errorf("11 characters where 12 are asked: got %v, want a refusal naming 12", err)
	}
	u, err := create(strict.ID, "twelve-chars")
	if err != nil {
		t.Fatalf("12 characters where 12 are asked: %v", err)
	}

	// The same on a reset and on a change.
	if err := svc.UpdatePassword(u.ID, "elevenchars"); refusal(err) == nil {
		t.Errorf("reset to 11 characters: got %v, want a refusal", err)
	}
	if err := svc.ChangePassword(u.ID, &ChangePasswordRequest{CurrentPassword: "twelve-chars", NewPassword: "elevenchars"}); refusal(err) == nil {
		t.Errorf("change to 11 characters: got %v, want a refusal", err)
	}
	stored, err := svc.GetByID(u.ID)
	if err != nil || !svc.VerifyPassword(stored, "twelve-chars") {
		t.Errorf("a refused password replaced the one in force (%v)", err)
	}
	if err := svc.UpdatePassword(u.ID, "another-twelve"); err != nil {
		t.Errorf("reset to 14 characters: %v", err)
	}

	// Length is counted in characters; the most is what the hash reads.
	if _, err := create(plain.ID, "비밀번호여덟글자"); err != nil {
		t.Errorf("8 characters outside ASCII: %v", err)
	}
	if _, err := create(plain.ID, strings.Repeat("a", 73)); refusal(err) == nil || refusal(err).Code() != "password_too_long" {
		t.Errorf("73 bytes: got %v, want password_too_long", err)
	}
	if _, err := create(plain.ID, strings.Repeat("a", 72)); err != nil {
		t.Errorf("72 bytes: %v", err)
	}
}
