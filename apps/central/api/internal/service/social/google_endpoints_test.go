package social

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"authway/apps/central/api/internal/config"
)

// Google is reached at its own endpoints unless the configuration names
// others — which is how a stand-in provider is put under test.

func TestGoogle_UsesItsOwnEndpointsByDefault(t *testing.T) {
	g := NewGoogleService(&config.GoogleOAuthConfig{ClientID: "id", RedirectURL: "http://localhost/cb"}, nil, nil, nil, zap.NewNop())
	if got := g.GetAuthURL("state-1"); !strings.HasPrefix(got, "https://accounts.google.com/o/oauth2/v2/auth?") {
		t.Errorf("authorization URL %q, want Google's own", got)
	}
}

func TestGoogle_ReachesTheConfiguredEndpoints(t *testing.T) {
	var asked []string
	standIn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			w.Write([]byte(`{"access_token":"at-1"}`))
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer at-1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"id":"sub-1","email":"user@example.com","verified_email":true}`))
		}
	}))
	defer standIn.Close()

	g := NewGoogleService(&config.GoogleOAuthConfig{
		ClientID:    "id",
		RedirectURL: "http://localhost/cb",
		AuthURL:     standIn.URL + "/authorize",
		TokenURL:    standIn.URL + "/token",
		UserInfoURL: standIn.URL + "/userinfo",
	}, nil, nil, nil, zap.NewNop())

	if got := g.GetAuthURL("state-1"); !strings.HasPrefix(got, standIn.URL+"/authorize?") || !strings.Contains(got, "state=state-1") {
		t.Errorf("authorization URL %q, want the configured one carrying the state", got)
	}
	tokens, err := g.ExchangeCode(context.Background(), "code-1")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	info, err := g.GetUserInfo(context.Background(), tokens.AccessToken)
	if err != nil || info.ID != "sub-1" || !info.VerifiedEmail {
		t.Fatalf("user info %+v, %v", info, err)
	}
	if strings.Join(asked, ", ") != "POST /token, GET /userinfo" {
		t.Errorf("the stand-in was asked %v", asked)
	}
}
