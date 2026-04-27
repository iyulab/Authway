# ============================================================
# Upgrade schema_migrations table structure
# ============================================================
# Upgrades from old structure (migration_file) to new (version)
# ============================================================

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🔄 Upgrade Schema Migrations Table" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

# Load environment variables
$ScriptDir = $PSScriptRoot
$EnvFile = Join-Path $ScriptDir ".env"

if (-not (Test-Path $EnvFile)) {
    Write-Host "❌ .env 파일을 찾을 수 없습니다: $EnvFile" -ForegroundColor Red
    exit 1
}

$envVars = @{}
Get-Content $EnvFile | ForEach-Object {
    if ($_ -match '^([^#][^=]+)=(.+)$') {
        $envVars[$matches[1].Trim()] = $matches[2].Trim()
    }
}

Write-Host "🗄️  데이터베이스: $($envVars['AUTHWAY_DATABASE_NAME'])" -ForegroundColor White
Write-Host "🌐 호스트: $($envVars['AUTHWAY_DATABASE_HOST'])" -ForegroundColor White
Write-Host ""

# Check Azure CLI
$azCliPath = Get-Command az -ErrorAction SilentlyContinue
if (-not $azCliPath) {
    Write-Host "❌ Azure CLI를 찾을 수 없습니다." -ForegroundColor Red
    exit 1
}

# Check Azure login
$azAccount = az account show 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Azure에 로그인되어 있지 않습니다." -ForegroundColor Red
    exit 1
}

$serverName = $envVars['AUTHWAY_DATABASE_HOST'] -replace '\.postgres\.database\.azure\.com$', ''

# Function to execute SQL
function Invoke-AzurePostgresQuery {
    param([string]$Query)

    $tempFile = [System.IO.Path]::GetTempFileName() + ".sql"
    try {
        $Query | Out-File -FilePath $tempFile -Encoding UTF8 -NoNewline

        $result = az postgres flexible-server execute `
            --name $serverName `
            --admin-user $envVars['AUTHWAY_DATABASE_USER'] `
            --admin-password $envVars['AUTHWAY_DATABASE_PASSWORD'] `
            --database-name $envVars['AUTHWAY_DATABASE_NAME'] `
            --file-path $tempFile `
            2>&1

        return @{
            Success = ($LASTEXITCODE -eq 0)
            Output = $result
        }
    } finally {
        Remove-Item $tempFile -ErrorAction SilentlyContinue
    }
}

Write-Host "⚠️  이 스크립트는 schema_migrations 테이블을 업그레이드합니다" -ForegroundColor Yellow
Write-Host ""
Write-Host "변경 사항:" -ForegroundColor White
Write-Host "  • 기존 데이터 백업" -ForegroundColor Gray
Write-Host "  • 테이블 재생성 (새 구조)" -ForegroundColor Gray
Write-Host "  • 데이터 마이그레이션" -ForegroundColor Gray
Write-Host ""

$confirmation = Read-Host "계속하시겠습니까? (yes/no)"
if ($confirmation -ne "yes") {
    Write-Host "❌ 작업이 취소되었습니다." -ForegroundColor Yellow
    exit 0
}

Write-Host ""
Write-Host "🔄 테이블 업그레이드 중..." -ForegroundColor Yellow

# The upgrade SQL - self-contained and handles all cases
$upgradeSQL = @"
DO `$`$
DECLARE
    old_structure boolean := false;
    new_structure boolean := false;
    migration_count integer := 0;
BEGIN
    -- Check if table exists
    IF EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' AND tablename = 'schema_migrations') THEN
        -- Check for old structure (migration_file column)
        SELECT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'schema_migrations' AND column_name = 'migration_file'
        ) INTO old_structure;

        -- Check for new structure (version column)
        SELECT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'schema_migrations' AND column_name = 'version'
        ) INTO new_structure;

        IF old_structure AND NOT new_structure THEN
            RAISE NOTICE '=== Upgrading from old structure to new structure ===';

            -- Backup existing data
            CREATE TEMP TABLE schema_migrations_backup AS SELECT * FROM schema_migrations;
            SELECT COUNT(*) INTO migration_count FROM schema_migrations_backup;
            RAISE NOTICE 'Backed up % existing migration records', migration_count;

            -- Drop old table
            DROP TABLE schema_migrations CASCADE;
            RAISE NOTICE 'Dropped old table';

            -- Create new structure
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
            RAISE NOTICE 'Created new table structure';

            -- Migrate data from backup
            INSERT INTO schema_migrations (version, name, executed_at, checksum, success)
            SELECT
                SUBSTRING(migration_file FROM '^(\d+)_'),
                REGEXP_REPLACE(migration_file, '^(\d+)_(.+)\.sql`$', '\2'),
                applied_at,
                checksum,
                true
            FROM schema_migrations_backup
            WHERE migration_file ~ '^\d+_';

            RAISE NOTICE 'Migrated % records', (SELECT COUNT(*) FROM schema_migrations);

            -- Add system records
            INSERT INTO schema_migrations (version, name, execution_time_ms, success)
            VALUES ('000', 'init_migration_system', 0, true)
            ON CONFLICT (version) DO NOTHING;

            INSERT INTO schema_migrations (version, name, execution_time_ms, success)
            VALUES ('001', 'migrate_tracking_table', 0, true)
            ON CONFLICT (version) DO NOTHING;

            RAISE NOTICE '=== Upgrade complete ===';

        ELSIF new_structure THEN
            RAISE NOTICE 'Table already using new structure - no upgrade needed';
        ELSE
            RAISE EXCEPTION 'Unknown table structure';
        END IF;
    ELSE
        RAISE NOTICE 'Table does not exist - will be created by first migration';
    END IF;
END `$`$;

-- Show current state
SELECT version, name, executed_at, success
FROM schema_migrations
ORDER BY version;
"@

$result = Invoke-AzurePostgresQuery -Query $upgradeSQL

if ($result.Success) {
    Write-Host ""
    Write-Host "✅ 업그레이드 완료!" -ForegroundColor Green
    Write-Host ""
    Write-Host "📊 현재 마이그레이션 상태:" -ForegroundColor Cyan
    Write-Host $result.Output -ForegroundColor Gray
    Write-Host ""
} else {
    Write-Host ""
    Write-Host "❌ 업그레이드 실패" -ForegroundColor Red
    Write-Host $result.Output -ForegroundColor Gray
    Write-Host ""
    exit 1
}
