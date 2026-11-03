package user

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// withoutPassword makes u an account that signs in some other way only.
func withoutPassword(t *testing.T, db *gorm.DB, u *User) {
	t.Helper()
	if err := db.Exec(`UPDATE users SET password_hash = '' WHERE id = ?`, u.ID).Error; err != nil {
		t.Fatal(err)
	}
}

func newPasskey(u *User, name string) *Passkey {
	return &Passkey{
		TenantID:     u.TenantID,
		UserID:       u.ID,
		CredentialID: []byte(uuid.NewString()),
		PublicKey:    []byte("public-key-" + name),
		Name:         name,
	}
}

// A passkey is one way to sign in. With a provider account beside it either may
// go; alone, it may not.
func TestPasskeys_TheLastWayToSignInStays(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	keys := NewPasskeyService(db, zap.NewNop())
	tn := freshTenant(t, db, ts)
	u := newAccount(t, svc, tn.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
	withoutPassword(t, db, u)

	key := newPasskey(u, "laptop")
	if err := keys.Add(key); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// The only way in.
	if err := keys.Remove(u.ID, key.ID); !errors.Is(err, ErrLastSignInMethod) {
		t.Fatalf("Remove of the only way to sign in = %v, want ErrLastSignInMethod", err)
	}

	// With a Google account beside it the passkey may go...
	if err := svc.LinkProvider(u.ID, ProviderLink{Provider: "google", Subject: "g-" + uuid.NewString(), Email: u.Email}); err != nil {
		t.Fatal(err)
	}
	// ...and the Google account may go while the passkey stays.
	got, err := svc.GetByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UnlinkIdentity(u.ID, got.Identities[0].ID); err != nil {
		t.Fatalf("UnlinkIdentity beside a passkey = %v, want it allowed", err)
	}
	// The passkey is the only way again.
	if err := keys.Remove(u.ID, key.ID); !errors.Is(err, ErrLastSignInMethod) {
		t.Fatalf("Remove after the provider account went = %v, want ErrLastSignInMethod", err)
	}
	// A provider account is the only way when the passkey went first.
	if err := svc.LinkProvider(u.ID, ProviderLink{Provider: "google", Subject: "g-" + uuid.NewString(), Email: u.Email}); err != nil {
		t.Fatal(err)
	}
	if err := keys.Remove(u.ID, key.ID); err != nil {
		t.Fatalf("Remove beside a provider account = %v, want it allowed", err)
	}
	got, _ = svc.GetByID(u.ID)
	if err := svc.UnlinkIdentity(u.ID, got.Identities[0].ID); !errors.Is(err, ErrLastSignInMethod) {
		t.Fatalf("UnlinkIdentity of the only way = %v, want ErrLastSignInMethod", err)
	}
}

// A password is a way too: an account with one may lose every passkey.
func TestPasskeys_APasswordIsAWayToSignIn(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	keys := NewPasskeyService(db, zap.NewNop())
	tn := freshTenant(t, db, ts)
	u := newAccount(t, svc, tn.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	key := newPasskey(u, "phone")
	if err := keys.Add(key); err != nil {
		t.Fatal(err)
	}
	if err := keys.Remove(u.ID, key.ID); err != nil {
		t.Fatalf("Remove beside a password = %v", err)
	}
	if err := keys.Remove(u.ID, key.ID); !errors.Is(err, ErrPasskeyNotFound) {
		t.Fatalf("second Remove = %v, want ErrPasskeyNotFound", err)
	}
}

// Another account's passkey is not found, whatever its id.
func TestPasskeys_AnAccountRemovesOnlyItsOwn(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	keys := NewPasskeyService(db, zap.NewNop())
	tn := freshTenant(t, db, ts)
	owner := newAccount(t, svc, tn.ID, "correct-horse-9")
	other := newAccount(t, svc, tn.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id IN (?, ?)`, owner.ID, other.ID) })

	key := newPasskey(owner, "tablet")
	if err := keys.Add(key); err != nil {
		t.Fatal(err)
	}
	if err := keys.Remove(other.ID, key.ID); !errors.Is(err, ErrPasskeyNotFound) {
		t.Fatalf("Remove by another account = %v, want ErrPasskeyNotFound", err)
	}
	if rows, _ := keys.List(owner.ID); len(rows) != 1 {
		t.Fatalf("the owner's passkeys = %d, want 1", len(rows))
	}
}

// A credential id names one passkey in a tenant, and may name another in
// another tenant: it is chosen by whoever makes the credential, so no one may
// keep it from being registered elsewhere.
func TestPasskeys_ACredentialIdIsUniqueWithinATenant(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	keys := NewPasskeyService(db, zap.NewNop())
	tnA := freshTenant(t, db, ts)
	tnB := freshTenant(t, db, ts)
	a := newAccount(t, svc, tnA.ID, "correct-horse-9")
	b := newAccount(t, svc, tnB.ID, "correct-horse-9")
	a2 := newAccount(t, svc, tnA.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id IN (?, ?, ?)`, a.ID, b.ID, a2.ID) })

	first := newPasskey(a, "one")
	if err := keys.Add(first); err != nil {
		t.Fatal(err)
	}
	sameInTenant := newPasskey(a2, "copy")
	sameInTenant.CredentialID = first.CredentialID
	if err := keys.Add(sameInTenant); !errors.Is(err, ErrPasskeyTaken) {
		t.Fatalf("the same credential id in the tenant = %v, want ErrPasskeyTaken", err)
	}
	sameElsewhere := newPasskey(b, "elsewhere")
	sameElsewhere.CredentialID = first.CredentialID
	if err := keys.Add(sameElsewhere); err != nil {
		t.Fatalf("the same credential id in another tenant = %v, want it allowed", err)
	}

	// Found by tenant and credential id, never by the credential id alone.
	got, err := keys.Find(tnA.ID, first.CredentialID)
	if err != nil || got.UserID != a.ID {
		t.Fatalf("Find in tenant A = %+v, %v; want account a's", got, err)
	}
	got, err = keys.Find(tnB.ID, first.CredentialID)
	if err != nil || got.UserID != b.ID {
		t.Fatalf("Find in tenant B = %+v, %v; want account b's", got, err)
	}
	if _, err := keys.Find(tnA.ID, []byte("never registered")); !errors.Is(err, ErrPasskeyNotFound) {
		t.Fatalf("Find of an unknown credential = %v, want ErrPasskeyNotFound", err)
	}
}

