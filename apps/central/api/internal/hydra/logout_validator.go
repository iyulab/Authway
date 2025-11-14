package hydra

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LogoutRedirectPolicy defines the validation strictness level
type LogoutRedirectPolicy string

const (
	PolicyStrict   LogoutRedirectPolicy = "strict"
	PolicyLenient  LogoutRedirectPolicy = "lenient"
	PolicyDisabled LogoutRedirectPolicy = "disabled"
)

// LogoutValidationConfig holds client-specific logout configuration
type LogoutValidationConfig struct {
	PostLogoutRedirectURIs []string
	Policy                 LogoutRedirectPolicy
	DefaultLogoutURI       string
	AllowWildcard          bool
}

// ValidateLogoutRedirectURI validates post_logout_redirect_uri based on client policy
func ValidateLogoutRedirectURI(uri string, config LogoutValidationConfig) error {
	// Environment safety check: disabled policy not allowed in production
	if config.Policy == PolicyDisabled {
		env := os.Getenv("ENVIRONMENT")
		if env == "production" || env == "prod" {
			return fmt.Errorf("logout redirect validation cannot be disabled in production environment")
		}
		// In non-production, allow any URI
		return nil
	}

	switch config.Policy {
	case PolicyStrict:
		return validateStrict(uri, config)
	case PolicyLenient:
		return validateLenient(uri, config)
	default:
		// Unknown policy defaults to strict
		return validateStrict(uri, config)
	}
}

// validateStrict requires URI to be provided and whitelisted
func validateStrict(uri string, config LogoutValidationConfig) error {
	if uri == "" {
		return fmt.Errorf("post_logout_redirect_uri is required (policy: strict)")
	}

	if !isWhitelisted(uri, config.PostLogoutRedirectURIs, config.AllowWildcard) {
		return fmt.Errorf("post_logout_redirect_uri '%s' is not whitelisted for this client", uri)
	}

	return nil
}

// validateLenient allows missing URI (uses default) but validates if provided
func validateLenient(uri string, config LogoutValidationConfig) error {
	// If URI not provided, it's okay (will use default)
	if uri == "" {
		return nil
	}

	// If URI provided, validate it
	if !isWhitelisted(uri, config.PostLogoutRedirectURIs, config.AllowWildcard) {
		return fmt.Errorf("post_logout_redirect_uri '%s' is not whitelisted for this client", uri)
	}

	return nil
}

// isWhitelisted checks if URI matches any whitelisted pattern
func isWhitelisted(uri string, whitelist []string, allowWildcard bool) bool {
	// Empty whitelist means no URIs allowed
	if len(whitelist) == 0 {
		return false
	}

	// Try exact match first
	for _, allowed := range whitelist {
		if uri == allowed {
			return true
		}
	}

	// Try wildcard matching if enabled
	if allowWildcard {
		for _, pattern := range whitelist {
			if matchesWildcard(uri, pattern) {
				return true
			}
		}
	}

	return false
}

// matchesWildcard performs wildcard pattern matching
// Supports:
// - http://localhost:* (any port)
// - https://*.example.com (subdomain wildcard)
func matchesWildcard(uri, pattern string) bool {
	// Use filepath.Match which supports * and ? wildcards
	matched, err := filepath.Match(pattern, uri)
	if err != nil {
		// Invalid pattern, no match
		return false
	}
	return matched
}

// GetDefaultLogoutURI returns the appropriate logout URI for lenient policy
func GetDefaultLogoutURI(config LogoutValidationConfig) string {
	// Use explicitly configured default
	if config.DefaultLogoutURI != "" {
		return config.DefaultLogoutURI
	}

	// Fallback: use first whitelisted URI
	if len(config.PostLogoutRedirectURIs) > 0 {
		return config.PostLogoutRedirectURIs[0]
	}

	// No default available
	return ""
}

// NormalizePolicy ensures policy value is valid
func NormalizePolicy(policy string) LogoutRedirectPolicy {
	normalized := strings.ToLower(strings.TrimSpace(policy))

	switch LogoutRedirectPolicy(normalized) {
	case PolicyStrict:
		return PolicyStrict
	case PolicyLenient:
		return PolicyLenient
	case PolicyDisabled:
		return PolicyDisabled
	default:
		// Default to strict for safety
		return PolicyStrict
	}
}
