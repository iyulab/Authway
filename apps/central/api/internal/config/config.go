package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// generateRandomKey returns a URL-safe base64-encoded random string.
// Used to auto-provision a dev-mode admin API key when one isn't configured.
func generateRandomKey(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// IsDevelopment reports whether env selects the relaxed local/dev codepath —
// auto-generated admin keys, verbose debug output. Unset defaults to development.
func IsDevelopment(env string) bool {
	return env == "" || env == "development"
}

// IsProduction reports whether env requires production-grade safety (fail-closed
// secret validation, no debug output): everything that is not development,
// including staging. Unrecognized values fail closed on purpose — a typo in the
// environment name must not silently relax a security gate.
func IsProduction(env string) bool {
	return !IsDevelopment(env)
}

type Config struct {
	App                 AppConfig                 `mapstructure:"app"`
	Database            DatabaseConfig            `mapstructure:"database"`
	Redis               RedisConfig               `mapstructure:"redis"`
	OAuth               OAuthConfig               `mapstructure:"oauth"`
	Hydra               HydraConfig               `mapstructure:"hydra"`
	CORS                CORSConfig                `mapstructure:"cors"`
	Email               EmailConfig               `mapstructure:"email"`
	Google              GoogleOAuthConfig         `mapstructure:"google"`
	GitHub              GitHubOAuthConfig         `mapstructure:"github"`
	Microsoft           MicrosoftOAuthConfig      `mapstructure:"microsoft"`
	Apple               AppleOAuthConfig          `mapstructure:"apple"`
	Tenant              TenantConfig              `mapstructure:"tenant"`
	Admin               AdminConfig               `mapstructure:"admin"`
	Security            SecurityConfig            `mapstructure:"security"`
	ApplicationInsights ApplicationInsightsConfig `mapstructure:"applicationinsights"`
}

type AppConfig struct {
	Name        string `mapstructure:"name"`
	Version     string `mapstructure:"version"`
	Environment string `mapstructure:"environment"`
	Port        string `mapstructure:"port"`
	// BaseURL is this API's own public address. It is what discovery advertises
	// as api_server — it is NOT where humans go. Anything a person clicks
	// belongs on FrontendURL.
	BaseURL string `mapstructure:"base_url"`
	// FrontendURL is the auth UI's public address, the host for every link we
	// put in an email: invitation accept, magic link, verify email, reset
	// password. These used to be built from BaseURL, which pointed at the API,
	// so every emailed link 404'd in every environment.
	FrontendURL string `mapstructure:"frontend_url"`
	// ConsoleURL is the console's public address (the admin SPA, e.g.
	// https://admin.example.com or https://auth.example.com/console). Console
	// accounts sign in to it through the console's client in the system
	// tenant, whose redirect is <ConsoleURL>/callback. Empty: the console
	// signs in only with the admin password.
	ConsoleURL string `mapstructure:"console_url"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
	SSLMode  string `mapstructure:"ssl_mode"`
}

type RedisConfig struct {
	// Enabled keeps sign-in state (challenges, state values, rate-limit
	// counters) in Redis, shared by every instance. Off, it is kept in the
	// process — for a deployment of one instance only.
	Enabled    bool   `mapstructure:"enabled"`
	Host       string `mapstructure:"host"`
	Port       int    `mapstructure:"port"`
	Password   string `mapstructure:"password"`
	DB         int    `mapstructure:"db"`
	TLSEnabled bool   `mapstructure:"tls_enabled"`
	// KeyPrefix names this deployment's share of a Redis that others use
	// too: every key it reads or writes starts with it. Empty for a Redis of
	// the deployment's own.
	KeyPrefix string `mapstructure:"key_prefix"`
	// Cluster connects with the Redis Cluster protocol, for a Redis that
	// shards keys over nodes and answers a key on the wrong node with MOVED
	// (a managed Redis with the OSS cluster policy, or a self-run cluster).
	// Host and Port name any node; the rest are discovered. A cluster has
	// database 0 only — separate deployments on one with KeyPrefix.
	Cluster bool `mapstructure:"cluster"`
}

type OAuthConfig struct {
	AuthorizeCodeExpiry string   `mapstructure:"authorize_code_expiry"`
	AllowedGrantTypes   []string `mapstructure:"allowed_grant_types"`
	AllowedScopes       []string `mapstructure:"allowed_scopes"`
	RequirePKCE         bool     `mapstructure:"require_pkce"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

type HydraConfig struct {
	AdminURL  string `mapstructure:"admin_url"`
	PublicURL string `mapstructure:"public_url"`
	// PublicBackendURL is where this API reaches the authorization server's
	// public endpoints when it passes a token request on
	// (AUTHWAY_HYDRA_PUBLIC_BACKEND_URL) — an address inside the deployment's
	// network, so the call does not leave it and come back through whatever
	// stands in front of the public address. Empty: PublicURL.
	PublicBackendURL string `mapstructure:"public_backend_url"`
	// TokenHookKeys are the keys the authorization server may present when it
	// calls the token hook (AUTHWAY_HYDRA_TOKEN_HOOK_KEY, comma-separated —
	// more than one while the key is being changed). None: the hook is not
	// served, and an authorization server configured to call it cannot issue
	// tokens.
	TokenHookKeys []string `mapstructure:"-"`
}

// DevelopmentTokenHookKey is the token hook key of local development: the
// value docker-compose.yml gives the local authorization server.
const DevelopmentTokenHookKey = "local-development-token-hook-key"

// EngineURL is where this API reaches the authorization server's public
// endpoints.
func (h HydraConfig) EngineURL() string {
	if h.PublicBackendURL != "" {
		return h.PublicBackendURL
	}
	return h.PublicURL
}

type EmailConfig struct {
	// Sendway (https://github.com/iyulab/Sendway). SendwayBaseURL has no default —
	// it names the deployment to talk to and is required when UseSendway is set.
	UseSendway     bool   `mapstructure:"use_sendway"`
	SendwayBaseURL string `mapstructure:"sendway_base_url"`
	SendwayAPIKey  string `mapstructure:"sendway_api_key"`

	// SMTP — the standard path, with any mail server. SMTPUser empty: the
	// server is not asked to authenticate. SMTPSecurity: "" (implicit TLS on
	// port 465, STARTTLS elsewhere when offered), "tls", "starttls" (required)
	// or "none".
	SMTPHost     string `mapstructure:"smtp_host"`
	SMTPPort     int    `mapstructure:"smtp_port"`
	SMTPUser     string `mapstructure:"smtp_user"`
	SMTPPassword string `mapstructure:"smtp_password"`
	SMTPSecurity string `mapstructure:"smtp_security"`

	// Common settings
	FromEmail string `mapstructure:"from_email"`
	FromName  string `mapstructure:"from_name"`
}

type GoogleOAuthConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
	Enabled      bool   `mapstructure:"enabled"`
	// AuthURL, TokenURL and UserInfoURL are where Google is reached
	// (AUTHWAY_GOOGLE_AUTH_URL, _TOKEN_URL, _USERINFO_URL). Empty means
	// Google's own endpoints. They exist to point sign-in at a stand-in
	// provider under test and are refused outside development: whatever
	// answers there decides who signs in.
	AuthURL     string `mapstructure:"auth_url"`
	TokenURL    string `mapstructure:"token_url"`
	UserInfoURL string `mapstructure:"userinfo_url"`
}

// EndpointsOverridden reports whether sign-in is pointed somewhere other than
// Google itself.
func (g GoogleOAuthConfig) EndpointsOverridden() bool {
	return g.AuthURL != "" || g.TokenURL != "" || g.UserInfoURL != ""
}

type GitHubOAuthConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
	Enabled      bool   `mapstructure:"enabled"`
}

