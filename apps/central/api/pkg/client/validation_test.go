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
