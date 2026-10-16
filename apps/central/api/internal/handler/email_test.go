package handler

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
)

// TestResolveUserByEmail_ScopesByClientTenant: the schema explicitly allows
// the same email in more than one tenant (idx_users_tenant_email), so the
// email self-service flows (send-verification, forgot-password) must resolve
// the requesting OAuth client's tenant when one is provided, not match the
// email globally.
func TestResolveUserByEmail_ScopesByClientTenant(t *testing.T) {
	rightUser := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "shared@example.com"}
	wrongTenantUser := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "shared@example.com"}
	users := newFakeUserService(rightUser, wrongTenantUser)
	clients := newFakeClientService(&client.Client{Active: true, ID: uuid.New(), TenantID: rightUser.TenantID, ClientID: testClientID})
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

// TestResolveUserByEmail_WithoutClientIDFindsTheOnlyAccount covers the entry
// points that have no OAuth context at all (a bookmarked reset-password
// link): an address with one account resolves to it.
func TestResolveUserByEmail_WithoutClientIDFindsTheOnlyAccount(t *testing.T) {
	solo := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "solo@example.com"}
	h := &EmailHandler{userSvc: newFakeUserService(solo), clientSvc: newFakeClientService(), logger: zap.NewNop()}

	got, err := h.resolveUserByEmail("", "solo@example.com")
	if err != nil {
		t.Fatalf("resolveUserByEmail: %v", err)
	}
	if got.ID != solo.ID {
		t.Errorf("resolved user = %s, want %s", got.ID, solo.ID)
	}
}

// TestResolveUserByEmail_WithoutClientIDPicksNoneOfSeveral: an address with
// accounts in more than one tenant names none of them by itself — mail for an
// account picked at random would reset or verify the wrong one.
func TestResolveUserByEmail_WithoutClientIDPicksNoneOfSeveral(t *testing.T) {
	a := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "shared@example.com"}
	b := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "shared@example.com"}
	h := &EmailHandler{userSvc: newFakeUserService(a, b), clientSvc: newFakeClientService(), logger: zap.NewNop()}

	if got, err := h.resolveUserByEmail("", "shared@example.com"); err == nil {
		t.Fatalf("resolved %s, want no account", got.ID)
	}
}

// TestResolveUserByEmail_UnknownClientIDIsRefused: a client_id that names no
// client does not fall back to a lookup across tenants.
func TestResolveUserByEmail_UnknownClientIDIsRefused(t *testing.T) {
	solo := &user.User{ID: uuid.New(), TenantID: uuid.New(), Email: "solo@example.com"}
	h := &EmailHandler{userSvc: newFakeUserService(solo), clientSvc: newFakeClientService(), logger: zap.NewNop()}

	if _, err := h.resolveUserByEmail("no-such-client", "solo@example.com"); !errors.Is(err, errUnknownClient) {
		t.Fatalf("err = %v, want errUnknownClient", err)
	}
}
