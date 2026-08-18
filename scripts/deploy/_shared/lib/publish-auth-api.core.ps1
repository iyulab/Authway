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

        # APP_VERSION build-arg stamps ImageTag into main.version (ldflag) →
        # HealthHandler → /health + /.well-known/authway-config. Same pattern
        # as publish-api.core.ps1 (Central API).
        docker build --no-cache `
            --build-arg APP_VERSION=$ImageTag `
            -t $ImageName -f Dockerfile.auth-api .

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
            "GOOGLE_REDIRECT_URI=$($envVars['GOOGLE_REDIRECT_URI'])" `
            "AUTHWAY_REDIS_HOST=$($envVars['AUTHWAY_REDIS_HOST'])" `
            "AUTHWAY_REDIS_PORT=$($envVars['AUTHWAY_REDIS_PORT'])" `
            "AUTHWAY_REDIS_PASSWORD=$($envVars['AUTHWAY_REDIS_PASSWORD'])" `
            "AUTHWAY_REDIS_DB=$($envVars['AUTHWAY_REDIS_DB'])" `
            "AUTHWAY_REDIS_TLS_ENABLED=$($envVars['AUTHWAY_REDIS_TLS_ENABLED'])"

    if ($LASTEXITCODE -ne 0) {
        throw "Container App 업데이트 실패"
    }

    # ============================================================
    # Post-deploy verification (version parity)
    # ============================================================
    # /health.version이 하드코딩 리터럴이던 시절엔 이 체크가 무의미했다 —
    # 이제 main.version이 실제로 빌드에 stamp되므로 (cycle-110) 의미가 생김.
    # 같은 패턴: publish-api.core.ps1(Central API).

    $AuthApiUrl = $envVars['AUTH_API_URL']
    Write-Host ""
    Write-Host "🩺 post-deploy 검증: Container App 기동 대기(최대 120초)..." -ForegroundColor Yellow

    $healthyVersion = $null
    for ($i = 1; $i -le 24; $i++) {
        Start-Sleep -Seconds 5
        try {
            $h = Invoke-RestMethod -Uri "$AuthApiUrl/health" -Method Get -TimeoutSec 10 -ErrorAction Stop
            if ($h.version -eq $ImageTag) {
                $healthyVersion = $h.version
                break
            }
        } catch {
            # 기동 중 — 계속 대기
        }
    }

    if (-not $healthyVersion) {
        Write-Host "❌ /health 응답이 새 이미지 버전($ImageTag)을 반환하지 않음" -ForegroundColor Red
        Write-Host "   이전 revision이 서빙 중이거나 새 revision이 기동 실패했을 가능성." -ForegroundColor Yellow
        Write-Host "   점검: az containerapp revision list -g $RESOURCE_GROUP -n $CONTAINER_APP_AUTH_API" -ForegroundColor Yellow
        throw "version-parity 검증 실패"
    }
    Write-Host "✓ /health.version = $healthyVersion (일치)" -ForegroundColor Green

    Write-Host ""
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
    Write-Host "  ✅ Auth Backend 배포 완료!" -ForegroundColor Green
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
    Write-Host ""
    Write-Host "🌐 Auth API URL: $($envVars['AUTH_API_URL'])" -ForegroundColor Cyan
    Write-Host "🔍 헬스 체크: $($envVars['AUTH_API_URL'])/health" -ForegroundColor Yellow
    Write-Host ""

} catch {
    Write-Host ""
    Write-Host "❌ 배포 실패: $_" -ForegroundColor Red
    Write-Host ""
    if ((Get-Location).Path -ne $LibDir) {
        Pop-Location -ErrorAction SilentlyContinue
    }
    exit 1
}
