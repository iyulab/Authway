package client

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"authway/apps/central/api/pkg/crypto"
)

// sealWith turns sealing on with a fresh key for the length of a test.
func sealWith(t *testing.T) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	before := atRest
	SealSecretsWith(mustCipher(base64.StdEncoding.EncodeToString(key)))
	t.Cleanup(func() { atRest = before })
}

// stored reads a column of a client row as the database holds it.
func stored(t *testing.T, db *gorm.DB, id uuid.UUID, column string) string {
	t.Helper()
	var value *string
	if err := db.Raw(`SELECT `+column+` FROM clients WHERE id = ?`, id).Scan(&value).Error; err != nil {
		t.Fatalf("read %s: %v", column, err)
	}
	if value == nil {
		return "<NULL>"
	}
	return *value
}

const (
	googleSecret    = "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gle"
	githubSecret    = "ghs_what-the-tenant-holds-at-github"
	microsoftSecret = "ms~what-the-tenant-holds-at-microsoft"
	appleKey        = "-----BEGIN PRIVATE KEY-----\nwhat-the-tenant-holds-at-apple\n-----END PRIVATE KEY-----"
)

func newClientWithProviderSecrets(t *testing.T, db *gorm.DB, svc Service) *Client {
	t.Helper()
	created, _, err := svc.Create(&CreateClientRequest{
		TenantID:              seedTenant(t, db),
		Name:                  "sealed-" + uuid.NewString()[:8],
		Public:                true,
		RedirectURIs:          []string{"https://app.example.com/callback"},
		AllowedOrigins:        []string{"https://app.example.com"},
		GrantTypes:            []string{"authorization_code"},
		Scopes:                []string{"openid"},
		GoogleOAuthEnabled:    true,
		GoogleClientID:        "google-app",
		GoogleClientSecret:    googleSecret,
		GithubOAuthEnabled:    true,
		GithubClientID:        "github-app",
		GithubClientSecret:    githubSecret,
		MicrosoftOAuthEnabled: true,
		MicrosoftClientID:     "microsoft-app",
		MicrosoftClientSecret: microsoftSecret,
		AppleOAuthEnabled:     true,
		AppleClientID:         "apple-app",
		AppleTeamID:           "TEAM",
		AppleKeyID:            "KEY",
		ApplePrivateKey:       appleKey,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM clients WHERE id = ?`, created.ID) })
	return created
}

var providerSecrets = map[string]string{
	"google_client_secret":    googleSecret,
	"github_client_secret":    githubSecret,
	"microsoft_client_secret": microsoftSecret,
	"apple_private_key":       appleKey,
}

func secretsOf(c *Client) map[string]string {
	deref := func(s *string) string {
		if s == nil {
			return "<nil>"
		}
		return *s
	}
	return map[string]string{
		"google_client_secret":    deref(c.GoogleClientSecret),
		"github_client_secret":    deref(c.GithubClientSecret),
		"microsoft_client_secret": deref(c.MicrosoftClientSecret),
		"apple_private_key":       deref(c.ApplePrivateKey),
	}
}

// The row holds none of the provider secrets in the clear, and every way of
// loading the client hands them back as they were given.
func TestProviderSecrets_AreSealedInTheRowAndClearInMemory(t *testing.T) {
	db := setupPostgres(t)
	sealWith(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	created := newClientWithProviderSecrets(t, db, svc)

	for column, secret := range providerSecrets {
		row := stored(t, db, created.ID, column)
		if !strings.HasPrefix(row, crypto.SealedPrefix) || strings.Contains(row, secret) {
			t.Errorf("%s is stored as %q, want it sealed", column, row)
		}
	}

	byID, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	byClientID, err := svc.GetByClientID(created.ClientID)
	if err != nil {
		t.Fatalf("GetByClientID: %v", err)
	}
	listed, _, err := svc.GetByTenant(created.TenantID, 100, 0)
	if err != nil {
		t.Fatalf("GetByTenant: %v", err)
	}
	var fromList *Client
	for _, c := range listed {
		if c.ID == created.ID {
			fromList = c
		}
	}
	if fromList == nil {
		t.Fatal("the client is not in its tenant's list (first 100)")
	}
	for how, c := range map[string]*Client{"as created": created, "by id": byID, "by client id": byClientID, "listed": fromList} {
		for column, secret := range providerSecrets {
			if got := secretsOf(c)[column]; got != secret {
				t.Errorf("%s: %s = %q, want the secret as it was given", how, column, got)
			}
		}
	}
}

// A change to the client — to a secret or to anything else — leaves the row
// sealed and the secrets intact.
func TestProviderSecrets_StaySealedThroughAnUpdate(t *testing.T) {
	db := setupPostgres(t)
	sealWith(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	created := newClientWithProviderSecrets(t, db, svc)

	renamed := created.Name + "-renamed"
	rotated := "GOCSPX-rotated"
	if _, _, err := svc.Update(created.ID, &UpdateClientRequest{Name: renamed, GoogleClientSecret: &rotated}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// GitHub's credentials change through the same request as the others' —
	// an update could not name them.
	githubRotated := "gh-rotated"
	if _, _, err := svc.Update(created.ID, &UpdateClientRequest{GithubClientSecret: &githubRotated}); err != nil {
		t.Fatalf("Update (GitHub): %v", err)
	}
	after, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got := secretsOf(after); got["google_client_secret"] != rotated || got["github_client_secret"] != githubRotated || got["apple_private_key"] != appleKey {
		t.Errorf("after the update the secrets read %v", got)
	}
	for column := range providerSecrets {
		if row := stored(t, db, created.ID, column); !strings.HasPrefix(row, crypto.SealedPrefix) || strings.Contains(row, rotated) || strings.Contains(row, githubRotated) {
			t.Errorf("%s is stored as %q after the update, want it sealed", column, row)
		}
	}
}

// A client with no provider secret has none after sealing either: an absent
// value stays absent, it is not turned into a sealed empty string.
func TestProviderSecrets_AbsentStaysAbsent(t *testing.T) {
	db := setupPostgres(t)
	sealWith(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	created, _, err := svc.Create(&CreateClientRequest{
		TenantID: seedTenant(t, db), Name: "sealed-none-" + uuid.NewString()[:8], Public: true,
		RedirectURIs: []string{"https://app.example.com/callback"}, AllowedOrigins: []string{"https://app.example.com"},
		GrantTypes: []string{"authorization_code"}, Scopes: []string{"openid"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM clients WHERE id = ?`, created.ID) })

	for column := range providerSecrets {
		if row := stored(t, db, created.ID, column); row != "<NULL>" && row != "" {
			t.Errorf("%s is stored as %q for a client that has none", column, row)
		}
	}
	loaded, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.GoogleClientSecret != nil && *loaded.GoogleClientSecret != "" {
		t.Errorf("google_client_secret reads %q for a client that has none", *loaded.GoogleClientSecret)
	}
}

