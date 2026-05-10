# ============================================================
# Authway Admin Dashboard 배포 코어 로직 (타겟 무관)
# ============================================================
# 배포 플랫폼 분기:
#   - ADMIN_DEPLOY_PLATFORM=azure       → Azure Static Web Apps (swa CLI)
#   - ADMIN_DEPLOY_PLATFORM=cloudflare  → Cloudflare Pages (wrangler)
#   - 미지정 시 기본값: azure (하위 호환)
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
} catch {
    Write-Host "❌ env 로드 실패: $_" -ForegroundColor Red
    exit 1
}

$platform = $envVars['ADMIN_DEPLOY_PLATFORM']
if ([string]::IsNullOrWhiteSpace($platform)) { $platform = 'azure' }
Write-Host "🧭 Deploy platform: $platform" -ForegroundColor Cyan

try {
    if ($platform -eq 'cloudflare') {
        Test-DeploySecrets -EnvVars $envVars -RequiredKeys @(
            'CLOUDFLARE_API_TOKEN',
            'CLOUDFLARE_ACCOUNT_ID',
            'CLOUDFLARE_PAGES_PROJECT_ADMIN'
        )
    } else {
        Test-DeploySecrets -EnvVars $envVars -RequiredKeys @('ADMIN_DEPLOYMENT_TOKEN')
    }
} catch {
    Write-Host "❌ preflight 실패: $_" -ForegroundColor Red
    if ($platform -eq 'cloudflare') {
        Write-Host "   Cloudflare 대시보드 → API Tokens / Account ID 확인" -ForegroundColor Gray
    } else {
        Write-Host "   Azure Portal → Static Web App → Manage deployment token" -ForegroundColor Gray
    }
    exit 1
}

Write-Host "✓ 환경 변수 로드 완료" -ForegroundColor Green
Write-Host ""

try {
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host "  📦 Admin Dashboard 배포 ($platform)" -ForegroundColor Cyan
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""

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
    if ($LASTEXITCODE -ne 0) { throw "프로젝트 빌드 실패" }
    Write-Host "✓ 빌드 완료" -ForegroundColor Green
    Write-Host ""

    if ($platform -eq 'cloudflare') {
        $projectName = $envVars['CLOUDFLARE_PAGES_PROJECT_ADMIN']
        Write-Host "☁️  Cloudflare Pages 배포: $projectName" -ForegroundColor Yellow

        $env:CLOUDFLARE_API_TOKEN = $envVars['CLOUDFLARE_API_TOKEN']
        $env:CLOUDFLARE_ACCOUNT_ID = $envVars['CLOUDFLARE_ACCOUNT_ID']

        $branch = if ($Target -eq 'prod') { 'main' } else { 'staging' }

        $shortHash = (git rev-parse --short HEAD).Trim()
        $deployStamp = Get-Date -Format "yyyyMMdd-HHmmss"
        $commitMessage = "staging deploy $deployStamp ($shortHash)"

        npx --yes wrangler@latest pages deploy ./dist `
            --project-name=$projectName `
            --branch=$branch `
            --commit-hash=$shortHash `
            --commit-message="$commitMessage" `
            --commit-dirty=true

        if ($LASTEXITCODE -ne 0) { throw "Cloudflare Pages 배포 실패" }

        Write-Host ""
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
        Write-Host "  ✅ Admin Dashboard 배포 완료! (Cloudflare Pages)" -ForegroundColor Green
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
        Write-Host ""
        Write-Host "🌐 URL: https://$projectName.pages.dev" -ForegroundColor Cyan
        Write-Host "🌐 ADMIN_URL: $($envVars['ADMIN_URL'])" -ForegroundColor Cyan
    }
    else {
        Write-Host "📍 Static Web App: $($envVars['STATIC_WEB_APP_ADMIN'])" -ForegroundColor Gray
        Write-Host "☁️  Azure Static Web App에 배포 중..." -ForegroundColor Yellow
        swa deploy ./dist `
            --env production `
            --deployment-token $envVars['ADMIN_DEPLOYMENT_TOKEN'] `
            --no-use-keychain

        if ($LASTEXITCODE -ne 0) { throw "Azure Static Web Apps 배포 실패" }

        Write-Host ""
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
        Write-Host "  ✅ Admin Dashboard 배포 완료! (Azure SWA)" -ForegroundColor Green
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
        Write-Host ""
        Write-Host "🌐 URL: $($envVars['ADMIN_URL'])" -ForegroundColor Cyan

        $tempZipFiles = Get-ChildItem -Path $AdminUIPath -Filter "*-app.zip" -ErrorAction SilentlyContinue
        if ($tempZipFiles) {
            Write-Host "🧹 임시 파일 정리 중..." -ForegroundColor Yellow
            $tempZipFiles | Remove-Item -Force
            Write-Host "✓ $($tempZipFiles.Count)개 임시 파일 삭제됨" -ForegroundColor Green
            Write-Host ""
        }
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
