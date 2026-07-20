# Database Migrations Guide

**Version**: 0.2.0+
**Last Updated**: 2026-07-20

> **How migrations actually run today**
>
> The Go migrator in `apps/central/api/internal/database/migrate.go` applies
> every migration at API startup, inside **one transaction for the whole run**.
> It is the only live path. The psql-driven flow described in
> "Migration Execution Methods" below is **deprecated** —
> `Invoke-AutoMigration` throws, and the deploy scripts no longer call it.
>
> Three consequences that contradict older sections of this document:
>
> 1. **Migration files must not contain `BEGIN;` / `COMMIT;`.** A nested COMMIT
>    commits the migrator's outer transaction early and voids the
>    all-or-nothing guarantee. (004 and 005 did exactly this until 2026-07-20.)
> 2. **`schema_migrations.version` holds migration versions only.** The
>    `('000', 'init_migration_system')` bookkeeping row shown below is gone: it
>    collided with `000_initial_schema.sql`, so the initial schema was skipped
>    on every database and no blank database could be provisioned.
> 3. **A blank database needs no manual step.** Start the API against it;
>    `000_initial_schema.sql` builds the base schema and 001..NNN evolve it.
>
> The authoritative, current reference is
> [`apps/central/api/internal/database/migrations/README.md`](../apps/central/api/internal/database/migrations/README.md).

## Overview

Authway uses a robust migration tracking system to manage database schema changes across environments. The system provides idempotent migrations, automatic tracking, and rollback support.

## Migration Tracking System

### Schema Migrations Table

All migrations are tracked in the `schema_migrations` table:

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

**Key Features**:
- **Version tracking**: Unique version number prevents duplicate execution
- **Success tracking**: Identifies failed migrations for investigation
- **Execution metrics**: Track how long each migration took
- **Error logging**: Stores error messages for failed migrations
- **Checksum validation**: Verify migration file integrity

## Migration File Structure

### Naming Convention

Migration files must follow this pattern:

```
{version}_{description}.sql
```

**Examples**:
- `001_initial_schema.sql`
- `002_add_allowed_origins.sql`
- `003_add_logout_policy.sql`

**Version Format**:
- Use 3-digit zero-padded numbers (001, 002, 003, ...)
- Sequential numbering ensures proper execution order
- Never reuse version numbers

### Migration Template

```sql
-- Migration: {Title}
-- Version: {version}
-- Date: {YYYY-MM-DD}
-- Description: {Detailed description}

-- Check if migration already applied
DO $$
BEGIN
    -- Check if migration tracking table exists
    IF NOT EXISTS (
        SELECT FROM pg_tables
        WHERE schemaname = 'public'
        AND tablename = 'schema_migrations'
    ) THEN
        -- Create migration tracking table
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

        CREATE INDEX idx_schema_migrations_version ON schema_migrations(version);
        CREATE INDEX idx_schema_migrations_executed_at ON schema_migrations(executed_at DESC);

        INSERT INTO schema_migrations (version, name, execution_time_ms, success)
        VALUES ('000', 'init_migration_system', 0, true);
    END IF;

    -- Check if this migration already applied
    IF EXISTS (
        SELECT 1 FROM schema_migrations
        WHERE version = '{version}' AND success = true
    ) THEN
        RAISE NOTICE 'Migration {version} already applied, skipping...';
        RETURN;
    END IF;
END $$;

-- Begin migration transaction
BEGIN;

-- Record migration start
INSERT INTO schema_migrations (version, name, success)
VALUES ('{version}', '{name}', false)
ON CONFLICT (version) DO UPDATE SET success = false, executed_at = CURRENT_TIMESTAMP;

-- YOUR MIGRATION CODE HERE (idempotent SQL)
ALTER TABLE your_table
ADD COLUMN IF NOT EXISTS new_column VARCHAR(255);

-- Update migration record to success
UPDATE schema_migrations
SET success = true,
    executed_at = CURRENT_TIMESTAMP
WHERE version = '{version}';

COMMIT;

-- Migration complete
SELECT version, name, executed_at, success
FROM schema_migrations
WHERE version = '{version}';
```

