# ============================================================
# Authway Auth Backend API 배포 코어 로직 (타겟 무관)
# ============================================================

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("prod", "staging")]
    [string]$Target,

    [string]$ImageTag = "",
    [switch]$SkipBuild
)

if (-not $ImageTag) {
    $ImageTag = "v" + (Get-Date -Format "yyyyMMddHHmmss")
}

Write-Host "🚀 Authway Auth Backend 배포 시작... (target=$Target)" -ForegroundColor Cyan
Write-Host ""

$LibDir = $PSScriptRoot
$SharedDir = Split-Path -Parent $LibDir
$DeployDir = Split-Path -Parent $SharedDir
$ScriptsDir = Split-Path -Parent $DeployDir
$ProjectRoot = Split-Path -Parent $ScriptsDir

. (Join-Path $SharedDir "load-env.ps1")

try {
    $envVars = Get-DeployEnv -Target $Target
    Set-AzureSubscription -EnvVars $envVars
} catch {
    Write-Host "❌ preflight 실패: $_" -ForegroundColor Red
    exit 1
}

$RESOURCE_GROUP = $envVars['RESOURCE_GROUP']
$CONTAINER_APP_AUTH_API = $envVars['CONTAINER_APP_AUTH_API']
$GITHUB_USER = $envVars['GITHUB_USER']
$ImageName = "ghcr.io/$GITHUB_USER/auth-api:$ImageTag"

Write-Host "✓ 환경 변수 로드 완료" -ForegroundColor Green
Write-Host ""

if ($envVars['GITHUB_TOKEN']) {
    Write-Host "🔑 GitHub Container Registry 인증 중..." -ForegroundColor Yellow
    $envVars['GITHUB_TOKEN'] | docker login ghcr.io -u $GITHUB_USER --password-stdin 2>&1 | Out-Null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✓ GHCR 로그인 성공" -ForegroundColor Green
        Write-Host ""
    }
}

try {
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host "  📦 Auth Backend Container App 배포" -ForegroundColor Cyan
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""

    if (-not $SkipBuild) {
        Write-Host "🔨 Docker 이미지 빌드 중..." -ForegroundColor Yellow
        Write-Host "   이미지: $ImageName" -ForegroundColor Gray
        Write-Host ""

        Push-Location $ProjectRoot
        if (Test-Path ".dockerignore.backup") {
            Remove-Item ".dockerignore.backup"
        }
        Move-Item ".dockerignore" ".dockerignore.backup"
        Copy-Item ".dockerignore.auth-api" ".dockerignore"

        docker build --no-cache -t $ImageName -f Dockerfile.auth-api .

        Remove-Item ".dockerignore"
        Move-Item ".dockerignore.backup" ".dockerignore"
        Pop-Location

        if ($LASTEXITCODE -ne 0) {
            throw "Docker 빌드 실패"
        }

        Write-Host ""
        Write-Host "☁️  GHCR에 푸시 중..." -ForegroundColor Yellow
        docker push $ImageName
        if ($LASTEXITCODE -ne 0) {
            throw "GHCR 푸시 실패"
        }

        Write-Host "✓ 이미지 빌드 및 푸시 완료" -ForegroundColor Green
        Write-Host ""
    }

    Write-Host "📦 Container App 업데이트 중..." -ForegroundColor Yellow
    Write-Host "   Resource Group: $RESOURCE_GROUP" -ForegroundColor Gray
    Write-Host "   Container App: $CONTAINER_APP_AUTH_API" -ForegroundColor Gray
    Write-Host ""

    Write-Host "🔐 레지스트리 인증 구성 중..." -ForegroundColor Yellow
    az containerapp registry set `
        --name $CONTAINER_APP_AUTH_API `
        --resource-group $RESOURCE_GROUP `
        --server ghcr.io `
        --username $GITHUB_USER `
        --password $envVars['GITHUB_TOKEN']

    if ($LASTEXITCODE -ne 0) {
        throw "레지스트리 인증 설정 실패"
    }
    Write-Host "✓ 레지스트리 인증 설정 완료" -ForegroundColor Green
    Write-Host ""

    Write-Host "🔄 이미지 및 환경 변수 업데이트 중..." -ForegroundColor Yellow

    az containerapp update `
        --name $CONTAINER_APP_AUTH_API `
        --resource-group $RESOURCE_GROUP `
        --image $ImageName `
        --set-env-vars `
            "ENVIRONMENT=$($envVars['APP_ENVIRONMENT'])" `
            "PORT=8081" `
            "AUTH_BACKEND_URL=$($envVars['AUTH_API_URL'])" `
            "CENTRAL_API_URL=$($envVars['API_INTERNAL_URL'])" `
            "INTERNAL_API_KEY=$($envVars['INTERNAL_API_KEY'])" `
            "HYDRA_ADMIN_URL=$($envVars['HYDRA_ADMIN_INTERNAL_URL'])" `
            "HYDRA_PUBLIC_URL=$($envVars['HYDRA_ISSUER'])" `
            "LOGIN_UI_URL=$($envVars['LOGIN_URL'])" `
            "GOOGLE_CLIENT_ID=$($envVars['GOOGLE_CLIENT_ID'])" `
            "GOOGLE_CLIENT_SECRET=$($envVars['GOOGLE_CLIENT_SECRET'])" `
            "GOOGLE_REDIRECT_URI=$($envVars['GOOGLE_REDIRECT_URI'])"

    if ($LASTEXITCODE -eq 0) {
        Write-Host ""
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
        Write-Host "  ✅ Auth Backend 배포 완료!" -ForegroundColor Green
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
        Write-Host ""
        Write-Host "🌐 Auth API URL: $($envVars['AUTH_API_URL'])" -ForegroundColor Cyan
        Write-Host "🔍 헬스 체크: $($envVars['AUTH_API_URL'])/health" -ForegroundColor Yellow
        Write-Host ""
    } else {
        throw "Container App 업데이트 실패"
    }

} catch {
    Write-Host ""
    Write-Host "❌ 배포 실패: $_" -ForegroundColor Red
    Write-Host ""
    if ((Get-Location).Path -ne $LibDir) {
        Pop-Location -ErrorAction SilentlyContinue
    }
    exit 1
}
