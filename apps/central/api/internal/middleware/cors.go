package middleware

import (
	"strings"
	"sync"
	"time"

	"authway/apps/central/api/pkg/tenantscope"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// corsAllowHeaders lists every request header a browser caller may send. A
// header the API reads but this list omits fails the preflight, and the
// browser then drops the request before it reaches a handler.
var corsAllowHeaders = []string{
	"Origin",
	"Content-Type",
	"Accept",
	"Authorization",
	tenantscope.Header,
	"Request-Id",
	"Traceparent",
	"Tracestate",
}

const corsAllowMethods = "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS"

// The path lists below hold exact paths; an entry ending in "/" matches
// everything under it.

// publicDocuments are readable by a page on any origin: they describe the
// deployment and carry nothing about a user.
var publicDocuments = []string{
	"/.well-known/authway-config",
	"/api/v1/capabilities",
}

// clientAuthenticatedPaths are called by OAuth clients, which authenticate
// themselves in the request (a secret, a signed assertion, or PKCE for a
// public client). A browser application on any origin exchanges its code at
// the token endpoint, as it did when the authorization server answered there
// directly — it admits every origin too. No cookies: a browser session
// authenticates nothing here.
var clientAuthenticatedPaths = []string{
	"/oauth2/token",
}

// userTokenPaths are the routes a signed-in user calls with their own access
// token. A browser application reaches them from the origins registered on
// its client.
var userTokenPaths = []string{
	"/api/v1/profile/",
	"/api/v1/users/mfa",
	"/api/v1/users/mfa/",
	"/api/v1/logout",
}

// CORS decides which pages may call the API from a browser:
//
//   - the public documents, from any origin;
//   - the token endpoint, from any origin, without cookies;
//   - everything, with cookies, from the deployment's own origins (its login
//     screens and console);
//   - the routes a user calls with their own access token, without cookies,
//     from an origin registered on a client (registered reports whether an
//     origin is). The caller still has to present a valid token; the origin
//     only decides whether the browser lets the page read the answer.
//
// registered may be nil, which admits no registered origin.
func CORS(deploymentOrigins []string, registered func(origin string) bool) fiber.Handler {
	deployment := cors.New(cors.Config{
		AllowOrigins:     strings.Join(deploymentOrigins, ","),
		AllowMethods:     corsAllowMethods,
		AllowHeaders:     strings.Join(corsAllowHeaders, ","),
		AllowCredentials: true,
	})
	own := make(map[string]bool, len(deploymentOrigins))
	for _, origin := range deploymentOrigins {
		own[origin] = true
	}

	return func(c *fiber.Ctx) error {
		origin := c.Get(fiber.HeaderOrigin)
		if origin == "" || own[origin] {
			return deployment(c)
		}
		path := c.Path()
		switch {
		case matchesAny(path, publicDocuments), matchesAny(path, clientAuthenticatedPaths):
			return admitOrigin(c, "*")
		case registered != nil && matchesAny(path, userTokenPaths) && registered(origin):
			return admitOrigin(c, origin)
		}
		return deployment(c)
	}
}

// admitOrigin lets a page on allowOrigin read the answer, without cookies.
// A preflight is answered here; any other request goes on to its handler.
func admitOrigin(c *fiber.Ctx, allowOrigin string) error {
	c.Set(fiber.HeaderAccessControlAllowOrigin, allowOrigin)
	if allowOrigin != "*" {
		c.Vary(fiber.HeaderOrigin)
	}
	if c.Method() != fiber.MethodOptions || c.Get(fiber.HeaderAccessControlRequestMethod) == "" {
		return c.Next()
	}
	c.Vary(fiber.HeaderAccessControlRequestMethod)
	c.Vary(fiber.HeaderAccessControlRequestHeaders)
	c.Set(fiber.HeaderAccessControlAllowMethods, corsAllowMethods)
	c.Set(fiber.HeaderAccessControlAllowHeaders, strings.Join(corsAllowHeaders, ","))
	return c.SendStatus(fiber.StatusNoContent)
}

// matchesAny reports whether path is one of patterns, where a pattern ending
// in "/" matches every path under it.
func matchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(p, "/") {
			if strings.HasPrefix(path, p) {
				return true
			}
		} else if path == p {
			return true
		}
	}
	return false
}

// RememberOrigins wraps a lookup of registered origins so that a burst of
// preflights does not become a burst of queries: an answer is kept for ttl.
// A newly registered origin is therefore admitted, and a removed one refused,
// at most ttl later. A failed lookup refuses the origin and is not kept.
func RememberOrigins(lookup func(origin string) (bool, error), ttl time.Duration) func(origin string) bool {
	type answer struct {
		allowed bool
		until   time.Time
	}
	var (
		mu      sync.Mutex
		answers = map[string]answer{}
	)
	const maxRemembered = 4096

	return func(origin string) bool {
		now := time.Now()
		mu.Lock()
		if a, ok := answers[origin]; ok && now.Before(a.until) {
			mu.Unlock()
			return a.allowed
		}
		mu.Unlock()

		allowed, err := lookup(origin)
		if err != nil {
			return false
		}

		mu.Lock()
		if len(answers) >= maxRemembered {
			// Origins are caller-supplied; drop everything rather than grow
			// without bound.
			answers = map[string]answer{}
		}
		answers[origin] = answer{allowed: allowed, until: now.Add(ttl)}
		mu.Unlock()
		return allowed
	}
}