// Rows written before sealing still read, and starting up with a key seals
// them — once, leaving already sealed values alone.
func TestProviderSecrets_StoredInTheClearAreReadAndThenSealed(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	// Written while nothing was sealed.
	created := newClientWithProviderSecrets(t, db, svc)
	if row := stored(t, db, created.ID, "google_client_secret"); row != googleSecret {
		t.Fatalf("precondition: google_client_secret is stored as %q, want it in the clear", row)
	}

	sealWith(t)
	loaded, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID over an unsealed row: %v", err)
	}
	if got := secretsOf(loaded)["google_client_secret"]; got != googleSecret {
		t.Fatalf("an unsealed row reads %q, want the secret", got)
	}

	if err := ResealStoredSecrets(svc, zap.NewNop()); err != nil {
		t.Fatalf("ResealStoredSecrets: %v", err)
	}
	sealedRows := map[string]string{}
	for column, secret := range providerSecrets {
		row := stored(t, db, created.ID, column)
		if !strings.HasPrefix(row, crypto.SealedPrefix) || strings.Contains(row, secret) {
			t.Errorf("%s is stored as %q after sealing", column, row)
		}
		sealedRows[column] = row
	}
	after, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after sealing: %v", err)
	}
	for column, secret := range providerSecrets {
		if got := secretsOf(after)[column]; got != secret {
			t.Errorf("after sealing %s reads %q, want the secret", column, got)
		}
	}

	// A second start finds nothing to do.
	if err := ResealStoredSecrets(svc, zap.NewNop()); err != nil {
		t.Fatalf("ResealStoredSecrets again: %v", err)
	}
	for column, before := range sealedRows {
		if row := stored(t, db, created.ID, column); row != before {
			t.Errorf("%s was sealed a second time", column)
		}
	}
}

// Sealed rows without the key are a misconfiguration: the client does not
// load, rather than loading with a secret that is not one.
func TestProviderSecrets_SealedRowsDoNotLoadWithoutTheKey(t *testing.T) {
	db := setupPostgres(t)
	sealWith(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	created := newClientWithProviderSecrets(t, db, svc)

	atRest = mustCipher("") // the key is gone
	if _, err := svc.GetByID(created.ID); err == nil {
		t.Fatal("a client with sealed secrets loaded without the key")
	}
}

// Changing the at-rest key: secrets sealed with the old key are resealed with
// the new one at startup, after which the new key alone loads the client.
func TestProviderSecrets_ResealedFromAPreviousKey(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	before := atRest
	t.Cleanup(func() { atRest = before })

	key := func() string {
		k := make([]byte, 32)
		if _, err := rand.Read(k); err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(k)
	}
	oldKey, newKey := key(), key()
	SealSecretsWith(mustCipher(oldKey))
	created := newClientWithProviderSecrets(t, db, svc)
	sealedWithOld := map[string]string{}
	for column := range providerSecrets {
		sealedWithOld[column] = stored(t, db, created.ID, column)
	}

	ring, err := crypto.NewCipher(newKey, oldKey)
	if err != nil {
		t.Fatalf("cipher with a previous key: %v", err)
	}
	SealSecretsWith(ring)
	if err := ResealStoredSecrets(svc, zap.NewNop()); err != nil {
		t.Fatalf("ResealStoredSecrets: %v", err)
	}
	for column, old := range sealedWithOld {
		if row := stored(t, db, created.ID, column); row == old || !strings.HasPrefix(row, crypto.SealedPrefix) {
			t.Errorf("%s was not resealed", column)
		}
	}

	SealSecretsWith(mustCipher(newKey))
	loaded, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID with only the new key: %v", err)
	}
	for column, secret := range providerSecrets {
		if got := secretsOf(loaded)[column]; got != secret {
			t.Errorf("with only the new key %s reads %q, want the secret", column, got)
		}
	}
}