### Idempotency Guidelines

All migration SQL must be idempotent (safe to run multiple times):

✅ **Good (Idempotent)**:
```sql
ALTER TABLE clients ADD COLUMN IF NOT EXISTS new_field VARCHAR(255);
CREATE INDEX IF NOT EXISTS idx_name ON table(column);
```

❌ **Bad (Not Idempotent)**:
```sql
ALTER TABLE clients ADD COLUMN new_field VARCHAR(255);  -- Fails if exists
CREATE INDEX idx_name ON table(column);  -- Fails if exists
```

**Idempotent Patterns**:

```sql
-- Adding columns
ALTER TABLE table_name
ADD COLUMN IF NOT EXISTS column_name TYPE DEFAULT value;

-- Adding constraints (with drop if exists)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'constraint_name') THEN
        ALTER TABLE table_name DROP CONSTRAINT constraint_name;
    END IF;

    ALTER TABLE table_name ADD CONSTRAINT constraint_name CHECK (condition);
END $$;

-- Creating indexes
CREATE INDEX IF NOT EXISTS idx_name ON table_name(column_name);

-- Updating existing data (with conditions)
UPDATE table_name
SET column = COALESCE(column, default_value)
WHERE column IS NULL;
```

## Migration Execution Methods

> **Deprecated except for §2.** Only the Go migration runner (§2) still runs.
> The psql-based methods below are kept as a record of how migrations were
> applied before v0.4.0; `Invoke-AutoMigration` now throws if called.
> Their `BEGIN;`/`COMMIT;` templates do **not** apply to migration files.

### 1. PowerShell Script (Azure CLI)

**For manual deployment and production environments**

```powershell
# Run single migration
.\scripts\deploy\run-migration-azure.ps1 -MigrationFile "003_add_logout_policy.sql"

# Dry run (preview SQL without execution)
.\scripts\deploy\run-migration-azure.ps1 -MigrationFile "003_add_logout_policy.sql" -DryRun

# Force re-run (even if already applied)
.\scripts\deploy\run-migration-azure.ps1 -MigrationFile "003_add_logout_policy.sql" -Force
```

**Features**:
- Pre-execution validation (checks if already applied)
- Transaction-based execution
- Automatic tracking table creation
- Detailed logging and error reporting
- Execution time measurement

### 2. Go Migration Runner (Automatic)

**For development and automatic deployment**

The Central API automatically runs migrations on startup:

```go
// In apps/central/api/cmd/main.go
if err := database.RunMigrations(db, zapLogger); err != nil {
    zapLogger.Fatal("Failed to run database migrations", zap.Error(err))
}
```

**Features**:
- Embedded migrations (compiled into binary)
- Automatic execution on service start
- Skips already-applied migrations
- Checksum validation
- Transaction-based with rollback on failure

**Migration Location**: `apps/central/api/internal/database/migrations/`

### 3. Intelligent Auto-Migration (Recommended)

**For deployment with minimal delay and automatic pending detection**

The intelligent auto-migration system (`migration-helpers.ps1`) provides fast, safe, and automatic migration execution:

```powershell
# Automatic detection and execution
.\scripts\deploy\deploy-all.ps1 -ForceMigration

# Manual execution with auto-detection
.\scripts\deploy\deploy-all.ps1
```

**Features**:
- ⚡ **Fast Detection**: 1-2 second check vs 5-10 second manual scan
- 🎯 **Intelligent Execution**: Runs only pending migrations
- ⏭️ **Zero Delay Skip**: Instantly skips if no migrations needed
- 🔒 **Parallel-Safe**: PostgreSQL advisory locks prevent concurrent execution
- 🔄 **Transaction-Based**: Combined SQL + tracking in single transaction
- 📊 **Progress Tracking**: Real-time execution status and metrics

**Auto-Migration Functions** (`scripts/deploy/migration-helpers.ps1`):

