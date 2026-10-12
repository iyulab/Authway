package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"authway/apps/central/api/internal/hydra"
)

// recordingHydra stands in for the authorization server and keeps the client
// registrations it is sent, by method.
func recordingHydra(t *testing.T) (*hydra.Client, func(method string) []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	sent := map[string][]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var registration map[string]any
		if json.Unmarshal(body, &registration) == nil {
			mu.Lock()
			sent[r.Method] = append(sent[r.Method], registration)
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = w.Write([]byte(`{"client_id":"stub"}`))
	}))
	t.Cleanup(srv.Close)
	return hydra.NewClient(srv.URL), func(method string) []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), sent[method]...)
	}
}

// A client's secret goes to the caller once and to the authorization server;
// what stays in the database is a hash that the secret can be checked against.
func TestClientSecret_IsStoredOnlyAsAHash(t *testing.T) {
	db := setupPostgres(t)
	hydraClient, sent := recordingHydra(t)
	svc := NewService(db, zap.NewNop(), hydraClient)
	tenantID := seedTenant(t, db)

	created, credentials, err := svc.Create(&CreateClientRequest{
		TenantID:   tenantID,
		Name:       "secret-" + uuid.NewString()[:8],
		GrantTypes: []string{"client_credentials"},
		Scopes:     []string{"api"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM clients WHERE id = ?`, created.ID) })
	secret := credentials.ClientSecret
	if secret == "" {
		t.Fatal("a confidential client was created without a secret")
	}

	var stored struct{ ClientSecret, ClientSecretHash string }
	if err := db.Raw(`SELECT client_secret, client_secret_hash FROM clients WHERE id = ?`, created.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ClientSecret != "" {
		t.Error("the secret itself was stored")
	}
	if !strings.HasPrefix(stored.ClientSecretHash, "$2") || strings.Contains(stored.ClientSecretHash, secret) {
		t.Errorf("stored hash %q, want a password hash that does not contain the secret", stored.ClientSecretHash)
	}
	if got := sent(http.MethodPost); len(got) != 1 || got[0]["client_secret"] != secret {
		t.Errorf("the authorization server was not given the secret at registration: %v", got)
	}

	if _, err := svc.ValidateClient(created.ClientID, secret); err != nil {
		t.Errorf("the right secret was refused: %v", err)
	}
	for _, wrong := range []string{"", secret + "x", strings.ToUpper(secret)} {
		if _, err := svc.ValidateClient(created.ClientID, wrong); err == nil {
			t.Errorf("secret %q was accepted", wrong)
		}
	}

	// An update names no secret, and the authorization server keeps its own.
	if _, _, err := svc.Update(created.ID, &UpdateClientRequest{Name: "secret-renamed-" + uuid.NewString()[:8]}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updates := sent(http.MethodPut)
	if len(updates) != 1 {
		t.Fatalf("expected one update at the authorization server, got %d", len(updates))
	}
	if _, has := updates[0]["client_secret"]; has {
		t.Errorf("an update sent a secret to the authorization server: %v", updates[0]["client_secret"])
	}
	if _, err := svc.ValidateClient(created.ClientID, secret); err != nil {
		t.Errorf("after an update the secret was refused: %v", err)
	}

	// A new secret replaces the old one, in both places.
	rotated, _, err := svc.RegenerateSecret(created.ID)
	if err != nil {
		t.Fatalf("RegenerateSecret: %v", err)
	}
	if updates = sent(http.MethodPut); updates[len(updates)-1]["client_secret"] != rotated.ClientSecret {
		t.Error("the authorization server was not given the new secret")
	}
	if _, err := svc.ValidateClient(created.ClientID, secret); err == nil {
		t.Error("the old secret is still accepted after a new one was issued")
	}
	if _, err := svc.ValidateClient(created.ClientID, rotated.ClientSecret); err != nil {
		t.Errorf("the new secret was refused: %v", err)
	}
	if err := db.Raw(`SELECT client_secret, client_secret_hash FROM clients WHERE id = ?`, created.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ClientSecret != "" {
		t.Error("the new secret itself was stored")
	}
}

// A client registered before secrets were hashed still has its secret stored.
// It keeps working, gets a hash on start, and the stored secret is left for a
// separate step to remove.
func TestBackfillSecretHashes_HashesWhatWasStoredAsItIs(t *testing.T) {
	db := setupPostgres(t)
	svc := NewService(db, zap.NewNop(), stubHydra(t))
	tenantID := seedTenant(t, db)

	created, _, err := svc.Create(&CreateClientRequest{
		TenantID:   tenantID,
		Name:       "legacy-" + uuid.NewString()[:8],
		GrantTypes: []string{"client_credentials"},
		Scopes:     []string{"api"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM clients WHERE id = ?`, created.ID) })

	// Put the row in the state rows had before the hash existed.
	const legacy = "legacy-secret-kept-as-it-was"
	if err := db.Exec(`UPDATE clients SET client_secret = ?, client_secret_hash = '' WHERE id = ?`, legacy, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateClient(created.ClientID, legacy); err != nil {
		t.Fatalf("a client without a hash yet was refused its secret: %v", err)
	}
	if _, err := svc.ValidateClient(created.ClientID, legacy+"x"); err == nil {
		t.Fatal("a wrong secret was accepted for a client without a hash")
	}

	if err := BackfillSecretHashes(svc, zap.NewNop()); err != nil {
		t.Fatalf("BackfillSecretHashes: %v", err)
	}
	var stored struct{ ClientSecret, ClientSecretHash string }
	if err := db.Raw(`SELECT client_secret, client_secret_hash FROM clients WHERE id = ?`, created.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored.ClientSecretHash, "$2") {
		t.Errorf("no hash after the backfill: %q", stored.ClientSecretHash)
	}
	if stored.ClientSecret != legacy {
		t.Error("the backfill removed the stored secret; that is a separate, deliberate step")
	}
	if _, err := svc.ValidateClient(created.ClientID, legacy); err != nil {
		t.Errorf("after the backfill the secret was refused: %v", err)
	}

	// A deleted client's row holds a secret all the same, and gets a hash too.
	if err := db.Exec(`UPDATE clients SET client_secret_hash = '', deleted_at = now() WHERE id = ?`, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := BackfillSecretHashes(svc, zap.NewNop()); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT client_secret, client_secret_hash FROM clients WHERE id = ?`, created.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored.ClientSecretHash, "$2") {
		t.Errorf("a deleted client was left without a hash: %q", stored.ClientSecretHash)
	}

	// Running it again changes nothing.
	before := stored.ClientSecretHash
	if err := BackfillSecretHashes(svc, zap.NewNop()); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT client_secret, client_secret_hash FROM clients WHERE id = ?`, created.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ClientSecretHash != before {
		t.Error("a second backfill replaced an existing hash")
	}
}
