package user

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

// A link to a sign-in provider's account is stored, finds the account
// afterwards within its tenant only, and marks the address verified.
func TestLinkProvider_StoresTheLinkAndFindsTheAccountByIt(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tnA := freshTenant(t, db, ts)
	tnB := freshTenant(t, db, ts)
	subject := "provider-subject-" + uuid.New().String()[:8]

	u, err := svc.Create(tnA.ID, &CreateUserRequest{Email: fmt.Sprintf("link-%s@example.com", uuid.New().String()[:8]), Name: "Linked"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
	if u.EmailVerified {
		t.Fatal("a new account starts unverified; the test needs that to see the link verify it")
	}

	if _, err := svc.GetByProviderSubject(tnA.ID, "google", subject); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("before linking: %v, want ErrUserNotFound", err)
	}
	if err := svc.LinkProvider(u.ID, "google", subject); err != nil {
		t.Fatalf("LinkProvider: %v", err)
	}

	found, err := svc.GetByProviderSubject(tnA.ID, "google", subject)
	if err != nil || found.ID != u.ID {
		t.Fatalf("after linking: %v, %v; want the account", found, err)
	}
	if !found.EmailVerified || found.GoogleID == nil || *found.GoogleID != subject {
		t.Errorf("stored verified=%v google_id=%v, want verified and the subject", found.EmailVerified, found.GoogleID)
	}

	for name, lookup := range map[string]func() error{
		"another tenant":   func() error { _, err := svc.GetByProviderSubject(tnB.ID, "google", subject); return err },
		"another provider": func() error { _, err := svc.GetByProviderSubject(tnA.ID, "github", subject); return err },
		"an empty subject": func() error { _, err := svc.GetByProviderSubject(tnA.ID, "google", ""); return err },
	} {
		if err := lookup(); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("%s: %v, want ErrUserNotFound", name, err)
		}
	}

	if _, err := svc.GetByProviderSubject(tnA.ID, "myspace", subject); err == nil || errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown provider: %v, want an error that is not \"not found\"", err)
	}
	if err := svc.LinkProvider(u.ID, "myspace", subject); err == nil {
		t.Error("linking an unknown provider must fail")
	}
	if err := svc.LinkProvider(uuid.New(), "google", subject); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("linking a missing account: %v, want ErrUserNotFound", err)
	}
}

// Within a tenant a provider account signs in to one account. The lookup
// before linking keeps to that in the ordinary case; the database keeps to it
// when two sign-ins arrive together.
func TestLinkProvider_OneAccountPerProviderAccountInATenant(t *testing.T) {
	db := setupPostgres(t)
	svc, ts := newTestService(t, db)
	tnA := freshTenant(t, db, ts)
	tnB := freshTenant(t, db, ts)
	subject := "provider-subject-" + uuid.New().String()[:8]

	account := func(tenantID uuid.UUID) *User {
		t.Helper()
		u, err := svc.Create(tenantID, &CreateUserRequest{Email: fmt.Sprintf("link-%s@example.com", uuid.New().String()[:8]), Name: "Linked"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })
		return u
	}
	first, second, elsewhere := account(tnA.ID), account(tnA.ID), account(tnB.ID)

	if err := svc.LinkProvider(first.ID, "google", subject); err != nil {
		t.Fatalf("the first link: %v", err)
	}
	if err := svc.LinkProvider(first.ID, "google", subject); err != nil {
		t.Errorf("linking the same account again: %v, want no error", err)
	}
	if err := svc.LinkProvider(second.ID, "google", subject); !errors.Is(err, ErrProviderSubjectTaken) {
		t.Fatalf("a second account in the tenant: %v, want ErrProviderSubjectTaken", err)
	}
	if found, err := svc.GetByProviderSubject(tnA.ID, "google", subject); err != nil || found.ID != first.ID {
		t.Errorf("after the refused link the provider account finds %v, %v; want the first account", found, err)
	}
	if again, err := svc.GetByID(second.ID); err != nil || again.GoogleID != nil || again.EmailVerified {
		t.Errorf("the refused link changed the second account: google_id=%v verified=%v (%v)", again.GoogleID, again.EmailVerified, err)
	}

	// The rule is per tenant and per provider.
	if err := svc.LinkProvider(elsewhere.ID, "google", subject); err != nil {
		t.Errorf("the same provider account in another tenant: %v", err)
	}
	if err := svc.LinkProvider(second.ID, "github", subject); err != nil {
		t.Errorf("the same identifier at another provider: %v", err)
	}

	// A deleted account no longer holds the provider account.
	if err := svc.Delete(first.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := svc.LinkProvider(second.ID, "google", subject); err != nil {
		t.Errorf("after the first account was deleted: %v", err)
	}
}