type TenantConfig struct {
	SingleTenantMode bool   `mapstructure:"single_tenant_mode"`
	TenantName       string `mapstructure:"tenant_name"`
	TenantSlug       string `mapstructure:"tenant_slug"`
}

type AdminConfig struct {
	APIKey   string `mapstructure:"api_key"`
	Password string `mapstructure:"password"`
}

type SecurityConfig struct {
	// TOTPEncryptionKey is a base64-encoded 32-byte AES-256 key used to encrypt
	// at rest what the server must be able to read back: TOTP shared secrets,
	// and the credentials clients hold at other identity providers. (It keeps
	// the name it had when it encrypted the first of these only.) Required in
	// production (fail-closed); when empty in development they are stored as
	// plaintext.
	TOTPEncryptionKey string `mapstructure:"totp_encryption_key"`
	// TOTPPreviousKeys are the keys TOTPEncryptionKey replaced, while values
	// sealed with them are moved to it (AUTHWAY_TOTP_ENCRYPTION_PREVIOUS_KEYS,
	// comma-separated, same format). They only open what they sealed; every
	// startup reseals such values with the current key, and once it reports
	// nothing left the previous keys can be removed.
	TOTPPreviousKeys []string `mapstructure:"-"`
	// AllowPrivateTargets lets the calls the deployment makes to addresses it
	// was given — webhook deliveries, a client's back-channel logout address —
	// reach this host and private networks, and lets those addresses be plain
	// http (AUTHWAY_ALLOW_PRIVATE_TARGETS). Off unless set: an unset
	// environment name reads as development, so tying this to the environment
	// would open it wherever the name was forgotten.
	AllowPrivateTargets bool `mapstructure:"allow_private_targets"`
}

