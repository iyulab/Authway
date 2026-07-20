package client

import "testing"

func TestValidateClientConfig(t *testing.T) {
	tests := []struct {
		name           string
		public         bool
		clientSecret   string
		grantTypes     []string
		redirectURIs   []string
		allowedOrigins []string
		wantCode       string // "" means expect nil
	}{
		// --- Public client violations ----
		{
			name:         "public client with secret rejected",
			public:       true,
			clientSecret: "leaked-secret",
			grantTypes:   []string{"authorization_code"},
			wantCode:     "public_client_with_secret",
		},
		{
			name:       "public client cannot use client_credentials",
			public:     true,
			grantTypes: []string{"authorization_code", "client_credentials"},
			wantCode:   "public_client_with_client_credentials",
		},
		{
			name:       "public client cannot use password grant",
			public:     true,
			grantTypes: []string{"password"},
			wantCode:   "public_client_with_password_grant",
		},
		{
			name:           "public client with auth code requires allowed_origins",
			public:         true,
			grantTypes:     []string{"authorization_code"},
			redirectURIs:   []string{"https://app.example/callback"},
			allowedOrigins: nil,
			wantCode:       "public_client_missing_allowed_origins",
		},
		{
			name:           "public client with auth code AND allowed_origins is OK",
			public:         true,
			grantTypes:     []string{"authorization_code"},
			redirectURIs:   []string{"https://app.example/callback"},
			allowedOrigins: []string{"https://app.example"},
			wantCode:       "",
		},

		// --- Confidential client (default ASP.NET-style) ----
		{
			name:         "confidential client with auth code (typical ASP.NET)",
			public:       false,
			clientSecret: "real-secret",
			grantTypes:   []string{"authorization_code", "refresh_token"},
			redirectURIs: []string{"https://app.example/signin-oidc"},
			wantCode:     "",
		},
		{
			name:         "confidential M2M client (client_credentials)",
			public:       false,
			clientSecret: "real-secret",
			grantTypes:   []string{"client_credentials"},
			wantCode:     "",
		},
		{
			name:         "confidential client with bogus grant rejected",
			public:       false,
			clientSecret: "real-secret",
			grantTypes:   []string{"unknown_grant"},
			wantCode:     "confidential_client_unsupported_grants",
		},
		{
			name:       "confidential client with no grants is allowed (defaults applied later)",
			public:     false,
			grantTypes: nil,
			wantCode:   "",
		},

		// --- redirect_uris conditional on grant type ----
		// Regression: redirect_uris used to be unconditionally required, forcing
		// M2M clients to register a dummy URI that then polluted the logout config.
		{
			name:         "M2M client without redirect_uris is OK",
			public:       false,
			clientSecret: "real-secret",
			grantTypes:   []string{"client_credentials"},
			redirectURIs: nil,
			wantCode:     "",
		},
		{
			name:         "confidential auth_code without redirect_uris rejected",
			public:       false,
			clientSecret: "real-secret",
			grantTypes:   []string{"authorization_code", "refresh_token"},
			redirectURIs: nil,
			wantCode:     "redirect_grant_without_redirect_uris",
		},
		{
			name:           "public auth_code without redirect_uris rejected",
			public:         true,
			grantTypes:     []string{"authorization_code"},
			redirectURIs:   nil,
			allowedOrigins: []string{"https://app.example"},
			wantCode:       "redirect_grant_without_redirect_uris",
		},
		{
			name:         "implicit without redirect_uris rejected",
			public:       false,
			clientSecret: "real-secret",
			grantTypes:   []string{"authorization_code", "implicit"},
			redirectURIs: nil,
			wantCode:     "redirect_grant_without_redirect_uris",
		},
		{
			name:         "refresh_token-only client needs no redirect_uris",
			public:       false,
			clientSecret: "real-secret",
			grantTypes:   []string{"refresh_token"},
			redirectURIs: nil,
			wantCode:     "",
		},
		{
			name:         "client-type violations outrank the missing redirect_uri",
			public:       true,
			clientSecret: "leaked-secret",
			grantTypes:   []string{"authorization_code"},
			redirectURIs: nil,
			wantCode:     "public_client_with_secret",
		},

		// --- Case insensitivity / whitespace tolerance ----
		{
			name:       "grant_types are case-insensitive",
			public:     true,
			grantTypes: []string{"  Client_Credentials "},
			wantCode:   "public_client_with_client_credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateClientConfig(tt.public, tt.clientSecret, tt.grantTypes, tt.redirectURIs, tt.allowedOrigins)
			if tt.wantCode == "" {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want code=%s, got nil", tt.wantCode)
			}
			if got.Code != tt.wantCode {
				t.Fatalf("want code=%s, got %s (msg=%s)", tt.wantCode, got.Code, got.Message)
			}
			if got.Hint == "" {
				t.Errorf("ConfigError missing Hint — every violation should be actionable")
			}
			if got.Field == "" {
				t.Errorf("ConfigError missing Field")
			}
		})
	}
}

// M2M clients now legitimately carry no redirect_uris; the Hydra payload must
// still say `[]`, never `null`.
func TestNonNilURIs(t *testing.T) {
	if got := nonNilURIs(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil must become an empty non-nil slice, got %#v", got)
	}
	in := []string{"https://app.example/cb"}
	if got := nonNilURIs(in); len(got) != 1 || got[0] != in[0] {
		t.Fatalf("non-nil input must pass through unchanged, got %#v", got)
	}
}

// A client that is not pinned must send no access_token_strategy at all, so the
// omitempty tag drops it and Hydra falls back to the deployment-wide setting.
// Client update is a PUT full-replace, so this is also how a pin gets cleared.
func TestDerefStrategy(t *testing.T) {
	if got := derefStrategy(nil); got != "" {
		t.Fatalf("unpinned client must send an empty strategy, got %q", got)
	}
	for _, s := range []string{"jwt", "opaque"} {
		v := s
		if got := derefStrategy(&v); got != s {
			t.Fatalf("pinned client must send %q, got %q", s, got)
		}
	}
}

func TestConfigError_Error(t *testing.T) {
	e := &ConfigError{Code: "x", Field: "y", Message: "z", Hint: "h"}
	if e.Error() == "" {
		t.Fatal("Error() returned empty string")
	}
}

func TestSyncStatus_OK(t *testing.T) {
	cases := []struct {
		state string
		want  bool
	}{
		{SyncStateOK, true},
		{SyncStateSkipped, true},
		{SyncStateFailed, false},
		{"", false},        // zero-value SyncStatus must NOT report OK — we
		{"unknown", false}, // don't want callers to mistake an uninitialized
		// status for "no problem".
	}
	for _, c := range cases {
		s := SyncStatus{State: c.state}
		if got := s.OK(); got != c.want {
			t.Errorf("SyncStatus{State:%q}.OK() = %v, want %v", c.state, got, c.want)
		}
	}
}
