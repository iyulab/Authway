package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/delegation"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type clientRowsByID map[string]*client.Client

func (m clientRowsByID) GetByClientID(id string) (*client.Client, error) {
	if c, ok := m[id]; ok {
		return c, nil
	}
	return nil, errors.New("not found")
}

// unsignedAssertion is an assertion whose signature the fake engine does not
// check: what is tested here is what Authway adds to the engine's answer.
func unsignedAssertion(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("not-checked-here"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEngineClientAuth(t *testing.T) {
	var asked url.Values
	var askedAuthorization string
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/revoke" {
			t.Errorf("asked %s", r.URL.Path)
		}
		_ = r.ParseForm()
		asked, askedAuthorization = r.PostForm, r.Header.Get("Authorization")
		if r.PostForm.Get("client_secret") == "right" || r.PostForm.Get("client_assertion") != "" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer engine.Close()
	mr := miniredis.RunT(t)
	agentRow, publicRow := uuid.New(), uuid.New()
	auth := NewEngineClientAuth(engine.URL, clientRowsByID{
		"agent":  {ID: agentRow, ClientID: "agent"},
		"public": {ID: publicRow, ClientID: "public", Public: true},
	}, redis.NewClient(&redis.Options{Addr: mr.Addr()}), zap.NewNop())
	ctx := context.Background()

	t.Run("a secret the server accepts names the client", func(t *testing.T) {
		id, row, ok := auth.VerifyClient(ctx, "", url.Values{"client_id": {"agent"}, "client_secret": {"right"}})
		if !ok || id != "agent" || row != agentRow {
			t.Fatalf("got %q %v %v", id, row, ok)
		}
		if asked.Get("token") == "" || asked.Get("client_secret") != "right" {
			t.Fatalf("the server was asked %v", asked)
		}
	})

	t.Run("a secret the server refuses is refused", func(t *testing.T) {
		if _, _, ok := auth.VerifyClient(ctx, "", url.Values{"client_id": {"agent"}, "client_secret": {"wrong"}}); ok {
			t.Fatal("accepted")
		}
	})

	t.Run("the Authorization header reaches the server", func(t *testing.T) {
		auth.VerifyClient(ctx, "Basic YWdlbnQ6d3Jvbmc=", url.Values{})
		if askedAuthorization != "Basic YWdlbnQ6d3Jvbmc=" {
			t.Fatalf("header %q", askedAuthorization)
		}
	})

	t.Run("a public client is not a confidential one, whatever the server says", func(t *testing.T) {
		if _, _, ok := auth.VerifyClient(ctx, "", url.Values{"client_id": {"public"}, "client_secret": {"right"}}); ok {
			t.Fatal("accepted")
		}
	})

	t.Run("an assertion is taken once", func(t *testing.T) {
		a := unsignedAssertion(t, jwt.MapClaims{"iss": "agent", "sub": "agent", "jti": "one", "exp": time.Now().Add(time.Minute).Unix()})
		form := url.Values{"client_assertion_type": {delegation.ClientAssertionType}, "client_assertion": {a}}
		if id, _, ok := auth.VerifyClient(ctx, "", form); !ok || id != "agent" {
			t.Fatalf("first use: %q %v", id, ok)
		}
		if _, _, ok := auth.VerifyClient(ctx, "", form); ok {
			t.Fatal("the same assertion was taken twice")
		}
	})

	t.Run("an assertion without an id, or living too long, is not taken", func(t *testing.T) {
		for name, claims := range map[string]jwt.MapClaims{
			"no jti":   {"iss": "agent", "sub": "agent", "exp": time.Now().Add(time.Minute).Unix()},
			"no exp":   {"iss": "agent", "sub": "agent", "jti": "two"},
			"too long": {"iss": "agent", "sub": "agent", "jti": "three", "exp": time.Now().Add(2 * time.Hour).Unix()},
		} {
			form := url.Values{"client_assertion_type": {delegation.ClientAssertionType}, "client_assertion": {unsignedAssertion(t, claims)}}
			if _, _, ok := auth.VerifyClient(ctx, "", form); ok {
				t.Errorf("%s: accepted", name)
			}
		}
	})

	t.Run("a request that authenticates as one client and names another is refused", func(t *testing.T) {
		// The server checks the assertion (its own client); the header names the victim.
		a := unsignedAssertion(t, jwt.MapClaims{"iss": "public", "sub": "public", "jti": "five", "exp": time.Now().Add(time.Minute).Unix()})
		form := url.Values{"client_assertion_type": {delegation.ClientAssertionType}, "client_assertion": {a}}
		if _, _, ok := auth.VerifyClient(ctx, "Basic YWdlbnQ6d3Jvbmc=", form); ok {
			t.Fatal("accepted as the client the header names")
		}
	})

	t.Run("an assertion the server refuses takes no id", func(t *testing.T) {
		refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
		defer refusing.Close()
		strict := NewEngineClientAuth(refusing.URL, clientRowsByID{"agent": {ID: agentRow}}, redis.NewClient(&redis.Options{Addr: mr.Addr()}), zap.NewNop())
		a := unsignedAssertion(t, jwt.MapClaims{"iss": "agent", "sub": "agent", "jti": "four", "exp": time.Now().Add(time.Minute).Unix()})
		if _, _, ok := strict.VerifyClient(ctx, "", url.Values{"client_assertion_type": {delegation.ClientAssertionType}, "client_assertion": {a}}); ok {
			t.Fatal("accepted")
		}
		if _, _, ok := auth.VerifyClient(ctx, "", url.Values{"client_assertion_type": {delegation.ClientAssertionType}, "client_assertion": {a}}); !ok {
			t.Fatal("a refused assertion used up its id")
		}
	})
}