type ApplicationInsightsConfig struct {
	ConnectionString string `mapstructure:"connection_string"`
	Enabled          bool   `mapstructure:"enabled"`
}

func Load() (*Config, error) {
	// Load .env from the working directory, i.e. apps/central/api when the API is
	// started the documented way. Absence is fine — the process environment alone
	// is how every deployment configures this service.
	//
	// This used to probe "../../.env" first, meaning apps/.env: the intent was the
	// repo root, but from apps/central/api that is three levels up, so the branch
	// could never fire. Rather than repair the probe, it is gone — the repo root
	// holds no .env, and reintroducing one would give two files a say over the
	// same settings with the winner decided by which directory you happened to
	// start from. See .env.example next to this app.
	if err := godotenv.Load(); err == nil {
		fmt.Println("✓ Loaded .env")
	} else {
		fmt.Println("⚠ No .env file found, using environment variables only")
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("/etc/authway")

	// Set environment variable prefix
	viper.SetEnvPrefix("AUTHWAY")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Set defaults
	setDefaults()

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, continue with environment variables and defaults
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Manual override for Google OAuth config (Viper AutomaticEnv doesn't work well with nested structs)
	if clientID := os.Getenv("AUTHWAY_GOOGLE_CLIENT_ID"); clientID != "" {
		config.Google.ClientID = clientID
	}
	if clientSecret := os.Getenv("AUTHWAY_GOOGLE_CLIENT_SECRET"); clientSecret != "" {
		config.Google.ClientSecret = clientSecret
	}
	if redirectURL := os.Getenv("AUTHWAY_GOOGLE_REDIRECT_URL"); redirectURL != "" {
		config.Google.RedirectURL = redirectURL
	}
	if enabled := os.Getenv("AUTHWAY_GOOGLE_ENABLED"); enabled != "" {
		config.Google.Enabled = (enabled == "true")
	}
	if v := os.Getenv("AUTHWAY_GOOGLE_AUTH_URL"); v != "" {
		config.Google.AuthURL = v
	}
	if v := os.Getenv("AUTHWAY_GOOGLE_TOKEN_URL"); v != "" {
		config.Google.TokenURL = v
	}
	if v := os.Getenv("AUTHWAY_GOOGLE_USERINFO_URL"); v != "" {
		config.Google.UserInfoURL = v
	}

	// Manual override for GitHub OAuth config
	if clientID := os.Getenv("AUTHWAY_GITHUB_CLIENT_ID"); clientID != "" {
		config.GitHub.ClientID = clientID
	}
	if clientSecret := os.Getenv("AUTHWAY_GITHUB_CLIENT_SECRET"); clientSecret != "" {
		config.GitHub.ClientSecret = clientSecret
	}
	if redirectURL := os.Getenv("AUTHWAY_GITHUB_REDIRECT_URL"); redirectURL != "" {
		config.GitHub.RedirectURL = redirectURL
	}
	if enabled := os.Getenv("AUTHWAY_GITHUB_ENABLED"); enabled != "" {
		config.GitHub.Enabled = (enabled == "true")
	}

	// Manual override for Hydra config (Viper AutomaticEnv doesn't work well with nested structs)
	if adminURL := os.Getenv("AUTHWAY_HYDRA_ADMIN_URL"); adminURL != "" {
		config.Hydra.AdminURL = adminURL
	}
	if publicURL := os.Getenv("AUTHWAY_HYDRA_PUBLIC_URL"); publicURL != "" {
		config.Hydra.PublicURL = publicURL
	}
	if backendURL := os.Getenv("AUTHWAY_HYDRA_PUBLIC_BACKEND_URL"); backendURL != "" {
		config.Hydra.PublicBackendURL = backendURL
	}
	for _, k := range strings.Split(os.Getenv("AUTHWAY_HYDRA_TOKEN_HOOK_KEY"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			config.Hydra.TokenHookKeys = append(config.Hydra.TokenHookKeys, k)
		}
	}

	// Manual override for Database config (Viper AutomaticEnv doesn't always work well)
	if host := os.Getenv("AUTHWAY_DATABASE_HOST"); host != "" {
		config.Database.Host = host
	}
	if port := os.Getenv("AUTHWAY_DATABASE_PORT"); port != "" {
		if portInt, err := strconv.Atoi(port); err == nil {
			config.Database.Port = portInt
		}
	}
	if user := os.Getenv("AUTHWAY_DATABASE_USER"); user != "" {
		config.Database.User = user
	}
	if password := os.Getenv("AUTHWAY_DATABASE_PASSWORD"); password != "" {
		config.Database.Password = password
	}
	if name := os.Getenv("AUTHWAY_DATABASE_NAME"); name != "" {
		config.Database.Name = name
	}
	if sslMode := os.Getenv("AUTHWAY_DATABASE_SSL_MODE"); sslMode != "" {
		config.Database.SSLMode = sslMode
	}

	// Manual override for Redis config
	if host := os.Getenv("AUTHWAY_REDIS_HOST"); host != "" {
		config.Redis.Host = host
	}
	if port := os.Getenv("AUTHWAY_REDIS_PORT"); port != "" {
		if portInt, err := strconv.Atoi(port); err == nil {
			config.Redis.Port = portInt
		}
	}
	if password := os.Getenv("AUTHWAY_REDIS_PASSWORD"); password != "" {
		config.Redis.Password = password
	}
	if db := os.Getenv("AUTHWAY_REDIS_DB"); db != "" {
		if dbInt, err := strconv.Atoi(db); err == nil {
			config.Redis.DB = dbInt
		}
	}
	if keyPrefix := os.Getenv("AUTHWAY_REDIS_KEY_PREFIX"); keyPrefix != "" {
		config.Redis.KeyPrefix = keyPrefix
	}
	if tlsEnabled := os.Getenv("AUTHWAY_REDIS_TLS_ENABLED"); tlsEnabled != "" {
		config.Redis.TLSEnabled = (tlsEnabled == "true")
	}
	if cluster := os.Getenv("AUTHWAY_REDIS_CLUSTER"); cluster != "" {
		config.Redis.Cluster = (cluster == "true")
	}

	// Manual override for Admin config
	if adminPassword := os.Getenv("AUTHWAY_ADMIN_PASSWORD"); adminPassword != "" {
		config.Admin.Password = adminPassword
	}
	if adminAPIKey := os.Getenv("AUTHWAY_ADMIN_API_KEY"); adminAPIKey != "" {
		config.Admin.APIKey = adminAPIKey
	}

	// Dev-mode fallback: auto-generate the admin API key in local
	// development so the admin console remains usable
	// without manual setup. Every other environment — including staging — fails
	// validation (below) to force explicit keys instead of silently minting
	// ephemeral ones that a peer service can't know.
	if IsDevelopment(config.App.Environment) {
		// The local authorization server (docker-compose.yml) presents this
		// key when it calls the token hook, so `go run` works against it
		// without setup. It guards nothing outside one machine.
		if len(config.Hydra.TokenHookKeys) == 0 {
			config.Hydra.TokenHookKeys = []string{DevelopmentTokenHookKey}
		}
		if config.Admin.APIKey == "" {
			key, err := generateRandomKey(32)
			if err != nil {
				return nil, fmt.Errorf("failed to generate dev admin API key: %w", err)
			}
			config.Admin.APIKey = key
			fmt.Printf("⚠️  [dev] Auto-generated AUTHWAY_ADMIN_API_KEY: %s\n", key)
			fmt.Printf("⚠️  [dev] Set AUTHWAY_ADMIN_API_KEY env var to use a stable key across restarts.\n")
		}
	}

	// Manual override for Security config
	if totpKey := os.Getenv("AUTHWAY_TOTP_ENCRYPTION_KEY"); totpKey != "" {
		config.Security.TOTPEncryptionKey = totpKey
	}
	config.Security.TOTPPreviousKeys = nil
	for _, k := range strings.Split(os.Getenv("AUTHWAY_TOTP_ENCRYPTION_PREVIOUS_KEYS"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			config.Security.TOTPPreviousKeys = append(config.Security.TOTPPreviousKeys, k)
		}
	}
	if os.Getenv("AUTHWAY_WEBHOOK_ALLOW_PRIVATE_TARGETS") != "" {
		// Starting without it would silently close what the operator opened.
		return nil, fmt.Errorf("AUTHWAY_WEBHOOK_ALLOW_PRIVATE_TARGETS is now AUTHWAY_ALLOW_PRIVATE_TARGETS (it also covers back-channel logout addresses): rename it")
	}
	config.Security.AllowPrivateTargets = false
	if v := os.Getenv("AUTHWAY_ALLOW_PRIVATE_TARGETS"); v != "" {
		allow, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("AUTHWAY_ALLOW_PRIVATE_TARGETS must be true or false: %w", err)
		}
		config.Security.AllowPrivateTargets = allow
	}

	// Manual override for Application Insights config
	if connectionString := os.Getenv("AUTHWAY_APPLICATIONINSIGHTS_CONNECTION_STRING"); connectionString != "" {
		config.ApplicationInsights.ConnectionString = connectionString
		config.ApplicationInsights.Enabled = true
	}
	if enabled := os.Getenv("AUTHWAY_APPLICATIONINSIGHTS_ENABLED"); enabled != "" {
		config.ApplicationInsights.Enabled = (enabled == "true")
	}

	// Manual override for CORS allowed origins (parse comma-separated string)
	if allowedOrigins := os.Getenv("AUTHWAY_CORS_ALLOWED_ORIGINS"); allowedOrigins != "" {
		config.CORS.AllowedOrigins = strings.Split(allowedOrigins, ",")
		// Trim spaces from each origin
		for i := range config.CORS.AllowedOrigins {
			config.CORS.AllowedOrigins[i] = strings.TrimSpace(config.CORS.AllowedOrigins[i])
		}
	}

	// Manual override for OAuth allowed grant types
	if grantTypes := os.Getenv("AUTHWAY_OAUTH_ALLOWED_GRANT_TYPES"); grantTypes != "" {
		config.OAuth.AllowedGrantTypes = strings.Split(grantTypes, ",")
		for i := range config.OAuth.AllowedGrantTypes {
			config.OAuth.AllowedGrantTypes[i] = strings.TrimSpace(config.OAuth.AllowedGrantTypes[i])
		}
	}

	// Manual override for OAuth allowed scopes
	if scopes := os.Getenv("AUTHWAY_OAUTH_ALLOWED_SCOPES"); scopes != "" {
		config.OAuth.AllowedScopes = strings.Split(scopes, ",")
		for i := range config.OAuth.AllowedScopes {
			config.OAuth.AllowedScopes[i] = strings.TrimSpace(config.OAuth.AllowedScopes[i])
		}
	}

	// Config 적용 확인용 stdout 출력 — development에서만 출력
	// (staging 이상은 zap logger가 별도로 startup state를 남기므로 중복이며,
	// staging도 실배포 환경이라 설정값을 stdout에 남기지 않는다).
	if IsDevelopment(config.App.Environment) {
		fmt.Printf("🔍 Database Config: Host=%s, Port=%d, User=%s, Name=%s\n",
			config.Database.Host, config.Database.Port, config.Database.User, config.Database.Name)
		fmt.Printf("🔍 Redis Config: Host=%s, Port=%d\n",
			config.Redis.Host, config.Redis.Port)
		fmt.Printf("🔍 Google OAuth Config: ClientID=%s, Enabled=%v, RedirectURL=%s\n",
			config.Google.ClientID, config.Google.Enabled, config.Google.RedirectURL)
		fmt.Printf("🔍 Hydra Config: AdminURL=%s, PublicURL=%s\n",
			config.Hydra.AdminURL, config.Hydra.PublicURL)
		fmt.Printf("🔍 CORS Config: AllowedOrigins=%v\n", config.CORS.AllowedOrigins)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &config, nil
}

// Validate checks if required configuration values are set
func (c *Config) Validate() error {
	var errors []string

	// Required: Database connection
	if c.Database.Host == "" {
		errors = append(errors, "database.host is required")
	}
	if c.Database.User == "" {
		errors = append(errors, "database.user is required")
	}
	if c.Database.Name == "" {
		errors = append(errors, "database.name is required")
	}

	// Fail-closed checks below apply to every environment that is not local
	// development — staging is a real deployed, internet-facing environment and
	// gets the same guarantees as production, not the relaxed dev fallback.
	if IsProduction(c.App.Environment) {
		if weakAdminPassword(c.Admin.Password) {
			errors = append(errors, fmt.Sprintf("CRITICAL: admin.password must be set to a strong password outside development — at least %d characters, not an example value", minAdminPasswordLength))
		}
		// Fail-closed: admin API key is required. Without it, every
		// adminAuth-protected endpoint rejects all traffic (and previously — before
		// 0.2.1 — it silently bypassed auth).
		if c.Admin.APIKey == "" {
			errors = append(errors, "CRITICAL: admin.api_key (AUTHWAY_ADMIN_API_KEY) must be set outside development — required for /api/v1/clients/* and other admin endpoints")
		}
		// Fail-closed: without the key the token hook is not served, and an
		// authorization server configured to call it issues no token at all.
		if len(c.Hydra.TokenHookKeys) == 0 {
			errors = append(errors, "CRITICAL: AUTHWAY_HYDRA_TOKEN_HOOK_KEY must be set outside development — the authorization server calls the token hook with it before it issues any token")
		}
		// Fail-closed: TOTP secrets and provider credentials must be encrypted
		// at rest. An absent or malformed key would silently fall back to
		// plaintext storage.
		if c.Security.TOTPEncryptionKey == "" {
			errors = append(errors, "CRITICAL: security.totp_encryption_key (AUTHWAY_TOTP_ENCRYPTION_KEY) must be set outside development — a base64-encoded 32-byte key that encrypts TOTP secrets and provider credentials at rest")
		} else if key, err := base64.StdEncoding.DecodeString(c.Security.TOTPEncryptionKey); err != nil {
			errors = append(errors, "CRITICAL: security.totp_encryption_key must be valid base64")
		} else if len(key) != 32 {
			errors = append(errors, fmt.Sprintf("CRITICAL: security.totp_encryption_key must decode to 32 bytes (AES-256), got %d", len(key)))
		}
	}
	// A previous key is checked in every environment: a malformed one would
	// only show when a value it sealed is read.
	if len(c.Security.TOTPPreviousKeys) > 0 && c.Security.TOTPEncryptionKey == "" {
		errors = append(errors, "AUTHWAY_TOTP_ENCRYPTION_PREVIOUS_KEYS is set without AUTHWAY_TOTP_ENCRYPTION_KEY — previous keys are only read while values are moved to a current key")
	}
	for i, k := range c.Security.TOTPPreviousKeys {
		if key, err := base64.StdEncoding.DecodeString(k); err != nil || len(key) != 32 {
			errors = append(errors, fmt.Sprintf("AUTHWAY_TOTP_ENCRYPTION_PREVIOUS_KEYS: key %d must be base64 of 32 bytes", i+1))
		} else if k == c.Security.TOTPEncryptionKey {
			errors = append(errors, fmt.Sprintf("AUTHWAY_TOTP_ENCRYPTION_PREVIOUS_KEYS: key %d is the current key", i+1))
		}
	}

	// Fail-closed: emailed links must point at the auth UI. Checked in every
	// environment because the failure is silent — a wrong value compiles,
	// boots, sends mail, and only the recipient ever sees the 404. The UI's
	// address may be the API's own: an installation that serves both from one
	// origin (deploy/onprem) has the two equal, and that is right.
	if c.App.FrontendURL == "" {
		errors = append(errors, "app.frontend_url (AUTHWAY_APP_FRONTEND_URL) is required — the auth UI's public URL, used for invitation/magic-link/verify-email/reset-password links")
	} else if IsProduction(c.App.Environment) && isLoopbackURL(c.App.FrontendURL) {
		// The required-check above cannot catch this on its own: viper's
		// AutomaticEnv ignores an empty environment variable (allowEmptyEnv is
		// false), so AUTHWAY_APP_FRONTEND_URL= silently falls back to the
		// localhost default instead of failing. A deployed environment — staging
		// included — would then mail out links to a host only the container can
		// reach.
		errors = append(errors, fmt.Sprintf("CRITICAL: app.frontend_url must be a publicly reachable URL outside development, got %q — emailed links would point at the container itself", c.App.FrontendURL))
	}

	if c.App.ConsoleURL != "" {
		if u, err := url.Parse(c.App.ConsoleURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errors = append(errors, fmt.Sprintf("app.console_url (AUTHWAY_APP_CONSOLE_URL) must be the console's absolute http(s) address, got %q", c.App.ConsoleURL))
		} else if IsProduction(c.App.Environment) && isLoopbackURL(c.App.ConsoleURL) {
			errors = append(errors, fmt.Sprintf("app.console_url must be a publicly reachable URL outside development, got %q — console sign-ins would return to the container itself", c.App.ConsoleURL))
		}
	}

	if c.Google.EndpointsOverridden() && IsProduction(c.App.Environment) {
		errors = append(errors, "google.auth_url, google.token_url and google.userinfo_url (AUTHWAY_GOOGLE_*_URL) point sign-in at a stand-in provider and are only accepted in development")
	}
	if c.Email.UseSendway && strings.TrimSpace(c.Email.SendwayBaseURL) == "" {
		errors = append(errors, "email.sendway_base_url (AUTHWAY_EMAIL_SENDWAY_BASE_URL) is required when email.use_sendway is true")
	}
	switch c.Email.SMTPSecurity {
	case "", "tls", "starttls", "none":
	default:
		errors = append(errors, fmt.Sprintf("email.smtp_security (AUTHWAY_EMAIL_SMTP_SECURITY) must be empty, tls, starttls or none, got %q", c.Email.SMTPSecurity))
	}
	// Mail goes out through SMTP unless Sendway is chosen. Outside development
	// a mail server on the container's own loopback is the default left in
	// place, not a choice: every invitation and sign-in link would fail at its
	// first send, long after the deployment looked healthy.
	if !c.Email.UseSendway && IsProduction(c.App.Environment) && isLoopbackHost(c.Email.SMTPHost) {
		errors = append(errors, fmt.Sprintf("email.smtp_host (AUTHWAY_EMAIL_SMTP_HOST) must name a mail server outside development, got %q — set it, or set AUTHWAY_EMAIL_USE_SENDWAY", c.Email.SMTPHost))
	}

	// Warn about missing admin password in all environments
	if c.Admin.Password == "" {
		errors = append(errors, "WARNING: admin.password is not set - admin console will be inaccessible")
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}

	return nil
}

// isLoopbackURL reports whether u points at the machine running the process.
// Parsing rather than substring-matching so that a legitimate public host that
// merely contains "localhost" in a path or query is not rejected.
func isLoopbackURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	return isLoopbackHost(parsed.Hostname())
}

// isLoopbackHost reports whether host names this machine: localhost, a
// loopback address, or none at all.
func isLoopbackHost(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsUnspecified()
	}
	return false
}

