package config

import (
	"strings"
	"testing"
)

// Before this change, every fail-closed check in Validate() (and the dev-mode
// key auto-generation) gated on the literal string "production". Staging is a
// real, internet-facing deployment — not local development — but it never
// matched that literal, so it silently got the same treatment as a laptop:
// missing admin/TOTP keys would auto-generate instead of failing to boot, and
// a loopback frontend_url would pass validation. IsDevelopment/IsProduction
// replace the literal comparisons so only development gets the relaxed path.

func TestIsDevelopment(t *testing.T) {
	cases := map[string]bool{
		"":            true,
		"development": true,
		"staging":     false,
		"production":  false,
		"prod":        false, // unrecognized values fail closed, not open
		"Production":  false, // no case-insensitive matching
	}
	for env, want := range cases {
		if got := IsDevelopment(env); got != want {
			t.Errorf("IsDevelopment(%q) = %v, want %v", env, got, want)
		}
	}
}

func TestIsProduction(t *testing.T) {
	cases := map[string]bool{
		"":            false,
		"development": false,
		"staging":     true,
		"production":  true,
		"prod":        true, // unrecognized values fail closed
	}
	for env, want := range cases {
		if got := IsProduction(env); got != want {
			t.Errorf("IsProduction(%q) = %v, want %v", env, got, want)
		}
	}
}

// stagingConfig mirrors what the staging deploy script actually injects today
// (scripts/deploy/staging/.env: ADMIN_API_KEY, INTERNAL_API_KEY,
// AUTHWAY_TOTP_ENCRYPTION_KEY, AUTHWAY_ADMIN_PASSWORD, and a public
// AUTH_UI_URL/API_URL pair) so this test fails first if a future deploy drops
// one of the values staging's fail-closed check now depends on.
func stagingConfig() *Config {
	c := validConfig()
	c.App.Environment = "staging"
	c.App.BaseURL = "https://authway-api-stg.example.azurecontainerapps.io"
	c.App.FrontendURL = "https://authway-auth-ui-stg.pages.dev"
	c.Admin.Password = "a-strong-staging-password"
	c.Admin.APIKey = "staging-admin-key"
	c.Admin.InternalAPIKey = "staging-internal-key"
	c.Security.TOTPEncryptionKey = "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="
	return c
}

func TestValidateAcceptsFullyConfiguredStaging(t *testing.T) {
	if err := stagingConfig().Validate(); err != nil {
		t.Fatalf("staging config matching the current deploy script should validate, got: %v", err)
	}
}

func TestValidateRejectsStagingMissingAdminAPIKey(t *testing.T) {
	c := stagingConfig()
	c.Admin.APIKey = ""

	err := c.Validate()
	if err == nil {
		t.Fatal("staging must fail closed on a missing admin API key, not just production")
	}
	if !strings.Contains(err.Error(), "admin.api_key") {
		t.Errorf("error should name admin.api_key, got: %v", err)
	}
}

func TestValidateRejectsStagingMissingInternalAPIKey(t *testing.T) {
	c := stagingConfig()
	c.Admin.InternalAPIKey = ""

	if err := c.Validate(); err == nil {
		t.Fatal("staging must fail closed on a missing internal API key, not just production")
	}
}

func TestValidateRejectsStagingMissingTOTPKey(t *testing.T) {
	c := stagingConfig()
	c.Security.TOTPEncryptionKey = ""

	if err := c.Validate(); err == nil {
		t.Fatal("staging must fail closed on a missing TOTP encryption key, not just production")
	}
}

func TestValidateRejectsStagingWeakAdminPassword(t *testing.T) {
	c := stagingConfig()
	c.Admin.Password = "admin123"

	if err := c.Validate(); err == nil {
		t.Fatal("staging must reject the well-known weak admin password, not just production")
	}
}

func TestValidateRejectsLoopbackFrontendURLInStaging(t *testing.T) {
	c := stagingConfig()
	c.App.FrontendURL = "http://localhost:3001"

	err := c.Validate()
	if err == nil {
		t.Fatal("a loopback frontend_url must be rejected in staging, not just production")
	}
	if !strings.Contains(err.Error(), "frontend_url") {
		t.Errorf("error should name frontend_url, got: %v", err)
	}
}

func TestValidateStillAcceptsDevelopmentWithoutAdminKeys(t *testing.T) {
	// The relaxed dev path must remain untouched: it's what lets a bare `go run`
	// boot without any manual key setup.
	c := validConfig() // environment: development, no Admin.APIKey/InternalAPIKey set
	if err := c.Validate(); err != nil {
		t.Fatalf("development config without admin keys should still validate, got: %v", err)
	}
}
