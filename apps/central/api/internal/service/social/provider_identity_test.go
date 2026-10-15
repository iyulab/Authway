package social

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/config"
	"authway/apps/central/api/pkg/client"
	"authway/apps/central/api/pkg/user"
)

// providerResponses answers a provider's HTTP calls from a table keyed by a
// fragment of the URL, so a test states exactly what the provider asserts.
type providerResponses map[string]string

func (p providerResponses) RoundTrip(req *http.Request) (*http.Response, error) {
	// The longest matching fragment wins, so ".../user/emails" is not
	// answered by the entry for ".../user".
	best := ""
	for fragment := range p {
		if strings.Contains(req.URL.String(), fragment) && len(fragment) > len(best) {
			best = fragment
		}
	}
	if best == "" {
		return nil, errors.New("unexpected provider call: " + req.URL.String())
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(p[best])),
		Request:    req,
	}, nil
}

func (p providerResponses) client() *http.Client { return &http.Client{Transport: p} }

// unsignedIDToken builds a JWT-shaped ID token carrying claims. The services
// read an ID token they received from the provider's token endpoint and do not
// check its signature, so the signature part is a placeholder.
func unsignedIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"none"}`)) + "." + enc.EncodeToString(payload) + ".sig"
}

// oneTenantClients resolves every client id to one tenant.
type oneTenantClients struct {
	client.Service
	tenantID uuid.UUID
}

func (c oneTenantClients) GetByClientID(string) (*client.Client, error) {
	return &client.Client{TenantID: c.tenantID}, nil
}

const victimEmail = "victim@example.com"

// A person who controls a Microsoft directory can set the mailbox address of
// an account in it to anything. Signing in with such an account must not reach
// the Authway account that owns the address.
func TestMicrosoftSignIn_UnverifiedAddressDoesNotReachAnExistingAccount(t *testing.T) {
	tenantID := uuid.New()
	accounts := newFakeAccounts()
	victim := accounts.add(tenantID, victimEmail)

	svc := NewMicrosoftService(&config.MicrosoftOAuthConfig{ClientID: "ms-app", TenantID: "common"},
		accounts, &stubGate{invited: true}, oneTenantClients{tenantID: tenantID}, zap.NewNop())
	svc.httpClient = providerResponses{
		"/oauth2/v2.0/token":     `{"access_token":"at","id_token":"` + unsignedIDToken(t, map[string]any{"sub": "attacker-sub", "email": victimEmail}) + `"}`,
		"graph.microsoft.com/v1": `{"id":"attacker-object-id","mail":"` + victimEmail + `","displayName":"Mallory"}`,
	}.client()

	got, err := svc.HandleCallbackForClient(context.Background(), "code", "state", "authway_client")
	if err == nil {
		t.Fatalf("signed in as %s (victim is %s) on an address the provider did not verify", got.ID, victim.ID)
	}
	if !errors.Is(err, ErrEmailNotVerified) {
		t.Errorf("error = %v, want ErrEmailNotVerified", err)
	}
}

// fakeAccounts is the part of the user service sign-in touches, in memory.
type fakeAccounts struct {
	user.Service
	users     []*user.User
	linked    int   // LinkProvider calls that changed an account
	lookupErr error // returned by GetByProviderSubject when set
	// linkedMeanwhile, when set, is linked to the provider account by
	// "another sign-in" just before the next LinkProvider call, which is then
	// refused as the database would refuse it.
	linkedMeanwhile *user.User
}

func (f *fakeAccounts) GetByProviderSubject(tenantID uuid.UUID, provider, subject string) (*user.User, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	for _, u := range f.users {
		if u.TenantID == tenantID && linkedSubject(u, provider) == subject {
			return u, nil
		}
	}
	return nil, user.ErrUserNotFound
}

func (f *fakeAccounts) LinkProvider(id uuid.UUID, provider, subject string) error {
	if other := f.linkedMeanwhile; other != nil {
		f.linkedMeanwhile = nil
		setLinkedSubject(other, provider, subject)
		return user.ErrProviderSubjectTaken
	}
	for _, u := range f.users {
		if u.ID == id {
			setLinkedSubject(u, provider, subject)
			u.EmailVerified = true
			f.linked++
			return nil
		}
	}
	return user.ErrUserNotFound
}

func newFakeAccounts() *fakeAccounts { return &fakeAccounts{} }

func (f *fakeAccounts) add(tenantID uuid.UUID, email string) *user.User {
	u := &user.User{ID: uuid.New(), TenantID: tenantID, Email: email, Provider: "local"}
	f.users = append(f.users, u)
	return u
}

func (f *fakeAccounts) GetByEmailAndTenant(tenantID uuid.UUID, email string) (*user.User, error) {
	for _, u := range f.users {
		if u.TenantID == tenantID && u.Email == email {
			return u, nil
		}
	}
	return nil, user.ErrUserNotFound
}

func (f *fakeAccounts) Create(tenantID uuid.UUID, req *user.CreateUserRequest) (*user.User, error) {
	u := f.add(tenantID, req.Email)
	if req.Name != "" {
		u.Name = &req.Name
	}
	return u, nil
}

func (f *fakeAccounts) Update(id uuid.UUID, req *user.UpdateUserRequest) (*user.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			if req.AvatarURL != nil {
				u.AvatarURL = req.AvatarURL
			}
			return u, nil
		}
	}
	return nil, errors.New("user not found")
}
