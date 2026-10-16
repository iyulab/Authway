package client

import "authway/apps/central/api/pkg/apierror"

// The four sign-in providers follow one rule. A client signs its users in
// with its own provider app when it is enabled for that provider; an enabled
// provider has every credential the provider needs, and the request that
// would leave one missing is refused. A provider that is not enabled uses the
// deployment's app, and its stored credentials wait until it is.

// The credentials each provider needs, by request member. The Google
// redirect URI is not among them: without one, the deployment's callback is
// used.
var providerCredentials = []struct {
	enabled string
	fields  []string
	values  func(c *Client) (bool, []*string)
}{
	{"google_oauth_enabled", []string{"google_client_id", "google_client_secret"}, func(c *Client) (bool, []*string) {
		return c.GoogleOAuthEnabled, []*string{c.GoogleClientID, c.GoogleClientSecret}
	}},
	{"github_oauth_enabled", []string{"github_client_id", "github_client_secret"}, func(c *Client) (bool, []*string) {
		return c.GithubOAuthEnabled, []*string{c.GithubClientID, c.GithubClientSecret}
	}},
	{"microsoft_oauth_enabled", []string{"microsoft_client_id", "microsoft_client_secret"}, func(c *Client) (bool, []*string) {
		return c.MicrosoftOAuthEnabled, []*string{c.MicrosoftClientID, c.MicrosoftClientSecret}
	}},
	{"apple_oauth_enabled", []string{"apple_client_id", "apple_team_id", "apple_key_id", "apple_private_key"}, func(c *Client) (bool, []*string) {
		return c.AppleOAuthEnabled, []*string{c.AppleClientID, c.AppleTeamID, c.AppleKeyID, c.ApplePrivateKey}
	}},
}

func set(s *string) bool { return s != nil && *s != "" }

// given stores a credential named in a request; an empty one is not stored.
func given(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func complete(values []*string) bool {
	for _, v := range values {
		if !set(v) {
			return false
		}
	}
	return true
}

// UsesOwnGoogle and its three siblings report whether sign-in with the
// provider uses the client's own app rather than the deployment's.
func (c *Client) UsesOwnGoogle() bool    { return c.usesOwn(0) }
func (c *Client) UsesOwnGithub() bool    { return c.usesOwn(1) }
func (c *Client) UsesOwnMicrosoft() bool { return c.usesOwn(2) }
func (c *Client) UsesOwnApple() bool     { return c.usesOwn(3) }

func (c *Client) usesOwn(i int) bool {
	enabled, values := providerCredentials[i].values(c)
	return enabled && complete(values)
}

// validateProviderCredentials refuses a client — as it would be stored, so
// that a change is judged together with what it does not name — that is
// enabled for a provider without every credential the provider needs.
func validateProviderCredentials(c *Client) *apierror.Refusal {
	for _, p := range providerCredentials {
		enabled, values := p.values(c)
		if !enabled {
			continue
		}
		for i, v := range values {
			if !set(v) {
				return &apierror.Refusal{
					Code:    "incomplete_provider_credentials",
					Field:   p.fields[i],
					Message: p.fields[i] + " is required while " + p.enabled + " is true",
					Hint:    "Give the application's own credentials for the provider, or set " + p.enabled + " to false to use the deployment's.",
				}
			}
		}
	}
	return nil
}
