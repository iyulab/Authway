package client

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/pkg/apierror"
)

func ptr(s string) *string { return &s }

// The four providers follow one rule: enabled means the client's own app, and
// an enabled provider has every credential. Google and GitHub used to refuse
// an incomplete one only when a client was created, Microsoft and Apple never,
// and a Google or GitHub credential cleared to "" still counted as set.
func TestValidateProviderCredentials(t *testing.T) {
	for _, c := range []struct {
		name  string
		c     Client
		field string
	}{
		{"none enabled, nothing stored", Client{}, ""},
		{"disabled, partly stored", Client{GoogleClientID: ptr("app")}, ""},
		{"google complete", Client{GoogleOAuthEnabled: true, GoogleClientID: ptr("app"), GoogleClientSecret: ptr("s")}, ""},
		{"google without secret", Client{GoogleOAuthEnabled: true, GoogleClientID: ptr("app")}, "google_client_secret"},
		{"google secret cleared", Client{GoogleOAuthEnabled: true, GoogleClientID: ptr("app"), GoogleClientSecret: ptr("")}, "google_client_secret"},
		{"github without id", Client{GithubOAuthEnabled: true, GithubClientSecret: ptr("s")}, "github_client_id"},
		{"microsoft enabled alone", Client{MicrosoftOAuthEnabled: true}, "microsoft_client_id"},
		{"microsoft without directory", Client{MicrosoftOAuthEnabled: true, MicrosoftClientID: ptr("app"), MicrosoftClientSecret: ptr("s")}, ""},
		{"apple without key", Client{AppleOAuthEnabled: true, AppleClientID: ptr("app"), AppleTeamID: ptr("T"), AppleKeyID: ptr("K")}, "apple_private_key"},
	} {
		cerr := validateProviderCredentials(&c.c)
		got := ""
		if cerr != nil {
			if cerr.Code != "incomplete_provider_credentials" {
				t.Errorf("%s: code = %q", c.name, cerr.Code)
			}
			got = cerr.Field
		}
		if got != c.field {
			t.Errorf("%s: refused field = %q, want %q", c.name, got, c.field)
		}
		// What is accepted and what sign-in uses are the same rule.
		if cerr == nil && c.c.GoogleOAuthEnabled != c.c.UsesOwnGoogle() {
			t.Errorf("%s: accepted, but UsesOwnGoogle = %v", c.name, c.c.UsesOwnGoogle())
		}
	}
}

func TestProviderCredentials_CreateAndUpdate(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	tenantID := seedTenant(t, db)
	base := func() *CreateClientRequest {
		return &CreateClientRequest{
			TenantID:     tenantID,
			Name:         "providers-" + uuid.NewString()[:8],
			RedirectURIs: []string{"https://app.example.com/callback"},
			GrantTypes:   []string{"authorization_code"},
			Scopes:       []string{"openid"},
		}
	}
	refusedOn := func(t *testing.T, err error, field string) {
		t.Helper()
		var r *apierror.Refusal
		if !errors.As(err, &r) || r.Code != "incomplete_provider_credentials" || r.Field != field {
			t.Fatalf("err = %v, want incomplete_provider_credentials on %s", err, field)
		}
	}

	// Creating: an enabled provider without its credentials is refused, as
	// it is for the other three.
	req := base()
	req.MicrosoftOAuthEnabled = true
	req.MicrosoftClientID = "app"
	_, _, err := svc.Create(req)
	refusedOn(t, err, "microsoft_client_secret")

	// Creating: credentials for a provider that is not enabled are kept, as
	// a change keeps them, ready for when it is.
	req = base()
	req.GoogleClientID = "app"
	req.GoogleClientSecret = "secret"
	created, _, err := svc.Create(req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM clients WHERE id = ?`, created.ID) })
	if !set(created.GoogleClientID) || created.UsesOwnGoogle() {
		t.Fatalf("stored id = %v, uses own = %v; want the credentials kept and the deployment's app used", created.GoogleClientID, created.UsesOwnGoogle())
	}

	// Changing: enabling it now uses what was kept.
	on := true
	updated, _, err := svc.Update(created.ID, &UpdateClientRequest{GoogleOAuthEnabled: &on})
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !updated.UsesOwnGoogle() {
		t.Fatal("enabled with complete credentials, but sign-in would not use them")
	}

	// Changing: clearing a credential of an enabled provider is refused and
	// nothing is stored.
	_, _, err = svc.Update(created.ID, &UpdateClientRequest{GoogleClientSecret: ptr("")})
	refusedOn(t, err, "google_client_secret")
	var secretSet bool
	if err := db.Raw(`SELECT coalesce(google_client_secret, '') <> '' FROM clients WHERE id = ?`, created.ID).Scan(&secretSet).Error; err != nil {
		t.Fatal(err)
	}
	if !secretSet {
		t.Error("a refused change cleared the stored secret")
	}

	// Changing: enabling Apple with three of its four values is refused.
	_, _, err = svc.Update(created.ID, &UpdateClientRequest{AppleOAuthEnabled: &on, AppleClientID: ptr("app"), AppleTeamID: ptr("T"), AppleKeyID: ptr("K")})
	refusedOn(t, err, "apple_private_key")
}
