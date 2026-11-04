package user

import (
	"errors"
	"fmt"
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

// An account may sign in with several provider accounts, at one provider and at
// several; each is found by its own and each is taken away on its own.
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
	if _, err := svc.GetByProviderSubject(tn.ID, "google", x); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("the removed link still finds an account: %v", err)
	}
	if err := svc.UnlinkProvider(u.ID, "google"); err != nil {
		t.Fatalf("unlinking the only google by name: %v", err)
	}
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

// A sign-in identity and a passkey name an account of their own tenant: the
// database refuses a row that repeats another tenant, and a row still goes with
// its account.
func TestIdentitiesAndPasskeys_BelongToTheTenantOfTheirAccount(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	home := freshTenant(t, db, ts)
	away := freshTenant(t, db, ts)
	u := newAccount(t, svc, home.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	err := db.Exec(`INSERT INTO user_identities (id, tenant_id, user_id, provider, subject)
		VALUES (?, ?, ?, 'google', ?)`, uuid.New(), away.ID, u.ID, "g-"+uuid.NewString()).Error
	if err == nil {
		t.Error("an identity named the account of another tenant and was stored")
	}
	err = db.Exec(`INSERT INTO user_passkeys (id, tenant_id, user_id, credential_id, public_key)
		VALUES (?, ?, ?, ?, ?)`, uuid.New(), away.ID, u.ID, []byte(uuid.NewString()), []byte("key")).Error
	if err == nil {
		t.Error("a passkey named the account of another tenant and was stored")
	}

	// Their own tenant is accepted, and the rows go with the account.
	if err := db.Exec(`INSERT INTO user_identities (id, tenant_id, user_id, provider, subject)
		VALUES (?, ?, ?, 'github', ?)`, uuid.New(), home.ID, u.ID, "gh-"+uuid.NewString()).Error; err != nil {
		t.Fatalf("an identity of the account's own tenant: %v", err)
	}
	if err := db.Exec(`INSERT INTO user_passkeys (id, tenant_id, user_id, credential_id, public_key)
		VALUES (?, ?, ?, ?, ?)`, uuid.New(), home.ID, u.ID, []byte(uuid.NewString()), []byte("key")).Error; err != nil {
		t.Fatalf("a passkey of the account's own tenant: %v", err)
	}
	if err := db.Exec(`DELETE FROM users WHERE id = ?`, u.ID).Error; err != nil {
		t.Fatalf("delete account: %v", err)
	}
	for _, table := range []string{"user_identities", "user_passkeys"} {
		var left int64
		db.Raw(`SELECT count(*) FROM `+table+` WHERE user_id = ?`, u.ID).Scan(&left)
		if left != 0 {
			t.Errorf("%s rows left after the account was deleted = %d, want 0", table, left)
		}
	}
}

// The same key holds for the claims, delegations and approval requests of an
// account: each names its account by tenant and id together.
func TestAccountData_IsKeyedToTheTenantOfItsAccount(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	home := freshTenant(t, db, ts)
	away := freshTenant(t, db, ts)
	u := newAccount(t, svc, home.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	for _, table := range []string{"user_identities", "user_passkeys", "user_claims", "delegation_grants", "approval_requests"} {
		var def string
		db.Raw(`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = ?::regclass AND contype = 'f' AND confrelid = 'users'::regclass`, table).Scan(&def)
		if def != "FOREIGN KEY (tenant_id, user_id) REFERENCES users(tenant_id, id) ON DELETE CASCADE" {
			t.Errorf("%s: the key to the account is %q, want the (tenant_id, user_id) pair", table, def)
		}
	}

	err := db.Exec(`INSERT INTO user_claims (id, user_id, tenant_id, claim_key, claim_value)
		VALUES (?, ?, ?, 'plan', '"pro"')`, uuid.New(), u.ID, away.ID).Error
	if err == nil {
		t.Error("a claim named the account of another tenant and was stored")
	}
	if err := db.Exec(`INSERT INTO user_claims (id, user_id, tenant_id, claim_key, claim_value)
		VALUES (?, ?, ?, 'plan', '"pro"')`, uuid.New(), u.ID, home.ID).Error; err != nil {
		t.Errorf("a claim of the account's own tenant: %v", err)
	}
}
