package hydra

import (
	"os"
	"testing"
)

func TestValidateLogoutRedirectURI_Strict(t *testing.T) {
	config := LogoutValidationConfig{
		PostLogoutRedirectURIs: []string{
			"https://app.example.com",
			"https://app.example.com/callback",
		},
		Policy:        PolicyStrict,
		AllowWildcard: false,
	}

	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{
			name:    "exact match allowed",
			uri:     "https://app.example.com",
			wantErr: false,
		},
		{
			name:    "exact match with path allowed",
			uri:     "https://app.example.com/callback",
			wantErr: false,
		},
		{
			name:    "empty URI rejected",
			uri:     "",
			wantErr: true,
		},
		{
			name:    "non-whitelisted URI rejected",
			uri:     "https://evil.com",
			wantErr: true,
		},
		{
			name:    "similar but different URI rejected",
			uri:     "https://app.example.com/other",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogoutRedirectURI(tt.uri, config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLogoutRedirectURI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogoutRedirectURI_Lenient(t *testing.T) {
	config := LogoutValidationConfig{
		PostLogoutRedirectURIs: []string{
			"https://app.example.com",
		},
		Policy:           PolicyLenient,
		DefaultLogoutURI: "https://app.example.com",
		AllowWildcard:    false,
	}

	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{
			name:    "exact match allowed",
			uri:     "https://app.example.com",
			wantErr: false,
		},
		{
			name:    "empty URI allowed (will use default)",
			uri:     "",
			wantErr: false,
		},
		{
			name:    "non-whitelisted URI still rejected",
			uri:     "https://evil.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogoutRedirectURI(tt.uri, config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLogoutRedirectURI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogoutRedirectURI_Disabled(t *testing.T) {
	// Set non-production environment
	os.Setenv("ENVIRONMENT", "development")
	defer os.Unsetenv("ENVIRONMENT")

	config := LogoutValidationConfig{
		PostLogoutRedirectURIs: []string{},
		Policy:                 PolicyDisabled,
		AllowWildcard:          false,
	}

	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{
			name:    "any URI allowed in development",
			uri:     "https://anywhere.com",
			wantErr: false,
		},
		{
			name:    "empty URI allowed",
			uri:     "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogoutRedirectURI(tt.uri, config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLogoutRedirectURI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogoutRedirectURI_DisabledInProduction(t *testing.T) {
	// Set production environment
	os.Setenv("ENVIRONMENT", "production")
	defer os.Unsetenv("ENVIRONMENT")

	config := LogoutValidationConfig{
		PostLogoutRedirectURIs: []string{},
		Policy:                 PolicyDisabled,
		AllowWildcard:          false,
	}

	err := ValidateLogoutRedirectURI("https://anywhere.com", config)
	if err == nil {
		t.Error("Expected error in production environment, got nil")
	}
}

func TestValidateLogoutRedirectURI_Wildcard(t *testing.T) {
	config := LogoutValidationConfig{
		PostLogoutRedirectURIs: []string{
			"http://localhost:*",
			"https://*.example.com",
		},
		Policy:        PolicyStrict,
		AllowWildcard: true,
	}

	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{
			name:    "localhost with any port matches",
			uri:     "http://localhost:3000",
			wantErr: false,
		},
		{
			name:    "localhost with different port matches",
			uri:     "http://localhost:5173",
			wantErr: false,
		},
		{
			name:    "subdomain wildcard matches",
			uri:     "https://app.example.com",
			wantErr: false,
		},
		{
			name:    "different subdomain wildcard matches",
			uri:     "https://staging.example.com",
			wantErr: false,
		},
		{
			name:    "non-matching domain rejected",
			uri:     "https://evil.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogoutRedirectURI(tt.uri, config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLogoutRedirectURI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetDefaultLogoutURI(t *testing.T) {
	tests := []struct {
		name   string
		config LogoutValidationConfig
		want   string
	}{
		{
			name: "uses explicit default",
			config: LogoutValidationConfig{
				DefaultLogoutURI: "https://app.example.com/logout",
				PostLogoutRedirectURIs: []string{
					"https://app.example.com",
				},
			},
			want: "https://app.example.com/logout",
		},
		{
			name: "falls back to first whitelist entry",
			config: LogoutValidationConfig{
				DefaultLogoutURI: "",
				PostLogoutRedirectURIs: []string{
					"https://app.example.com",
					"https://staging.example.com",
				},
			},
			want: "https://app.example.com",
		},
		{
			name: "returns empty when no default available",
			config: LogoutValidationConfig{
				DefaultLogoutURI:       "",
				PostLogoutRedirectURIs: []string{},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDefaultLogoutURI(tt.config)
			if got != tt.want {
				t.Errorf("GetDefaultLogoutURI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizePolicy(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  LogoutRedirectPolicy
	}{
		{
			name:  "strict lowercase",
			input: "strict",
			want:  PolicyStrict,
		},
		{
			name:  "strict uppercase",
			input:  "STRICT",
			want:  PolicyStrict,
		},
		{
			name:  "lenient with spaces",
			input: "  lenient  ",
			want:  PolicyLenient,
		},
		{
			name:  "disabled",
			input: "disabled",
			want:  PolicyDisabled,
		},
		{
			name:  "invalid defaults to strict",
			input: "invalid",
			want:  PolicyStrict,
		},
		{
			name:  "empty defaults to strict",
			input: "",
			want:  PolicyStrict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizePolicy(tt.input)
			if got != tt.want {
				t.Errorf("NormalizePolicy() = %v, want %v", got, tt.want)
			}
		})
	}
}
