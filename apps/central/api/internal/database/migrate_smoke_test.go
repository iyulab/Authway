package database

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// These tests run the REAL migrator (RunMigrations) against a live Postgres, in
// the exact single-outer-transaction context production uses.
//
// MIGRATE_SMOKE_DSN points at any Postgres the test may create databases on;
// each test provisions its own throwaway database and drops it afterwards, so
// nothing here depends on a hand-bootstrapped schema. That is only possible
// since 000_initial_schema.sql became a real, applicable migration — before
// that, a blank database failed at 001 with `relation "users" does not exist`.

// adminDB connects to the server named by MIGRATE_SMOKE_DSN for CREATE/DROP DATABASE.
func adminDB(t *testing.T) (*gorm.DB, *url.URL) {
	t.Helper()
	dsn := os.Getenv("MIGRATE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MIGRATE_SMOKE_DSN not set; skipping live migration tests")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("MIGRATE_SMOKE_DSN must be a postgres:// URL: %v", err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return db, u
}

// freshDatabase creates an empty database and returns a connection to it.
// CREATE DATABASE cannot run inside a transaction, hence the raw Exec on the
// admin connection rather than anything transactional.
func freshDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	admin, u := adminDB(t)

	// Test names are unique per run and safe as identifiers once lowercased with
	// non-alphanumerics folded away.
	name := "authway_mt_" + strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		default:
			return '_'
		}
	}, t.Name())

	admin.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, name))
	if err := admin.Exec(fmt.Sprintf(`CREATE DATABASE %s`, name)).Error; err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}
	t.Cleanup(func() {
		admin.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, name))
	})

	target := *u
	target.Path = "/" + name
	db, err := gorm.Open(postgres.Open(target.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect to %s: %v", name, err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

func mustBool(t *testing.T, db *gorm.DB, query string, args ...any) bool {
	t.Helper()
	var v bool
	if err := db.Raw(query, args...).Scan(&v).Error; err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return v
}

// schemaFingerprint renders every column of every Authway table as one sorted
// string, so two schema states can be compared for exact equality.
func schemaFingerprint(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var rows []string
	err := db.Raw(`
		SELECT table_name || '.' || column_name || ':' || data_type || ':' || is_nullable
		  FROM information_schema.columns
		 WHERE table_schema = 'public'
		 ORDER BY 1
	`).Scan(&rows).Error
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	return strings.Join(rows, "\n")
}

// TestMigrateFreshDatabase provisions a blank database end to end. This is the
// case that was impossible until the `000` version collision was removed: the
// bookkeeping sentinel occupied the same version as the initial schema file, so
// the schema was skipped on every database and nothing could be provisioned
// without running SQL by hand.
func TestMigrateFreshDatabase(t *testing.T) {
	db := freshDatabase(t)

	if err := RunMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("RunMigrations on a blank database failed: %v", err)
	}

	if !mustBool(t, db, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000' AND success)`) {
		t.Fatal("000 was not applied — the initial schema is being skipped again")
	}
	// The sentinel's name is the tell: if this ever reads 'init_migration_system'
	// the bookkeeping row is back and 000 is a phantom, not a real migration.
	var name string
	if err := db.Raw(`SELECT name FROM schema_migrations WHERE version='000'`).Scan(&name).Error; err != nil {
		t.Fatalf("read 000 name: %v", err)
	}
	if name != "initial_schema" {
		t.Errorf("schema_migrations 000 name = %q; want %q", name, "initial_schema")
	}

	// Verify the columns/renames every later migration is responsible for.
	checks := []struct {
		query string
		want  string
	}{
		{`SELECT to_regclass('public.clients') IS NOT NULL`, "clients exists"},
		{`SELECT to_regclass('public.users') IS NOT NULL`, "users exists"},
		{`SELECT to_regclass('public.tenants') IS NOT NULL`, "tenants exists"},
		{`SELECT EXISTS(SELECT 1 FROM tenants WHERE slug='default')`, "default tenant seeded"},
		{`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='clients' AND column_name='skip_consent')`, "clients.skip_consent"},
		{`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='clients' AND column_name='skip_logout_consent')`, "clients.skip_logout_consent"},
		{`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='password_resets' AND column_name='token_hash')`, "password_resets.token_hash"},
		{`SELECT NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='password_resets' AND column_name='token')`, "password_resets.token dropped"},
		{`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='email_verifications' AND column_name='token_hash')`, "email_verifications.token_hash"},
		{`SELECT NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='email_verifications' AND column_name='token')`, "email_verifications.token dropped"},
		// redirect_uris is the constraint the SQLite harness cannot express, and
		// the one a nil slice violated in production. Pin it here.
		{`SELECT is_nullable='NO' FROM information_schema.columns WHERE table_name='clients' AND column_name='redirect_uris'`, "clients.redirect_uris is NOT NULL"},
		{`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='clients' AND column_name='access_token_strategy')`, "clients.access_token_strategy"},
		// 015 must not opt any client in — enabling JWT is an operational decision.
		// Assert this at the schema level (no column default, so adding the column
		// pins nobody) rather than by counting rows: a row count also reflects
		// whatever the application did after migrating, which is not 015's doing.
		// Postgres records the migration's explicit `DEFAULT NULL` as the default
		// expression `NULL::character varying`, not as an absent default — so match
		// on "the default evaluates to NULL" rather than "there is no default".
		{`SELECT coalesce(column_default, 'NULL') ILIKE 'null%' FROM information_schema.columns WHERE table_name='clients' AND column_name='access_token_strategy'`, "access_token_strategy defaults to NULL (opts nobody in)"},
		{`SELECT EXISTS(SELECT 1 FROM information_schema.constraint_column_usage WHERE table_name='clients' AND constraint_name='clients_access_token_strategy_check')`, "access_token_strategy CHECK constraint present"},
		{`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='012' AND success)`, "012 recorded"},
		{`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='013' AND success)`, "013 recorded"},
		{`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='014' AND success)`, "014 recorded"},
		{`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='015' AND success)`, "015 recorded"},
	}
	for _, c := range checks {
		if !mustBool(t, db, c.query) {
			t.Errorf("check FAILED: %s", c.want)
		} else {
			t.Logf("check ok: %s", c.want)
		}
	}
}

// TestMigrateRerunIsNoOp covers the redeploy path: a database that is already at
// the head migration must come out of a second run byte-identical.
func TestMigrateRerunIsNoOp(t *testing.T) {
	db := freshDatabase(t)
	if err := RunMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("first run: %v", err)
	}
	before := schemaFingerprint(t, db)

	if err := RunMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if after := schemaFingerprint(t, db); after != before {
		t.Error("re-running the migrator changed the schema; it must be a no-op")
	}
}

