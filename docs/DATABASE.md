# Database Migrations

How the Central API's schema evolves, and how to check it in a deployed
environment. Rules for writing a migration file live next to the files:
[`apps/central/api/internal/database/migrations/README.md`](../apps/central/api/internal/database/migrations/README.md).

## How migrations run

There is one path. The API embeds every file in
`apps/central/api/internal/database/migrations/` and, at startup, before it
serves anything:

1. takes `pg_advisory_xact_lock(999999)`, so concurrent replicas migrate one at a
   time;
2. creates `schema_migrations` if it is missing;
3. applies, in version order, every version that has no successful row there;
4. commits — **all pending migrations share one transaction**.

If any statement fails, the whole run rolls back, nothing is recorded, and the API
exits instead of starting on a half-migrated schema. Deploying a new API image is
therefore what applies its migrations; the deploy scripts do not run SQL
themselves.

The API also refuses to start when the directory is ambiguous: a `.sql` file whose
name is not `<version>_<name>.sql`, or two files with the same version. Both would
otherwise be skipped silently.

A blank database needs no manual step — `000_initial_schema.sql` builds the base
schema and the rest evolve it.

## Tracking table

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

- `version` belongs to migration files only. Never record bookkeeping rows in it.
- `checksum` (SHA-256 of the file) is recorded for reference; it is not compared on
  later runs. Never edit a migration that has been deployed — add a new one.
- Databases older than the startup migrator may carry rows written by earlier
  tooling (different `name` spelling, a `000` row named after the old migration
  system). They are harmless: the version is what counts.

## Checking a deployed environment

```powershell
.\scripts\deploy\staging\check-migration-status.ps1
.\scripts\deploy\prod\check-migration-status.ps1
```

Read-only. It compares the target's `schema_migrations` with the migration files
in your working tree and prints applied rows, the latest version on each side, and
anything pending. Exit code `0` = nothing pending or failed, `1` = could not
connect or query, `2` = pending or failed rows. Pending versions are expected
before a deploy and a defect after one.

It needs `psql` and the target's `.env` (see [scripts/deploy/README.md](../scripts/deploy/README.md)).

For an ad-hoc look:

```sql
SELECT version, name, success, executed_at
FROM schema_migrations
ORDER BY version;
```

## Changes that cannot be undone

There are no down migrations. A mistake is corrected by a new forward migration.

That makes destructive changes — dropping a column or table, narrowing a type,
deleting data — a two-step change:

1. Ship application code that no longer reads or writes the column, and deploy it.
2. In a **later** deployment that contains no other application change, ship the
   migration that drops it, and run `check-migration-status` against the target
   before and after. Immediately before that deployment, query what the drop
   would delete on the target itself — a column the code stopped reading can
   still hold values written before then — and export anything worth keeping.

Doing both at once leaves no version of the application to roll back to: the
previous image still expects the column. Keep the deployment that drops it free of
other changes so that a problem after it can only have one cause.

Before a destructive migration in production, confirm a restorable backup exists.
If reversal SQL is worth writing down, keep it with the change description, not in
the migrations directory — a `*_rollback.sql` there shares its migration's version
and the API refuses to start.

## Troubleshooting

**The API exits at startup with a migration error.** Read the API log: the failing
version and the database error are logged. Nothing from that run was applied. Fix
the migration (or the data it tripped on) and redeploy.

**"share version" / "does not match `<version>_<name>.sql`".** Two files claim one
version, or a `.sql` file has no version prefix. Renumber or move the file.

**A migration seems stuck.** Another instance holds the migration lock and is still
running, or a long transaction blocks a DDL statement:

```sql
SELECT pid, state, wait_event_type, query
FROM pg_stat_activity
WHERE datname = current_database() AND state <> 'idle';
```

**"column already exists" / "constraint already exists".** Write the statement
idempotently (`ADD COLUMN IF NOT EXISTS`, a `DO $$ … $$` guard for constraints) —
see the migrations README.

## See also

- [Deployment Guide](./DEPLOYMENT.md)
- [Migration file rules](../apps/central/api/internal/database/migrations/README.md)
