package maillink

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The deploy-time smoke test cannot cover this. A single-page app served with a
// 200-rewrite fallback answers *every* path with the same shell — a cold GET to
// /definitely-not-a-route returns 200 just like a real route does (verified
// against auth.authway.in). So "the link resolves" tells us the host is right
// and the UI is live, and nothing at all about whether the route exists.
//
// This is not hypothetical: the magic-link mail once pointed at
// /auth/magic-link/verify, an API route the UI never served, and every emailed
// link was dead. Only comparing against the UI's router catches that.
func TestEveryMailLinkPathHasAMatchingUIRoute(t *testing.T) {
	routes := parseUIRoutes(t)

	for _, path := range AllPaths() {
		if !routes[path] {
			t.Errorf("mail links point at %q but auth-ui declares no such route; declared: %v",
				path, sortedKeys(routes))
		}
	}
}

func TestBuildersProduceTheDeclaredPaths(t *testing.T) {
	const front = "https://auth.example.test"

	cases := []struct {
		got  string
		want string
	}{
		{InvitationAccept(front, "abc"), front + "/invitation/accept?token=abc"},
		{MagicLink(front, "abc"), front + "/magic-link?token=abc"},
		{VerifyEmail(front, "abc"), front + "/verify-email?token=abc"},
		{ResetPassword(front, "abc"), front + "/reset-password?token=abc"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestTokensAreQueryEscaped(t *testing.T) {
	// Magic-link tokens are base64 and can contain '+' and '=', which change
	// meaning in a query string if passed through raw.
	got := MagicLink("https://auth.example.test", "a+b/c=")
	if strings.Contains(got, "a+b/c=") {
		t.Errorf("token was not escaped: %s", got)
	}
}

var routeRe = regexp.MustCompile(`<Route\s+path="([^"]+)"`)

func parseUIRoutes(t *testing.T) map[string]bool {
	t.Helper()

	appTSX := filepath.Join("..", "..", "..", "..", "branding", "auth-ui", "src", "App.tsx")
	src, err := os.ReadFile(appTSX)
	if err != nil {
		t.Fatalf("cannot read the auth UI router at %s: %v\n"+
			"If the UI moved, update this path — do not delete the test; it is the "+
			"only thing tying emailed links to routes that actually exist.", appTSX, err)
	}

	routes := map[string]bool{}
	for _, m := range routeRe.FindAllStringSubmatch(string(src), -1) {
		routes[m[1]] = true
	}
	if len(routes) == 0 {
		t.Fatalf("no <Route path=...> found in %s — the parser or the UI's routing style changed", appTSX)
	}
	return routes
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