// Two removals racing for an account's last two ways to sign in are made one
// after the other: one succeeds, the other is told it is the only way left.
func TestPasskeys_ConcurrentRemovalsLeaveAWayToSignIn(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	keys := NewPasskeyService(db, zap.NewNop())
	tn := freshTenant(t, db, ts)
	u := newAccount(t, svc, tn.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
	withoutPassword(t, db, u)

	one, two := newPasskey(u, "one"), newPasskey(u, "two")
	for _, k := range []*Passkey{one, two} {
		if err := keys.Add(k); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i, k := range []*Passkey{one, two} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = keys.Remove(u.ID, k.ID)
		}()
	}
	wg.Wait()
	removed, refused := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			removed++
		case errors.Is(err, ErrLastSignInMethod):
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if removed != 1 || refused != 1 {
		t.Fatalf("removed %d and refused %d, want one of each", removed, refused)
	}
	if rows, _ := keys.List(u.ID); len(rows) != 1 {
		t.Fatalf("passkeys left = %d, want 1", len(rows))
	}
}

// What the device reported at its last use is what the next sign-in is checked
// against.
func TestPasskeys_RecordUseKeepsTheCounterAndFlags(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	keys := NewPasskeyService(db, zap.NewNop())
	tn := freshTenant(t, db, ts)
	u := newAccount(t, svc, tn.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	key := newPasskey(u, "laptop")
	if err := keys.Add(key); err != nil {
		t.Fatal(err)
	}
	if err := keys.RecordUse(key.ID, 7, 0x1d); err != nil {
		t.Fatal(err)
	}
	got, err := keys.Find(tn.ID, key.CredentialID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SignCount != 7 || got.Flags != 0x1d || got.LastUsedAt == nil {
		t.Fatalf("after RecordUse: sign_count=%d flags=%#x last_used_at=%v", got.SignCount, got.Flags, got.LastUsedAt)
	}
}

// Two sign-ins at once can store out of order; the counter must not go back, or
// a copy replaying the higher number would pass for the device.
func TestPasskeys_TheStoredCounterOnlyRises(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	keys := NewPasskeyService(db, zap.NewNop())
	tn := freshTenant(t, db, ts)
	u := newAccount(t, svc, tn.ID, "correct-horse-9")
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	key := newPasskey(u, "laptop")
	if err := keys.Add(key); err != nil {
		t.Fatal(err)
	}
	for _, count := range []uint32{9, 7, 8, 0} {
		if err := keys.RecordUse(key.ID, count, 0x1d); err != nil {
			t.Fatal(err)
		}
	}
	got, err := keys.Find(tn.ID, key.CredentialID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SignCount != 9 {
		t.Fatalf("after 9, 7, 8, 0 the counter is %d, want 9", got.SignCount)
	}
}
