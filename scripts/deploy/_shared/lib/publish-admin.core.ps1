# ============================================================
# Authway Admin Dashboard 배포 코어 로직 (타겟 무관)
# ============================================================
# Azure Static Web App에 Admin Dashboard 배포.
# ============================================================

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("prod", "staging")]
    [string]$Target
)

Write-Host "🚀 Authway Admin Dashboard 배포 시작... (target=$Target)" -ForegroundColor Cyan
Write-Host ""

$LibDir = $PSScriptRoot
$SharedDir = Split-Path -Parent $LibDir
$DeployDir = Split-Path -Parent $SharedDir
$ScriptsDir = Split-Path -Parent $DeployDir
$ProjectRoot = Split-Path -Parent $ScriptsDir
$AdminUIPath = Join-Path $ProjectRoot "apps\central\admin"

if (-not (Test-Path $AdminUIPath)) {
    Write-Host "❌ Admin Dashboard 프로젝트를 찾을 수 없습니다: $AdminUIPath" -ForegroundColor Red
    exit 1
}

. (Join-Path $SharedDir "load-env.ps1")

try {
    $envVars = Get-DeployEnv -Target $Target
    Test-DeploySecrets -EnvVars $envVars -RequiredKeys @('ADMIN_DEPLOYMENT_TOKEN')
} catch {
    Write-Host "❌ preflight 실패: $_" -ForegroundColor Red
    Write-Host "   Azure Portal → Static Web App → Manage deployment token" -ForegroundColor Gray
    exit 1
}

Write-Host "✓ 환경 변수 로드 완료" -ForegroundColor Green
Write-Host ""

try {
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host "  📦 Admin Dashboard 배포" -ForegroundColor Cyan
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""

    Write-Host "📍 Static Web App: $($envVars['STATIC_WEB_APP_ADMIN'])" -ForegroundColor Gray
    Write-Host "📂 프로젝트 경로: $AdminUIPath" -ForegroundColor Gray
    Write-Host ""

    Push-Location $AdminUIPath

    Write-Host "📝 환경 변수 설정 중..." -ForegroundColor Yellow
    $prodEnvContent = "VITE_API_URL=$($envVars['API_URL'])"
    $prodEnvContent | Out-File -FilePath ".env.production" -Encoding UTF8 -Force
    Write-Host "✓ API URL: $($envVars['API_URL'])" -ForegroundColor Green
    Write-Host ""

    Write-Host "🔨 프로젝트 빌드 중..." -ForegroundColor Yellow
    npm run build

    if ($LASTEXITCODE -eq 0) {
        Write-Host "✓ 빌드 완료" -ForegroundColor Green
        Write-Host ""

        Write-Host "☁️  Azure Static Web App에 배포 중..." -ForegroundColor Yellow
        swa deploy ./dist `
            --env production `
            --deployment-token $envVars['ADMIN_DEPLOYMENT_TOKEN'] `
            --no-use-keychain

        if ($LASTEXITCODE -eq 0) {
            Write-Host ""
            Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
            Write-Host "  ✅ Admin Dashboard 배포 완료!" -ForegroundColor Green
            Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
            Write-Host ""
            Write-Host "🌐 URL: $($envVars['ADMIN_URL'])" -ForegroundColor Cyan
            Write-Host ""

            $tempZipFiles = Get-ChildItem -Path $AdminUIPath -Filter "*-app.zip" -ErrorAction SilentlyContinue
            if ($tempZipFiles) {
                Write-Host "🧹 임시 파일 정리 중..." -ForegroundColor Yellow
                $tempZipFiles | Remove-Item -Force
                Write-Host "✓ $($tempZipFiles.Count)개 임시 파일 삭제됨" -ForegroundColor Green
                Write-Host ""
            }
        } else {
            throw "Azure Static Web Apps 배포 실패"
        }
    } else {
        throw "프로젝트 빌드 실패"
    }

    Pop-Location

} catch {
    Write-Host ""
    Write-Host "❌ 배포 실패: $_" -ForegroundColor Red
    Write-Host ""
    if ((Get-Location).Path -ne $LibDir) {
        Pop-Location -ErrorAction SilentlyContinue
    }
    exit 1
}
