# ============================================================
# Authway Landing Page 배포 코어 로직 (타겟 무관)
# ============================================================
# 정적 HTML — 빌드 스텝 없음. Azure Static Web Apps에 디렉터리를 그대로 업로드.
# ============================================================

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("prod", "staging")]
    [string]$Target
)

Write-Host "🚀 Authway Landing Page 배포 시작... (target=$Target)" -ForegroundColor Cyan
Write-Host ""

$LibDir = $PSScriptRoot
$SharedDir = Split-Path -Parent $LibDir
$DeployDir = Split-Path -Parent $SharedDir
$ScriptsDir = Split-Path -Parent $DeployDir
$ProjectRoot = Split-Path -Parent $ScriptsDir
$LandingPath = Join-Path $ProjectRoot "apps\marketing\landing"

if (-not (Test-Path $LandingPath)) {
    Write-Host "❌ Landing 프로젝트를 찾을 수 없습니다: $LandingPath" -ForegroundColor Red
    exit 1
}

. (Join-Path $SharedDir "load-env.ps1")

try {
    $envVars = Get-DeployEnv -Target $Target
} catch {
    Write-Host "❌ env 로드 실패: $_" -ForegroundColor Red
    exit 1
}

try {
    Test-DeploySecrets -EnvVars $envVars -RequiredKeys @('LANDING_DEPLOYMENT_TOKEN')
} catch {
    Write-Host "❌ preflight 실패: $_" -ForegroundColor Red
    Write-Host "   Azure Portal → Static Web App → Manage deployment token" -ForegroundColor Gray
    exit 1
}

Write-Host "✓ 환경 변수 로드 완료" -ForegroundColor Green
Write-Host ""

try {
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host "  📦 Landing Page 배포 (Azure SWA)" -ForegroundColor Cyan
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""

    Write-Host "📂 프로젝트 경로: $LandingPath" -ForegroundColor Gray
    Write-Host "📍 Static Web App: $($envVars['STATIC_WEB_APP_LANDING'])" -ForegroundColor Gray
    Write-Host "☁️  Azure Static Web App에 배포 중..." -ForegroundColor Yellow

    swa deploy $LandingPath `
        --env production `
        --deployment-token $envVars['LANDING_DEPLOYMENT_TOKEN'] `
        --no-use-keychain

    if ($LASTEXITCODE -ne 0) { throw "Azure Static Web Apps 배포 실패" }

    Write-Host ""
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
    Write-Host "  ✅ Landing Page 배포 완료! (Azure SWA)" -ForegroundColor Green
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
    Write-Host ""
    Write-Host "🌐 URL: https://authway.in" -ForegroundColor Cyan

} catch {
    Write-Host ""
    Write-Host "❌ 배포 실패: $_" -ForegroundColor Red
    Write-Host ""
    exit 1
}
