package handler

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"authway/apps/central/api/pkg/claims"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/email"
	"authway/apps/central/api/pkg/invitation"
	"authway/apps/central/api/pkg/serviceclient"
	"authway/apps/central/api/pkg/tenant"
	"authway/apps/central/api/pkg/terms"
	"authway/apps/central/api/pkg/user"
	"authway/apps/central/api/pkg/webhook"
)

// The members a request body is read into are the members the published
// contract declares for it — no more (a member the contract does not name is
// one no caller can rely on, and one a second implementation will not have)
// and no fewer (a declared member the server ignores is a false promise).
func TestRequestBodies_AreTheContractsOwn(t *testing.T) {
	declared := contractRequestBodies(t)
	for _, tc := range []struct {
		op   string
		body any
	}{
		{"POST /api/v1/login-flows/{flow}/password", PasswordLoginRequest{}},
		{"POST /api/v1/login-flows/{flow}/mfa", VerifyMFALoginRequest{}},
		{"POST /api/v1/login-flows/{flow}/mfa/recovery", VerifyMFALoginRequest{}},
		{"POST /api/v1/login-flows/{flow}/terms", AcceptTermsRequest{}},
		{"POST /api/v1/login-flows/{flow}/terms/decline", DeclineTermsRequest{}},
		{"POST /api/v1/login-flows/{flow}/magic-link", sendMagicLinkRequest{}},
		{"POST /api/v1/login-flows/{flow}/magic-link/code", magicLinkCodeRequest{}},
		{"POST /api/v1/magic-links/inspect", magicLinkTokenRequest{}},
		{"POST /api/v1/magic-links/redeem", magicLinkTokenRequest{}},
		{"POST /api/v1/consent-flows/{flow}/accept", AcceptConsentRequest{}},
		{"POST /api/v1/device-flows/{flow}", DeviceCodeSubmission{}},
		{"POST /api/v1/users/password", user.ChangePasswordRequest{}},
		{"POST /api/v1/users/mfa/verify", VerifyMFARequest{}},
		{"POST /api/v1/email/send-verification", email.SendVerificationRequest{}},
		{"POST /api/v1/email/forgot-password", email.ForgotPasswordRequest{}},
		{"POST /api/v1/email/reset-password", email.ResetPasswordRequest{}},
		{"POST /api/v1/invitations", invitation.CreateInvitationRequest{}},
		{"POST /api/v1/invitations/accept", invitation.AcceptInvitationRequest{}},
		{"POST /api/v1/tenants", tenant.CreateTenantRequest{}},
		{"PUT /api/v1/tenants/{id}", tenant.UpdateTenantRequest{}},
		{"POST /api/v1/tenants/{id}/service-clients", serviceclient.CreateServiceClientRequest{}},
		{"PUT /api/v1/tenants/{id}/terms/{kind}", terms.PublishRequest{}},
		{"POST /api/v1/clients", client.CreateClientRequest{}},
		{"PUT /api/v1/clients/{id}", client.UpdateClientRequest{}},
		{"POST /api/v1/webhooks", webhook.CreateWebhookRequest{}},
		{"PATCH /api/v1/webhooks/{id}", webhook.UpdateWebhookRequest{}},
		{"PUT /api/v1/users/{id}", user.UpdateUserRequest{}},
		{"PATCH /api/v1/users/{id}/claims", claims.PatchClaimsRequest{}},
	} {
		inContract, ok := declared[tc.op]
		if !ok {
			t.Errorf("%s: the contract declares no JSON body", tc.op)
			continue
		}
		accepted := jsonMembers(reflect.TypeOf(tc.body))
		if extra, missing := setDiff(accepted, inContract), setDiff(inContract, accepted); len(extra) > 0 || len(missing) > 0 {
			t.Errorf("%s: read but not in the contract %v; in the contract but not read %v", tc.op, extra, missing)
		}
	}
}

// contractRequestBodies maps "METHOD path" to the members of its JSON body.
func contractRequestBodies(t *testing.T) map[string]map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "packages", "contract", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	type body struct {
		Ref     string `yaml:"$ref"`
		Content map[string]struct {
			Schema struct {
				Properties map[string]any `yaml:"properties"`
			} `yaml:"schema"`
		} `yaml:"content"`
	}
	var doc struct {
		// A path item holds its parameters beside its operations, so each
		// operation is decoded on its own.
		Paths      map[string]map[string]yaml.Node `yaml:"paths"`
		Components struct {
			RequestBodies map[string]body `yaml:"requestBodies"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]bool{}
	for path, item := range doc.Paths {
		for method, node := range item {
			var op struct {
				RequestBody *body `yaml:"requestBody"`
			}
			if method == "parameters" || node.Decode(&op) != nil || op.RequestBody == nil {
				continue
			}
			b := *op.RequestBody
			if b.Ref != "" {
				b = doc.Components.RequestBodies[b.Ref[strings.LastIndex(b.Ref, "/")+1:]]
			}
			media, ok := b.Content["application/json"]
			if !ok {
				continue
			}
			members := map[string]bool{}
			for name := range media.Schema.Properties {
				members[name] = true
			}
			out[strings.ToUpper(method)+" "+path] = members
		}
	}
	return out
}

func jsonMembers(typ reflect.Type) map[string]bool {
	out := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			out[name] = true
		}
	}
	return out
}

func setDiff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