| Function | Purpose | Usage |
|----------|---------|-------|
| `Initialize-PsqlPath` | Find and set psql executable path | Auto-called |
| `Invoke-FastQuery` | Execute SQL via psql with BOM-free UTF-8 | Internal |
| `Get-PendingMigrations` | Fast detection of pending migrations | Returns array |
| `Get-MigrationLock` | Acquire advisory lock for safe execution | Auto-managed |
| `Release-MigrationLock` | Release advisory lock after execution | Auto-managed |
| `Invoke-Migration` | Execute single migration in transaction | Internal |
| `Invoke-AutoMigration` | Main orchestration function | Called by deploy |

**Migration Location**: `D:\data\Authway\migrations\`

**Performance**:
- Detection: ~1-2 seconds (psql query)
- No pending: 0 seconds (immediate skip)
- Execution: ~100-500ms per migration

### 4. Full Deployment Script

**For complete system deployment with auto-migration**

```powershell
.\scripts\deploy\deploy-all.ps1

# Skip migrations (if already applied manually)
.\scripts\deploy\deploy-all.ps1 -SkipMigration
```

The deployment script:
1. Detects pending migrations
2. Asks for confirmation
3. Runs migrations using Azure CLI
4. Deploys services (Auth API, Central API, Admin UI, Auth UI)

## Migration Status Management

### Check Migration Status

```powershell
.\scripts\deploy\check-migration-status.ps1

# Verbose mode (shows error messages)
.\scripts\deploy\check-migration-status.ps1 -Verbose
```

**Output Example**:
```
📋 적용된 마이그레이션:

  Version  | Name                         | Status    | Executed At        | Time (ms)
  ─────────┼──────────────────────────────┼───────────┼────────────────────┼──────────
  000      | init_migration_system        | ✅ SUCCESS  | 2025-11-16 10:00:00|        0
  002      | add_allowed_origins          | ✅ SUCCESS  | 2025-11-16 10:05:00|     1234
  003      | add_logout_policy            | ✅ SUCCESS  | 2025-11-16 10:10:00|      567

📁 보류 중인 마이그레이션:
  ✅ 모든 마이그레이션이 적용되었습니다

📊 요약:
  ✅ 성공: 3개
  ⏳ 보류: 0개
```

### Query Migration Status (SQL)

```sql
-- List all migrations
SELECT version, name, executed_at, success, execution_time_ms
FROM schema_migrations
ORDER BY version;

-- Check specific migration
SELECT * FROM schema_migrations WHERE version = '003';

-- Find failed migrations
SELECT * FROM schema_migrations WHERE success = false;

-- Get latest migration
SELECT * FROM schema_migrations
ORDER BY executed_at DESC
LIMIT 1;
```

## Rollback Procedures

### Automatic Rollback

Failed migrations automatically rollback due to transaction-based execution:

```sql
BEGIN;  -- Start transaction

-- Migration SQL here...

-- If error occurs, automatic ROLLBACK happens
-- schema_migrations record marked with success=false

COMMIT;  -- Only if no errors
```

### Manual Rollback

Each migration should have a corresponding rollback file:

**Migration**: `003_add_logout_policy.sql`
**Rollback**: `003_add_logout_policy_rollback.sql`

```sql
-- 003_add_logout_policy_rollback.sql
BEGIN;

-- Reverse the changes
ALTER TABLE clients DROP COLUMN IF EXISTS post_logout_redirect_uris;
ALTER TABLE clients DROP COLUMN IF EXISTS logout_redirect_policy;
ALTER TABLE clients DROP COLUMN IF EXISTS default_logout_uri;
ALTER TABLE clients DROP COLUMN IF EXISTS allow_wildcard_logout;

DROP INDEX IF EXISTS idx_clients_logout_policy;

-- Remove from tracking (optional - keep for audit trail)
-- DELETE FROM schema_migrations WHERE version = '003';

