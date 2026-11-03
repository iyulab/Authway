// Package passkey registers and verifies passkeys (WebAuthn credentials) for
// accounts.
//
// A ceremony has two halves: the server hands the browser options, the browser
// has the device sign them, and the server checks the answer. The options and
// the answer are tied by a challenge the server must remember between the two,
// so the first half stores what the library calls session data, and the second
// takes it back — once, and only for the account it was made for.
package passkey

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"authway/apps/central/api/pkg/ephemeral"
	"authway/apps/central/api/pkg/user"
)

// ceremonyTTL is how long the browser has between the options and the answer:
// a person is choosing a device and confirming with a fingerprint or a PIN.
const ceremonyTTL = 5 * time.Minute

// maxNameRunes is the longest name a passkey may be given.
const maxNameRunes = 100

// DefaultName is what a passkey is called when the user gave no name.
const DefaultName = "Passkey"

var (
	// ErrCeremonyNotFound is a ceremony that was never started, was already
	// finished, has expired, or was started for another account. The reasons
	// are not told apart: each one means the browser starts over.
	ErrCeremonyNotFound = errors.New("the passkey ceremony was not started, has expired, or was already used")

	// ErrRejected is an answer the library refused: a wrong challenge, origin or
	// relying party, a bad signature, a device that was not verified, a
	// malformed body. Its text names the cause for the log, not for the caller.
	ErrRejected = errors.New("the passkey was not accepted")

	// ErrCloneSuspected is a sign-in whose signature counter did not rise: two
	// copies of the credential may exist. The answer was otherwise valid, so it
	// is told apart from ErrRejected for the log, not for the caller.
	ErrCloneSuspected = errors.New("the passkey's signature counter went back: it may have been cloned")
)

// Config is what the relying party — this deployment — is to the browser.
type Config struct {
	// RPID is the registrable domain suffix passkeys are scoped to. A passkey
	// registered for it works only on pages whose host is it or below it, and
	// stops working if it changes, so it is the login UI's host and not more.
	RPID string
	// RPName is what the browser may show for the relying party.
	RPName string
	// Origins are the pages that may start a ceremony: the login UI's origin.
	Origins []string
}

// Accounts finds the account a passkey belongs to.
type Accounts interface {
	GetByID(id uuid.UUID) (*user.User, error)
}

// Ceremonies runs passkey ceremonies.
type Ceremonies struct {
	wa       *webauthn.WebAuthn
	store    ephemeral.Store
	passkeys user.PasskeyService
	accounts Accounts
}

// New returns the ceremonies of one relying party.
func New(cfg Config, store ephemeral.Store, passkeys user.PasskeyService, accounts Accounts) (*Ceremonies, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                  cfg.RPID,
		RPDisplayName:         cfg.RPName,
		RPOrigins:             cfg.Origins,
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTTL, TimeoutUVD: ceremonyTTL},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTTL, TimeoutUVD: ceremonyTTL},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("invalid passkey configuration: %w", err)
	}
	return &Ceremonies{wa: wa, store: store, passkeys: passkeys, accounts: accounts}, nil
}

// account is an account as the library sees it: an opaque handle, two names
// and the credentials it already has.
type account struct {
	user        *user.User
	displayName string
	credentials []webauthn.Credential
}

// WebAuthnID is the user handle: the account's id, not its address, which is
// personal data and can change. A passkey names its account by this handle.
func (a *account) WebAuthnID() []byte { return a.user.ID[:] }

func (a *account) WebAuthnName() string { return a.user.Email }

func (a *account) WebAuthnDisplayName() string { return a.displayName }

func (a *account) WebAuthnCredentials() []webauthn.Credential { return a.credentials }

// Registration is the first half of registering a passkey.
type Registration struct {
	// CeremonyID names the ceremony in the second half.
	CeremonyID string
	// Options are the browser's input for navigator.credentials.create, in the
	// shape PublicKeyCredential.parseCreationOptionsFromJSON takes.
	Options protocol.PublicKeyCredentialCreationOptions
}

// BeginRegistration starts registering a passkey for u. displayName is how the
// browser labels the account (the address and the tenant, so the same address
// in two tenants is two entries). The passkeys u has are excluded, so a device
// that holds one is not offered a second.
func (c *Ceremonies) BeginRegistration(ctx context.Context, u *user.User, displayName string) (*Registration, error) {
	acct, err := c.account(u, displayName)
	if err != nil {
		return nil, err
	}
	creation, session, err := c.wa.BeginRegistration(acct,
		webauthn.WithExclusions(webauthn.Credentials(acct.credentials).CredentialDescriptors()),
		webauthn.WithExtensions(webauthn.WithExtensionCredProps()),
	)
	if err != nil {
		return nil, fmt.Errorf("could not begin the passkey registration: %w", err)
	}
	id, err := c.keep(ctx, kindRegister, binding{UserID: u.ID, TenantID: u.TenantID}, session)
	if err != nil {
		return nil, err
	}
	return &Registration{CeremonyID: id, Options: creation.Response}, nil
}

