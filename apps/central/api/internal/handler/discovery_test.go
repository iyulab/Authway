package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// The provider's metadata says what the deployment offers: the authorization
// server's document, with Authway's endpoints and grants added and what no
// client can be registered for taken out.
func TestDiscovery_SaysWhatTheDeploymentOffers(t *testing.T) {
	reads := 0
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reads++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                "https://issuer.example",
			"token_endpoint":                        "https://api.example/oauth2/token",
			"response_types_supported":              []string{"code", "id_token", "token id_token code"},
			"grant_types_supported":                 []string{"authorization_code", "implicit", "client_credentials", "refresh_token"},
			"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic", "private_key_jwt", "none"},
			"frontchannel_logout_supported":         true,
			"request_parameter_supported":           true,
			"request_uri_parameter_supported":       true,
		})
	}))
	defer engine.Close()

	app := fiber.New()
	app.Get("/.well-known/openid-configuration", NewDiscovery(engine.URL, "https://api.example/", true, true, zap.NewNop()).Serve)
	read := func() map[string]any {
		res, err := app.Test(httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil))
		if err != nil || res.StatusCode != http.StatusOK {
			t.Fatalf("answer %v %v", res, err)
		}
		body, _ := io.ReadAll(res.Body)
		var doc map[string]any
		if err := json.Unmarshal(body, &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	doc := read()

	if doc["issuer"] != "https://issuer.example" {
		t.Errorf("issuer: %v", doc["issuer"])
	}
	list := func(name string) []string { return stringList(doc[name]) }
	if got := list("response_types_supported"); !slices.Equal(got, []string{"code"}) {
		t.Errorf("response types: %v", got)
	}
	grants := list("grant_types_supported")
	if slices.Contains(grants, "implicit") || !slices.Contains(grants, "urn:ietf:params:oauth:grant-type:token-exchange") || !slices.Contains(grants, "urn:authway:params:oauth:grant-type:approval") {
		t.Errorf("grants: %v", grants)
	}
	if got := list("token_endpoint_auth_methods_supported"); slices.Contains(got, "client_secret_basic") || !slices.Contains(got, "private_key_jwt") {
		t.Errorf("auth methods: %v", got)
	}
	if got := list("token_endpoint_auth_signing_alg_values_supported"); !slices.Contains(got, "ES256") {
		t.Errorf("signing algs: %v", got)
	}
	if doc["frontchannel_logout_supported"] != false || doc["request_uri_parameter_supported"] != false || doc["request_parameter_supported"] != true {
		t.Errorf("logout/request: %v %v %v", doc["frontchannel_logout_supported"], doc["request_uri_parameter_supported"], doc["request_parameter_supported"])
	}
	if doc["introspection_endpoint"] != "https://api.example/api/v1/oauth2/introspect" || doc["approval_request_endpoint"] != "https://api.example/oauth2/approval-requests" {
		t.Errorf("endpoints: %v %v", doc["introspection_endpoint"], doc["approval_request_endpoint"])
	}

	read()
	if reads != 1 {
		t.Errorf("the authorization server was read %d times within the cache's life", reads)
	}
}

// Without token exchange and approvals, neither is advertised.
func TestDiscovery_AdvertisesOnlyWhatIsOffered(t *testing.T) {
	d := NewDiscovery("http://unused", "https://api.example", false, false, zap.NewNop())
	doc := d.overlay(map[string]any{"grant_types_supported": []any{"authorization_code"}})
	if got := doc["grant_types_supported"].([]string); !slices.Equal(got, []string{"authorization_code"}) {
		t.Errorf("grants: %v", got)
	}
	if _, has := doc["approval_request_endpoint"]; has {
		t.Error("approval endpoint advertised without approvals")
	}
}
