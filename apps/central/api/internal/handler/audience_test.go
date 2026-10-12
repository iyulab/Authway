package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"authway/apps/central/api/internal/hydra"
)

// The audience a consent grants is the client's registered resource servers,
// narrowed to those a request names — and never one the client is not
// registered for, because the authorization server would put it in the token.
func TestGrantedAudience(t *testing.T) {
	client := &hydra.OAuth2Client{Audience: []string{"https://api.a", "https://api.b"}}
	for name, tc := range map[string]struct {
		req  *hydra.ConsentRequest
		want string
	}{
		"nothing requested: all the client is registered for": {&hydra.ConsentRequest{Client: client}, "https://api.a,https://api.b"},
		"one requested":               {&hydra.ConsentRequest{Client: client, RequestedAudience: []string{"https://api.b"}}, "https://api.b"},
		"one of two not registered":   {&hydra.ConsentRequest{Client: client, RequestedAudience: []string{"https://api.b", "https://api.other"}}, "https://api.b"},
		"only unregistered requested": {&hydra.ConsentRequest{Client: client, RequestedAudience: []string{"https://api.other"}}, ""},
		"a client with none":          {&hydra.ConsentRequest{Client: &hydra.OAuth2Client{}}, ""},
		"no client in the request":    {&hydra.ConsentRequest{}, ""},
	} {
		if got := strings.Join(grantedAudience(tc.req), ","); got != tc.want {
			t.Errorf("%s: granted %q, want %q", name, got, tc.want)
		}
	}
}

// The consent request names the audience the client asked for under
// requested_access_token_audience. Reading another name left it empty, and a
// requested audience was never granted.
func TestConsentRequest_ReadsTheRequestedAudience(t *testing.T) {
	var req hydra.ConsentRequest
	raw := `{"challenge":"c","requested_scope":["openid"],"requested_access_token_audience":["https://api.a"],"client":{"client_id":"app","audience":["https://api.a"]}}`
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatal(err)
	}
	if len(req.RequestedAudience) != 1 || req.RequestedAudience[0] != "https://api.a" {
		t.Errorf("requested audience = %v", req.RequestedAudience)
	}
	if req.Client == nil || len(req.Client.Audience) != 1 {
		t.Errorf("client audience = %+v", req.Client)
	}
}
