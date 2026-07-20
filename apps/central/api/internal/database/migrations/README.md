# Database Migrations

This directory contains all SQL migration files for the Authway Central API.

## Migration System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    SINGLE SOURCE OF TRUTH                        │
│   apps/central/api/internal/database/migrations/*.sql            │
└─────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              │                               │
              ▼                               ▼
   ┌─────────────────────┐       ┌─────────────────────┐
   │   Go App Startup    │       │   Deploy Script     │
   │   (Embedded SQL)    │       │   (Pre-deployment)  │
   └─────────────────────┘       └─────────────────────┘
```

## Creating New Migrations

### 1. Naming Convention

```
{VERSION}_{DESCRIPTION}.sql

Examples:
- 000_initial_schema.sql      (Base schema — guarded, applies only to a blank DB)
- 001_add_user_claims.sql     (Feature addition)
- 008_add_impersonation_missing_columns.sql  (Schema fix)
```

- **VERSION**: 3-digit zero-padded number (000, 001, 002...)
- **DESCRIPTION**: Snake_case description of the change

### 2. File Structure

```sql
-- Migration: Brief description
-- Version: XXX
-- Date: YYYY-MM-DD

-- ============================================================
-- 1. Section Name
-- ============================================================

-- Your SQL statements here

-- ============================================================
-- Migration Complete
-- ============================================================
-- Summary of changes
```

### 3. Best Practices

#### DO:
- **Write NO `BEGIN;` / `COMMIT;`.** `RunMigrations` wraps the entire run in one
  transaction, so a migration that opens its own commits the *outer* one early and
  silently destroys the all-or-nothing guarantee for everything after it. An earlier
  version of this document advised the opposite, and 004 and 005 followed it.
- Use `IF NOT EXISTS` / `IF EXISTS` for idempotency
- Add comments explaining the purpose of each section
- Create indexes for frequently queried columns
- Handle existing data when adding NOT NULL columns:
  ```sql
  ALTER TABLE foo ADD COLUMN bar VARCHAR(255);
  UPDATE foo SET bar = 'default_value' WHERE bar IS NULL;
  ALTER TABLE foo ALTER COLUMN bar SET NOT NULL;
  ```

#### DON'T:
- Never modify existing migration files after deployment
- Never use columns that don't exist in the current schema
- Never assume specific data exists (use safe lookups)
- Never remove columns that are still referenced in code

### 4. Adding New Columns to Existing Tables

```sql
-- Step 1: Add column as nullable
ALTER TABLE table_name
ADD COLUMN IF NOT EXISTS new_column TYPE;

-- Step 2: Populate existing rows
UPDATE table_name SET new_column = default_value WHERE new_column IS NULL;

-- Step 3: Add NOT NULL constraint if needed
ALTER TABLE table_name ALTER COLUMN new_column SET NOT NULL;

-- Step 4: Add index if needed
CREATE INDEX IF NOT EXISTS idx_table_column ON table_name(new_column);
```

## Provisioning a Blank Database

Nothing manual is required: start the API against an empty database and
`000_initial_schema.sql` creates the base schema, then 001..NNN evolve it.

`000_initial_schema.sql` is guarded — it does nothing at all if `tenants` already
exists — so it is safe on every existing deployment. Per-statement
`IF NOT EXISTS` would not be enough, because later migrations drop columns the
000-era indexes reference.

To reset a development database, run the destructive script by hand and restart
the app:

```bash
psql "$DSN" -f scripts/bootstrap/dev-clean-slate.sql
```

That script only drops (including `schema_migrations`); it never recreates, so
the migrations stay the single source of truth for schema.

## Execution Flow

### Local Development (`start-dev.ps1`)

1. Go application starts
2. `internal/database/migrate.go` runs
3. Reads embedded SQL files from this directory
4. Applies pending migrations in version order
5. Records applied migrations in `schema_migrations` table

### Azure Deployment (`deploy-all.ps1`)

1. Pre-deployment: `migration-helpers.ps1` checks for pending migrations
2. Prompts user for confirmation (unless `-ForceMigration`)
3. Applies migrations via psql
4. On success: proceeds with container deployment
5. Container startup: Go app verifies migrations (already applied)

## Rollback Strategy

For complex changes, create a companion rollback file:

```
008_add_feature.sql          # Forward migration
008_add_feature_rollback.sql # Rollback migration (optional)
```

Rollback files are not auto-executed. They serve as documentation for manual recovery.

## Troubleshooting

### Migration Failed Mid-way

If a migration fails, check `schema_migrations` table:

```sql
SELECT version, name, success, error_message, executed_at
FROM schema_migrations
ORDER BY executed_at DESC;
```

Failed migrations are rolled back by the transaction. Fix the SQL and retry.

### Column/Table Already Exists

Use idempotent statements:

```sql
ALTER TABLE foo ADD COLUMN IF NOT EXISTS bar VARCHAR(255);
CREATE TABLE IF NOT EXISTS new_table (...);
CREATE INDEX IF NOT EXISTS idx_name ON table(column);
```

### Version Conflict

Each version number must be unique. Check existing versions before creating new migrations:

```bash
ls -la apps/central/api/internal/database/migrations/*.sql
```

`schema_migrations.version` belongs to migration files and nothing else. It used
to also hold bookkeeping rows (`('000', 'init_migration_system')`), which claimed
the same version as the initial schema — so `000` was treated as applied on every
database, was never executed, and no blank database could be provisioned without
running SQL by hand. If you ever feel like recording a meta event in this table,
that is the failure you would be recreating.

## Schema Migrations Table

```sql
CREATE TABLE schema_migrations (
    id SERIAL PRIMARY KEY,
    version VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    execution_time_ms INTEGER,
    checksum VARCHAR(64),
    success BOOLEAN NOT NULL DEFAULT TRUE,
    error_message TEXT
);
```

## Quick Reference

| Action | Location |
|--------|----------|
| Create new migration | `apps/central/api/internal/database/migrations/` |
| Check applied migrations | Query `schema_migrations` table |
| Run locally | Start Go app (`start-dev.ps1`) |
| Deploy to Azure | `scripts/deploy/deploy-all.ps1` |
| Manual migration | `scripts/deploy/run-migration-azure.ps1` |
