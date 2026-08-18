package handler

import (
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
)

// TestResolveUserByEmail_ScopesByClientTenant is the regression this cycle
// exists for (ISSUE-Authway-20260817-115815, HD-10): the schema explicitly
// allows the same email in more than one tenant (idx_users_tenant_email), so
// the email self-service flows (send-verification, forgot-password) must
// resolve the requesting OAuth client's tenant when one is provided, not
// match the email globally.
func TestResolveUserByEmail_ScopesByClientTenant(t *testing.T) {
	rightUser := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "shared@example.com"}
	wrongTenantUser := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "shared@example.com"}
	users := newFakeUserService(rightUser, wrongTenantUser)
	clients := newFakeClientService(&client.Client{ID: uuid.New(), TenantID: rightUser.TenantID, ClientID: testClientID})
	h := &EmailHandler{userSvc: users, clientSvc: clients, logger: zap.NewNop()}

	got, err := h.resolveUserByEmail(testClientID, "shared@example.com")
	if err != nil {
		t.Fatalf("resolveUserByEmail: %v", err)
	}
	if got.ID != rightUser.ID {
		t.Errorf("resolved user = %s, want the requesting client's tenant's user %s (not the other tenant's %s)",
			got.ID, rightUser.ID, wrongTenantUser.ID)
	}
}

// TestResolveUserByEmail_FallsBackToUnscopedWhenNoClientID covers the entry
// points that have no OAuth context at all (a bookmarked reset-password
// link) — client_id is optional, and omitting it must not error, only
// forgo tenant scoping.
func TestResolveUserByEmail_FallsBackToUnscopedWhenNoClientID(t *testing.T) {
	solo := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "solo@example.com"}
	users := newFakeUserService(solo)
	clients := newFakeClientService()
	h := &EmailHandler{userSvc: users, clientSvc: clients, logger: zap.NewNop()}

	got, err := h.resolveUserByEmail("", "solo@example.com")
	if err != nil {
		t.Fatalf("resolveUserByEmail: %v", err)
	}
	if got.ID != solo.ID {
		t.Errorf("resolved user = %s, want %s", got.ID, solo.ID)
	}
}

// TestResolveUserByEmail_UnknownClientIDFallsBackToUnscoped covers a
// caller-supplied client_id that this API no longer (or never did)
// recognize — must degrade to the pre-fix unscoped behavior rather than
// hard-failing the whole request.
func TestResolveUserByEmail_UnknownClientIDFallsBackToUnscoped(t *testing.T) {
	solo := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "solo@example.com"}
	users := newFakeUserService(solo)
	clients := newFakeClientService() // no clients registered
	h := &EmailHandler{userSvc: users, clientSvc: clients, logger: zap.NewNop()}

	got, err := h.resolveUserByEmail("no-such-client", "solo@example.com")
	if err != nil {
		t.Fatalf("resolveUserByEmail: %v", err)
	}
	if got.ID != solo.ID {
		t.Errorf("resolved user = %s, want %s", got.ID, solo.ID)
	}
}