// TestInitialSchemaIsIdempotentOnPopulatedDatabase is the production-safety gate.
//
// Existing deployments carry a legacy `('000', 'init_migration_system')` row from
// the removed sentinel, so they skip 000 and never execute it. This test refuses
// to rely on that: it deletes the bookkeeping row from a fully-migrated database
// holding real rows, forcing 000_initial_schema.sql to run against a populated,
// 015-era schema — and asserts the schema and the data both come out untouched.
//
// If someone ever reintroduces a destructive statement into the initial schema,
// or writes CREATE TABLE without IF NOT EXISTS, this fails instead of production.
func TestInitialSchemaIsIdempotentOnPopulatedDatabase(t *testing.T) {
	db := freshDatabase(t)
	if err := RunMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("initial provisioning: %v", err)
	}

	// Seed data that a wipe would destroy, including a column that only exists
	// because of a later migration.
	var tenantID string
	if err := db.Raw(`
		INSERT INTO tenants (name, slug, description, active)
		VALUES ('Idempotence Fixture', 'idempotence-fixture', 'must survive', true)
		RETURNING id::text
	`).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO clients (tenant_id, client_id, client_secret, name, redirect_uris, grant_types, scopes, access_token_strategy)
		VALUES (?::uuid, 'fixture-client', 'secret', 'Fixture', '{}', '{client_credentials}', '{api}', 'jwt')
	`, tenantID).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}

	before := schemaFingerprint(t, db)

	// Force 000 to be eligible again — exactly the situation a naive "renumber
	// the sentinel away" fix would have created on every existing deployment.
	if err := db.Exec(`DELETE FROM schema_migrations WHERE version='000'`).Error; err != nil {
		t.Fatalf("unrecord 000: %v", err)
	}
	if err := RunMigrations(db, zap.NewNop()); err != nil {
		t.Fatalf("re-applying 000 against a populated database failed: %v", err)
	}

	if after := schemaFingerprint(t, db); after != before {
		t.Error("re-applying the initial schema changed the schema of a populated database")
	}
	if !mustBool(t, db, `SELECT EXISTS(SELECT 1 FROM tenants WHERE slug='idempotence-fixture')`) {
		t.Fatal("seeded tenant is gone — the initial schema destroyed existing data")
	}
	if !mustBool(t, db, `SELECT EXISTS(SELECT 1 FROM clients WHERE client_id='fixture-client' AND access_token_strategy='jwt')`) {
		t.Fatal("seeded client (or its post-000 column value) is gone")
	}
	// The default tenant must not be duplicated by the second insert.
	if !mustBool(t, db, `SELECT count(*)=1 FROM tenants WHERE slug='default'`) {
		t.Error("default tenant was inserted twice; the seed is not ON CONFLICT-guarded")
	}
}
