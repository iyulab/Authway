package delegation

import (
	"context"
	"encoding/base64"
	"net/url"
	"testing"
	"time"

	"authway/apps/central/api/internal/hydra"
	"authway/apps/central/api/pkg/crypto"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type standings struct {
	Service
	byHandle map[string]*Standing
	byID     map[uuid.UUID]*Standing
}

func (s standings) Resolve(_ context.Context, handle string) (*Standing, error) {
	if d, ok := s.byHandle[handle]; ok {
		return d, nil
	}
	return nil, ErrNotFound
}

func (s standings) Standing(_ context.Context, id uuid.UUID) (*Standing, error) {
	if d, ok := s.byID[id]; ok {
		return d, nil
	}
	return nil, ErrNotFound
}

type engineTrust struct{ trusted []hydra.TrustedIssuer }

func (e *engineTrust) TrustedIssuers(issuer string) ([]hydra.TrustedIssuer, error) {
	var out []hydra.TrustedIssuer
	for _, t := range e.trusted {
		if t.Issuer == issuer {
			out = append(out, t)
		}
	}
	return out, nil
}
func (e *engineTrust) TrustIssuer(t hydra.TrustedIssuer) error {
	t.ID = uuid.NewString()
	e.trusted = append(e.trusted, t)
	return nil
}
func (e *engineTrust) DistrustIssuer(id string) error {
	kept := e.trusted[:0]
	for _, t := range e.trusted {
		if t.ID != id {
			kept = append(kept, t)
		}
	}
	e.trusted = kept
	return nil
}

type clientScopes map[string][]string

func (c clientScopes) ExchangeScopes(id string) ([]string, []string, bool, error) {
	s, ok := c[id]
	return s, []string{"https://orders.example"}, ok, nil
}

func newTestExchanger(t *testing.T) (*Exchanger, *Standing, *engineTrust) {
	t.Helper()
	sealer, _ := crypto.NewCipher(testSealingKey)
	k, _ := newSigningKey(sealer)
	signer := mustSigner(t, sealer, k)
	d := &Standing{ID: uuid.New(), TenantID: uuid.New(), UserID: uuid.New(), ClientID: "agent", Scopes: []string{"openid", "orders.read"}, UserActive: true, TenantActive: true, ClientActive: true}
	trust := &engineTrust{}
	x := NewExchanger(standings{byHandle: map[string]*Standing{"h": d}, byID: map[uuid.UUID]*Standing{d.ID: d}}, signer, trust,
		clientScopes{"agent": {"openid", "orders.read", "orders.write", "delegation"}, "other": {"openid"}}, "https://api.example/")
	return x, d, trust
}

func exchangeForm(extra map[string]string) url.Values {
	f := url.Values{"subject_token": {"h"}, "subject_token_type": {SubjectTokenType}, "client_id": {"agent"}, "client_secret": {"s"}}
	for k, v := range extra {
		f.Set(k, v)
	}
	return f
}

func TestPrepare_SignsAnAssertionForTheUserAndPassesTheClientsAuthenticationOn(t *testing.T) {
	x, d, trust := newTestExchanger(t)
	got, refused := x.Prepare(context.Background(), exchangeForm(nil), "agent")
	if refused != nil {
		t.Fatalf("refused: %v", refused)
	}
	f := got.Form
	if f.Get("grant_type") != hydra.JWTBearerGrant || f.Get("client_id") != "agent" || f.Get("client_secret") != "s" || f.Get("scope") != "openid orders.read" {
		t.Fatalf("engine form = %v", f)
	}
	claims, err := x.signer.Verify(f.Get("assertion"))
	if err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != d.UserID.String() || claims["azp"] != "agent" || claims["iss"] != "https://api.example/delegation/agent" || claims["aud"] != "https://api.example/oauth2/token" {
		t.Fatalf("assertion claims = %v", claims)
	}
	if len(trust.trusted) != 1 || trust.trusted[0].Issuer != "https://api.example/delegation/agent" {
		t.Fatalf("trust = %+v", trust.trusted)
	}
	// Trust is made once, then remembered.
	x.Prepare(context.Background(), exchangeForm(nil), "agent")
	if len(trust.trusted) != 1 {
		t.Fatalf("trust made %d times", len(trust.trusted))
	}
}

func TestPrepare_Refusals(t *testing.T) {
	cases := []struct {
		name   string
		form   map[string]string
		client string
		code   string
	}{
		{"another subject token type", map[string]string{"subject_token_type": AccessTokenType}, "agent", "invalid_request"},
		{"an actor token", map[string]string{"actor_token": "x"}, "agent", "invalid_request"},
		{"a refresh token asked for", map[string]string{"requested_token_type": "urn:ietf:params:oauth:token-type:refresh_token"}, "agent", "invalid_request"},
		{"no client", nil, "", "invalid_client"},
		{"an unknown handle", map[string]string{"subject_token": "nope"}, "agent", "invalid_grant"},
		{"another client's handle", nil, "other", "invalid_grant"},
		{"a scope beyond the delegation", map[string]string{"scope": "orders.write"}, "agent", "invalid_scope"},
		{"an audience not registered", map[string]string{"audience": "https://elsewhere.example"}, "agent", "invalid_target"},
	}
	for _, tc := range cases {
		x, _, _ := newTestExchanger(t)
		_, refused := x.Prepare(context.Background(), exchangeForm(tc.form), tc.client)
		if refused == nil || refused.Code != tc.code {
			t.Errorf("%s: got %v, want %s", tc.name, refused, tc.code)
		}
	}
	x, d, _ := newTestExchanger(t)
	d.UserActive = false
	if _, refused := x.Prepare(context.Background(), exchangeForm(nil), "agent"); refused == nil || refused.Code != "invalid_grant" {
		t.Errorf("a suspended user: %v", refused)
	}
}

func TestJudge_NamesTheActorOnlyForItsOwnAssertionOfAUsableDelegation(t *testing.T) {
	x, d, _ := newTestExchanger(t)
	prepared, _ := x.Prepare(context.Background(), exchangeForm(nil), "agent")
	assertion := prepared.Form.Get("assertion")

	ok := x.Judge(context.Background(), assertion, "agent")
	if ok.Refuse != "" || ok.Claims["tenant_id"] != d.TenantID.String() {
		t.Fatalf("judge = %+v", ok)
	}
	if act, _ := ok.Claims["act"].(map[string]any); act["sub"] != "agent" {
		t.Fatalf("act = %v", ok.Claims["act"])
	}
	if got := x.Judge(context.Background(), assertion, "other"); got.Refuse == "" {
		t.Fatal("an assertion for one client was accepted for another")
	}
	forged, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"azp": "agent", "exp": time.Now().Add(time.Minute).Unix()}).SignedString([]byte("k"))
	if got := x.Judge(context.Background(), forged, "agent"); got.Refuse == "" {
		t.Fatal("an assertion this deployment did not sign was accepted")
	}
	d.ClientActive = false
	if got := x.Judge(context.Background(), assertion, "agent"); got.Refuse == "" {
		t.Fatal("a delegation of a turned-off client was accepted")
	}
}

