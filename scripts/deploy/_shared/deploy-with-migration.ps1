# ============================================================
# Authway 전체 배포 + Migration 스크립트
# ============================================================
# Database migration을 먼저 실행한 후 전체 배포 수행
# ============================================================

param(
    [switch]$SkipMigration,
    [switch]$SkipBuild,
    [switch]$SkipHealthCheck,
    [string[]]$Services = @("hydra", "api", "auth-api", "admin", "auth-ui")
)

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
Write-Host "  🚀 Authway 배포 + Migration 시작" -ForegroundColor Magenta
Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
Write-Host ""

$ScriptDir = $PSScriptRoot
$StartTime = Get-Date

# ============================================================
# Step 1: Database Migration (CORS Support)
# ============================================================

if (-not $SkipMigration) {
    Write-Host ""
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host "  📊 Step 1: Database Migration" -ForegroundColor Cyan
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""

    Write-Host "🔄 Migration: allowed_origins 컬럼 추가" -ForegroundColor White
    Write-Host "   목적: OAuth 2.0 표준 준수 CORS 지원" -ForegroundColor Gray
    Write-Host ""

    # Try Azure CLI method first (more reliable for Azure)
    $migrationScript = Join-Path $ScriptDir "run-migration-azure.ps1"

    if (Test-Path $migrationScript) {
        Write-Host "📝 Azure CLI를 사용한 마이그레이션 실행..." -ForegroundColor Yellow

        try {
            & $migrationScript -MigrationFile "002_add_allowed_origins.sql"

            if ($LASTEXITCODE -ne 0) {
                throw "Migration 실행 실패"
            }

            Write-Host ""
            Write-Host "✅ Migration 완료!" -ForegroundColor Green
            Write-Host ""

        } catch {
            Write-Host ""
            Write-Host "❌ Migration 실패: $_" -ForegroundColor Red
            Write-Host ""
            Write-Host "💡 수동 Migration 방법:" -ForegroundColor Yellow
            Write-Host ""
            Write-Host "1. Azure Portal에서 Cloud Shell 열기:" -ForegroundColor White
            Write-Host "   https://portal.azure.com" -ForegroundColor Gray
            Write-Host ""
            Write-Host "2. 다음 명령 실행:" -ForegroundColor White
            Write-Host "   az postgres flexible-server execute \\" -ForegroundColor Gray
            Write-Host "     --name authway \\" -ForegroundColor Gray
            Write-Host "     --admin-user authwayadmin \\" -ForegroundColor Gray
            Write-Host "     --database-name authway \\" -ForegroundColor Gray
            Write-Host "     --file-path scripts/migrations/002_add_allowed_origins.sql" -ForegroundColor Gray
            Write-Host ""

            $continueAnyway = Read-Host "Migration 없이 배포를 계속하시겠습니까? (yes/no)"
            if ($continueAnyway -ne "yes") {
                Write-Host "❌ 배포가 취소되었습니다." -ForegroundColor Yellow
                exit 1
            }
        }
    } else {
        Write-Host "⚠️  Migration 스크립트를 찾을 수 없습니다: $migrationScript" -ForegroundColor Yellow
        Write-Host "   Migration을 건너뜁니다..." -ForegroundColor Yellow
        Write-Host ""
    }

} else {
    Write-Host "⏭️  Migration 건너뜀 (--SkipMigration 옵션 사용)" -ForegroundColor Yellow
    Write-Host ""
}

# ============================================================
# Step 2: Update Client Data (Add allowed_origins)
# ============================================================

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  📝 Step 2: Client 데이터 업데이트" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

