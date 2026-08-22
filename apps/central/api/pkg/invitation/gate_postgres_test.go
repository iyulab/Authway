package invitation

import (
	"testing"

	"authway/apps/central/api/pkg/tenant"
	"github.com/google/uuid"
)

// TestMayProvision_InviteOnlyTenant guards the default: a tenant with no
// signup_mode set (every tenant created before this field existed, and every
// tenant created without explicitly opting into open signup) behaves exactly
// like HasValidInvitation alone always did.
func TestMayProvision_InviteOnlyTenant(t *testing.T) {
	db := setupPostgres(t)
	ts := tenant.NewService(db)
	tn := freshTenant(t, ts)
	gate := NewGate(db)

	allowed, err := gate.MayProvision(tn.ID, "uninvited@example.com")
	if err != nil {
		t.Fatalf("MayProvision: %v", err)
	}
	if allowed {
		t.Fatal("expected an uninvited address to be denied on an invite-only tenant")
	}
}

// TestMayProvision_OpenSignupTenant_BypassesInvitationCheck guards the actual
// point of the feature: an "open" tenant must admit an address that holds no
// invitation at all.
func TestMayProvision_OpenSignupTenant_BypassesInvitationCheck(t *testing.T) {
	db := setupPostgres(t)
	ts := tenant.NewService(db)
	suffix := uuid.New().String()[:8]
	tn, err := ts.CreateTenant(tenant.CreateTenantRequest{
		Name:     "open-signup-test-" + suffix,
		Slug:     "open-signup-test-" + suffix,
		Settings: tenant.TenantSettings{SignupMode: tenant.SignupModeOpen},
	})
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	gate := NewGate(db)

	allowed, err := gate.MayProvision(tn.ID, "anyone@example.com")
	if err != nil {
		t.Fatalf("MayProvision: %v", err)
	}
	if !allowed {
		t.Fatal("expected an uninvited address to be allowed on an open-signup tenant")
	}
}

// TestMayProvision_OpenSignupTenant_ScopedPerTenant guards against the
// obvious way this could go wrong: one tenant opting into open signup must
// not relax the check for any other tenant.
func TestMayProvision_OpenSignupTenant_ScopedPerTenant(t *testing.T) {
	db := setupPostgres(t)
	ts := tenant.NewService(db)
	suffix := uuid.New().String()[:8]
	openTenant, err := ts.CreateTenant(tenant.CreateTenantRequest{
		Name:     "open-scope-test-" + suffix,
		Slug:     "open-scope-test-" + suffix,
		Settings: tenant.TenantSettings{SignupMode: tenant.SignupModeOpen},
	})
	if err != nil {
		t.Fatalf("CreateTenant (open): %v", err)
	}
	closedTenant := freshTenant(t, ts)
	gate := NewGate(db)

	if allowed, err := gate.MayProvision(openTenant.ID, "anyone@example.com"); err != nil || !allowed {
		t.Fatalf("MayProvision (open tenant) = %v, %v; want true, nil", allowed, err)
	}
	if allowed, err := gate.MayProvision(closedTenant.ID, "anyone@example.com"); err != nil || allowed {
		t.Fatalf("MayProvision (closed tenant) = %v, %v; want false, nil", allowed, err)
	}
}
