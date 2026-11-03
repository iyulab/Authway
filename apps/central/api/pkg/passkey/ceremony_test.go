package passkey

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"authway/apps/central/api/pkg/ephemeral"
	"authway/apps/central/api/pkg/passkey/passkeytest"
	"authway/apps/central/api/pkg/user"
)

const (
	testRPID   = "auth.example.com"
	testOrigin = "https://auth.example.com"
)

// fakeKeys is the passkey store, in memory.
type fakeKeys struct {
	mu   sync.Mutex
	rows []user.Passkey
	// findErr, when set, is what Find fails with: the store is down.
	findErr error
	// lastUse is what the last RecordUse stored.
	lastUse struct {
		id    uuid.UUID
		count uint32
		flags byte
	}
}

func (f *fakeKeys) List(userID uuid.UUID) ([]user.Passkey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []user.Passkey
	for _, r := range f.rows {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeKeys) Add(p *user.Passkey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.TenantID == p.TenantID && bytes.Equal(r.CredentialID, p.CredentialID) {
			return user.ErrPasskeyTaken
		}
	}
	f.rows = append(f.rows, *p)
	return nil
}

func (f *fakeKeys) Remove(userID, id uuid.UUID) error { return errors.New("not used here") }

func (f *fakeKeys) Find(tenantID uuid.UUID, credentialID []byte) (*user.Passkey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.findErr != nil {
		return nil, f.findErr
	}
	for _, r := range f.rows {
		if r.TenantID == tenantID && bytes.Equal(r.CredentialID, credentialID) {
			return &r, nil
		}
	}
	return nil, user.ErrPasskeyNotFound
}

// RecordUse keeps what the store would, so the next sign-in is checked against
// the counter and flags of this one.
func (f *fakeKeys) RecordUse(id uuid.UUID, count uint32, flags byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastUse.id, f.lastUse.count, f.lastUse.flags = id, count, flags
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows[i].SignCount, f.rows[i].Flags = int64(count), int16(flags)
		}
	}
	return nil
}

// fakeAccounts is the accounts, in memory.
type fakeAccounts map[uuid.UUID]*user.User

func (f fakeAccounts) GetByID(id uuid.UUID) (*user.User, error) {
	if u, ok := f[id]; ok {
		return u, nil
	}
	return nil, user.ErrUserNotFound
}

type fixture struct {
	c        *Ceremonies
	keys     *fakeKeys
	accounts fakeAccounts
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{keys: &fakeKeys{}, accounts: fakeAccounts{}}
	c, err := New(Config{RPID: testRPID, RPName: "Authway", Origins: []string{testOrigin}}, ephemeral.NewMemory(context.Background()), f.keys, f.accounts)
	if err != nil {
		t.Fatal(err)
	}
	f.c = c
	return f
}

func (f *fixture) newUser(tenant uuid.UUID, email string) *user.User {
	u := &user.User{ID: uuid.New(), TenantID: tenant, Email: email, Active: true}
	f.accounts[u.ID] = u
	return u
}

// begin starts a registration and returns the options as the browser receives
// them.
func begin(t *testing.T, c *Ceremonies, u *user.User) (string, []byte) {
	t.Helper()
	reg, err := c.BeginRegistration(context.Background(), u, u.Email+" · Acme")
	if err != nil {
		t.Fatal(err)
	}
	options, err := json.Marshal(reg.Options)
	if err != nil {
		t.Fatal(err)
	}
	return reg.CeremonyID, options
}

// register gives u a passkey on a new device.
func (f *fixture) register(t *testing.T, u *user.User) *passkeytest.Device {
	t.Helper()
	dev := passkeytest.New(t)
	id, options := begin(t, f.c, u)
	body := dev.Register(t, options, passkeytest.GoodRegistration(testOrigin, testRPID, options))
	if _, err := f.c.FinishRegistration(context.Background(), u, id, body, "device"); err != nil {
		t.Fatalf("register: %v", err)
	}
	return dev
}

// beginLogin starts a sign-in and returns the options as the browser receives
// them.
func beginLogin(t *testing.T, c *Ceremonies, flow string, tenant uuid.UUID) (string, []byte) {
	t.Helper()
	login, err := c.BeginLogin(context.Background(), flow, tenant)
	if err != nil {
		t.Fatal(err)
	}
	options, err := json.Marshal(login.Options)
	if err != nil {
		t.Fatal(err)
	}
	return login.CeremonyID, options
}

