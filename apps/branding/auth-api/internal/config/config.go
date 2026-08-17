package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	App         AppConfig
	Google      GoogleOAuthConfig
	CentralAPI  CentralAPIConfig
	Hydra       HydraConfig
}

// IsDevelopment reports whether env selects the relaxed local/dev codepath.
// Unset defaults to development.
func IsDevelopment(env string) bool {
	return env == "" || env == "development"
}

// IsProduction reports whether env requires production-grade behavior —
// everything that is not development, including staging. Unrecognized values
// fail closed on purpose — a typo in the environment name must not silently
// relax production-only handling.
func IsProduction(env string) bool {
	return !IsDevelopment(env)
}

type AppConfig struct {
	Port        string
	Environment string
	StaticPath  string
	LoginUIURL  string
}

type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type CentralAPIConfig struct {
	BaseURL      string
	InternalKey  string
}

type HydraConfig struct {
	AdminURL  string
	PublicURL string
}

func Load() (*Config, error) {
	// Load .env file if exists
	_ = godotenv.Load()

	config := &Config{
		App: AppConfig{
			Port:        getEnv("PORT", "8081"),
			Environment: getEnv("ENVIRONMENT", "development"),
			StaticPath:  getEnv("STATIC_PATH", "./static"),
			LoginUIURL:  getEnv("LOGIN_UI_URL", "http://localhost:3001"),
		},
		Google: GoogleOAuthConfig{
			ClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
			ClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
			RedirectURL:  getEnv("GOOGLE_REDIRECT_URI", "http://localhost:8081/auth/google/callback"),
		},
		CentralAPI: CentralAPIConfig{
			BaseURL:     getEnv("CENTRAL_API_URL", "http://localhost:8080"),
			InternalKey: getEnv("INTERNAL_API_KEY", "dev-internal-key"),
		},
		Hydra: HydraConfig{
			AdminURL:  getEnv("HYDRA_ADMIN_URL", "http://localhost:4445"),
			PublicURL: getEnv("HYDRA_PUBLIC_URL", "http://localhost:4444"),
		},
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

func (c *Config) Validate() error {
	if c.Google.ClientID == "" {
		return fmt.Errorf("GOOGLE_CLIENT_ID is required")
	}
	if c.Google.ClientSecret == "" {
		return fmt.Errorf("GOOGLE_CLIENT_SECRET is required")
	}
	if c.Google.RedirectURL == "" {
		return fmt.Errorf("GOOGLE_REDIRECT_URI is required")
	}
	if c.CentralAPI.BaseURL == "" {
		return fmt.Errorf("CENTRAL_API_URL is required")
	}
	if c.CentralAPI.InternalKey == "" {
		return fmt.Errorf("INTERNAL_API_KEY is required")
	}
	if c.Hydra.AdminURL == "" {
		return fmt.Errorf("HYDRA_ADMIN_URL is required")
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
