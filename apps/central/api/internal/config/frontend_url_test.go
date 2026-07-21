package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// Every emailed link — invitation accept, magic link, verify email, reset
// password — was built from app.base_url, which is this API's own address. The
// result 404'd in every environment, and nothing caught it: the config loads,
// the app boots, the mail sends. Only the recipient ever saw the failure.
//
// So the guard lives in Validate, and these tests pin it.

func validConfig() *Config {
	c := &Config{}
	c.Database.Host = "localhost"
	c.Database.User = "authway"
	c.Database.Name = "authway"
	c.Admin.Password = "not-empty"
	c.App.Environment = "development"
	c.App.BaseURL = "http://localhost:8080"
	c.App.FrontendURL = "http://localhost:3001"
	return c
}

func TestValidateAcceptsDistinctFrontendURL(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("baseline config should validate, got: %v", err)
	}
}

func TestValidateRejectsFrontendURLEqualToBaseURL(t *testing.T) {
	// This is the actual production wiring that shipped: the deploy script set
	// AUTHWAY_APP_BASE_URL=$API_URL and nothing set a frontend URL at all.
	c := validConfig()
	c.App.FrontendURL = c.App.BaseURL

	err := c.Validate()
	if err == nil {
		t.Fatal("frontend_url equal to base_url must be rejected — emailed links would 404")
	}
	if !strings.Contains(err.Error(), "frontend_url") {
		t.Errorf("error should name the offending key, got: %v", err)
	}
}

func TestValidateRejectsMissingFrontendURL(t *testing.T) {
	c := validConfig()
	c.App.FrontendURL = ""

	err := c.Validate()
	if err == nil {
		t.Fatal("an absent frontend_url must fail closed, not fall back silently")
	}
	if !strings.Contains(err.Error(), "AUTHWAY_APP_FRONTEND_URL") {
		t.Errorf("error should name the env var an operator has to set, got: %v", err)
	}
}

func TestValidateRejectsLoopbackFrontendURLInProduction(t *testing.T) {
	// The required-check alone does not cover this. Viper's AutomaticEnv treats
	// an empty environment variable as unset (allowEmptyEnv defaults to false),
	// so a deploy that emits `AUTHWAY_APP_FRONTEND_URL=` silently keeps the
	// localhost default — non-empty, different from base_url, and useless.
	for _, u := range []string{
		"http://localhost:3001",
		"http://127.0.0.1:3001",
		"http://0.0.0.0:3001",
	} {
		c := validConfig()
		c.App.Environment = "production"
		c.App.FrontendURL = u
		// Satisfy the unrelated production fail-closed checks.
		c.Admin.APIKey = "k"
		c.Admin.InternalAPIKey = "k"
		c.Security.TOTPEncryptionKey = "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="

		err := c.Validate()
		if err == nil {
			t.Errorf("%s: a loopback frontend_url must be rejected in production", u)
			continue
		}
		if !strings.Contains(err.Error(), "frontend_url") {
			t.Errorf("%s: error should name frontend_url, got: %v", u, err)
		}
	}
}

func TestValidateAllowsLoopbackFrontendURLOutsideProduction(t *testing.T) {
	// Local development is the whole reason the default is localhost.
	c := validConfig() // environment: development
	if err := c.Validate(); err != nil {
		t.Fatalf("localhost must stay valid in development, got: %v", err)
	}
}

func TestViperFallsBackToDefaultOnEmptyEnv(t *testing.T) {
	// Pins the viper behaviour the production guard exists to compensate for.
	// If a future viper upgrade starts honouring empty values, the guard becomes
	// belt-and-braces rather than load-bearing — worth knowing either way.
	t.Setenv("AUTHWAY_APP_FRONTEND_URL", "")
	setDefaults()

	if got := viper.GetString("app.frontend_url"); got != "http://localhost:3001" {
		t.Logf("viper no longer falls back on empty env (got %q) — the production loopback guard may now be redundant", got)
	}
}

func TestDefaultFrontendURLDiffersFromBaseURL(t *testing.T) {
	// The defaults have to satisfy the guard on their own, or a bare `go run`
	// fails to boot and the next person just deletes the check.
	setDefaults()

	base := viper.GetString("app.base_url")
	front := viper.GetString("app.frontend_url")

	if front == "" {
		t.Fatal("app.frontend_url needs a default")
	}
	if front == base {
		t.Errorf("default frontend_url (%q) must differ from base_url (%q)", front, base)
	}
}
