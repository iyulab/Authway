// Traefik Dynamic CORS Plugin for Authway
// Purpose: Validate CORS requests against client-specific allowed_origins from database
// Compatible with: Traefik v2.x plugin system

package traefik_cors_plugin

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// Config holds the plugin configuration
type Config struct {
	DBConnection string `json:"dbConnection,omitempty"` // PostgreSQL connection string
	CacheTTL     int    `json:"cacheTTL,omitempty"`     // Cache TTL in seconds (default: 300)
}

// CreateConfig creates the default plugin configuration
func CreateConfig() *Config {
	return &Config{
		CacheTTL: 300, // 5 minutes default cache
	}
}

// DynamicCORS holds the plugin state
type DynamicCORS struct {
	next   http.Handler
	name   string
	db     *sql.DB
	cache  map[string]cacheEntry
	config *Config
}

type cacheEntry struct {
	origins   []string
	expiresAt time.Time
}

// New creates a new DynamicCORS plugin
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// Connect to PostgreSQL
	db, err := sql.Open("postgres", config.DBConnection)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DynamicCORS{
		next:   next,
		name:   name,
		db:     db,
		cache:  make(map[string]cacheEntry),
		config: config,
	}, nil
}

// ServeHTTP implements the http.Handler interface
func (d *DynamicCORS) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	origin := req.Header.Get("Origin")

	// If no Origin header, pass through (not a CORS request)
	if origin == "" {
		d.next.ServeHTTP(rw, req)
		return
	}

	// Handle preflight (OPTIONS) requests
	if req.Method == http.MethodOptions {
		d.handlePreflight(rw, req, origin)
		return
	}

	// Extract client_id from request body (for POST /oauth2/token)
	clientID := d.extractClientID(req)
	if clientID == "" {
		// No client_id found, deny CORS
		http.Error(rw, "client_id required", http.StatusBadRequest)
		return
	}

	// Check if origin is allowed for this client
	allowed, err := d.isOriginAllowed(clientID, origin)
	if err != nil {
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !allowed {
		// Origin not allowed, return 403
		http.Error(rw, "Origin not allowed", http.StatusForbidden)
		return
	}

	// Add CORS headers for allowed origin
	rw.Header().Set("Access-Control-Allow-Origin", origin)
	rw.Header().Set("Access-Control-Allow-Credentials", "true")
	rw.Header().Set("Vary", "Origin")

	// Continue to next handler
	d.next.ServeHTTP(rw, req)
}

// handlePreflight handles CORS preflight requests
func (d *DynamicCORS) handlePreflight(rw http.ResponseWriter, req *http.Request, origin string) {
	// For preflight, we allow if the request looks valid
	// Actual validation happens on the real request
	rw.Header().Set("Access-Control-Allow-Origin", origin)
	rw.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	rw.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	rw.Header().Set("Access-Control-Max-Age", "3600")
	rw.Header().Set("Access-Control-Allow-Credentials", "true")
	rw.WriteHeader(http.StatusNoContent)
}

// extractClientID extracts client_id from request body
func (d *DynamicCORS) extractClientID(req *http.Request) string {
	// Parse form data
	if err := req.ParseForm(); err != nil {
		return ""
	}
	return req.FormValue("client_id")
}

// isOriginAllowed checks if the origin is allowed for the given client
func (d *DynamicCORS) isOriginAllowed(clientID, origin string) (bool, error) {
	// Check cache first
	if entry, ok := d.cache[clientID]; ok {
		if time.Now().Before(entry.expiresAt) {
			return d.contains(entry.origins, origin), nil
		}
		// Cache expired, delete entry
		delete(d.cache, clientID)
	}

	// Query database
	var allowedOrigins []string
	rows, err := d.db.Query(`
		SELECT unnest(allowed_origins) as origin
		FROM clients
		WHERE client_id = $1 AND active = true
	`, clientID)
	if err != nil {
		return false, fmt.Errorf("database query failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			return false, err
		}
		allowedOrigins = append(allowedOrigins, o)
	}

	// Cache the result
	d.cache[clientID] = cacheEntry{
		origins:   allowedOrigins,
		expiresAt: time.Now().Add(time.Duration(d.config.CacheTTL) * time.Second),
	}

	return d.contains(allowedOrigins, origin), nil
}

// contains checks if slice contains the string
func (d *DynamicCORS) contains(slice []string, item string) bool {
	for _, s := range slice {
		// Support wildcard subdomains (e.g., *.example.com)
		if strings.HasPrefix(s, "*.") {
			domain := s[2:] // Remove *.
			if strings.HasSuffix(item, domain) {
				return true
			}
		}
		// Exact match
		if s == item {
			return true
		}
	}
	return false
}
