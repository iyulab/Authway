// Package passkeytest is a software passkey device for tests: it makes
// credentials and signs challenges the way a real authenticator does, so a
// ceremony can be run against the server without a browser.
//
// It is for tests only. Nothing in a deployment imports it.
package passkeytest

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// Authenticator-data flags.
const (
	FlagUserPresent    byte = 0x01
	FlagUserVerified   byte = 0x04
	FlagBackupEligible byte = 0x08
	FlagBackupState    byte = 0x10
	FlagAttested       byte = 0x40
)

// Ceremony is what the device and browser put into an answer. A test starts
// from the right values (Good...) and bends one of them at a time.
type Ceremony struct {
	Origin    string
	RPID      string
	Challenge string
	Flags     byte
	// Kind is the client data type: "webauthn.create" or "webauthn.get".
	Kind string
	// UserHandle is what a sign-in answers as the user handle; nil answers none.
	UserHandle []byte
}

// Device is one passkey on one authenticator.
type Device struct {
	key          *ecdsa.PrivateKey
	CredentialID []byte
	// UserHandle is the user.id the credential was created for.
	UserHandle []byte
	counter    uint32
	// Counts makes the device report a signature counter, as a hardware key
	// does; a synced passkey reports zero.
	Counts bool
}

// New returns a device with a fresh key and credential id.
func New(t *testing.T) *Device {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}
	return &Device{key: key, CredentialID: id}
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// GoodRegistration is the answer a real browser and device give to creation
// options (the JSON the server returned): this origin and relying party, the
// options' challenge, user presence and verification, a backup-eligible synced
// credential.
func GoodRegistration(origin, rpID string, options []byte) Ceremony {
	var parsed struct {
		Challenge string `json:"challenge"`
	}
	_ = json.Unmarshal(options, &parsed)
	return Ceremony{Origin: origin, RPID: rpID, Challenge: parsed.Challenge, Kind: "webauthn.create",
		Flags: FlagUserPresent | FlagUserVerified | FlagBackupEligible | FlagBackupState | FlagAttested}
}

// GoodSignIn is the answer to request options: as GoodRegistration, for a
// sign-in, answering the user handle the credential was created for.
func (d *Device) GoodSignIn(origin, rpID string, options []byte) Ceremony {
	var parsed struct {
		Challenge string `json:"challenge"`
	}
	_ = json.Unmarshal(options, &parsed)
	return Ceremony{Origin: origin, RPID: rpID, Challenge: parsed.Challenge, Kind: "webauthn.get", UserHandle: d.UserHandle,
		Flags: FlagUserPresent | FlagUserVerified | FlagBackupEligible | FlagBackupState}
}

func clientData(c Ceremony) []byte {
	out, _ := json.Marshal(map[string]any{"type": c.Kind, "challenge": c.Challenge, "origin": c.Origin, "crossOrigin": false})
	return out
}

// Register is the browser's answer to creation options: the credential as
// PublicKeyCredential.toJSON() writes it, with a "none" attestation. It keeps
// the user id the options named, to answer it at sign-in.
func (d *Device) Register(t *testing.T, options []byte, c Ceremony) []byte {
	t.Helper()
	var parsed struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	_ = json.Unmarshal(options, &parsed)
	if handle, err := base64.RawURLEncoding.DecodeString(parsed.User.ID); err == nil {
		d.UserHandle = handle
	}

	// 0x04 || X || Y: the uncompressed point.
	point, err := d.key.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: point[1:33], -3: point[33:65]})
	if err != nil {
		t.Fatal(err)
	}
	rpHash := sha256.Sum256([]byte(c.RPID))
	var data bytes.Buffer
	data.Write(rpHash[:])
	data.WriteByte(c.Flags)
	data.Write([]byte{0, 0, 0, 0})
	data.Write(make([]byte, 16)) // aaguid
	_ = binary.Write(&data, binary.BigEndian, uint16(len(d.CredentialID)))
	data.Write(d.CredentialID)
	data.Write(cose)

	attestation, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": data.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(map[string]any{
		"id":    b64(d.CredentialID),
		"rawId": b64(d.CredentialID),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64(clientData(c)),
			"attestationObject": b64(attestation),
			"transports":        []string{"internal", "hybrid"},
		},
		"authenticatorAttachment": "platform",
		"clientExtensionResults":  map[string]any{},
	})
	return out
}

// SignIn is the browser's answer to request options: an assertion signed with
// the device's key, as PublicKeyCredential.toJSON() writes it. A device that
// counts reports a counter one higher than the last.
func (d *Device) SignIn(t *testing.T, c Ceremony) []byte {
	t.Helper()
	if d.Counts {
		d.counter++
	}
	rpHash := sha256.Sum256([]byte(c.RPID))
	var data bytes.Buffer
	data.Write(rpHash[:])
	data.WriteByte(c.Flags)
	_ = binary.Write(&data, binary.BigEndian, d.counter)

	clientJSON := clientData(c)
	clientHash := sha256.Sum256(clientJSON)
	signed := sha256.Sum256(append(append([]byte{}, data.Bytes()...), clientHash[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, d.key, signed[:])
	if err != nil {
		t.Fatal(err)
	}
	response := map[string]any{
		"clientDataJSON":    b64(clientJSON),
		"authenticatorData": b64(data.Bytes()),
		"signature":         b64(signature),
	}
	if c.UserHandle != nil {
		response["userHandle"] = b64(c.UserHandle)
	}
	out, _ := json.Marshal(map[string]any{
		"id":                      b64(d.CredentialID),
		"rawId":                   b64(d.CredentialID),
		"type":                    "public-key",
		"response":                response,
		"authenticatorAttachment": "platform",
		"clientExtensionResults":  map[string]any{},
	})
	return out
}

// Rewind sets the counter the device reports next, to play a cloned device.
func (d *Device) Rewind(counter uint32) { d.counter = counter }
