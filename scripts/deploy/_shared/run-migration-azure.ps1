# ============================================================
# Database Migration Runner using Azure CLI
# ============================================================
# Migration runner with tracking system support
# Handles idempotent migrations with schema_migrations table
# ============================================================

param(
    [Parameter(Mandatory=$true)]
    [string]$MigrationFile,
    [switch]$DryRun,
    [switch]$Force  # Force re-run even if already applied
)

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🔄 Azure CLI Migration Runner" -ForegroundColor Cyan
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

# Check if migration file exists
$MigrationsDir = Join-Path (Split-Path $ScriptDir -Parent) "migrations"
$MigrationPath = Join-Path $MigrationsDir $MigrationFile
if (-not (Test-Path $MigrationPath)) {
    Write-Host "❌ 마이그레이션 파일을 찾을 수 없습니다: $MigrationPath" -ForegroundColor Red
    exit 1
}

# Extract version from filename (e.g., 003_add_logout_policy.sql -> 003)
if ($MigrationFile -match '^(\d+)_') {
    $version = $matches[1]
} else {
    Write-Host "❌ 마이그레이션 파일명 형식 오류: 버전 번호로 시작해야 합니다 (예: 003_name.sql)" -ForegroundColor Red
    exit 1
}

Write-Host "📄 마이그레이션 파일: $MigrationFile" -ForegroundColor White
Write-Host "🔢 마이그레이션 버전: $version" -ForegroundColor White
Write-Host "🗄️  데이터베이스: $($envVars['AUTHWAY_DATABASE_NAME'])" -ForegroundColor White
Write-Host "🌐 호스트: $($envVars['AUTHWAY_DATABASE_HOST'])" -ForegroundColor White
Write-Host ""

# Read migration SQL
$migrationSQL = Get-Content $MigrationPath -Raw

if ($DryRun) {
    Write-Host "🔍 Dry Run 모드: SQL 내용만 표시합니다" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "─────────────────────────────────────────" -ForegroundColor Gray
    Write-Host $migrationSQL
    Write-Host "─────────────────────────────────────────" -ForegroundColor Gray
    Write-Host ""
    Write-Host "✅ Dry Run 완료 (실제 실행 없음)" -ForegroundColor Green
    exit 0
}

# Check if Azure CLI is installed
$azCliPath = Get-Command az -ErrorAction SilentlyContinue
if (-not $azCliPath) {
    Write-Host "❌ Azure CLI를 찾을 수 없습니다." -ForegroundColor Red
    Write-Host ""
    Write-Host "Azure CLI를 설치해주세요:" -ForegroundColor Yellow
    Write-Host "  https://docs.microsoft.com/cli/azure/install-azure-cli" -ForegroundColor Gray
    Write-Host ""
    exit 1
}

# Check if logged in to Azure
Write-Host "🔐 Azure 로그인 확인 중..." -ForegroundColor Yellow
$azAccount = az account show 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Azure에 로그인되어 있지 않습니다." -ForegroundColor Red
    Write-Host ""
    Write-Host "다음 명령으로 로그인해주세요:" -ForegroundColor Yellow
    Write-Host "  az login" -ForegroundColor Gray
    Write-Host ""
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

# Check migration status
Write-Host "🔍 마이그레이션 상태 확인 중..." -ForegroundColor Yellow

$checkQuery = @"
SELECT EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public'
    AND table_name = 'schema_migrations'
) AS tracking_exists;
"@

$trackingResult = Invoke-AzurePostgresQuery -Query $checkQuery

if ($trackingResult.Success) {
    Write-Host "✅ 데이터베이스 연결 성공" -ForegroundColor Green

    # Check if this specific migration was already applied
    if ($trackingResult.Output -match "t|true") {
        $statusQuery = @"
SELECT version, name, executed_at, success
FROM schema_migrations
WHERE version = '$version'
ORDER BY executed_at DESC
LIMIT 1;
"@

        $statusResult = Invoke-AzurePostgresQuery -Query $statusQuery

        if ($statusResult.Output -match $version) {
            Write-Host ""
            Write-Host "ℹ️  마이그레이션 $version 는 이미 실행되었습니다" -ForegroundColor Cyan
            Write-Host ""
            Write-Host $statusResult.Output -ForegroundColor Gray
            Write-Host ""

            if (-not $Force) {
                Write-Host "💡 재실행하려면 -Force 플래그를 사용하세요" -ForegroundColor Yellow
                Write-Host ""
                exit 0
            } else {
                Write-Host "⚠️  -Force 플래그로 인해 마이그레이션을 재실행합니다" -ForegroundColor Yellow
                Write-Host ""
            }
        }
    } else {
        Write-Host "ℹ️  마이그레이션 추적 테이블이 없습니다 (첫 마이그레이션)" -ForegroundColor Cyan
        Write-Host ""
    }
} else {
    Write-Host "❌ 데이터베이스 상태 확인 실패" -ForegroundColor Red
    Write-Host $trackingResult.Output -ForegroundColor Gray
    Write-Host ""
    exit 1
}

