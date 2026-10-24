package main

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"authway/apps/central/api/internal/config"
	"go.uber.org/zap"
	"go.yaml.in/yaml/v3"
)

// Served on purpose without being in the contract, each with why. Everything
// else the API serves is in packages/contract/openapi.yaml, and everything
// there is served.
var outsideContract = map[string]string{
	// Where the authorization server sends the browser (docs/api/login-flows.md).
	"GET /flows/login":   "browser entry point",
	"GET /flows/consent": "browser entry point",
	"GET /flows/logout":  "browser entry point",
	"GET /flows/device":  "browser entry point",
	// Where an identity provider sends the browser back.
	"GET /auth/google/callback":    "provider callback",
	"GET /auth/github/callback":    "provider callback",
	"GET /auth/microsoft/callback": "provider callback",
	"GET /auth/apple/callback":     "provider callback",
	"POST /auth/apple/callback":    "provider callback (form_post)",
	// OAuth endpoints answered in the protocol's own form encoding: the token
	// endpoint discovery advertises (RFC 6749, exchange RFC 8693), and the
	// approval request the contract describes in schema ApprovalRequest
	// (docs/api/delegation.md).
	"POST /oauth2/token":             "OAuth token endpoint",
	"POST /oauth2/approval-requests": "approval request (schema ApprovalRequest)",
	// Called by the authorization server alone, with the hook key.
	"POST /internal/token-hook": "authorization server's token hook",
	// The console's own session, signed in with the shared admin password.
	"POST /admin/login":   "console session",
	"POST /admin/logout":  "console session",
	"GET /admin/validate": "console session",
	"GET /admin/info":     "console session",
}

// The server as configured here registers every optional route: the token
// hook (a hook key) and token exchange with approvals (a signing key, which
// the server creates in the database when there is none).
func TestRoutesMatchTheContract(t *testing.T) {
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping the route table against a live Postgres")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("MIGRATE_SMOKE_DSN: %v", err)
	}
	password, _ := u.User.Password()
	t.Setenv("AUTHWAY_DATABASE_HOST", u.Hostname())
	t.Setenv("AUTHWAY_DATABASE_PORT", u.Port())
	t.Setenv("AUTHWAY_DATABASE_USER", u.User.Username())
	t.Setenv("AUTHWAY_DATABASE_PASSWORD", password)
	t.Setenv("AUTHWAY_DATABASE_NAME", strings.TrimPrefix(u.Path, "/"))
	t.Setenv("AUTHWAY_DATABASE_SSL_MODE", u.Query().Get("sslmode"))
	t.Setenv("AUTHWAY_REDIS_ENABLED", "false")
	t.Setenv("AUTHWAY_HYDRA_TOKEN_HOOK_KEY", "route-table-test")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	srv, err := newServer(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	served := map[string]bool{}
	for _, r := range srv.app.GetRoutes(true) {
		if r.Method == "HEAD" { // Fiber adds one to every GET
			continue
		}
		served[r.Method+" "+normalize(r.Path)] = true
	}
	contract := contractRoutes(t)
	t.Logf("%d routes served, %d operations in the contract, %d outside it on purpose", len(served), len(contract), len(outsideContract))

	var missing, extra, stale []string
	for route := range contract {
		if !served[route] {
			missing = append(missing, route)
		}
	}
	for route := range served {
		if contract[route] {
			if _, listed := outsideContract[route]; listed {
				stale = append(stale, route)
			}
			continue
		}
		if _, listed := outsideContract[route]; !listed {
			extra = append(extra, route)
		}
	}
	for route := range outsideContract {
		if !served[route] {
			stale = append(stale, route)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("in the contract but not served (%d):\n  %s", len(missing), strings.Join(missing, "\n  "))
	}
	if len(extra) > 0 {
		t.Errorf("served but not in the contract — describe them there, or list them in outsideContract with why (%d):\n  %s", len(extra), strings.Join(extra, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("outsideContract lists routes that are in the contract or no longer served (%d):\n  %s", len(stale), strings.Join(stale, "\n  "))
	}
}

var pathParam = regexp.MustCompile(`\{[^}]+\}|:[A-Za-z0-9_]+\??`)

// normalize names every path parameter the same, so `/users/:id` (Fiber)
// and `/users/{user_id}` (the contract) compare equal.
func normalize(path string) string {
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return pathParam.ReplaceAllString(path, "{}")
}

func contractRoutes(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "packages", "contract", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read the contract: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse the contract: %v", err)
	}
	methods := map[string]bool{"get": true, "put": true, "post": true, "delete": true, "patch": true, "options": true}
	routes := map[string]bool{}
	for path, item := range doc.Paths {
		for method := range item {
			if methods[method] {
				routes[strings.ToUpper(method)+" "+normalize(path)] = true
			}
		}
	}
	if len(routes) == 0 {
		t.Fatal("the contract lists no operations")
	}
	return routes
}
