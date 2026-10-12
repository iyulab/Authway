package client

import (
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestValidateAudience(t *testing.T) {
	for name, tc := range map[string]struct {
		audience []string
		ok       bool
	}{
		"none":            {nil, true},
		"a url":           {[]string{"https://api.example.com"}, true},
		"a plain name":    {[]string{"orders-api"}, true},
		"several":         {[]string{"https://api.example.com", "https://billing.example.com"}, true},
		"empty value":     {[]string{""}, false},
		"with whitespace": {[]string{"orders api"}, false},
		"repeated":        {[]string{"orders-api", "orders-api"}, false},
		"too long":        {[]string{strings.Repeat("a", maxAudienceLength+1)}, false},
	} {
		cerr := validateAudience(tc.audience)
		if (cerr == nil) != tc.ok {
			t.Errorf("%s: validateAudience(%v) = %v, want ok=%v", name, tc.audience, cerr, tc.ok)
		}
		if cerr != nil && (cerr.Code != "invalid_audience" || cerr.Field != "audience") {
			t.Errorf("%s: refusal = %+v", name, cerr)
		}
	}
}

// Creating, updating and regenerating the secret send the authorization
// server one shape. An update replaces the whole client there, so a field one
// of them left out — the post-logout addresses, once — would be reset.
func TestHydraRegistration_CarriesEveryField(t *testing.T) {
	strategy := "jwt"
	c := &Client{
		ClientID:               "app",
		Name:                   "App",
		RedirectURIs:           pq.StringArray{"https://app.example.com/cb"},
		PostLogoutRedirectURIs: pq.StringArray{"https://app.example.com/bye"},
		Audience:               pq.StringArray{"https://api.example.com"},
		GrantTypes:             pq.StringArray{"authorization_code"},
		Scopes:                 pq.StringArray{"openid", "email"},
		SkipConsent:            true,
		AccessTokenStrategy:    &strategy,
	}

	reg := c.hydraRegistration("s3cret")
	if reg.ClientID != "app" || reg.ClientSecret != "s3cret" || reg.ClientName != "App" {
		t.Errorf("identity: %+v", reg)
	}
	if len(reg.Audience) != 1 || reg.Audience[0] != "https://api.example.com" {
		t.Errorf("audience = %v", reg.Audience)
	}
	if len(reg.PostLogoutRedirectUris) != 1 || reg.PostLogoutRedirectUris[0] != "https://app.example.com/bye" {
		t.Errorf("post-logout addresses = %v", reg.PostLogoutRedirectUris)
	}
	if reg.Scope != "openid email" || reg.TokenEndpointAuthMethod != "client_secret_post" || !reg.SkipConsent || reg.AccessTokenStrategy != "jwt" {
		t.Errorf("settings: %+v", reg)
	}

	// No audience is an empty list, not an absent field: it has to clear one
	// the authorization server holds.
	c.Audience = nil
	c.PostLogoutRedirectURIs = nil
	c.Public = true
	reg = c.hydraRegistration("")
	if reg.Audience == nil || len(reg.Audience) != 0 {
		t.Errorf("no audience = %#v, want an empty list", reg.Audience)
	}
	if len(reg.PostLogoutRedirectUris) != 1 || reg.PostLogoutRedirectUris[0] != "https://app.example.com/cb" {
		t.Errorf("post-logout fallback = %v, want the redirect addresses", reg.PostLogoutRedirectUris)
	}
	if reg.TokenEndpointAuthMethod != "none" {
		t.Errorf("a public client authenticates with %q", reg.TokenEndpointAuthMethod)
	}
}