// FinishRegistration checks the browser's answer — the credential as the
// browser's toJSON() writes it — and stores the passkey under name. The answer
// must be for a ceremony begun for u.
func (c *Ceremonies) FinishRegistration(ctx context.Context, u *user.User, ceremonyID string, answer []byte, name string) (*user.Passkey, error) {
	session, err := c.recall(ctx, kindRegister, binding{UserID: u.ID, TenantID: u.TenantID}, ceremonyID)
	if err != nil {
		return nil, err
	}
	acct, err := c.account(u, "")
	if err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(answer)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	credential, err := c.wa.CreateCredential(acct, *session, parsed)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	p := &user.Passkey{
		ID:           uuid.New(),
		TenantID:     u.TenantID,
		UserID:       u.ID,
		CredentialID: credential.ID,
		PublicKey:    credential.PublicKey,
		SignCount:    int64(credential.Authenticator.SignCount),
		AAGUID:       credential.Authenticator.AAGUID,
		Transports:   joinTransports(credential.Transport),
		Flags:        int16(credential.Flags.ProtocolValue()),
		Name:         cleanName(name),
		CreatedAt:    time.Now(),
	}
	if err := c.passkeys.Add(p); err != nil {
		return nil, err
	}
	return p, nil
}

// account loads u's passkeys into the shape the library wants.
func (c *Ceremonies) account(u *user.User, displayName string) (*account, error) {
	rows, err := c.passkeys.List(u.ID)
	if err != nil {
		return nil, err
	}
	if displayName == "" {
		displayName = u.Email
	}
	credentials := make([]webauthn.Credential, len(rows))
	for i, row := range rows {
		credentials[i] = credentialOf(row)
	}
	return &account{user: u, displayName: displayName, credentials: credentials}, nil
}

// credentialOf is a stored passkey as the library keeps it. The flags go back
// in as they were stored: the library refuses a sign-in whose backup-eligible
// flag differs from the one the credential was registered with.
func credentialOf(p user.Passkey) webauthn.Credential {
	transports := make([]protocol.AuthenticatorTransport, 0, 2)
	for _, t := range p.TransportList() {
		transports = append(transports, protocol.AuthenticatorTransport(t))
	}
	return webauthn.Credential{
		ID:        p.CredentialID,
		PublicKey: p.PublicKey,
		Transport: transports,
		Flags:     webauthn.CredentialFlagsFromMsgpByte(byte(p.Flags)),
		Authenticator: webauthn.Authenticator{
			AAGUID:    p.AAGUID,
			SignCount: uint32(p.SignCount),
		},
	}
}

func joinTransports(ts []protocol.AuthenticatorTransport) string {
	parts := make([]string, 0, len(ts))
	for _, t := range ts {
		parts = append(parts, string(t))
	}
	return strings.Join(parts, ",")
}

// cleanName is the name a user gave a passkey, trimmed and cut to length; a
// passkey with none is called DefaultName.
func cleanName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultName
	}
	if utf8.RuneCountInString(name) > maxNameRunes {
		name = string([]rune(name)[:maxNameRunes])
	}
	return name
}

// ceremonyKind keeps what one half stored from being taken by the other
// kind of ceremony: a sign-in cannot finish a registration.
type ceremonyKind string

const (
	kindRegister ceremonyKind = "register"
	kindLogin    ceremonyKind = "login"
)

// binding is what a ceremony is for. The second half must present the same:
// a registration is one account's, a sign-in is one login flow's in one
// tenant (the flow's client says which), so an answer cannot be carried to
// another account, another flow or another tenant.
type binding struct {
	UserID   uuid.UUID
	TenantID uuid.UUID
	Flow     string
}

func ceremonyKey(kind ceremonyKind, id string) string {
	return "webauthn:" + string(kind) + ":" + id
}

// keep stores the session data of a ceremony begun for b and returns the id the
// browser will name it by. The id is a random value no one else knows: it is
// the browser's claim on the stored challenge.
func (c *Ceremonies) keep(ctx context.Context, kind ceremonyKind, b binding, session *webauthn.SessionData) (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("could not make a ceremony id: %w", err)
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	data, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("could not keep the ceremony: %w", err)
	}
	fields := map[string]string{"session": string(data), "user_id": b.UserID.String(), "tenant_id": b.TenantID.String(), "flow": b.Flow}
	if err := c.store.Put(ctx, ceremonyKey(kind, id), fields, ceremonyTTL); err != nil {
		return "", fmt.Errorf("could not keep the ceremony: %w", err)
	}
	return id, nil
}