func setDefaults() {
	// App defaults
	viper.SetDefault("app.name", "Authway")
	viper.SetDefault("app.version", "0.1.0")
	viper.SetDefault("app.environment", "development")
	viper.SetDefault("app.port", "8080")
	viper.SetDefault("app.base_url", "http://localhost:8080")
	// The auth UI's vite dev server, not the API port — see AppConfig.FrontendURL.
	viper.SetDefault("app.frontend_url", "http://localhost:3001")
	// No console sign-in until the console's address is configured — a key
	// without a default is not read from the environment.
	viper.SetDefault("app.console_url", "")

	// Database defaults
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "authway")
	viper.SetDefault("database.password", "authway")
	viper.SetDefault("database.name", "authway")
	viper.SetDefault("database.ssl_mode", "disable")

	// Redis defaults
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.key_prefix", "")
	viper.SetDefault("redis.tls_enabled", false)
	viper.SetDefault("redis.cluster", false)
	viper.SetDefault("redis.enabled", true)

	// OAuth defaults
	viper.SetDefault("oauth.authorize_code_expiry", "10m")
	viper.SetDefault("oauth.allowed_grant_types", []string{"authorization_code", "refresh_token"})
	viper.SetDefault("oauth.allowed_scopes", []string{"openid", "profile", "email"})
	viper.SetDefault("oauth.require_pkce", true)

	// Hydra defaults
	viper.SetDefault("hydra.admin_url", "http://localhost:4445")
	viper.SetDefault("hydra.public_url", "http://localhost:4444")

	// CORS defaults
	viper.SetDefault("cors.allowed_origins", []string{"http://localhost:3000", "http://localhost:3001"})

	// Email defaults
	viper.SetDefault("email.use_sendway", false) // Default to SMTP for development
	viper.SetDefault("email.sendway_base_url", "")
	viper.SetDefault("email.sendway_api_key", "")
	viper.SetDefault("email.smtp_host", "localhost")
	viper.SetDefault("email.smtp_port", 587)
	// Every key the struct maps needs a default here: Unmarshal fills only the
	// keys viper knows, so an AUTHWAY_EMAIL_* variable without one is ignored.
	viper.SetDefault("email.smtp_user", "")
	viper.SetDefault("email.smtp_password", "")
	viper.SetDefault("email.smtp_security", "")
	viper.SetDefault("email.from_email", "noreply@authway.in")
	viper.SetDefault("email.from_name", "Authway")

	// Google OAuth defaults
	viper.SetDefault("google.enabled", false)
	viper.SetDefault("google.redirect_url", "http://localhost:8080/auth/google/callback")

	// GitHub OAuth defaults
	viper.SetDefault("github.enabled", false)
	viper.SetDefault("github.redirect_url", "http://localhost:8080/auth/github/callback")

	// Tenant defaults
	viper.SetDefault("tenant.single_tenant_mode", false)
	viper.SetDefault("tenant.tenant_name", "")
	viper.SetDefault("tenant.tenant_slug", "")

	// Admin defaults (use strong password in production)
	viper.SetDefault("admin.api_key", "")
	viper.SetDefault("admin.password", "admin123") // Default for development only

	// Security defaults (TOTP encryption key must be provided in production)
	viper.SetDefault("security.totp_encryption_key", "")

	// Application Insights defaults (completely optional)
	viper.SetDefault("applicationinsights.enabled", false)
	viper.SetDefault("applicationinsights.connection_string", "")
}

// minAdminPasswordLength is the least length of the shared admin password
// outside development. The on-premises setup generates 32 characters.
const minAdminPasswordLength = 16

// weakAdminPassword reports a shared admin password a deployment must not run
// with: none, too short, or a value the repository itself shows as an example
// (the development default and .env.example's).
func weakAdminPassword(p string) bool {
	switch p {
	case "admin123", "authway0":
		return true
	}
	return len(p) < minAdminPasswordLength
}
