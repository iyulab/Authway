package handler

import (
	"testing"

	"go.uber.org/zap"
)

func newTestLogoutHandler() *LogoutHandler {
	return &LogoutHandler{logger: zap.NewNop()}
}

func TestValidateLogoutRedirect_Strict(t *testing.T) {
	h := newTestLogoutHandler()
	config := &ClientConfig{
		LogoutRedirectPolicy:   "strict",
		PostLogoutRedirectURIs: []string{"https://app.example.com", "https://app.example.com/callback"},
	}

	tests := []struct {
		name    string
		uri     string
		want    string
		wantErr bool
	}{
		{"exact match allowed", "https://app.example.com", "https://app.example.com", false},
		{"whitelisted path allowed", "https://app.example.com/callback", "https://app.example.com/callback", false},
		{"empty URI rejected", "", "", true},
		{"non-whitelisted URI rejected", "https://evil.com", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := h.validateLogoutRedirect(config, tt.uri)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateLogoutRedirect() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("validateLogoutRedirect() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateLogoutRedirect_Lenient(t *testing.T) {
	h := newTestLogoutHandler()
	defaultURI := "https://app.example.com/logged-out"
	config := &ClientConfig{
		LogoutRedirectPolicy:   "lenient",
		PostLogoutRedirectURIs: []string{"https://app.example.com"},
		DefaultLogoutURI:       &defaultURI,
	}

	tests := []struct {
		name    string
		uri     string
		want    string
		wantErr bool
	}{
		{"whitelisted URI allowed", "https://app.example.com", "https://app.example.com", false},
		{"empty URI falls back to default", "", defaultURI, false},
		{"non-whitelisted URI falls back to default", "https://evil.com", defaultURI, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := h.validateLogoutRedirect(config, tt.uri)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateLogoutRedirect() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("validateLogoutRedirect() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateLogoutRedirect_Disabled(t *testing.T) {
	h := newTestLogoutHandler()
	config := &ClientConfig{LogoutRedirectPolicy: "disabled"}

	got, err := h.validateLogoutRedirect(config, "https://anywhere.com")
	if err != nil {
		t.Fatalf("validateLogoutRedirect() unexpected error: %v", err)
	}
	if got != "https://anywhere.com" {
		t.Errorf("validateLogoutRedirect() = %q, want unchanged pass-through", got)
	}
}

func TestValidateLogoutRedirect_UnknownPolicy(t *testing.T) {
	h := newTestLogoutHandler()
	config := &ClientConfig{LogoutRedirectPolicy: "made-up"}

	if _, err := h.validateLogoutRedirect(config, "https://app.example.com"); err == nil {
		t.Error("validateLogoutRedirect() expected error for unknown policy, got nil")
	}
}

// TestMatchesWildcard_HostBoundary is a regression test for the suffix-comparison
// bypass: matching used to run against the raw URI string, so a query string or
// path ending in the wildcard suffix satisfied it regardless of actual host.
func TestMatchesWildcard_HostBoundary(t *testing.T) {
	h := newTestLogoutHandler()

	tests := []struct {
		name    string
		uri     string
		pattern string
		want    bool
	}{
		{"exact domain matches", "https://example.com", "*.example.com", true},
		{"exact domain with path matches", "https://example.com/callback", "*.example.com", true},
		{"subdomain matches", "https://app.example.com", "*.example.com", true},
		{"unrelated domain rejected", "https://evil.com", "*.example.com", false},
		{"lookalike suffix without dot boundary rejected", "https://notexample.com", "*.example.com", false},
		{"query-string bypass rejected", "https://evil.com/?x=y.example.com", "*.example.com", false},
		{"path bypass rejected", "https://evil.com/foo.example.com", "*.example.com", false},
		{"fragment bypass rejected", "https://evil.com/#foo.example.com", "*.example.com", false},
		{"localhost any port matches", "http://localhost:3000", "http://localhost:*", true},
		{"localhost wrong scheme rejected", "https://localhost:3000", "http://localhost:*", false},
		{"localhost lookalike host rejected", "http://localhost.evil.com:3000", "http://localhost:*", false},
		{"invalid uri rejected", "://not a url", "*.example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.matchesWildcard(tt.uri, tt.pattern); got != tt.want {
				t.Errorf("matchesWildcard(%q, %q) = %v, want %v", tt.uri, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestIsWhitelisted(t *testing.T) {
	h := newTestLogoutHandler()
	whitelist := []string{"https://app.example.com", "*.staging.example.com"}

	tests := []struct {
		name          string
		uri           string
		allowWildcard bool
		want          bool
	}{
		{"exact match without wildcard flag", "https://app.example.com", false, true},
		{"wildcard pattern ignored when flag off", "https://foo.staging.example.com", false, false},
		{"wildcard pattern honored when flag on", "https://foo.staging.example.com", true, true},
		{"non-whitelisted uri rejected", "https://evil.com", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.isWhitelisted(tt.uri, whitelist, tt.allowWildcard); got != tt.want {
				t.Errorf("isWhitelisted(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

func TestDetermineFallbackURI(t *testing.T) {
	h := newTestLogoutHandler()
	defaultURI := "https://app.example.com/logged-out"

	tests := []struct {
		name   string
		config *ClientConfig
		req    string
		want   string
	}{
		{
			name:   "default_logout_uri takes priority",
			config: &ClientConfig{DefaultLogoutURI: &defaultURI, PostLogoutRedirectURIs: []string{"https://other.example.com"}, Website: "https://example.com"},
			want:   defaultURI,
		},
		{
			name:   "falls back to first whitelist entry",
			config: &ClientConfig{PostLogoutRedirectURIs: []string{"https://app.example.com", "https://app2.example.com"}, Website: "https://example.com"},
			want:   "https://app.example.com",
		},
		{
			name:   "falls back to website",
			config: &ClientConfig{Website: "https://example.com"},
			want:   "https://example.com",
		},
		{
			name:   "falls back to requested URI origin",
			config: &ClientConfig{},
			req:    "https://requested.example.com/callback?x=1",
			want:   "https://requested.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.determineFallbackURI(tt.config, tt.req); got != tt.want {
				t.Errorf("determineFallbackURI() = %q, want %q", got, tt.want)
			}
		})
	}
}
