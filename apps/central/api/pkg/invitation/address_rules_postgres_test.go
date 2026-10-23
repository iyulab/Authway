package invitation

import (
	"errors"
	"fmt"
	"testing"

	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/user"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func tenantWith(t *testing.T, ts *tenant.Service, settings tenant.TenantSettings) *tenant.Tenant {
	t.Helper()
	suffix := uuid.New().String()[:8]
	tn, err := ts.CreateTenant(tenant.CreateTenantRequest{Name: "rules-" + suffix, Slug: "rules-" + suffix, Settings: settings})
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	return tn
}

// An invitation is where an address enters a tenant, so that is where a
// tenant that names its domains turns the others away.
func TestCreate_RefusesAnAddressOutsideTheDomainsTheTenantAllows(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, _ := newTestService(t, db)
	tn := tenantWith(t, ts, tenant.TenantSettings{AllowedDomains: []string{"Example.com"}})

	if _, err := svc.Create(tn.ID, Inviter{}, &CreateInvitationRequest{Email: "someone@elsewhere.example"}); !errors.Is(err, ErrAddressNotAllowed) {
		t.Fatalf("an outside address: got %v, want ErrAddressNotAllowed", err)
	}
	// A subdomain is a different domain.
	if _, err := svc.Create(tn.ID, Inviter{}, &CreateInvitationRequest{Email: "someone@mail.example.com"}); !errors.Is(err, ErrAddressNotAllowed) {
		t.Fatalf("a subdomain: got %v, want ErrAddressNotAllowed", err)
	}
	inv, err := svc.Create(tn.ID, Inviter{}, &CreateInvitationRequest{Email: fmt.Sprintf("in-%s@example.com", uuid.NewString()[:8])})
	if err != nil {
		t.Fatalf("an address in the domain: %v", err)
	}
	db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID)
}

// The same rule for an account a first sign-in would create: open sign-up
// means open to the tenant's domains, when it names any.
func TestMayProvision_HoldsFirstSignInsToTheTenantsDomains(t *testing.T) {
	db := setupPostgres(t)
	ts := tenant.NewService(db)
	gate := NewGate(db)

	open := tenantWith(t, ts, tenant.TenantSettings{SignupMode: tenant.SignupModeOpen, AllowedDomains: []string{"example.com"}})
	for email, want := range map[string]bool{"a@example.com": true, "a@EXAMPLE.COM": true, "a@elsewhere.example": false, "a@notexample.com": false} {
		got, err := gate.MayProvision(open.ID, email)
		if err != nil || got != want {
			t.Errorf("open sign-up, %s: got %v (%v), want %v", email, got, err, want)
		}
	}

	// A tenant that names no domain is as before, whatever is stored for
	// "none": an absent key, an empty list, a list of blanks.
	for _, none := range [][]string{nil, {}, {""}} {
		anyDomain := tenantWith(t, ts, tenant.TenantSettings{SignupMode: tenant.SignupModeOpen, AllowedDomains: none})
		if got, err := gate.MayProvision(anyDomain.ID, "a@elsewhere.example"); err != nil || !got {
			t.Errorf("no domain named (%v): got %v (%v), want allowed", none, got, err)
		}
	}
}

// Accepting an invitation proves the address it was sent to, and the password
// set on the way in meets the rule like any other.
func TestAccept_ProvesTheAddressAndHoldsThePasswordToTheRule(t *testing.T) {
	db := setupPostgres(t)
	tenantService := tenant.NewService(db)
	userService := user.NewService(db, zap.NewNop(), user.PasswordMinLength(tenantService.PasswordMinLength))
	sender := &stubEmailSender{}
	svc := NewService(db, userService, tenantService, sender, zap.NewNop(), "http://localhost:3001")
	tn := tenantWith(t, tenantService, tenant.TenantSettings{PasswordMinLength: 12})

	email := fmt.Sprintf("invited-%s@example.com", uuid.NewString()[:8])
	inv, err := svc.Create(tn.ID, Inviter{}, &CreateInvitationRequest{Email: email})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID)
	token := tokenFromLinkURL(t, sender.lastLinkURL)

	var refused *user.PasswordError
	if _, err := svc.Accept(token, nil, "Invited", "x"); !errors.As(err, &refused) || refused.MinLength != 12 {
		t.Fatalf("a one-character password: got %v, want a password refusal naming 12", err)
	}
	if _, err := svc.Accept(token, nil, "Invited", "elevenchars"); !errors.As(err, &refused) {
		t.Fatalf("a password under the tenant's length: got %v, want a password refusal", err)
	}

	u, err := svc.Accept(token, nil, "Invited", "twelve-chars")
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	defer db.Unscoped().Delete(&user.User{}, "id = ?", u.ID)

	stored, err := userService.GetByID(u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !u.EmailVerified || !stored.EmailVerified {
		t.Errorf("email_verified: returned %v, stored %v — accepting the invitation proves the address", u.EmailVerified, stored.EmailVerified)
	}
}

// Accounts created by an accepted invitation before acceptance recorded the
// proof are corrected — and only those: an account whose address is no longer
// the invited one, or that no invitation created, is left as it is.
func TestMarkAcceptedAddressesVerified(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, us, sender := newTestServiceWithSender(t, db)
	tn := freshTenant(t, ts)

	invite := func(email string) *user.User {
		t.Helper()
		inv, err := svc.Create(tn.ID, Inviter{}, &CreateInvitationRequest{Email: email})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		t.Cleanup(func() { db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID) })
		u, err := svc.Accept(tokenFromLinkURL(t, sender.lastLinkURL), nil, "Invited", "correct-horse-battery")
		if err != nil {
			t.Fatalf("Accept: %v", err)
		}
		t.Cleanup(func() { db.Unscoped().Delete(&user.User{}, "id = ?", u.ID) })
		// As such an account was left before.
		if err := db.Exec(`UPDATE users SET email_verified = false WHERE id = ?`, u.ID).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
		return u
	}
	suffix := uuid.NewString()[:8]
	invited := invite("invited-" + suffix + "@example.com")
	moved := invite("moved-" + suffix + "@example.com")
	if err := db.Exec(`UPDATE users SET email = ? WHERE id = ?`, "elsewhere-"+suffix+"@example.com", moved.ID).Error; err != nil {
		t.Fatalf("seed a changed address: %v", err)
	}
	uninvited, err := us.Create(tn.ID, &user.CreateUserRequest{Email: "uninvited-" + suffix + "@example.com", Name: "Uninvited"})
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&user.User{}, "id = ?", uninvited.ID) })

	for range 2 {
		if err := MarkAcceptedAddressesVerified(db, zap.NewNop()); err != nil {
			t.Fatalf("MarkAcceptedAddressesVerified: %v", err)
		}
	}

	for id, want := range map[uuid.UUID]bool{invited.ID: true, moved.ID: false, uninvited.ID: false} {
		got, err := us.GetByID(id)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.EmailVerified != want {
			t.Errorf("%s: email_verified = %v, want %v", got.Email, got.EmailVerified, want)
		}
	}
}