func TestClientFromRequest(t *testing.T) {
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("my%3Aclient:secret"))
	if got := ClientFromRequest(basic, url.Values{}); got != "my:client" {
		t.Errorf("basic: %q", got)
	}
	if got := ClientFromRequest("", url.Values{"client_id": {"form-client"}}); got != "form-client" {
		t.Errorf("form: %q", got)
	}
	a, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": "key-client", "sub": "key-client"}).SignedString([]byte("k"))
	keyed := url.Values{"client_assertion_type": {ClientAssertionType}, "client_assertion": {a}}
	if got := ClientFromRequest("", keyed); got != "key-client" {
		t.Errorf("assertion: %q", got)
	}
	if got := ClientFromRequest("", url.Values{}); got != "" {
		t.Errorf("none: %q", got)
	}
}

// A request that authenticates in two ways, or names two clients, names
// none: the server would check one, and the other is what would be believed.
func TestClientFromRequest_NamesNoneWhenTheWaysDisagree(t *testing.T) {
	other, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": "attacker", "sub": "attacker"}).SignedString([]byte("k"))
	victim := "Basic " + base64.StdEncoding.EncodeToString([]byte("victim:wrong"))
	split, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": "one", "sub": "two"}).SignedString([]byte("k"))
	for name, c := range map[string]struct {
		authorization string
		form          url.Values
	}{
		"basic and an assertion":       {victim, url.Values{"client_assertion_type": {ClientAssertionType}, "client_assertion": {other}}},
		"basic and a form secret":      {victim, url.Values{"client_secret": {"s"}}},
		"basic and another client_id":  {victim, url.Values{"client_id": {"attacker"}}},
		"an assertion for another id":  {"", url.Values{"client_id": {"victim"}, "client_assertion_type": {ClientAssertionType}, "client_assertion": {other}}},
		"an assertion and a secret":    {"", url.Values{"client_secret": {"s"}, "client_assertion_type": {ClientAssertionType}, "client_assertion": {other}}},
		"an assertion of another type": {"", url.Values{"client_assertion_type": {"urn:other"}, "client_assertion": {other}}},
		"issuer and subject differ":    {"", url.Values{"client_assertion_type": {ClientAssertionType}, "client_assertion": {split}}},
		"a bearer header":              {"Bearer x", url.Values{"client_id": {"victim"}}},
	} {
		if got := ClientFromRequest(c.authorization, c.form); got != "" {
			t.Errorf("%s: named %q", name, got)
		}
	}
	if got := ClientFromRequest(victim, url.Values{"client_id": {"victim"}}); got != "victim" {
		t.Errorf("basic with the same client_id: %q", got)
	}
}

// A deleted client's trust at the authorization server goes with it, and the
// next exchange (were the client to come back) sets it up again.
func TestDistrust_RemovesTheClientsTrust(t *testing.T) {
	x, _, trust := newTestExchanger(t)
	if _, refused := x.Prepare(context.Background(), exchangeForm(nil), "agent"); refused != nil {
		t.Fatal(refused)
	}
	if err := x.Distrust("agent"); err != nil {
		t.Fatal(err)
	}
	if len(trust.trusted) != 0 {
		t.Fatalf("trust left behind: %+v", trust.trusted)
	}
	if _, refused := x.Prepare(context.Background(), exchangeForm(nil), "agent"); refused != nil || len(trust.trusted) != 1 {
		t.Fatalf("trust not set up again: %v %d", refused, len(trust.trusted))
	}
}

// A refusal of a delegation that exists names it, so it can be recorded.
func TestPrepare_ARefusalOfAKnownDelegationNamesIt(t *testing.T) {
	x, d, _ := newTestExchanger(t)
	got, refused := x.Prepare(context.Background(), exchangeForm(map[string]string{"scope": "orders.write"}), "agent")
	if refused == nil || got == nil || got.Delegation.ID != d.ID || got.Form != nil {
		t.Fatalf("got %+v, %v", got, refused)
	}
	if got, _ := x.Prepare(context.Background(), exchangeForm(map[string]string{"subject_token": "nope"}), "agent"); got != nil {
		t.Fatal("an unknown handle named a delegation")
	}
}
