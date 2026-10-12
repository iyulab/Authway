package client

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestValidateBackChannelLogoutURI(t *testing.T) {
	for _, uri := range []string{
		"",
		"https://app.example.com/backchannel-logout",
		"http://localhost:5000/logout/backchannel",
		"https://app.example.com:8443/hooks/logout?tenant=a",
	} {
		if cerr := validateBackChannelLogoutURI(uri); cerr != nil {
			t.Errorf("%q: %v, want it accepted", uri, cerr)
		}
	}
	for _, uri := range []string{
		"app.example.com/logout",
		"/backchannel-logout",
		"myapp://logout",
		"https:///logout",
		"https://app.example.com/logout#fragment",
		"https://app.example.com/" + strings.Repeat("a", maxBackChannelLogoutURILength),
	} {
		cerr := validateBackChannelLogoutURI(uri)
		if cerr == nil || cerr.Code != "invalid_backchannel_logout_uri" {
			t.Errorf("%.60q: %v, want invalid_backchannel_logout_uri", uri, cerr)
		}
	}
}

// The authorization server is told where to send the logout token, and to
// name the session in it; a client without an address is registered without
// either, which on an update clears what was there.
func TestHydraRegistration_CarriesTheBackChannelLogoutAddress(t *testing.T) {
	c := &Client{ClientID: "app", Name: "app", BackChannelLogoutURI: "https://app.example.com/backchannel-logout"}
	reg := c.hydraRegistration("")
	if reg.BackChannelLogoutURI != c.BackChannelLogoutURI || !reg.BackChannelLogoutSessionRequired {
		t.Errorf("registered %q session_required=%v, want the address with the session named", reg.BackChannelLogoutURI, reg.BackChannelLogoutSessionRequired)
	}

	reg = (&Client{ClientID: "app", Name: "app"}).hydraRegistration("")
	if reg.BackChannelLogoutURI != "" || reg.BackChannelLogoutSessionRequired {
		t.Errorf("registered %q session_required=%v for a client without an address", reg.BackChannelLogoutURI, reg.BackChannelLogoutSessionRequired)
	}
}

// The address is stored, returned, changed and cleared through the service.
func TestBackChannelLogoutURI_IsStoredChangedAndCleared(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	tenantID := seedTenant(t, db)

	created, _, err := svc.Create(&CreateClientRequest{
		TenantID:             tenantID,
		Name:                 "bcl-" + uuid.NewString()[:8],
		RedirectURIs:         []string{"https://app.example.com/callback"},
		GrantTypes:           []string{"authorization_code"},
		Scopes:               []string{"openid"},
		BackChannelLogoutURI: "https://app.example.com/backchannel-logout",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM clients WHERE id = ?`, created.ID) })

	stored, err := svc.GetByID(created.ID)
	if err != nil || stored.BackChannelLogoutURI != "https://app.example.com/backchannel-logout" {
		t.Fatalf("stored %q, %v; want the address", stored.BackChannelLogoutURI, err)
	}
	if got := stored.ToPublic().BackChannelLogoutURI; got != stored.BackChannelLogoutURI {
		t.Errorf("the client's public form carries %q, want the address", got)
	}

	// An update that does not mention the address leaves it alone.
	renamed := "bcl-renamed-" + uuid.NewString()[:8]
	if _, _, err := svc.Update(created.ID, &UpdateClientRequest{Name: renamed}); err != nil {
		t.Fatalf("Update (name only): %v", err)
	}
	if stored, _ = svc.GetByID(created.ID); stored.BackChannelLogoutURI == "" {
		t.Error("an update that did not mention the address cleared it")
	}

	bad := "not a url"
	if _, _, err := svc.Update(created.ID, &UpdateClientRequest{BackChannelLogoutURI: &bad}); err == nil {
		t.Error("an invalid address was accepted on update")
	}

	empty := ""
	if _, _, err := svc.Update(created.ID, &UpdateClientRequest{BackChannelLogoutURI: &empty}); err != nil {
		t.Fatalf("Update (clear): %v", err)
	}
	if stored, _ = svc.GetByID(created.ID); stored.BackChannelLogoutURI != "" {
		t.Errorf("after clearing, stored %q", stored.BackChannelLogoutURI)
	}

	if _, _, err := svc.Create(&CreateClientRequest{
		TenantID: tenantID, Name: "bcl-bad-" + uuid.NewString()[:8],
		RedirectURIs: []string{"https://app.example.com/callback"}, GrantTypes: []string{"authorization_code"}, Scopes: []string{"openid"},
		BackChannelLogoutURI: "myapp://logout",
	}); err == nil {
		t.Error("an invalid address was accepted on create")
	}
}
