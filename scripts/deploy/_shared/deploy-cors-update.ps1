# ============================================================
# Authway CORS Update Deployment Script
# ============================================================
# Automated deployment with migration for CORS support
# No user interaction required
# ============================================================

param(
    [switch]$SkipBuild,
    [switch]$SkipHealthCheck,
    [switch]$DryRun,
    [string[]]$Services = @("api", "auth-api")  # Only update backend services
)

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
Write-Host "  🚀 Authway CORS Update Deployment" -ForegroundColor Magenta
Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
Write-Host ""

$ScriptDir = $PSScriptRoot
$StartTime = Get-Date

# Load environment variables
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

$serverName = $envVars['AUTHWAY_DATABASE_HOST'] -replace '\.postgres\.database\.azure\.com$', ''

# ============================================================
# Step 1: Database Migration
# ============================================================

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  📊 Step 1/3: Database Migration" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

Write-Host "🎯 Migration: 002_add_allowed_origins.sql" -ForegroundColor White
Write-Host "   - clients 테이블에 allowed_origins TEXT[] 컬럼 추가" -ForegroundColor Gray
Write-Host "   - GIN 인덱스 생성으로 빠른 조회 지원" -ForegroundColor Gray
Write-Host "   - OAuth 2.0 표준 준수 CORS 검증 지원" -ForegroundColor Gray
Write-Host ""

if ($DryRun) {
    Write-Host "🔍 Dry Run 모드: Migration SQL 미리보기" -ForegroundColor Yellow
    Write-Host ""
    $MigrationPath = Join-Path (Split-Path $ScriptDir -Parent) "migrations" "002_add_allowed_origins.sql"
    Get-Content $MigrationPath | ForEach-Object {
        Write-Host "   $_" -ForegroundColor Gray
    }
    Write-Host ""
    Write-Host "✅ Dry Run 완료" -ForegroundColor Green
    exit 0
}

Write-Host "🔄 Migration 실행 중..." -ForegroundColor Yellow

try {
    # Check if Azure CLI is available
    $azCli = Get-Command az -ErrorAction SilentlyContinue
    if (-not $azCli) {
        throw "Azure CLI를 찾을 수 없습니다. Azure CLI를 설치해주세요."
    }

    # Check Azure login
    $null = az account show 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Azure에 로그인되어 있지 않습니다. 'az login'을 실행해주세요."
    }

    # Read migration SQL
    $MigrationPath = Join-Path (Split-Path $ScriptDir -Parent) "migrations" "002_add_allowed_origins.sql"
    if (-not (Test-Path $MigrationPath)) {
        throw "마이그레이션 파일을 찾을 수 없습니다: $MigrationPath"
    }

    $migrationSQL = Get-Content $MigrationPath -Raw

    # Create temporary SQL file
    $tempSQLFile = [System.IO.Path]::GetTempFileName() + ".sql"
    $migrationSQL | Out-File -FilePath $tempSQLFile -Encoding UTF8 -NoNewline

    # Execute migration
    Write-Host "   - PostgreSQL 서버: $serverName" -ForegroundColor Gray
    Write-Host "   - 데이터베이스: $($envVars['AUTHWAY_DATABASE_NAME'])" -ForegroundColor Gray
    Write-Host ""

    $result = az postgres flexible-server execute `
        --name $serverName `
        --admin-user $envVars['AUTHWAY_DATABASE_USER'] `
        --admin-password $envVars['AUTHWAY_DATABASE_PASSWORD'] `
        --database-name $envVars['AUTHWAY_DATABASE_NAME'] `
        --file-path $tempSQLFile `
        2>&1

    if ($LASTEXITCODE -ne 0) {
        # Check if error is because column already exists
        if ($result -match "already exists" -or $result -match "duplicate") {
            Write-Host "ℹ️  allowed_origins 컬럼이 이미 존재합니다 (건너뜀)" -ForegroundColor Yellow
        } else {
            throw "Migration 실행 실패: $result"
        }
    }

    # Verify migration
    Write-Host "🔍 검증 중..." -ForegroundColor Yellow
    $verifySQL = "SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'clients' AND column_name = 'allowed_origins';"
    $verifyFile = [System.IO.Path]::GetTempFileName() + ".sql"
    $verifySQL | Out-File -FilePath $verifyFile -Encoding UTF8 -NoNewline

    $verifyResult = az postgres flexible-server execute `
        --name $serverName `
        --admin-user $envVars['AUTHWAY_DATABASE_USER'] `
        --admin-password $envVars['AUTHWAY_DATABASE_PASSWORD'] `
        --database-name $envVars['AUTHWAY_DATABASE_NAME'] `
        --file-path $verifyFile `
        2>&1

    Remove-Item $verifyFile -ErrorAction SilentlyContinue

    if ($verifyResult -match "allowed_origins" -and $verifyResult -match "ARRAY") {
        Write-Host "✅ Migration 완료: allowed_origins 컬럼 확인됨" -ForegroundColor Green
    } else {
        Write-Host "⚠️  검증 경고: allowed_origins 컬럼 확인 불가" -ForegroundColor Yellow
        Write-Host "   수동 확인 필요: SELECT * FROM information_schema.columns WHERE table_name = 'clients';" -ForegroundColor Gray
    }

    Remove-Item $tempSQLFile -ErrorAction SilentlyContinue

} catch {
    Write-Host ""
    Write-Host "❌ Migration 실패: $_" -ForegroundColor Red
    Write-Host ""
    Write-Host "💡 수동 Migration 방법:" -ForegroundColor Yellow
    Write-Host "1. Azure Portal Cloud Shell: https://portal.azure.com" -ForegroundColor Gray
    Write-Host "2. 실행: az postgres flexible-server execute --name authway ..." -ForegroundColor Gray
    Write-Host "3. 또는 psql 직접 접속하여 SQL 실행" -ForegroundColor Gray
    Write-Host ""
    exit 1
}

