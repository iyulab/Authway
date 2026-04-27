# ============================================================
# Database Migration Runner for Azure PostgreSQL
# ============================================================
# Runs SQL migration files against Azure PostgreSQL
# ============================================================

param(
    [Parameter(Mandatory=$true)]
    [string]$MigrationFile,
    [switch]$DryRun
)

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🔄 Database Migration Runner" -ForegroundColor Cyan
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
$MigrationPath = Join-Path (Split-Path $ScriptDir -Parent) "migrations" $MigrationFile
if (-not (Test-Path $MigrationPath)) {
    Write-Host "❌ 마이그레이션 파일을 찾을 수 없습니다: $MigrationPath" -ForegroundColor Red
    exit 1
}

Write-Host "📄 마이그레이션 파일: $MigrationFile" -ForegroundColor White
Write-Host "🗄️  데이터베이스: $($envVars['AUTHWAY_DATABASE_NAME'])" -ForegroundColor White
Write-Host "🌐 호스트: $($envVars['AUTHWAY_DATABASE_HOST'])" -ForegroundColor White
Write-Host ""

if ($DryRun) {
    Write-Host "🔍 Dry Run 모드: SQL 내용만 표시합니다" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "─────────────────────────────────────────" -ForegroundColor Gray
    Get-Content $MigrationPath
    Write-Host "─────────────────────────────────────────" -ForegroundColor Gray
    Write-Host ""
    Write-Host "✅ Dry Run 완료 (실제 실행 없음)" -ForegroundColor Green
    exit 0
}

# Confirm before running
Write-Host "⚠️  이 작업은 데이터베이스를 변경합니다!" -ForegroundColor Yellow
Write-Host ""
$confirmation = Read-Host "계속하시겠습니까? (yes/no)"
if ($confirmation -ne "yes") {
    Write-Host "❌ 마이그레이션이 취소되었습니다." -ForegroundColor Yellow
    exit 0
}

Write-Host ""
Write-Host "🔄 마이그레이션 실행 중..." -ForegroundColor Yellow

try {
    # Set environment variable for psql password
    $env:PGPASSWORD = $envVars['AUTHWAY_DATABASE_PASSWORD']

    # Build connection string
    $dbHost = $envVars['AUTHWAY_DATABASE_HOST']
    $dbPort = $envVars['AUTHWAY_DATABASE_PORT']
    $dbName = $envVars['AUTHWAY_DATABASE_NAME']
    $dbUser = $envVars['AUTHWAY_DATABASE_USER']

    # Check if psql is available
    $psqlPath = Get-Command psql -ErrorAction SilentlyContinue

    if (-not $psqlPath) {
        Write-Host ""
        Write-Host "❌ psql 명령어를 찾을 수 없습니다." -ForegroundColor Red
        Write-Host ""
        Write-Host "PostgreSQL 클라이언트를 설치해주세요:" -ForegroundColor Yellow
        Write-Host "  - Windows: https://www.postgresql.org/download/windows/" -ForegroundColor Gray
        Write-Host "  - 또는 Azure CLI 사용: 아래 대체 방법 참조" -ForegroundColor Gray
        Write-Host ""
        exit 1
    }

    # Run migration using psql
    $result = psql `
        -h $dbHost `
        -p $dbPort `
        -U $dbUser `
        -d $dbName `
        -f $MigrationPath `
        --set=sslmode=require `
        2>&1

    if ($LASTEXITCODE -eq 0) {
        Write-Host ""
        Write-Host "✅ 마이그레이션이 성공적으로 완료되었습니다!" -ForegroundColor Green
        Write-Host ""

        # Show migration result
        if ($result) {
            Write-Host "📊 실행 결과:" -ForegroundColor Cyan
            Write-Host $result -ForegroundColor Gray
            Write-Host ""
        }

        exit 0
    } else {
        throw "psql 실행 실패 (Exit Code: $LASTEXITCODE)"
    }

} catch {
    Write-Host ""
    Write-Host "❌ 마이그레이션 실행 실패: $_" -ForegroundColor Red
    Write-Host ""
    Write-Host "오류 상세:" -ForegroundColor Yellow
    Write-Host $result -ForegroundColor Gray
    Write-Host ""
    exit 1
} finally {
    # Clear password from environment
    Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue
}