COMMIT;
```

**Execute Rollback**:
```powershell
.\scripts\deploy\run-migration-azure.ps1 -MigrationFile "003_add_logout_policy_rollback.sql" -Force
```

## Best Practices

### 1. Development Workflow

```
1. Create migration file
   ├─ Use template structure
   ├─ Follow naming convention
   └─ Make SQL idempotent

2. Test locally
   ├─ Run migration on dev database
   ├─ Verify schema changes
   └─ Test rollback

3. Test idempotency
   ├─ Run migration twice
   └─ Verify no errors

4. Commit to repository
   ├─ Add migration file
   ├─ Add rollback file
   └─ Update documentation

5. Deploy to staging
   ├─ Test automatic migration (Go runner)
   └─ Verify application behavior

6. Deploy to production
   ├─ Run manually via PowerShell
   └─ Monitor execution
```

### 2. Production Deployment

**Pre-Deployment Checklist**:
- [ ] Migration tested on dev/staging
- [ ] Rollback script prepared
- [ ] Database backup created
- [ ] Downtime window scheduled (if needed)
- [ ] Team notified

**Deployment Steps**:
```powershell
# 1. Check current status
.\scripts\deploy\check-migration-status.ps1

# 2. Dry run migration
.\scripts\deploy\run-migration-azure.ps1 -MigrationFile "XXX_name.sql" -DryRun

# 3. Execute migration
.\scripts\deploy\run-migration-azure.ps1 -MigrationFile "XXX_name.sql"

# 4. Verify success
.\scripts\deploy\check-migration-status.ps1 -Verbose

# 5. Deploy services
.\scripts\deploy\deploy-all.ps1 -SkipMigration
```

### 3. Error Handling

**If Migration Fails**:

1. **Check error message**:
   ```powershell
   .\scripts\deploy\check-migration-status.ps1 -Verbose
   ```

2. **Review schema_migrations table**:
   ```sql
   SELECT * FROM schema_migrations WHERE success = false ORDER BY executed_at DESC LIMIT 1;
   ```

3. **Fix the issue**:
   - Correct SQL syntax errors
   - Resolve constraint violations
   - Fix data type mismatches

4. **Re-run with -Force**:
   ```powershell
   .\scripts\deploy\run-migration-azure.ps1 -MigrationFile "XXX_name.sql" -Force
   ```

5. **If unfixable, rollback**:
   ```powershell
   .\scripts\deploy\run-migration-azure.ps1 -MigrationFile "XXX_name_rollback.sql" -Force
   ```

## Migration Types

### 1. Schema Changes

Add/modify tables, columns, constraints:

```sql
-- Add column
ALTER TABLE clients ADD COLUMN IF NOT EXISTS new_field VARCHAR(255);

-- Modify column type
ALTER TABLE clients ALTER COLUMN field TYPE TEXT;

-- Add constraint
ALTER TABLE clients ADD CONSTRAINT check_field CHECK (field IN ('value1', 'value2'));
```

### 2. Data Migrations

Update existing data:

```sql
-- Set default values for existing rows
UPDATE clients
SET new_field = COALESCE(new_field, 'default_value')
WHERE new_field IS NULL;

-- Migrate data format
UPDATE clients
SET redirect_uris = ARRAY[old_redirect_uri]
WHERE redirect_uris IS NULL AND old_redirect_uri IS NOT NULL;
```

### 3. Index Changes

Add/remove indexes for performance:

```sql
-- Add index
CREATE INDEX IF NOT EXISTS idx_clients_field ON clients(field);

