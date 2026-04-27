# ============================================================
# Migrate schema_migrations table to new structure
# ============================================================
# One-time migration to update tracking table schema
# Run this before running other migrations
# ============================================================

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🔄 Schema Migrations Table Updater" -ForegroundColor Cyan
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

# Check if Azure CLI is installed
$azCliPath = Get-Command az -ErrorAction SilentlyContinue
if (-not $azCliPath) {
    Write-Host "❌ Azure CLI를 찾을 수 없습니다." -ForegroundColor Red
    exit 1
}

# Check if logged in to Azure
Write-Host "🔐 Azure 로그인 확인 중..." -ForegroundColor Yellow
$azAccount = az account show 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Azure에 로그인되어 있지 않습니다." -ForegroundColor Red
    Write-Host "다음 명령으로 로그인해주세요: az login" -ForegroundColor Yellow
    exit 1
}
Write-Host "✅ Azure 로그인 확인됨" -ForegroundColor Green
Write-Host ""

# Extract server name from host
$serverName = $envVars['AUTHWAY_DATABASE_HOST'] -replace '\.postgres\.database\.azure\.com$', ''

# Function to execute SQL query
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
            ExitCode = $LASTEXITCODE
        }
    } finally {
        Remove-Item $tempFile -ErrorAction SilentlyContinue
    }
}

# Check current table structure
Write-Host "🔍 현재 테이블 구조 확인 중..." -ForegroundColor Yellow

$checkStructureQuery = @"
SELECT column_name
FROM information_schema.columns
WHERE table_name = 'schema_migrations'
ORDER BY ordinal_position;
"@

$structureResult = Invoke-AzurePostgresQuery -Query $checkStructureQuery

if ($structureResult.Success) {
    if ($structureResult.Output -match "migration_file") {
        Write-Host "✅ 구 버전 테이블 발견 (migration_file 컬럼 사용)" -ForegroundColor Yellow
        Write-Host "   → 신규 구조로 마이그레이션이 필요합니다" -ForegroundColor Yellow
    } elseif ($structureResult.Output -match "version") {
        Write-Host "✅ 이미 신규 구조를 사용하고 있습니다" -ForegroundColor Green
        Write-Host "   마이그레이션이 필요하지 않습니다" -ForegroundColor Gray
        exit 0
    } else {
        Write-Host "⚠️  schema_migrations 테이블이 없습니다" -ForegroundColor Yellow
        Write-Host "   첫 마이그레이션 실행 시 자동으로 생성됩니다" -ForegroundColor Gray
        exit 0
    }
} else {
    Write-Host "❌ 테이블 구조 확인 실패" -ForegroundColor Red
    Write-Host $structureResult.Output -ForegroundColor Gray
    exit 1
}

Write-Host ""
Write-Host "⚠️  schema_migrations 테이블을 업데이트합니다" -ForegroundColor Yellow
Write-Host ""
Write-Host "변경 내용:" -ForegroundColor White
Write-Host "  - migration_file 컬럼 → version + name 컬럼으로 분리" -ForegroundColor Gray
Write-Host "  - success, error_message, execution_time_ms 컬럼 추가" -ForegroundColor Gray
Write-Host "  - 기존 마이그레이션 기록은 보존됩니다" -ForegroundColor Gray
Write-Host ""
$confirmation = Read-Host "계속하시겠습니까? (yes/no)"
if ($confirmation -ne "yes") {
    Write-Host "❌ 작업이 취소되었습니다." -ForegroundColor Yellow
    exit 0
}

Write-Host ""
Write-Host "🔄 테이블 구조 업데이트 중..." -ForegroundColor Yellow

$migrationSQL = @"
DO `$`$
DECLARE
    migration_count integer;
BEGIN
    -- Create temporary backup
    CREATE TEMP TABLE schema_migrations_backup AS
    SELECT * FROM schema_migrations;

    SELECT COUNT(*) INTO migration_count FROM schema_migrations_backup;
    RAISE NOTICE 'Backed up % existing migrations', migration_count;

    -- Drop old table
    DROP TABLE schema_migrations;
    RAISE NOTICE 'Dropped old table structure';

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

    -- Migrate old data
    INSERT INTO schema_migrations (version, name, executed_at, checksum, success)
    SELECT
        SUBSTRING(migration_file FROM '^(\d+)_'),
        REGEXP_REPLACE(migration_file, '^(\d+)_(.+)\.sql`$', '\2'),
        applied_at,
        checksum,
        true
    FROM schema_migrations_backup
    WHERE migration_file ~ '^\d+_';

    RAISE NOTICE 'Migrated % records to new structure', (SELECT COUNT(*) FROM schema_migrations);

    -- Add system records
    INSERT INTO schema_migrations (version, name, execution_time_ms, success)
    VALUES ('000', 'init_migration_system', 0, true)
    ON CONFLICT (version) DO NOTHING;

    INSERT INTO schema_migrations (version, name, execution_time_ms, success)
    VALUES ('001', 'migrate_tracking_table', 0, true)
    ON CONFLICT (version) DO NOTHING;

    RAISE NOTICE 'Added system records';
END `$`$;

-- Show results
SELECT version, name, executed_at, success
FROM schema_migrations
ORDER BY version;
"@

$result = Invoke-AzurePostgresQuery -Query $migrationSQL

if ($result.Success) {
    Write-Host ""
    Write-Host "✅ 테이블 구조 업데이트 성공!" -ForegroundColor Green
    Write-Host ""
    Write-Host "📊 마이그레이션 결과:" -ForegroundColor Cyan
    Write-Host $result.Output -ForegroundColor Gray
    Write-Host ""
    Write-Host "✅ 이제 일반 마이그레이션을 실행할 수 있습니다" -ForegroundColor Green
    Write-Host ""
} else {
    Write-Host ""
    Write-Host "❌ 테이블 구조 업데이트 실패" -ForegroundColor Red
    Write-Host ""
    Write-Host $result.Output -ForegroundColor Gray
    Write-Host ""
    exit 1
}
