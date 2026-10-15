package social

import (
	"errors"
	"testing"

	"go.uber.org/zap"

	"authway/apps/central/api/internal/config"
	"authway/apps/central/api/pkg/client"
)

// oneClient answers GetByClientID for a single client; every other method of
// client.Service is left to the embedded nil interface (a call panics).
type oneClient struct {
	client.Service
	c *client.Client
}

func (o oneClient) GetByClientID(id string) (*client.Client, error) {
	if o.c == nil || o.c.ClientID != id {
		return nil, errors.New("not found")
	}
	return o.c, nil
}

func str(s string) *string { return &s }

// A client's own Microsoft and Apple credentials were accepted, sealed and
// stored, and sign-in used the deployment's anyway. They are now used the
// way a client's own Google and GitHub credentials are.
func TestMicrosoftOAuthConfig_UsesTheClientsOwnCredentials(t *testing.T) {
	central := &config.MicrosoftOAuthConfig{ClientID: "central", ClientSecret: "central-secret", TenantID: "central-dir", RedirectURL: "https://id.example/cb"}
	own := &client.Client{ClientID: "app", MicrosoftOAuthEnabled: true, MicrosoftClientID: str("own"), MicrosoftClientSecret: str("own-secret")}
	svc := NewMicrosoftService(central, nil, nil, oneClient{c: own}, zap.NewNop())

	cfg, err := svc.GetOAuthConfig("app")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "own" || cfg.ClientSecret != "own-secret" || cfg.RedirectURL != "https://id.example/cb" {
		t.Errorf("config = %+v, want the client's own credentials with the deployment's callback", cfg)
	}
	if cfg.TenantID != "" {
		t.Errorf("TenantID = %q, want empty (common) — a client's app does not inherit the deployment's directory", cfg.TenantID)
	}

	own.MicrosoftTenantID = str("app-dir")
	if cfg, _ := svc.GetOAuthConfig("app"); cfg.TenantID != "app-dir" {
		t.Errorf("TenantID = %q, want the client's directory", cfg.TenantID)
	}

	own.MicrosoftOAuthEnabled = false
	if cfg, _ := svc.GetOAuthConfig("app"); cfg.ClientID != "central" {
		t.Errorf("disabled: ClientID = %q, want the deployment's", cfg.ClientID)
	}

	if _, err := svc.GetOAuthConfig("unknown"); err == nil {
		t.Error("an unknown client must not be configured")
	}
	if svc.ConfiguredFor("unknown") {
		t.Error("ConfiguredFor(unknown) = true")
	}
}

func TestAppleOAuthConfig_UsesTheClientsOwnCredentialsOnlyWhenComplete(t *testing.T) {
	central := &config.AppleOAuthConfig{ClientID: "central", TeamID: "T", KeyID: "K", PrivateKey: "P", RedirectURL: "https://id.example/cb"}
	own := &client.Client{ClientID: "app", AppleOAuthEnabled: true, AppleClientID: str("own"), AppleTeamID: str("OT"), AppleKeyID: str("OK"), ApplePrivateKey: str("OP")}
	svc := NewAppleService(central, nil, nil, oneClient{c: own}, zap.NewNop())

	cfg, err := svc.GetOAuthConfig("app")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "own" || cfg.TeamID != "OT" || cfg.KeyID != "OK" || cfg.PrivateKey != "OP" || cfg.RedirectURL != "https://id.example/cb" {
		t.Errorf("config = %+v, want the client's own credentials with the deployment's callback", cfg)
	}

	own.ApplePrivateKey = nil
	if cfg, _ := svc.GetOAuthConfig("app"); cfg.ClientID != "central" {
		t.Errorf("incomplete: ClientID = %q, want the deployment's (a half-set key cannot sign)", cfg.ClientID)
	}

	// An unknown client falls back to the deployment's URL rather than
	// failing with a nil configuration.
	if u := svc.GetAuthURLForClient("s", "unknown"); u == "" {
		t.Error("GetAuthURLForClient(unknown) returned nothing")
	}
}
