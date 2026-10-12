package client

import (
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// An origin belongs to the deployment while an active client lists it, and
// stops belonging when that client is deactivated or deleted.
func TestAllowsOrigin_FollowsTheActiveClientsThatListIt(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	tenantID := seedTenant(t, db)

	suffix := uuid.NewString()[:8]
	origin := "https://app-" + suffix + ".example.org"

	if allowed, err := svc.AllowsOrigin(origin); err != nil || allowed {
		t.Fatalf("before any client lists it: %v, %v; want not allowed", allowed, err)
	}

	created, _, err := svc.Create(&CreateClientRequest{
		TenantID:       tenantID,
		Name:           "origin-test-" + suffix,
		Public:         true,
		RedirectURIs:   []string{origin + "/callback"},
		AllowedOrigins: []string{origin},
		GrantTypes:     []string{"authorization_code"},
		Scopes:         []string{"openid"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM clients WHERE id = ?`, created.ID) })

	if allowed, err := svc.AllowsOrigin(origin); err != nil || !allowed {
		t.Fatalf("listed on an active client: %v, %v; want allowed", allowed, err)
	}
	for _, other := range []string{origin + ".evil.example", "https://app-" + suffix + ".example.org:8443", ""} {
		if allowed, err := svc.AllowsOrigin(other); err != nil || allowed {
			t.Errorf("%q: %v, %v; want not allowed (an origin matches exactly)", other, allowed, err)
		}
	}

	if err := db.Model(&Client{}).Where("id = ?", created.ID).Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, err := svc.AllowsOrigin(origin); err != nil || allowed {
		t.Errorf("listed only on a deactivated client: %v, %v; want not allowed", allowed, err)
	}

	if err := db.Model(&Client{}).Where("id = ?", created.ID).Update("active", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&Client{}, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, err := svc.AllowsOrigin(origin); err != nil || allowed {
		t.Errorf("listed only on a deleted client: %v, %v; want not allowed", allowed, err)
	}
}
