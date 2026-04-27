# ============================================================
# Initialize Migration Tracking System
# ============================================================
# Creates the schema_migrations table if it doesn't exist
# Run this once before any migrations
# ============================================================

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🔧 Initialize Migration Tracking System" -ForegroundColor Cyan
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
Write-Host "🔐 Azure 로그인 확인 중..." -ForegroundColor Yellow
$azAccount = az account show 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Azure에 로그인되어 있지 않습니다." -ForegroundColor Red
    exit 1
}
Write-Host "✅ Azure 로그인 확인됨" -ForegroundColor Green
Write-Host ""

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

Write-Host "🔍 schema_migrations 테이블 확인 중..." -ForegroundColor Yellow

$checkTableQuery = @"
SELECT EXISTS (
    SELECT FROM pg_tables
    WHERE schemaname = 'public'
    AND tablename = 'schema_migrations'
) AS table_exists;
"@

$checkResult = Invoke-AzurePostgresQuery -Query $checkTableQuery

if (-not $checkResult.Success) {
    Write-Host "❌ 데이터베이스 연결 실패" -ForegroundColor Red
    Write-Host $checkResult.Output -ForegroundColor Gray
    exit 1
}

if ($checkResult.Output -match "t|true") {
    Write-Host "✅ schema_migrations 테이블이 이미 존재합니다" -ForegroundColor Green
    Write-Host ""

    # Check structure
    $checkColumnsQuery = @"
SELECT column_name
FROM information_schema.columns
WHERE table_name = 'schema_migrations'
ORDER BY ordinal_position;
"@

    $columnsResult = Invoke-AzurePostgresQuery -Query $checkColumnsQuery

    if ($columnsResult.Output -match "version") {
        Write-Host "✅ 올바른 구조로 되어 있습니다 (version 컬럼 확인됨)" -ForegroundColor Green
    } else {
        Write-Host "⚠️  구 버전 구조입니다 (migration_file 컬럼)" -ForegroundColor Yellow
        Write-Host "   migrate-tracking-table.ps1을 실행하여 업그레이드하세요" -ForegroundColor Yellow
    }

    Write-Host ""
    exit 0
}

Write-Host "⚠️  schema_migrations 테이블이 존재하지 않습니다" -ForegroundColor Yellow
Write-Host "   → 새로 생성합니다" -ForegroundColor Cyan
Write-Host ""

$createTableSQL = @"
-- Create schema_migrations tracking table
CREATE TABLE IF NOT EXISTS schema_migrations (
    id SERIAL PRIMARY KEY,
    version VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    execution_time_ms INTEGER,
    checksum VARCHAR(64),
    success BOOLEAN NOT NULL DEFAULT TRUE,
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_schema_migrations_version ON schema_migrations(version);
CREATE INDEX IF NOT EXISTS idx_schema_migrations_executed_at ON schema_migrations(executed_at DESC);

-- Record initialization
INSERT INTO schema_migrations (version, name, execution_time_ms, success)
VALUES ('000', 'init_migration_system', 0, true)
ON CONFLICT (version) DO NOTHING;

-- Show result
SELECT version, name, executed_at, success
FROM schema_migrations;
"@

Write-Host "🔄 테이블 생성 중..." -ForegroundColor Yellow
$createResult = Invoke-AzurePostgresQuery -Query $createTableSQL

if ($createResult.Success) {
    Write-Host ""
    Write-Host "✅ schema_migrations 테이블 생성 완료!" -ForegroundColor Green
    Write-Host ""
    Write-Host "📊 초기 상태:" -ForegroundColor Cyan
    Write-Host $createResult.Output -ForegroundColor Gray
    Write-Host ""
    Write-Host "✅ 이제 마이그레이션을 실행할 수 있습니다" -ForegroundColor Green
    Write-Host ""
} else {
    Write-Host ""
    Write-Host "❌ 테이블 생성 실패" -ForegroundColor Red
    Write-Host $createResult.Output -ForegroundColor Gray
    Write-Host ""
    exit 1
}
