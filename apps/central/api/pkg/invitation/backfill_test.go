package invitation

import (
	"fmt"
	"testing"
	"time"

	"authway/apps/central/api/pkg/tokenhash"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TestBackfillTokenHashes_PreservesRedeemability is the key regression guard
// for the design choice migration 020 documents: unlike 010/013/014/019,
// which invalidate every outstanding token on deploy, hashing the existing
// plaintext in place must let an invitation issued before the migration
// stay redeemable by the exact token its recipient was already emailed.
func TestBackfillTokenHashes_PreservesRedeemability(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, _ := newTestService(t, db)
	tn := freshTenant(t, ts)

	email := fmt.Sprintf("legacy-%s@example.com", uuid.New().String()[:8])
	plaintext := "legacy-plaintext-token-" + uuid.New().String()
	id := uuid.New()
	if err := db.Exec(
		`INSERT INTO invitations (id, tenant_id, email, role, token_hash, status, expires_at, created_at, updated_at)
		 VALUES (?, ?, ?, 'member', ?, 'pending', ?, NOW(), NOW())`,
		id, tn.ID, email, plaintext, time.Now().Add(time.Hour),
	).Error; err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM invitations WHERE id = ?`, id) })

	// Before backfill: the column really is plaintext, and a hash-based
	// lookup (what GetByToken does) does not find it yet.
	if _, err := svc.GetByToken(plaintext); err == nil {
		t.Fatal("expected lookup to miss before backfill (row is stored as plaintext, not its hash)")
	}

	if err := BackfillTokenHashes(db, zap.NewNop()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// After backfill: the same plaintext the recipient was emailed must
	// still redeem the invitation.
	inv, err := svc.GetByToken(plaintext)
	if err != nil {
		t.Fatalf("expected the original plaintext to still resolve after backfill: %v", err)
	}
	if inv.ID != id {
		t.Errorf("resolved wrong invitation: got %v, want %v", inv.ID, id)
	}

	// And the column must actually hold the hash now, not plaintext.
	var stored string
	if err := db.Raw(`SELECT token_hash FROM invitations WHERE id = ?`, id).Scan(&stored).Error; err != nil {
		t.Fatalf("read back token_hash: %v", err)
	}
	if stored != tokenhash.Hash(plaintext) {
		t.Errorf("token_hash = %q, want the SHA-256 hex digest of the plaintext", stored)
	}
}

// TestBackfillTokenHashes_IdempotentOnAlreadyHashedRows guards against the
// backfill re-hashing a value that is already a hash — which would silently
// and permanently break every invitation created normally through Create
// (which stores tokenhash.Hash(token) from the start, never plaintext).
func TestBackfillTokenHashes_IdempotentOnAlreadyHashedRows(t *testing.T) {
	db := setupPostgres(t)
	svc, ts, _, sender := newTestServiceWithSender(t, db)
	tn := freshTenant(t, ts)

	email := fmt.Sprintf("normal-%s@example.com", uuid.New().String()[:8])
	inv, err := svc.Create(tn.ID, nil, &CreateInvitationRequest{Email: email})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID) })
	token := tokenFromLinkURL(t, sender.lastLinkURL)

	if err := BackfillTokenHashes(db, zap.NewNop()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// Running the backfill again must not touch an already-hashed row: if it
	// did, tokenhash.Hash(tokenhash.Hash(token)) would be stored instead, and
	// the invitation issued moments ago would become permanently
	// unredeemable by anyone who has only ever seen the plaintext token.
	got, err := svc.GetByToken(token)
	if err != nil {
		t.Fatalf("expected a normally-created invitation to remain redeemable after backfill: %v", err)
	}
	if got.ID != inv.ID {
		t.Errorf("resolved wrong invitation: got %v, want %v", got.ID, inv.ID)
	}
}
