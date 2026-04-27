# ============================================================
# Force Upgrade Tracking Table
# ============================================================
# Direct SQL approach - no detection logic
# ============================================================

param(
    [switch]$Confirm
)

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🔧 Force Tracking Table Upgrade" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

# Load environment
$ScriptDir = $PSScriptRoot
$EnvFile = Join-Path $ScriptDir ".env"

if (-not (Test-Path $EnvFile)) {
    Write-Host "❌ .env 파일을 찾을 수 없습니다" -ForegroundColor Red
    exit 1
}

$envVars = @{}
Get-Content $EnvFile | ForEach-Object {
    if ($_ -match '^([^#][^=]+)=(.+)$') {
        $envVars[$matches[1].Trim()] = $matches[2].Trim()
    }
}

$serverName = $envVars['AUTHWAY_DATABASE_HOST'] -replace '\.postgres\.database\.azure\.com$', ''

Write-Host "🗄️  데이터베이스: $($envVars['AUTHWAY_DATABASE_NAME'])" -ForegroundColor White
Write-Host "🌐 서버: $serverName" -ForegroundColor White
Write-Host ""

# Check Azure CLI
if (-not (Get-Command az -ErrorAction SilentlyContinue)) {
    Write-Host "❌ Azure CLI 없음" -ForegroundColor Red
    exit 1
}

# Check login
az account show 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Azure 로그인 필요" -ForegroundColor Red
    exit 1
}

if (-not $Confirm) {
    Write-Host "⚠️  이 작업은 schema_migrations 테이블을 재생성합니다" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "작업 내용:" -ForegroundColor White
    Write-Host "  1. 기존 데이터 백업 (임시 테이블)" -ForegroundColor Gray
    Write-Host "  2. 기존 테이블 삭제" -ForegroundColor Gray
    Write-Host "  3. 새 구조로 테이블 생성" -ForegroundColor Gray
    Write-Host "  4. 데이터 복원 (버전 추출)" -ForegroundColor Gray
    Write-Host ""
    $response = Read-Host "계속하시겠습니까? (yes/no)"
    if ($response -ne "yes") {
        Write-Host "취소됨" -ForegroundColor Yellow
        exit 0
    }
}

Write-Host ""
Write-Host "🔄 실행 중..." -ForegroundColor Yellow

# Create SQL file
$sql = @"
-- Upgrade schema_migrations table
DO `$`$
DECLARE
    rec record;
    migrated_count integer := 0;
BEGIN
    -- Step 1: Backup existing data if table exists
    IF EXISTS (SELECT FROM pg_tables WHERE tablename = 'schema_migrations') THEN
        RAISE NOTICE 'Backing up existing schema_migrations table...';
        CREATE TEMP TABLE sm_backup AS SELECT * FROM schema_migrations;
        SELECT COUNT(*) INTO migrated_count FROM sm_backup;
        RAISE NOTICE 'Backed up % records', migrated_count;

        -- Drop existing table
        DROP TABLE schema_migrations CASCADE;
        RAISE NOTICE 'Dropped old table';
    ELSE
        RAISE NOTICE 'No existing table found';
    END IF;

    -- Step 2: Create new structure
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

    -- Step 3: Migrate old data if backup exists
    IF EXISTS (SELECT FROM pg_tables WHERE tablename = 'sm_backup') THEN
        -- Try to migrate from old structure (migration_file column)
        IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sm_backup' AND column_name = 'migration_file') THEN
            RAISE NOTICE 'Migrating from old structure (migration_file)...';

            INSERT INTO schema_migrations (version, name, executed_at, checksum, success)
            SELECT
                SUBSTRING(migration_file FROM '^(\d+)_'),
                REGEXP_REPLACE(migration_file, '^(\d+)_(.+)\.sql`$', '\2'),
                applied_at,
                checksum,
                true
            FROM sm_backup
            WHERE migration_file ~ '^\d+_';

            RAISE NOTICE 'Migrated % records', (SELECT COUNT(*) FROM schema_migrations);
        ELSE
            RAISE NOTICE 'Old table had unknown structure';
        END IF;
    END IF;

    -- Step 4: Add system records
    INSERT INTO schema_migrations (version, name, execution_time_ms, success)
    VALUES ('000', 'init_migration_system', 0, true)
    ON CONFLICT (version) DO NOTHING;

    INSERT INTO schema_migrations (version, name, execution_time_ms, success)
    VALUES ('001', 'migrate_tracking_table', 0, true)
    ON CONFLICT (version) DO NOTHING;

    RAISE NOTICE 'Upgrade complete!';
END `$`$;

-- Show results
SELECT version, name, TO_CHAR(executed_at, 'YYYY-MM-DD HH24:MI:SS') as executed_at, success
FROM schema_migrations
ORDER BY version;
"@

# Write to temp file
$tempFile = [System.IO.Path]::GetTempFileName() + ".sql"
$sql | Out-File -FilePath $tempFile -Encoding UTF8 -NoNewline

try {
    # Execute
    $output = az postgres flexible-server execute `
        --name $serverName `
        --admin-user $envVars['AUTHWAY_DATABASE_USER'] `
        --admin-password $envVars['AUTHWAY_DATABASE_PASSWORD'] `
        --database-name $envVars['AUTHWAY_DATABASE_NAME'] `
        --file-path $tempFile `
        2>&1

    if ($LASTEXITCODE -eq 0) {
        Write-Host ""
        Write-Host "✅ 업그레이드 성공!" -ForegroundColor Green
        Write-Host ""
        Write-Host "📊 현재 상태:" -ForegroundColor Cyan
        Write-Host $output -ForegroundColor Gray
        Write-Host ""
        Write-Host "✅ 이제 deploy-all.ps1을 실행할 수 있습니다" -ForegroundColor Green
        Write-Host ""
    } else {
        Write-Host ""
        Write-Host "❌ 실행 실패" -ForegroundColor Red
        Write-Host $output -ForegroundColor Gray
        Write-Host ""
        exit 1
    }
} finally {
    Remove-Item $tempFile -ErrorAction SilentlyContinue
}