-- Add GIN index for arrays
CREATE INDEX IF NOT EXISTS idx_clients_array_field ON clients USING GIN (array_field);
```

### 4. Reference Data

Insert initial/reference data:

```sql
-- Insert default configuration (idempotent)
INSERT INTO config (key, value)
VALUES ('setting', 'value')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;
```

## Environment-Specific Considerations

### Development
- Automatic migrations on startup (Go runner)
- Fast iteration
- Easy rollback

### Staging
- Test automatic and manual migration
- Verify with production-like data
- Performance testing

### Production
- Manual migration execution
- Database backup before migration
- Maintenance window if needed
- Monitoring during and after migration

## Testing Migrations

### Test Scripts

Located in `scripts/test/`, these scripts help validate the migration system:

#### Migration System Tests

**`test-auto-migration.ps1`**
- Tests `Get-PendingMigrations` function independently
- Verifies return value types and array handling
- Validates migration directory paths
- Usage: `.\scripts\test\test-auto-migration.ps1`

**`test-psql-connection.ps1`**
- Validates PostgreSQL database connectivity
- Tests environment variable configuration
- Executes simple query to verify access
- Usage: `.\scripts\test\test-psql-connection.ps1`

**`test-tracking-table.ps1`**
- Displays `schema_migrations` table contents
- Shows applied versions, execution times, and status
- Useful for manual verification
- Usage: `.\scripts\test\test-tracking-table.ps1`

**`test-env-loading.ps1`**
- Validates `.env` file loading
- Checks required database environment variables
- Masks sensitive values in output
- Usage: `.\scripts\test\test-env-loading.ps1`

#### Migration Status Check

```powershell
# Check current migration status (fast psql-based)
.\scripts\deploy\check-migration-status-psql.ps1

# Output shows:
# - Applied migrations with timestamps
# - Pending migrations awaiting execution
# - Success/failure counts
```

### PowerShell Empty Array Fix

**Critical Issue**: PowerShell converts empty arrays to `$null` when returned from functions.

**Problem**:
```powershell
function Get-Data {
    $items = @()  # Empty array
    return $items  # PowerShell converts to $null!
}

$result = Get-Data
if ($null -eq $result) {  # TRUE! Even though we returned an array
    # This executes unexpectedly
}
```

**Solution**: Use unary array operator `, `
```powershell
function Get-Data {
    $items = @()
    return , $items  # Comma prevents null conversion
}

$result = Get-Data
if ($null -eq $result) {  # FALSE - correctly returns empty array
    # This won't execute
}
```

Applied in `Get-PendingMigrations`:
```powershell
# PowerShell이 빈 배열을 $null로 변환하지 않도록 명시적으로 배열로 반환
return , $pendingMigrations
```

## Troubleshooting

### Issue: "Database connection failure" (데이터베이스 연결 실패)

**Symptom**: Auto-migration reports connection failure even when database is accessible.

**Cause**: Empty array returned as `$null` (see PowerShell Empty Array Fix above).

**Solution**: Already fixed in `migration-helpers.ps1` line 162 with `, $pendingMigrations`.

### Issue: "psql not found in VSCode"

**Symptom**: psql works in regular terminal but not in VSCode terminal.

**Cause**: VSCode caches PATH at startup before psql installation.

**Solution**:
1. Restart VSCode after installing PostgreSQL, OR
2. Open new PowerShell terminal, OR
3. Auto-detection finds psql in common paths (already implemented)

### Issue: "constraint already exists"

**Solution**: Make constraint creation idempotent:
```sql
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'constraint_name') THEN
        ALTER TABLE table_name DROP CONSTRAINT constraint_name;
    END IF;
    ALTER TABLE table_name ADD CONSTRAINT constraint_name CHECK (condition);
END $$;
```

### Issue: "column already exists"

**Solution**: Use `IF NOT EXISTS`:
```sql
ALTER TABLE table_name ADD COLUMN IF NOT EXISTS column_name TYPE;
```

### Issue: Migration appears stuck

**Solution**: Check for locks:
```sql
SELECT * FROM pg_locks WHERE relation = 'table_name'::regclass;
```

### Issue: Rollback needed after partial migration

**Solution**: Transaction-based migrations prevent partial state - entire migration rolls back on error.

## See Also

- [Deployment Guide](./DEPLOYMENT.md)
- [Logout Implementation](./LOGOUT_IMPLEMENTATION.md)
- [Database Schema](./DATABASE_SCHEMA.md)
