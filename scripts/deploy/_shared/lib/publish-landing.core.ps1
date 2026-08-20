# ============================================================
# Authway Landing Page 배포 코어 로직 (타겟 무관)
# ============================================================
# 정적 HTML — 빌드 스텝 없음. Cloudflare Pages(apex 도메인 authway.in이
# Cloudflare DNS에 있어 apex CNAME flattening이 가능한 쪽)에 디렉터리를
# 그대로 업로드.
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
    Test-DeploySecrets -EnvVars $envVars -RequiredKeys @(
        'CLOUDFLARE_API_TOKEN',
        'CLOUDFLARE_ACCOUNT_ID',
        'CLOUDFLARE_PAGES_PROJECT_LANDING'
    )
} catch {
    Write-Host "❌ preflight 실패: $_" -ForegroundColor Red
    Write-Host "   Cloudflare 대시보드 → API Tokens / Account ID 확인" -ForegroundColor Gray
    exit 1
}

Write-Host "✓ 환경 변수 로드 완료" -ForegroundColor Green
Write-Host ""

try {
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host "  📦 Landing Page 배포 (Cloudflare Pages)" -ForegroundColor Cyan
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""

    $projectName = $envVars['CLOUDFLARE_PAGES_PROJECT_LANDING']
    Write-Host "📂 프로젝트 경로: $LandingPath" -ForegroundColor Gray
    Write-Host "☁️  Cloudflare Pages 배포: $projectName" -ForegroundColor Yellow

    $env:CLOUDFLARE_API_TOKEN = $envVars['CLOUDFLARE_API_TOKEN']
    $env:CLOUDFLARE_ACCOUNT_ID = $envVars['CLOUDFLARE_ACCOUNT_ID']

    $shortHash = (git rev-parse --short HEAD).Trim()
    $deployStamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $commitMessage = "$Target deploy $deployStamp ($shortHash)"

    npx --yes wrangler@latest pages deploy $LandingPath `
        --project-name=$projectName `
        --branch=main `
        --commit-hash=$shortHash `
        --commit-message="$commitMessage" `
        --commit-dirty=true

    if ($LASTEXITCODE -ne 0) { throw "Cloudflare Pages 배포 실패" }

    Write-Host ""
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
    Write-Host "  ✅ Landing Page 배포 완료! (Cloudflare Pages)" -ForegroundColor Green
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
    Write-Host ""
    Write-Host "🌐 URL: https://authway.in" -ForegroundColor Cyan
    Write-Host "🌐 Pages URL: https://$projectName.pages.dev" -ForegroundColor Cyan

} catch {
    Write-Host ""
    Write-Host "❌ 배포 실패: $_" -ForegroundColor Red
    Write-Host ""
    exit 1
}