// recall takes back the session data of a ceremony — once — and checks it is of
// the kind and for what is asked. The library checks the challenge in it
// against the answer; the session handed back is the stored one, never one
// built again.
func (c *Ceremonies) recall(ctx context.Context, kind ceremonyKind, want binding, id string) (*webauthn.SessionData, error) {
	if id == "" {
		return nil, ErrCeremonyNotFound
	}
	fields, err := c.store.Take(ctx, ceremonyKey(kind, id))
	if err != nil {
		return nil, fmt.Errorf("could not read the ceremony: %w", err)
	}
	if len(fields) == 0 || fields["user_id"] != want.UserID.String() || fields["tenant_id"] != want.TenantID.String() || fields["flow"] != want.Flow {
		return nil, ErrCeremonyNotFound
	}
	var session webauthn.SessionData
	if err := json.Unmarshal([]byte(fields["session"]), &session); err != nil {
		return nil, ErrCeremonyNotFound
	}
	return &session, nil
}

// Login is the first half of signing in with a passkey.
type Login struct {
	// CeremonyID names the ceremony in the second half.
	CeremonyID string
	// Options are the browser's input for navigator.credentials.get, in the
	// shape PublicKeyCredential.parseRequestOptionsFromJSON takes. They name no
	// credential: the device offers the passkeys it has for this relying party.
	Options protocol.PublicKeyCredentialRequestOptions
}

// BeginLogin starts a passkey sign-in for a login flow whose client is of
// tenant. The sign-in is discoverable — no one has said who is signing in — and
// asks for user verification.
func (c *Ceremonies) BeginLogin(ctx context.Context, flow string, tenantID uuid.UUID) (*Login, error) {
	assertion, session, err := c.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, fmt.Errorf("could not begin the passkey sign-in: %w", err)
	}
	id, err := c.keep(ctx, kindLogin, binding{TenantID: tenantID, Flow: flow}, session)
	if err != nil {
		return nil, err
	}
	return &Login{CeremonyID: id, Options: assertion.Response}, nil
}

// SignIn is a passkey sign-in that checked out: whose it is, and which passkey.
// It proves who the person is; whether the account may sign in (suspended, a
// second factor owed) is the sign-in steps' to decide.
type SignIn struct {
	User    *user.User
	Passkey *user.Passkey
}

// FinishLogin checks the browser's answer — the assertion as the browser's
// toJSON() writes it — to a ceremony begun for this flow. The passkey must be
// of tenant and name its account's id as the user handle, and the account must
// be of tenant: a passkey registered in one tenant signs in nowhere else, however
// the answer is carried.
func (c *Ceremonies) FinishLogin(ctx context.Context, flow string, tenantID uuid.UUID, ceremonyID string, answer []byte) (*SignIn, error) {
	session, err := c.recall(ctx, kindLogin, binding{TenantID: tenantID, Flow: flow}, ceremonyID)
	if err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(answer)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	var (
		stored *user.Passkey
		owner  *user.User
	)
	// The library calls this with what the device answered, to learn whose
	// credential it is. What it returns is trusted to be that account's.
	var lookupFailed error
	whose := func(credentialID, userHandle []byte) (webauthn.User, error) {
		p, err := c.passkeys.Find(tenantID, credentialID)
		if err != nil {
			if !errors.Is(err, user.ErrPasskeyNotFound) {
				lookupFailed = err
			}
			return nil, err
		}
		if !bytes.Equal(userHandle, p.UserID[:]) {
			return nil, errors.New("the user handle is not the passkey's account")
		}
		u, err := c.accounts.GetByID(p.UserID)
		if err != nil {
			if !errors.Is(err, user.ErrUserNotFound) {
				lookupFailed = err
			}
			return nil, err
		}
		if u.TenantID != tenantID || p.TenantID != tenantID {
			return nil, errors.New("the passkey's account is of another tenant")
		}
		acct, err := c.account(u, "")
		if err != nil {
			return nil, err
		}
		stored, owner = p, u
		return acct, nil
	}
	_, credential, err := c.wa.ValidatePasskeyLogin(whose, *session, parsed)
	if lookupFailed != nil {
		// The database or the store failed; the passkey was not judged. That is
		// the server's failure to report, not a refusal that counts against the user.
		return nil, fmt.Errorf("could not look the passkey up: %w", lookupFailed)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	if credential.Authenticator.CloneWarning {
		return nil, ErrCloneSuspected
	}
	if err := c.passkeys.RecordUse(stored.ID, credential.Authenticator.SignCount, byte(credential.Flags.ProtocolValue())); err != nil {
		return nil, err
	}
	return &SignIn{User: owner, Passkey: stored}, nil
}