# ============================================================
# Step 2: Deploy Backend Services
# ============================================================

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🚀 Step 2/3: Backend Services 배포" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

Write-Host "📦 배포 대상:" -ForegroundColor White
foreach ($service in $Services) {
    Write-Host "   - $service" -ForegroundColor Gray
}
Write-Host ""

$deployScript = Join-Path $ScriptDir "deploy-all.ps1"

if (-not (Test-Path $deployScript)) {
    Write-Host "❌ deploy-all.ps1을 찾을 수 없습니다" -ForegroundColor Red
    exit 1
}

try {
    $deployParams = @{
        Services = $Services
    }
    if ($SkipBuild) { $deployParams["SkipBuild"] = $true }
    if ($SkipHealthCheck) { $deployParams["SkipHealthCheck"] = $true }

    & $deployScript @deployParams

    if ($LASTEXITCODE -ne 0) {
        throw "서비스 배포 실패"
    }

} catch {
    Write-Host ""
    Write-Host "❌ 배포 실패: $_" -ForegroundColor Red
    exit 1
}

# ============================================================
# Step 3: Post-Deployment Instructions
# ============================================================

$totalDuration = [math]::Round(((Get-Date) - $StartTime).TotalMinutes, 1)

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
Write-Host "  ✅ CORS Update 배포 완료!" -ForegroundColor Magenta
Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
Write-Host ""
Write-Host "  총 소요 시간: $totalDuration 분" -ForegroundColor White
Write-Host ""
Write-Host "  📋 완료된 작업:" -ForegroundColor Cyan
Write-Host "    ✅ Database migration (allowed_origins 컬럼 추가)" -ForegroundColor Green
Write-Host "    ✅ Backend API 배포 (CORS 지원 활성화)" -ForegroundColor Green
Write-Host ""
Write-Host "  ⏳ 다음 단계:" -ForegroundColor Yellow
Write-Host ""
Write-Host "  1. 기존 클라이언트 업데이트" -ForegroundColor White
Write-Host "     Azure Portal → PostgreSQL → Query editor에서 실행:" -ForegroundColor Gray
Write-Host ""
Write-Host "     UPDATE clients" -ForegroundColor Cyan
Write-Host "     SET allowed_origins = ARRAY[" -ForegroundColor Cyan
Write-Host "       'https://manuals.alldot.ai'," -ForegroundColor Cyan
Write-Host "       'https://nice-moss-08ac84200.3.azurestaticapps.net'" -ForegroundColor Cyan
Write-Host "     ]" -ForegroundColor Cyan
Write-Host "     WHERE client_id = 'authway_2qfEM6ccGYfmxh8bC6hjng';" -ForegroundColor Cyan
Write-Host ""
Write-Host "  2. CORS 테스트" -ForegroundColor White
Write-Host "     테스트 스크립트 실행:" -ForegroundColor Gray
Write-Host ""
Write-Host "     curl -X OPTIONS https://oauth.authway.in/oauth2/token \\" -ForegroundColor Cyan
Write-Host "       -H ""Origin: https://manuals.alldot.ai"" \\" -ForegroundColor Cyan
Write-Host "       -H ""Access-Control-Request-Method: POST"" \\" -ForegroundColor Cyan
Write-Host "       -v" -ForegroundColor Cyan
Write-Host ""
Write-Host "  3. 새 클라이언트 생성 예시" -ForegroundColor White
Write-Host "     API 요청:" -ForegroundColor Gray
Write-Host ""
Write-Host "     POST https://api.authway.in/api/v1/clients" -ForegroundColor Cyan
Write-Host "     {" -ForegroundColor Cyan
Write-Host "       ""tenant_id"": ""uuid""," -ForegroundColor Cyan
Write-Host "       ""name"": ""My App""," -ForegroundColor Cyan
Write-Host "       ""allowed_origins"": [""https://myapp.com""]," -ForegroundColor Cyan
Write-Host "       ""redirect_uris"": [""https://myapp.com/callback""]" -ForegroundColor Cyan
Write-Host "     }" -ForegroundColor Cyan
Write-Host ""
Write-Host "  📚 참고 문서:" -ForegroundColor Cyan
Write-Host "    - claudedocs/CORS_ISSUE_RESPONSE.md" -ForegroundColor Gray
Write-Host "    - docs/CORS_SOLUTION_IMPLEMENTATION.md" -ForegroundColor Gray
Write-Host "    - docs/deployment/AZURE_CORS_DEPLOYMENT.md" -ForegroundColor Gray
Write-Host ""
Write-Host "  🌐 서비스 URL:" -ForegroundColor Cyan
Write-Host "    - Central API: $($envVars['API_URL'])" -ForegroundColor Gray
Write-Host "    - Auth Backend: $($envVars['AUTH_API_URL'])" -ForegroundColor Gray
Write-Host "    - OAuth: $($envVars['HYDRA_ISSUER'])" -ForegroundColor Gray
Write-Host ""

exit 0