# Confirm before running
Write-Host "⚠️  이 작업은 데이터베이스를 변경합니다!" -ForegroundColor Yellow
Write-Host ""
Write-Host "대상 정보:" -ForegroundColor White
Write-Host "  - Resource Group: $($envVars['RESOURCE_GROUP'])" -ForegroundColor Gray
Write-Host "  - Server: $serverName" -ForegroundColor Gray
Write-Host "  - Database: $($envVars['AUTHWAY_DATABASE_NAME'])" -ForegroundColor Gray
Write-Host "  - Migration: $MigrationFile (version $version)" -ForegroundColor Gray
Write-Host ""
$confirmation = Read-Host "계속하시겠습니까? (yes/no)"
if ($confirmation -ne "yes") {
    Write-Host "❌ 마이그레이션이 취소되었습니다." -ForegroundColor Yellow
    exit 0
}

Write-Host ""
Write-Host "🔄 마이그레이션 실행 중..." -ForegroundColor Yellow

try {
    $startTime = Get-Date

    $result = Invoke-AzurePostgresQuery -Query $migrationSQL

    $endTime = Get-Date
    $duration = ($endTime - $startTime).TotalMilliseconds

    if ($result.Success) {
        Write-Host ""
        Write-Host "✅ 마이그레이션이 성공적으로 완료되었습니다!" -ForegroundColor Green
        Write-Host "⏱️  실행 시간: $([math]::Round($duration))ms" -ForegroundColor Gray
        Write-Host ""

        # Show migration result
        if ($result.Output) {
            Write-Host "📊 실행 결과:" -ForegroundColor Cyan
            Write-Host $result.Output -ForegroundColor Gray
            Write-Host ""
        }

        # Verify migration from tracking table
        Write-Host "🔍 마이그레이션 검증 중..." -ForegroundColor Yellow
        $verifyQuery = @"
SELECT version, name, executed_at, success
FROM schema_migrations
WHERE version = '$version'
ORDER BY executed_at DESC
LIMIT 1;
"@

        $verifyResult = Invoke-AzurePostgresQuery -Query $verifyQuery

        if ($verifyResult.Success -and $verifyResult.Output -match "t|true") {
            Write-Host "✅ 검증 성공: 마이그레이션이 추적 테이블에 기록되었습니다" -ForegroundColor Green
            Write-Host ""
            Write-Host $verifyResult.Output -ForegroundColor Gray
            Write-Host ""
        } else {
            Write-Host "⚠️  검증 경고: 추적 테이블 업데이트를 확인할 수 없습니다" -ForegroundColor Yellow
            Write-Host ""
        }

        exit 0
    } else {
        throw "Azure CLI 실행 실패 (Exit Code: $($result.ExitCode))"
    }

} catch {
    Write-Host ""
    Write-Host "❌ 마이그레이션 실행 실패: $_" -ForegroundColor Red
    Write-Host ""
    Write-Host "오류 상세:" -ForegroundColor Yellow
    Write-Host $result.Output -ForegroundColor Gray
    Write-Host ""

    Write-Host "💡 문제 해결:" -ForegroundColor Cyan
    Write-Host "  1. Azure 로그인 확인: az login" -ForegroundColor Gray
    Write-Host "  2. 올바른 구독 선택: az account set --subscription <subscription-id>" -ForegroundColor Gray
    Write-Host "  3. PostgreSQL 방화벽 규칙 확인" -ForegroundColor Gray
    Write-Host "  4. 데이터베이스 접속 권한 확인" -ForegroundColor Gray
    Write-Host "  5. SQL 구문 오류 확인" -ForegroundColor Gray
    Write-Host ""

    exit 1
}