Write-Host "💡 배포 후 다음 작업을 수행해주세요:" -ForegroundColor Yellow
Write-Host ""
Write-Host "1. 기존 클라이언트에 allowed_origins 추가:" -ForegroundColor White
Write-Host ""
Write-Host "   UPDATE clients" -ForegroundColor Gray
Write-Host "   SET allowed_origins = ARRAY[" -ForegroundColor Gray
Write-Host "     'https://manuals.alldot.ai'," -ForegroundColor Gray
Write-Host "     'https://your-app-domain.com'" -ForegroundColor Gray
Write-Host "   ]" -ForegroundColor Gray
Write-Host "   WHERE client_id = 'your_client_id';" -ForegroundColor Gray
Write-Host ""
Write-Host "2. 새 클라이언트 생성 시 allowed_origins 포함:" -ForegroundColor White
Write-Host ""
Write-Host "   POST /api/v1/clients" -ForegroundColor Gray
Write-Host "   {" -ForegroundColor Gray
Write-Host "     ""allowed_origins"": [""https://app.example.com""]" -ForegroundColor Gray
Write-Host "   }" -ForegroundColor Gray
Write-Host ""

$continueDeployment = Read-Host "배포를 계속하시겠습니까? (yes/no)"
if ($continueDeployment -ne "yes") {
    Write-Host "❌ 배포가 취소되었습니다." -ForegroundColor Yellow
    exit 0
}

# ============================================================
# Step 3: Deploy All Services
# ============================================================

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🚀 Step 3: 서비스 배포" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

$deployScript = Join-Path $ScriptDir "deploy-all.ps1"

if (-not (Test-Path $deployScript)) {
    Write-Host "❌ 배포 스크립트를 찾을 수 없습니다: $deployScript" -ForegroundColor Red
    exit 1
}

# Build parameters for deploy-all.ps1
$deployParams = @()
if ($SkipBuild) { $deployParams += "-SkipBuild" }
if ($SkipHealthCheck) { $deployParams += "-SkipHealthCheck" }
if ($Services) {
    $serviceList = $Services -join ","
    $deployParams += "-Services"
    $deployParams += $serviceList
}

Write-Host "📦 전체 배포 스크립트 실행 중..." -ForegroundColor Yellow
Write-Host ""

try {
    if ($deployParams.Count -gt 0) {
        & $deployScript @deployParams
    } else {
        & $deployScript
    }

    $deployExitCode = $LASTEXITCODE

    # ============================================================
    # Final Summary
    # ============================================================

    $totalDuration = [math]::Round(((Get-Date) - $StartTime).TotalMinutes, 1)

    Write-Host ""
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
    Write-Host "  🎉 배포 프로세스 완료" -ForegroundColor Magenta
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
    Write-Host ""
    Write-Host "  총 소요 시간: $totalDuration 분" -ForegroundColor White
    Write-Host ""

    if ($deployExitCode -eq 0) {
        Write-Host "  ✅ 모든 단계가 성공적으로 완료되었습니다!" -ForegroundColor Green
        Write-Host ""
        Write-Host "  📋 다음 단계:" -ForegroundColor Cyan
        Write-Host "    1. ✅ Migration 완료" -ForegroundColor Gray
        Write-Host "    2. ✅ 서비스 배포 완료" -ForegroundColor Gray
        Write-Host "    3. ⏳ 기존 클라이언트 allowed_origins 업데이트 필요" -ForegroundColor Yellow
        Write-Host "    4. ⏳ CORS 테스트 수행" -ForegroundColor Yellow
        Write-Host ""
        Write-Host "  🔗 참고 문서:" -ForegroundColor Cyan
        Write-Host "    - docs/CORS_SOLUTION_IMPLEMENTATION.md" -ForegroundColor Gray
        Write-Host "    - docs/deployment/AZURE_CORS_DEPLOYMENT.md" -ForegroundColor Gray
        Write-Host ""
        exit 0
    } else {
        Write-Host "  ⚠️  배포 중 일부 오류가 발생했습니다." -ForegroundColor Yellow
        Write-Host "     개별 서비스를 확인해주세요." -ForegroundColor Yellow
        Write-Host ""
        exit 1
    }

} catch {
    Write-Host ""
    Write-Host "❌ 배포 프로세스 오류: $_" -ForegroundColor Red
    Write-Host ""
    Write-Host "스택 트레이스:" -ForegroundColor Yellow
    Write-Host $_.ScriptStackTrace -ForegroundColor Gray
    Write-Host ""
    exit 1
}
