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
