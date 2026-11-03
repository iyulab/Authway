package user

import (
	"testing"

	"authway/apps/central/api/pkg/tenant"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// An address that differs only in case is the same address: the account is
// stored in lower case, found however it is written, and a second account for
// the same address in another case is refused.
func TestEmailAddress_IsOneAccountWhateverTheCase(t *testing.T) {
	db := setupPostgres(t)
	ts := tenant.NewService(db)
	here := freshTenant(t, db, ts)
	elsewhere := freshTenant(t, db, ts)
	svc := NewService(db, zap.NewNop())

	local := "k-case-" + uuid.NewString()[:8]
	created, err := svc.Create(here.ID, &CreateUserRequest{Email: " " + local + "@Example.COM", Name: "C", Password: "correct-horse-9"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { db.Delete(&User{}, "id = ?", created.ID) })

	if want := local + "@example.com"; created.Email != want {
		t.Fatalf("stored address = %q, want %q", created.Email, want)
	}
	for _, written := range []string{local + "@example.com", local + "@EXAMPLE.com", " " + local + "@Example.Com "} {
		got, err := svc.GetByEmailAndTenant(here.ID, written)
		if err != nil || got.ID != created.ID {
			t.Errorf("GetByEmailAndTenant(%q) = %v, %v; want the account", written, got, err)
		}
		found, err := svc.FindByEmail(here.ID, written)
		if err != nil || len(found) != 1 || found[0].ID != created.ID {
			t.Errorf("FindByEmail(%q) = %v, %v; want the account", written, found, err)
		}
	}

	if _, err := svc.Create(here.ID, &CreateUserRequest{Email: local + "@EXAMPLE.COM", Name: "Again", Password: "correct-horse-9"}); err == nil {
		t.Error("a second account for the same address in another case was created")
	}

	// A letter outside ASCII is not folded: the Kelvin sign is not "k", so an
	// address spelled with it is another address, not this one.
	kelvin := "K" + local[1:] + "@example.com"
	got, err := svc.GetByEmailAndTenant(here.ID, kelvin)
	if err == nil {
		t.Errorf("the Kelvin-sign spelling found %v; it is not the same address", got.ID)
	}

	// Another tenant is another set of accounts: the address is free there, and
	// the sole-account lookup (no tenant to scope by) then finds two, not one.
	other, err := svc.Create(elsewhere.ID, &CreateUserRequest{Email: local + "@EXAMPLE.COM", Name: "Other", Password: "correct-horse-9"})
	if err != nil {
		t.Fatalf("the same address in another tenant: %v", err)
	}
	t.Cleanup(func() { db.Delete(&User{}, "id = ?", other.ID) })
	if _, err := svc.GetSoleAccountByEmail(local + "@Example.com"); err == nil {
		t.Error("GetSoleAccountByEmail answered for an address with accounts in two tenants")
	}
}
