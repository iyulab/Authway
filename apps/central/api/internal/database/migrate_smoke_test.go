package database

import (
	"os"
	"testing"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestMigrateSmoke runs the REAL migrator (RunMigrations) against a live Postgres
// bootstrapped with 000_v0_clean_slate.sql, exercising migrations 001..014 in the
// exact single-outer-transaction context prod uses — including the nested BEGIN/COMMIT
// inside individual migration files. Skips unless MIGRATE_SMOKE_DSN is set.
func TestMigrateSmoke(t *testing.T) {
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live migration smoke test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	if err := RunMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// Verify the new columns/renames landed exactly as the deploy expects.
	checks := []struct {
		query string
		want  string
	}{
		{`SELECT to_regclass('public.clients') IS NOT NULL`, "clients exists"},
		{`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='clients' AND column_name='skip_consent')`, "clients.skip_consent"},
		{`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='clients' AND column_name='skip_logout_consent')`, "clients.skip_logout_consent"},
		{`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='password_resets' AND column_name='token_hash')`, "password_resets.token_hash"},
		{`SELECT NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='password_resets' AND column_name='token')`, "password_resets.token dropped"},
		{`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='email_verifications' AND column_name='token_hash')`, "email_verifications.token_hash"},
		{`SELECT NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='email_verifications' AND column_name='token')`, "email_verifications.token dropped"},
		{`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='012' AND success)`, "012 recorded"},
		{`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='013' AND success)`, "013 recorded"},
		{`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='014' AND success)`, "014 recorded"},
	}
	for _, c := range checks {
		var ok bool
		if err := db.Raw(c.query).Scan(&ok).Error; err != nil {
			t.Fatalf("check %q query error: %v", c.want, err)
		}
		if !ok {
			t.Errorf("check FAILED: %s", c.want)
		} else {
			t.Logf("check ok: %s", c.want)
		}
	}
}
