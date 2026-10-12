package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/outbound"
)

// namesFor answers lookups from a table, so the tests say what each name
// resolves to instead of asking the network.
type namesFor map[string]string

func (n namesFor) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	addr, ok := n[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	return []net.IPAddr{{IP: net.ParseIP(addr)}}, nil
}

var testNames = namesFor{
	"app.example.com":      "93.184.216.34",
	"intranet.example.com": "10.0.0.5",
	"metadata.example.com": "169.254.169.254",
}

func serviceCalling(policy outbound.Policy) *service {
	return &service{outbound: policy, names: testNames}
}

func TestValidateBackChannelLogoutURI(t *testing.T) {
	s := serviceCalling(outbound.Policy{})
	for _, uri := range []string{
		"",
		"https://app.example.com/backchannel-logout",
		"https://app.example.com:8443/hooks/logout?tenant=a",
		"https://not-registered-yet.example.com/logout", // does not resolve: nothing can be said
	} {
		if cerr := s.validateBackChannelLogoutURI(uri); cerr != nil {
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
		cerr := s.validateBackChannelLogoutURI(uri)
		if cerr == nil || cerr.Code != "invalid_backchannel_logout_uri" {
			t.Errorf("%.60q: %v, want invalid_backchannel_logout_uri", uri, cerr)
		}
	}
}

// The authorization server calls the address from inside the deployment's
// network. An address in there — written as such, or behind a name — would
// let whoever registers a client have it reach what they cannot.
func TestValidateBackChannelLogoutURI_RefusesWhatTheDeploymentMustNotCall(t *testing.T) {
	strict, lax := serviceCalling(outbound.Policy{}), serviceCalling(outbound.Policy{AllowPrivate: true})

	inward := []string{
		"https://localhost/logout",
		"https://127.0.0.1:8443/logout",
		"https://10.0.0.5/logout",
		"https://[::1]/logout",
		"https://intranet.example.com/logout", // a public-looking name that resolves inward
		"http://app.example.com/logout",       // the logout token in the clear
	}
	for _, uri := range inward {
		if cerr := strict.validateBackChannelLogoutURI(uri); cerr == nil || cerr.Code != "invalid_backchannel_logout_uri" {
			t.Errorf("%q: %v, want it refused", uri, cerr)
		}
		// Where receivers run next to the deployment, the same addresses are how tests reach them.
		if cerr := lax.validateBackChannelLogoutURI(uri); cerr != nil {
			t.Errorf("%q with private targets allowed: %v", uri, cerr)
		}
	}

	// No one's server — a cloud metadata endpoint above all: refused whatever the setting.
	for _, uri := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"https://169.254.169.254/",
		"https://metadata.example.com/",
		"https://0.0.0.0/logout",
	} {
		for name, s := range map[string]*service{"by default": strict, "with private targets allowed": lax} {
			if cerr := s.validateBackChannelLogoutURI(uri); cerr == nil || cerr.Code != "invalid_backchannel_logout_uri" {
				t.Errorf("%q %s: %v, want it refused", uri, name, cerr)
			}
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

// The authorization server checks the address on its own side too, and can
// refuse one this service let through — a host name that resolved differently
// for it. That is the caller's address to correct, not a server fault, and it
// leaves no client behind.
func TestCreate_AnAddressTheAuthorizationServerRefusesIsTheCallersError(t *testing.T) {
	db := setupPostgres(t)
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client_metadata","error_description":"Client IP address is not allowed: key backchannel_logout_uri validation is failing: ip 10.0.0.5 is in the 10.0.0.0/8 range"}`))
	}))
	t.Cleanup(refusing.Close)
	svc := NewService(db, zap.NewNop(), hydra.NewClient(refusing.URL))

	name := "bcl-refused-" + uuid.NewString()[:8]
	_, _, err := svc.Create(&CreateClientRequest{
		TenantID:             seedTenant(t, db),
		Name:                 name,
		RedirectURIs:         []string{"https://app.example.com/callback"},
		GrantTypes:           []string{"authorization_code"},
		Scopes:               []string{"openid"},
		BackChannelLogoutURI: "https://app.example.com/backchannel-logout",
	})
	var cerr *ConfigError
	if !errors.As(err, &cerr) || cerr.Code != "invalid_backchannel_logout_uri" {
		t.Fatalf("got %v, want invalid_backchannel_logout_uri", err)
	}
	var left int64
	if err := db.Model(&Client{}).Where("name = ?", name).Count(&left).Error; err != nil || left != 0 {
		t.Errorf("%d client(s) left behind (%v), want none", left, err)
	}
}
