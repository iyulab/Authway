package config

import (
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig         `mapstructure:"app"`
	Database DatabaseConfig    `mapstructure:"database"`
	Redis    RedisConfig       `mapstructure:"redis"`
	JWT      JWTConfig         `mapstructure:"jwt"`
	OAuth    OAuthConfig       `mapstructure:"oauth"`
	Hydra    HydraConfig       `mapstructure:"hydra"`
	CORS     CORSConfig        `mapstructure:"cors"`
	Email    EmailConfig       `mapstructure:"email"`
	Google   GoogleOAuthConfig `mapstructure:"google"`
	GitHub   GitHubOAuthConfig `mapstructure:"github"`
	Tenant   TenantConfig      `mapstructure:"tenant"`
	Admin    AdminConfig       `mapstructure:"admin"`
}

type AppConfig struct {
	Name        string `mapstructure:"name"`
	Version     string `mapstructure:"version"`
	Environment string `mapstructure:"environment"`
	Port        string `mapstructure:"port"`
	BaseURL     string `mapstructure:"base_url"`
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
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	AccessTokenSecret  string `mapstructure:"access_token_secret"`
	RefreshTokenSecret string `mapstructure:"refresh_token_secret"`
	AccessTokenExpiry  string `mapstructure:"access_token_expiry"`
	RefreshTokenExpiry string `mapstructure:"refresh_token_expiry"`
	Issuer             string `mapstructure:"issuer"`
	PrivateKeyPath     string `mapstructure:"private_key_path"`
	PublicKeyPath      string `mapstructure:"public_key_path"`
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
}

type EmailConfig struct {
	SMTPHost     string `mapstructure:"smtp_host"`
	SMTPPort     int    `mapstructure:"smtp_port"`
	SMTPUser     string `mapstructure:"smtp_user"`
	SMTPPassword string `mapstructure:"smtp_password"`
	FromEmail    string `mapstructure:"from_email"`
	FromName     string `mapstructure:"from_name"`
}

type GoogleOAuthConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
	Enabled      bool   `mapstructure:"enabled"`
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

func Load() (*Config, error) {
	// Load .env file if it exists (silently ignore if not found)
	// Try multiple locations
	_ = godotenv.Load("../../.env")  // From project root
	_ = godotenv.Load(".env")        // From current directory
	_ = godotenv.Load()              // Default location

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

	// Warn about insecure JWT secrets in production
	if c.App.Environment == "production" {
		if c.JWT.AccessTokenSecret == "your-secret-key-change-in-production" {
			errors = append(errors, "CRITICAL: jwt.access_token_secret must be changed in production")
		}
		if c.JWT.RefreshTokenSecret == "your-refresh-secret-key-change-in-production" {
			errors = append(errors, "CRITICAL: jwt.refresh_token_secret must be changed in production")
		}
		if c.Admin.Password == "" || c.Admin.Password == "admin123" {
			errors = append(errors, "CRITICAL: admin.password must be set to a strong password in production")
		}
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

func setDefaults() {
	// App defaults
	viper.SetDefault("app.name", "Authway")
	viper.SetDefault("app.version", "0.1.0")
	viper.SetDefault("app.environment", "development")
	viper.SetDefault("app.port", "8080")
	viper.SetDefault("app.base_url", "http://localhost:8080")

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

	// JWT defaults
	viper.SetDefault("jwt.access_token_secret", "your-secret-key-change-in-production")
	viper.SetDefault("jwt.refresh_token_secret", "your-refresh-secret-key-change-in-production")
	viper.SetDefault("jwt.access_token_expiry", "15m")
	viper.SetDefault("jwt.refresh_token_expiry", "7d")
	viper.SetDefault("jwt.issuer", "authway")

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
	viper.SetDefault("email.smtp_host", "localhost")
	viper.SetDefault("email.smtp_port", 587)
	viper.SetDefault("email.from_email", "noreply@authway.dev")
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
}
