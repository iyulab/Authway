package user

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
)

func newAccount(t *testing.T, svc Service, tenantID uuid.UUID, password string) *User {
	t.Helper()
	u, err := svc.Create(tenantID, &CreateUserRequest{Email: fmt.Sprintf("ident-%s@example.com", uuid.NewString()[:8]), Name: "Ident", Password: password})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return u
}

// Before the rows are read they are made to match the columns once: a link an
// API from before the rows made has no row, and one it removed still has one.
func TestReconcile_MakesTheIdentityRowsMatchTheProviderColumns(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tn := freshTenant(t, db, ts)
	u := newAccount(t, svc, tn.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
	gone, kept, replaced := "gh-"+uuid.NewString(), "g-"+uuid.NewString(), "g-"+uuid.NewString()

	// An old API linked google (column only), removed github (row left behind).
	db.Exec(`UPDATE users SET google_id = ? WHERE id = ?`, replaced, u.ID)
	db.Exec(`INSERT INTO user_identities (id, tenant_id, user_id, provider, subject, email_at_link) VALUES (?, ?, ?, 'github', ?, ?)`, uuid.New(), tn.ID, u.ID, gone, u.Email)
	db.Exec(`INSERT INTO user_identities (id, tenant_id, user_id, provider, subject, email_at_link) VALUES (?, ?, ?, 'google', ?, ?)`, uuid.New(), tn.ID, u.ID, kept, u.Email)

	migration, err := os.ReadFile("../../internal/database/migrations/055_reconcile_user_identities.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(migration)).Error; err != nil {
		t.Fatalf("migration: %v", err)
	}
	got, err := svc.GetByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Identities) != 1 || got.Identities[0].Provider != "google" || got.Identities[0].Subject != replaced {
		t.Fatalf("identities = %+v, want only google %q (the column's)", got.Identities, replaced)
	}
}

// An account may sign in with several provider accounts, at one provider and at
// several; each is found by its own and each is taken away on its own. The
// provider columns on users are left alone: nothing writes them any more.
func TestIdentities_AnAccountMayHaveSeveralAtOneProvider(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tn := freshTenant(t, db, ts)
	u := newAccount(t, svc, tn.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
	x, y, z := "g-"+uuid.NewString(), "g-"+uuid.NewString(), "gh-"+uuid.NewString()

	for _, link := range []ProviderLink{
		{Provider: "google", Subject: x, Email: "work@corp.example"},
		{Provider: "google", Subject: y, Email: "home@gmail.example"},
		{Provider: "github", Subject: z},
	} {
		if err := svc.LinkProvider(u.ID, link); err != nil {
			t.Fatalf("LinkProvider %s: %v", link.Subject, err)
		}
	}
	// The same link again changes nothing.
	if err := svc.LinkProvider(u.ID, ProviderLink{Provider: "google", Subject: y}); err != nil {
		t.Errorf("linking again: %v", err)
	}

	got, _ := svc.GetByID(u.ID)
	if len(got.Identities) != 3 || got.Identities[0].Subject != x || got.Identities[1].Subject != y {
		t.Fatalf("identities = %+v, want x, y, z oldest first", got.Identities)
	}
	if got.Identities[0].EmailAtLink != "work@corp.example" || got.Identities[2].EmailAtLink != u.Email {
		t.Errorf("addresses at link = %q, %q; want the provider's, then the account's own", got.Identities[0].EmailAtLink, got.Identities[2].EmailAtLink)
	}
	if providers := got.LinkedProviders(); len(providers) != 2 || providers[0] != "google" || providers[1] != "github" {
		t.Errorf("providers = %v, want [google github] once each", providers)
	}
	if got.EmailVerified {
		t.Error("a link that does not verify the address verified it")
	}
	for _, subject := range []string{x, y} {
		if found, err := svc.GetByProviderSubject(tn.ID, "google", subject); err != nil || found.ID != u.ID {
			t.Errorf("google %s finds %v, %v; want the account", subject, found, err)
		}
	}
	// Removed together with the column by the migration that drops it.
	legacyColumnEmpty := func(when string) {
		t.Helper()
		var c *string
		if err := db.Raw(`SELECT google_id FROM users WHERE id = ?`, u.ID).Scan(&c).Error; err != nil {
			t.Fatalf("reading google_id: %v", err)
		}
		if c != nil {
			t.Errorf("%s: google_id = %q, want it left alone (nothing writes the column any more)", when, *c)
		}
	}
	legacyColumnEmpty("after linking")

	// Another account cannot take either.
	other := newAccount(t, svc, tn.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, other.ID) })
	if err := svc.LinkProvider(other.ID, ProviderLink{Provider: "google", Subject: y}); !errors.Is(err, ErrProviderSubjectTaken) {
		t.Errorf("a second account taking y: %v, want ErrProviderSubjectTaken", err)
	}

	// A provider's name no longer says which; the id does.
	if err := svc.UnlinkProvider(u.ID, "google"); !errors.Is(err, ErrSeveralLinked) {
		t.Errorf("unlinking google by name with two: %v, want ErrSeveralLinked", err)
	}
	if err := svc.UnlinkIdentity(u.ID, got.Identities[0].ID); err != nil {
		t.Fatalf("UnlinkIdentity x: %v", err)
	}
	legacyColumnEmpty("after removing the oldest")
	if _, err := svc.GetByProviderSubject(tn.ID, "google", x); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("the removed link still finds an account: %v", err)
	}
	if err := svc.UnlinkProvider(u.ID, "google"); err != nil {
		t.Fatalf("unlinking the only google by name: %v", err)
	}
	legacyColumnEmpty("after removing the last")
	// Another account's identity is not this account's to remove.
	if err := svc.UnlinkIdentity(other.ID, got.Identities[2].ID); !errors.Is(err, ErrNotLinked) {
		t.Errorf("removing another account's identity: %v, want ErrNotLinked", err)
	}
}

// The account's last way to sign in stays, whichever identity is named.
func TestIdentities_TheLastWayToSignInStays(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tn := freshTenant(t, db, ts)
	u := newAccount(t, svc, tn.ID, "")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
	for _, subject := range []string{"g-" + uuid.NewString(), "g-" + uuid.NewString()} {
		if err := svc.LinkProvider(u.ID, ProviderLink{Provider: "google", Subject: subject, VerifiesAddress: true}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := svc.GetByID(u.ID)
	if err := svc.UnlinkIdentity(u.ID, got.Identities[0].ID); err != nil {
		t.Fatalf("with two links and no password the first may go: %v", err)
	}
	if err := svc.UnlinkIdentity(u.ID, got.Identities[1].ID); !errors.Is(err, ErrLastSignInMethod) {
		t.Errorf("the last link of an account with no password: %v, want ErrLastSignInMethod", err)
	}
	if after, _ := svc.GetByID(u.ID); len(after.Identities) != 1 {
		t.Errorf("identities after refusing = %d, want 1", len(after.Identities))
	}
}
