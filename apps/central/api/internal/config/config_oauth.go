package config

// MicrosoftOAuthConfig holds Microsoft OAuth configuration
type MicrosoftOAuthConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	TenantID     string `mapstructure:"tenant_id"`
	RedirectURL  string `mapstructure:"redirect_url"`
	Enabled      bool   `mapstructure:"enabled"`
}

// AppleOAuthConfig holds Apple Sign-in configuration
type AppleOAuthConfig struct {
	ClientID    string `mapstructure:"client_id"`
	TeamID      string `mapstructure:"team_id"`
	KeyID       string `mapstructure:"key_id"`
	PrivateKey  string `mapstructure:"private_key"`
	RedirectURL string `mapstructure:"redirect_url"`
	Enabled     bool   `mapstructure:"enabled"`
}