func TestRegistration_OptionsAskForADiscoverableVerifiedCredentialNamingTheAccount(t *testing.T) {
	f := newFixture(t)
	u := f.newUser(uuid.New(), "pat@example.com")
	_, options := begin(t, f.c, u)

	var got struct {
		RP struct {
			ID string `json:"id"`
		} `json:"rp"`
		User struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"user"`
		AuthenticatorSelection struct {
			ResidentKey      string `json:"residentKey"`
			UserVerification string `json:"userVerification"`
		} `json:"authenticatorSelection"`
		Attestation string `json:"attestation"`
		Exclude     []any  `json:"excludeCredentials"`
	}
	if err := json.Unmarshal(options, &got); err != nil {
		t.Fatal(err)
	}
	if got.RP.ID != testRPID {
		t.Errorf("rp.id = %q, want %q", got.RP.ID, testRPID)
	}
	// The handle is the account's id, not its address.
	if want := base64.RawURLEncoding.EncodeToString(u.ID[:]); got.User.ID != want {
		t.Errorf("user.id = %q, want the account id %q", got.User.ID, want)
	}
	if got.User.Name != "pat@example.com" || got.User.DisplayName != "pat@example.com · Acme" {
		t.Errorf("user names = %q / %q", got.User.Name, got.User.DisplayName)
	}
	if got.AuthenticatorSelection.ResidentKey != "required" || got.AuthenticatorSelection.UserVerification != "required" {
		t.Errorf("selection = %+v, want a discoverable credential with user verification", got.AuthenticatorSelection)
	}
	if got.Attestation != "none" {
		t.Errorf("attestation = %q, want none", got.Attestation)
	}
	if len(got.Exclude) != 0 {
		t.Errorf("a new account excludes %d credentials, want none", len(got.Exclude))
	}
}

func TestRegistration_StoresThePasskeyWithWhatTheDeviceReported(t *testing.T) {
	f := newFixture(t)
	tenant := uuid.New()
	u := f.newUser(tenant, "pat@example.com")
	dev := passkeytest.New(t)
	id, options := begin(t, f.c, u)

	p, err := f.c.FinishRegistration(context.Background(), u, id, dev.Register(t, options, passkeytest.GoodRegistration(testOrigin, testRPID, options)), "  Work laptop  ")
	if err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}
	if p.Name != "Work laptop" || p.UserID != u.ID || p.TenantID != tenant {
		t.Errorf("passkey = %+v", p)
	}
	if !bytes.Equal(p.CredentialID, dev.CredentialID) {
		t.Error("the credential id is not the device's")
	}
	if p.Transports != "internal,hybrid" {
		t.Errorf("transports = %q", p.Transports)
	}
	// The flags are kept as reported: a later sign-in is checked against the
	// backup-eligible bit.
	if want := int16(passkeytest.FlagUserPresent | passkeytest.FlagUserVerified | passkeytest.FlagBackupEligible | passkeytest.FlagBackupState | passkeytest.FlagAttested); p.Flags != want {
		t.Errorf("flags = %#x, want %#x", p.Flags, want)
	}
	if rows, _ := f.keys.List(u.ID); len(rows) != 1 {
		t.Fatalf("stored %d passkeys, want 1", len(rows))
	}
}

func TestRegistration_ANewCeremonyExcludesWhatTheAccountAlreadyHas(t *testing.T) {
	f := newFixture(t)
	u := f.newUser(uuid.New(), "pat@example.com")
	dev := f.register(t, u)
	_, again := begin(t, f.c, u)
	var got struct {
		Exclude []struct {
			ID string `json:"id"`
		} `json:"excludeCredentials"`
	}
	if err := json.Unmarshal(again, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Exclude) != 1 || got.Exclude[0].ID != base64.RawURLEncoding.EncodeToString(dev.CredentialID) {
		t.Errorf("excludeCredentials = %+v, want the registered credential", got.Exclude)
	}
}

func TestRegistration_ACeremonyWorksOnce(t *testing.T) {
	f := newFixture(t)
	u := f.newUser(uuid.New(), "pat@example.com")
	dev := passkeytest.New(t)
	id, options := begin(t, f.c, u)
	body := dev.Register(t, options, passkeytest.GoodRegistration(testOrigin, testRPID, options))

	if _, err := f.c.FinishRegistration(context.Background(), u, id, body, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.FinishRegistration(context.Background(), u, id, body, ""); !errors.Is(err, ErrCeremonyNotFound) {
		t.Fatalf("second finish = %v, want ErrCeremonyNotFound", err)
	}
	if rows, _ := f.keys.List(u.ID); len(rows) != 1 {
		t.Fatalf("stored %d passkeys, want 1", len(rows))
	}
}

// A ceremony is the account's it was started for: another account's finish,
// even with the right answer for the challenge, is not accepted - and an
// unknown id is not told apart from a used one.
func TestRegistration_ACeremonyBelongsToTheAccountItWasStartedFor(t *testing.T) {
	f := newFixture(t)
	tenant := uuid.New()
	owner, intruder := f.newUser(tenant, "pat@example.com"), f.newUser(tenant, "eve@example.com")
	dev := passkeytest.New(t)
	id, options := begin(t, f.c, owner)
	body := dev.Register(t, options, passkeytest.GoodRegistration(testOrigin, testRPID, options))

	if _, err := f.c.FinishRegistration(context.Background(), intruder, id, body, ""); !errors.Is(err, ErrCeremonyNotFound) {
		t.Fatalf("another account's finish = %v, want ErrCeremonyNotFound", err)
	}
	if _, err := f.c.FinishRegistration(context.Background(), owner, "no-such-ceremony", body, ""); !errors.Is(err, ErrCeremonyNotFound) {
		t.Fatalf("unknown ceremony = %v, want ErrCeremonyNotFound", err)
	}
	if _, err := f.c.FinishRegistration(context.Background(), owner, "", body, ""); !errors.Is(err, ErrCeremonyNotFound) {
		t.Fatalf("empty ceremony = %v, want ErrCeremonyNotFound", err)
	}
	if rows, _ := f.keys.List(owner.ID); len(rows) != 0 {
		t.Fatalf("stored %d passkeys, want none", len(rows))
	}
}

// Each of these is an answer a real browser and device would not give; the
// server must refuse it, and store nothing.
func TestRegistration_RefusesAnAnswerThatIsNotForThisCeremony(t *testing.T) {
	for name, bend := range map[string]func(*passkeytest.Ceremony){
		"another origin":        func(a *passkeytest.Ceremony) { a.Origin = "https://evil.example" },
		"another relying party": func(a *passkeytest.Ceremony) { a.RPID = "evil.example" },
		"another challenge": func(a *passkeytest.Ceremony) {
			a.Challenge = base64.RawURLEncoding.EncodeToString([]byte("not the challenge at all, 32 by"))
		},
		"a sign-in ceremony's client data": func(a *passkeytest.Ceremony) { a.Kind = "webauthn.get" },
		"no user verification":             func(a *passkeytest.Ceremony) { a.Flags &^= passkeytest.FlagUserVerified },
		"no user presence":                 func(a *passkeytest.Ceremony) { a.Flags &^= passkeytest.FlagUserPresent },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			u := f.newUser(uuid.New(), "pat@example.com")
			dev := passkeytest.New(t)
			id, options := begin(t, f.c, u)
			ans := passkeytest.GoodRegistration(testOrigin, testRPID, options)
			bend(&ans)

			_, err := f.c.FinishRegistration(context.Background(), u, id, dev.Register(t, options, ans), "")
			if !errors.Is(err, ErrRejected) {
				t.Fatalf("FinishRegistration = %v, want ErrRejected", err)
			}
			if rows, _ := f.keys.List(u.ID); len(rows) != 0 {
				t.Fatalf("stored %d passkeys, want none", len(rows))
			}
		})
	}
}

func TestRegistration_RefusesAMalformedAnswer(t *testing.T) {
	f := newFixture(t)
	u := f.newUser(uuid.New(), "pat@example.com")
	for name, body := range map[string][]byte{
		"not json":     []byte("nope"),
		"empty object": []byte("{}"),
	} {
		id, _ := begin(t, f.c, u)
		if _, err := f.c.FinishRegistration(context.Background(), u, id, body, ""); !errors.Is(err, ErrRejected) {
			t.Errorf("%s: FinishRegistration = %v, want ErrRejected", name, err)
		}
	}
}

func TestRegistration_ACredentialAlreadyInTheTenantIsRefused(t *testing.T) {
	f := newFixture(t)
	tenant := uuid.New()
	first, second := f.newUser(tenant, "pat@example.com"), f.newUser(tenant, "sam@example.com")
	dev := passkeytest.New(t)
	id, options := begin(t, f.c, first)
	if _, err := f.c.FinishRegistration(context.Background(), first, id, dev.Register(t, options, passkeytest.GoodRegistration(testOrigin, testRPID, options)), ""); err != nil {
		t.Fatal(err)
	}
	id, options = begin(t, f.c, second)
	if _, err := f.c.FinishRegistration(context.Background(), second, id, dev.Register(t, options, passkeytest.GoodRegistration(testOrigin, testRPID, options)), ""); !errors.Is(err, user.ErrPasskeyTaken) {
		t.Fatalf("the same credential for another account = %v, want ErrPasskeyTaken", err)
	}
}

func TestCleanName(t *testing.T) {
	long := string(bytes.Repeat([]byte("가"), 150))
	for in, want := range map[string]string{"": DefaultName, "   ": DefaultName, " Phone ": "Phone", long: long[:3*maxNameRunes]} {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%.12q) = %.12q, want %.12q", in, got, want)
		}
	}
}

func TestLogin_OptionsNameNoOneAndAskForVerification(t *testing.T) {
	f := newFixture(t)
	_, options := beginLogin(t, f.c, "flow-1", uuid.New())
	var got struct {
		RPID             string `json:"rpId"`
		UserVerification string `json:"userVerification"`
		Challenge        string `json:"challenge"`
		Allow            []any  `json:"allowCredentials"`
	}
	if err := json.Unmarshal(options, &got); err != nil {
		t.Fatal(err)
	}
	if got.RPID != testRPID || got.UserVerification != "required" || got.Challenge == "" {
		t.Errorf("options = %+v", got)
	}
	// Discoverable: no one has said who is signing in, so there is no list.
	if len(got.Allow) != 0 {
		t.Errorf("allowCredentials = %v, want none", got.Allow)
	}
}

func TestLogin_SignsTheOwnerIn(t *testing.T) {
	f := newFixture(t)
	tenant := uuid.New()
	u := f.newUser(tenant, "pat@example.com")
	dev := f.register(t, u)

	id, options := beginLogin(t, f.c, "flow-1", tenant)
	answer := dev.SignIn(t, dev.GoodSignIn(testOrigin, testRPID, options))
	got, err := f.c.FinishLogin(context.Background(), "flow-1", tenant, id, answer)
	if err != nil {
		t.Fatalf("FinishLogin: %v", err)
	}
	if got.User.ID != u.ID || !bytes.Equal(got.Passkey.CredentialID, dev.CredentialID) {
		t.Fatalf("signed in %v with %x, want %v with the device's credential", got.User.ID, got.Passkey.CredentialID, u.ID)
	}
	if f.keys.lastUse.id != got.Passkey.ID {
		t.Error("the passkey's use was not recorded")
	}
}

func TestLogin_ACeremonyWorksOnceAndOnlyForItsFlow(t *testing.T) {
	f := newFixture(t)
	tenant := uuid.New()
	u := f.newUser(tenant, "pat@example.com")
	dev := f.register(t, u)

	id, options := beginLogin(t, f.c, "flow-1", tenant)
	answer := dev.SignIn(t, dev.GoodSignIn(testOrigin, testRPID, options))
	// Another login flow, though the same tenant, cannot take it - and taking
	// it spends it.
	if _, err := f.c.FinishLogin(context.Background(), "flow-2", tenant, id, answer); !errors.Is(err, ErrCeremonyNotFound) {
		t.Fatalf("another flow = %v, want ErrCeremonyNotFound", err)
	}
	if _, err := f.c.FinishLogin(context.Background(), "flow-1", tenant, id, answer); !errors.Is(err, ErrCeremonyNotFound) {
		t.Fatalf("after another flow took it = %v, want ErrCeremonyNotFound", err)
	}

	id, options = beginLogin(t, f.c, "flow-1", tenant)
	answer = dev.SignIn(t, dev.GoodSignIn(testOrigin, testRPID, options))
	if _, err := f.c.FinishLogin(context.Background(), "flow-1", tenant, id, answer); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.FinishLogin(context.Background(), "flow-1", tenant, id, answer); !errors.Is(err, ErrCeremonyNotFound) {
		t.Fatalf("second finish = %v, want ErrCeremonyNotFound", err)
	}
}

// A passkey registered in one tenant signs in nowhere else: the login UI is one
// relying party for every tenant, so the browser offers a person all of their
// passkeys, and it is the server that says which tenant's flow each may open.
func TestLogin_APasskeyOfOneTenantSignsInToNoOther(t *testing.T) {
	f := newFixture(t)
	acme, globex := uuid.New(), uuid.New()
	u := f.newUser(acme, "pat@example.com")
	dev := f.register(t, u)

	// A flow of the other tenant: the ceremony is of that tenant, the answer is
	// valid for the challenge, and the passkey is not found there.
	id, options := beginLogin(t, f.c, "flow-globex", globex)
	answer := dev.SignIn(t, dev.GoodSignIn(testOrigin, testRPID, options))
	if _, err := f.c.FinishLogin(context.Background(), "flow-globex", globex, id, answer); !errors.Is(err, ErrRejected) {
		t.Fatalf("a passkey of tenant acme in a flow of tenant globex = %v, want ErrRejected", err)
	}

	// A ceremony begun for one tenant cannot be finished as another's.
	id, options = beginLogin(t, f.c, "flow-acme", acme)
	answer = dev.SignIn(t, dev.GoodSignIn(testOrigin, testRPID, options))
	if _, err := f.c.FinishLogin(context.Background(), "flow-acme", globex, id, answer); !errors.Is(err, ErrCeremonyNotFound) {
		t.Fatalf("a ceremony of tenant acme finished as tenant globex = %v, want ErrCeremonyNotFound", err)
	}
}

// The same credential id may be registered in two tenants (it is chosen by
// whoever creates the credential). Each tenant's sign-in finds its own.
func TestLogin_AccountsAreFoundInTheTenantOfTheFlow(t *testing.T) {
	f := newFixture(t)
	acme, globex := uuid.New(), uuid.New()
	inAcme, inGlobex := f.newUser(acme, "pat@example.com"), f.newUser(globex, "pat@example.com")
	// One device, registered in both tenants.
	dev := passkeytest.New(t)
	for _, u := range []*user.User{inAcme, inGlobex} {
		id, options := begin(t, f.c, u)
		if _, err := f.c.FinishRegistration(context.Background(), u, id, dev.Register(t, options, passkeytest.GoodRegistration(testOrigin, testRPID, options)), ""); err != nil {
			t.Fatal(err)
		}
		dev.UserHandle = u.ID[:]
	}
	// It now answers the last user handle; in acme the handle is globex's, which
	// is not the passkey's account there.
	id, options := beginLogin(t, f.c, "flow-acme", acme)
	answer := dev.SignIn(t, dev.GoodSignIn(testOrigin, testRPID, options))
	if _, err := f.c.FinishLogin(context.Background(), "flow-acme", acme, id, answer); !errors.Is(err, ErrRejected) {
		t.Fatalf("another account's user handle = %v, want ErrRejected", err)
	}
	id, options = beginLogin(t, f.c, "flow-globex", globex)
	answer = dev.SignIn(t, dev.GoodSignIn(testOrigin, testRPID, options))
	got, err := f.c.FinishLogin(context.Background(), "flow-globex", globex, id, answer)
	if err != nil || got.User.ID != inGlobex.ID {
		t.Fatalf("globex's own: %v, %v", got, err)
	}
}

func TestLogin_RefusesAnAnswerThatIsNotForThisCeremony(t *testing.T) {
	for name, bend := range map[string]func(*passkeytest.Ceremony){
		"another origin":        func(a *passkeytest.Ceremony) { a.Origin = "https://evil.example" },
		"another relying party": func(a *passkeytest.Ceremony) { a.RPID = "evil.example" },
		"another challenge": func(a *passkeytest.Ceremony) {
			a.Challenge = base64.RawURLEncoding.EncodeToString([]byte("not the challenge at all, 32 by"))
		},
		"a creation ceremony":       func(a *passkeytest.Ceremony) { a.Kind = "webauthn.create" },
		"no user verification":      func(a *passkeytest.Ceremony) { a.Flags &^= passkeytest.FlagUserVerified },
		"no user presence":          func(a *passkeytest.Ceremony) { a.Flags &^= passkeytest.FlagUserPresent },
		"no user handle":            func(a *passkeytest.Ceremony) { a.UserHandle = nil },
		"another account's handle":  func(a *passkeytest.Ceremony) { a.UserHandle = uuid.New().NodeID() },
		"a backup flag out of line": func(a *passkeytest.Ceremony) { a.Flags &^= passkeytest.FlagBackupEligible },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			tenant := uuid.New()
			u := f.newUser(tenant, "pat@example.com")
			dev := f.register(t, u)
			id, options := beginLogin(t, f.c, "flow-1", tenant)
			ans := dev.GoodSignIn(testOrigin, testRPID, options)
			bend(&ans)

			if _, err := f.c.FinishLogin(context.Background(), "flow-1", tenant, id, dev.SignIn(t, ans)); !errors.Is(err, ErrRejected) {
				t.Fatalf("FinishLogin = %v, want ErrRejected", err)
			}
			if f.keys.lastUse.id != uuid.Nil {
				t.Error("a refused sign-in recorded a use")
			}
		})
	}
}

func TestLogin_RefusesADeviceThatWasNeverRegistered(t *testing.T) {
	f := newFixture(t)
	tenant := uuid.New()
	u := f.newUser(tenant, "pat@example.com")
	stranger := passkeytest.New(t)
	stranger.UserHandle = u.ID[:]
	id, options := beginLogin(t, f.c, "flow-1", tenant)
	if _, err := f.c.FinishLogin(context.Background(), "flow-1", tenant, id, stranger.SignIn(t, stranger.GoodSignIn(testOrigin, testRPID, options))); !errors.Is(err, ErrRejected) {
		t.Fatalf("FinishLogin = %v, want ErrRejected", err)
	}
}

// A registration's ceremony cannot finish a sign-in, nor the other way round.
func TestLogin_ACeremonyIsOfItsKind(t *testing.T) {
	f := newFixture(t)
	tenant := uuid.New()
	u := f.newUser(tenant, "pat@example.com")
	dev := f.register(t, u)

	regID, _ := begin(t, f.c, u)
	_, options := beginLogin(t, f.c, "flow-1", tenant)
	if _, err := f.c.FinishLogin(context.Background(), "", tenant, regID, dev.SignIn(t, dev.GoodSignIn(testOrigin, testRPID, options))); !errors.Is(err, ErrCeremonyNotFound) {
		t.Fatalf("a registration's ceremony finishing a sign-in = %v, want ErrCeremonyNotFound", err)
	}

	loginID, loginOptions := beginLogin(t, f.c, "", tenant)
	if _, err := f.c.FinishRegistration(context.Background(), u, loginID, dev.Register(t, loginOptions, passkeytest.GoodRegistration(testOrigin, testRPID, loginOptions)), ""); !errors.Is(err, ErrCeremonyNotFound) {
		t.Fatalf("a sign-in's ceremony finishing a registration = %v, want ErrCeremonyNotFound", err)
	}
}

// A device that counts reports a higher number each time; a number that does
// not rise means two copies of the credential may exist.
func TestLogin_ACounterThatDoesNotRiseIsSuspected(t *testing.T) {
	f := newFixture(t)
	tenant := uuid.New()
	u := f.newUser(tenant, "pat@example.com")
	dev := f.register(t, u)
	dev.Counts = true

	for i := 0; i < 2; i++ {
		id, options := beginLogin(t, f.c, "flow-1", tenant)
		if _, err := f.c.FinishLogin(context.Background(), "flow-1", tenant, id, dev.SignIn(t, dev.GoodSignIn(testOrigin, testRPID, options))); err != nil {
			t.Fatalf("sign-in %d: %v", i, err)
		}
	}
	if f.keys.lastUse.count != 2 {
		t.Errorf("the counter stored = %d, want 2", f.keys.lastUse.count)
	}

	dev.Rewind(0) // the next answer reports 1, which the passkey passed
	id, options := beginLogin(t, f.c, "flow-1", tenant)
	if _, err := f.c.FinishLogin(context.Background(), "flow-1", tenant, id, dev.SignIn(t, dev.GoodSignIn(testOrigin, testRPID, options))); !errors.Is(err, ErrCloneSuspected) {
		t.Fatalf("a counter that went back = %v, want ErrCloneSuspected", err)
	}
}

// When the store fails the passkey was not judged: that is the server's failure,
// not an answer to refuse - and not one to count against the user.
func TestLogin_AFailedLookupIsNotARefusal(t *testing.T) {
	f := newFixture(t)
	tenant := uuid.New()
	u := f.newUser(tenant, "pat@example.com")
	dev := f.register(t, u)

	id, options := beginLogin(t, f.c, "flow-1", tenant)
	answer := dev.SignIn(t, dev.GoodSignIn(testOrigin, testRPID, options))
	f.keys.findErr = errors.New("connection refused")
	_, err := f.c.FinishLogin(context.Background(), "flow-1", tenant, id, answer)
	if err == nil || errors.Is(err, ErrRejected) || errors.Is(err, ErrCeremonyNotFound) || errors.Is(err, ErrCloneSuspected) {
		t.Fatalf("FinishLogin with the store down = %v, want an error that is none of the refusals", err)
	}
}
