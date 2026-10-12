package config

import (
	"strings"
	"testing"
)

// Whatever answers at the provider's endpoints decides who signs in, so
// pointing them anywhere but the provider is for tests only: a deployment
// that is not a development one refuses to start that way.

func TestValidateRefusesProviderEndpointOverridesOutsideDevelopment(t *testing.T) {
	for _, env := range []string{"production", "staging", "prod"} {
		c := validConfig()
		c.App.Environment = env
		c.Google.TokenURL = "http://localhost:9912/token"

		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "AUTHWAY_GOOGLE_") {
			t.Errorf("%s: got %v, want the endpoint override refused by name", env, err)
		}
	}
}

func TestValidateAcceptsProviderEndpointOverridesInDevelopment(t *testing.T) {
	c := validConfig()
	c.App.Environment = "development"
	c.Google.AuthURL = "http://localhost:9912/authorize"
	c.Google.TokenURL = "http://localhost:9912/token"
	c.Google.UserInfoURL = "http://localhost:9912/userinfo"
	if err := c.Validate(); err != nil {
		t.Fatalf("a development configuration with a stand-in provider should validate, got: %v", err)
	}
}
