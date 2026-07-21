# ============================================================
# Authway Central API 배포 코어 로직 (타겟 무관)
# ============================================================
# 호출: prod/publish-api.ps1, staging/publish-api.ps1 에서 -Target 고정 후 invoke.
# 직접 호출도 가능하지만 권장하지 않음 (대상별 wrapper 사용).
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

Write-Host "🚀 Authway Central API 배포 시작... (target=$Target)" -ForegroundColor Cyan
Write-Host ""

$LibDir = $PSScriptRoot
$SharedDir = Split-Path -Parent $LibDir
$DeployDir = Split-Path -Parent $SharedDir
$ScriptsDir = Split-Path -Parent $DeployDir
$ProjectRoot = Split-Path -Parent $ScriptsDir

. (Join-Path $SharedDir "load-env.ps1")

try {
    $envVars = Get-DeployEnv -Target $Target

    # 중앙 API는 ADMIN_API_KEY / INTERNAL_API_KEY / TOTP 암호화 키가 비어 있으면
    # 기동 시점에 fail-closed로 거부한다. 2026-04 prod 사고 재발 방지 게이트.
    # (AUTHWAY_TOTP_ENCRYPTION_KEY 는 D-e — TOTP secret at-rest 암호화 — 필수)
    Test-DeploySecrets -EnvVars $envVars -RequiredKeys @('ADMIN_API_KEY', 'INTERNAL_API_KEY', 'AUTHWAY_TOTP_ENCRYPTION_KEY')

    Set-AzureSubscription -EnvVars $envVars
} catch {
    Write-Host "❌ preflight 실패: $_" -ForegroundColor Red
    exit 1
}

$RESOURCE_GROUP = $envVars['RESOURCE_GROUP']
$CONTAINER_APP_API = $envVars['CONTAINER_APP_API']
$GITHUB_USER = $envVars['GITHUB_USER']
$ImageName = "ghcr.io/$GITHUB_USER/authway-api:$ImageTag"

Write-Host "✓ 환경 변수 로드 완료 (target=$Target)" -ForegroundColor Green
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
    Write-Host "  📦 Central API Container App 배포" -ForegroundColor Cyan
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""

    if (-not $SkipBuild) {
        Write-Host "🔨 Docker 이미지 빌드 중..." -ForegroundColor Yellow
        Write-Host "   이미지: $ImageName" -ForegroundColor Gray
        Write-Host ""

        Push-Location $ProjectRoot
        # APP_VERSION build-arg stamps ImageTag into main.version (ldflag) →
        # cfg.App.Version → /health. 이 신호로 "새 이미지 배포됨" vs "이전 revision 서빙 중" 구분.
        docker build --no-cache `
            --build-arg APP_VERSION=$ImageTag `
            -t $ImageName -f Dockerfile .
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
    Write-Host "   Container App: $CONTAINER_APP_API" -ForegroundColor Gray
    Write-Host ""

    Write-Host "🔐 레지스트리 인증 구성 중..." -ForegroundColor Yellow
    az containerapp registry set `
        --name $CONTAINER_APP_API `
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
        --name $CONTAINER_APP_API `
        --resource-group $RESOURCE_GROUP `
        --image $ImageName `
        --set-env-vars `
            "AUTHWAY_APP_ENVIRONMENT=$($envVars['APP_ENVIRONMENT'])" `
            "PORT=8080" `
            "AUTHWAY_APP_PORT=8080" `
            "AUTHWAY_APP_BASE_URL=$($envVars['API_URL'])" `
            "AUTHWAY_APP_FRONTEND_URL=$($envVars['AUTH_UI_URL'])" `
            "AUTHWAY_ADMIN_PASSWORD=$($envVars['AUTHWAY_ADMIN_PASSWORD'])" `
            "AUTHWAY_DATABASE_HOST=$($envVars['AUTHWAY_DATABASE_HOST'])" `
            "AUTHWAY_DATABASE_PORT=$($envVars['AUTHWAY_DATABASE_PORT'])" `
            "AUTHWAY_DATABASE_NAME=$($envVars['AUTHWAY_DATABASE_NAME'])" `
            "AUTHWAY_DATABASE_USER=$($envVars['AUTHWAY_DATABASE_USER'])" `
            "AUTHWAY_DATABASE_PASSWORD=$($envVars['AUTHWAY_DATABASE_PASSWORD'])" `
            "AUTHWAY_DATABASE_SSL_MODE=require" `
            "AUTHWAY_REDIS_HOST=$($envVars['AUTHWAY_REDIS_HOST'])" `
            "AUTHWAY_REDIS_PORT=$($envVars['AUTHWAY_REDIS_PORT'])" `
            "AUTHWAY_REDIS_PASSWORD=$($envVars['AUTHWAY_REDIS_PASSWORD'])" `
            "AUTHWAY_REDIS_DB=$($envVars['AUTHWAY_REDIS_DB'])" `
            "AUTHWAY_REDIS_TLS_ENABLED=$($envVars['AUTHWAY_REDIS_TLS_ENABLED'])" `
            "AUTHWAY_JWT_ACCESS_TOKEN_SECRET=$($envVars['JWT_ACCESS_SECRET'])" `
            "AUTHWAY_JWT_REFRESH_TOKEN_SECRET=$($envVars['JWT_REFRESH_SECRET'])" `
            "AUTHWAY_HYDRA_ADMIN_URL=$($envVars['HYDRA_ADMIN_INTERNAL_URL'])" `
            "AUTHWAY_HYDRA_PUBLIC_URL=$($envVars['HYDRA_ISSUER'])" `
            "AUTHWAY_CORS_ALLOWED_ORIGINS=$($envVars['CORS_ALLOWED_ORIGINS'])" `
            "AUTHWAY_ADMIN_API_KEY=$($envVars['ADMIN_API_KEY'])" `
            "AUTHWAY_ADMIN_INTERNAL_API_KEY=$($envVars['INTERNAL_API_KEY'])" `
            "AUTHWAY_TOTP_ENCRYPTION_KEY=$($envVars['AUTHWAY_TOTP_ENCRYPTION_KEY'])" `
            "AUTHWAY_EMAIL_USE_AZURE=$($envVars['EMAIL_USE_AZURE'])" `
            "AUTHWAY_EMAIL_AZURE_BASE_URL=$($envVars['EMAIL_AZURE_BASE_URL'])" `
            "AUTHWAY_EMAIL_AZURE_FUNCTION_KEY=$($envVars['EMAIL_AZURE_FUNCTION_KEY'])" `
            "AUTHWAY_EMAIL_AZURE_PROFILE=$($envVars['EMAIL_AZURE_PROFILE'])" `
            "AUTHWAY_EMAIL_FROM_EMAIL=$($envVars['EMAIL_FROM_EMAIL'])" `
            "AUTHWAY_EMAIL_FROM_NAME=$($envVars['EMAIL_FROM_NAME'])" `
            "AUTHWAY_LOG_LEVEL=$($envVars['LOG_LEVEL'])"

    if ($LASTEXITCODE -ne 0) {
        throw "Container App 업데이트 실패"
    }

    # ============================================================
    # Post-deploy verification (version parity + admin-auth smoke)
    # ============================================================
    # ISSUE-Authway-20260415-prod-admin-api-unauthenticated 재발 방지 게이트.
    #   1. /health.version == ImageTag        → 실제 새 이미지가 서빙 중인가
    #   2. adminAuth 라우트 4종 == 401|503     → adminAuth 미들웨어가 걸려 있나

    $ApiUrl = $envVars['API_URL']
    Write-Host ""
    Write-Host "🩺 post-deploy 검증: Container App 기동 대기(최대 120초)..." -ForegroundColor Yellow

    $healthyVersion = $null
    for ($i = 1; $i -le 24; $i++) {
        Start-Sleep -Seconds 5
        try {
            $h = Invoke-RestMethod -Uri "$ApiUrl/health" -Method Get -TimeoutSec 10 -ErrorAction Stop
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
        Write-Host "   점검: az containerapp revision list -g $RESOURCE_GROUP -n $CONTAINER_APP_API" -ForegroundColor Yellow
        throw "version-parity 검증 실패"
    }
    Write-Host "✓ /health.version = $healthyVersion (일치)" -ForegroundColor Green

    Write-Host "🔒 adminAuth smoke test..." -ForegroundColor Yellow
    $endpoints = @(
        @{Method='GET';  Path='/api/v1/clients'},
        @{Method='GET';  Path='/api/v1/users'},
        @{Method='PUT';  Path='/api/v1/clients/00000000-0000-0000-0000-000000000000'; Body='{}'},
        @{Method='POST'; Path='/api/v1/clients/00000000-0000-0000-0000-000000000000/regenerate-secret'}
    )
    $smokeFailures = @()
    foreach ($ep in $endpoints) {
        try {
            $params = @{
                Uri = "$ApiUrl$($ep.Path)"
                Method = $ep.Method
                TimeoutSec = 10
                SkipHttpErrorCheck = $true
                ErrorAction = 'Stop'
            }
            if ($ep.Body) {
                $params.Body = $ep.Body
                $params.ContentType = 'application/json'
            }
            $resp = Invoke-WebRequest @params
            $code = [int]$resp.StatusCode
        } catch {
            $code = 0
            if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        }
        $label = "$($ep.Method.PadRight(4)) $($ep.Path)"
        if ($code -eq 401 -or $code -eq 503) {
            Write-Host "  ✓ $label → $code" -ForegroundColor Green
        } else {
            Write-Host "  ❌ $label → $code (401/503 기대)" -ForegroundColor Red
            $smokeFailures += "$label=$code"
        }
    }

    if ($smokeFailures.Count -gt 0) {
        Write-Host ""
        Write-Host "❌ adminAuth smoke 실패: $($smokeFailures -join ', ')" -ForegroundColor Red
        Write-Host "   배포된 빌드가 adminAuth를 적용하지 않거나 API key env-var가 주입되지 않았습니다." -ForegroundColor Yellow
        Write-Host "   즉시 롤백 권고: az containerapp revision set-mode --mode single" -ForegroundColor Yellow
        throw "adminAuth smoke 검증 실패"
    }

    # ------------------------------------------------------------
    # Mail-link smoke: can a human actually reach what we email them?
    # ------------------------------------------------------------
    # ISSUE-Authway-20260721-170000-frontend-url-config-missing 재발 방지 게이트.
    # 초대·매직링크·인증·재설정 링크는 전부 auth UI 호스트로 만들어진다. 그 호스트가
    # 틀리거나(구성) 딥링크를 서빙하지 않으면(CDN SPA fallback) 발송은 성공하는데
    # 수신자만 404 를 본다 — 어떤 API 응답으로도 드러나지 않는 침묵형 고장이다.
    #   1. /api/v1/config.auth_ui == AUTH_UI_URL  → 배포된 바이너리가 받은 값이 맞나
    #   2. 링크 4종 cold GET == 200               → 그 호스트가 딥링크를 서빙하나
    # 메일을 실제로 보내지 않으므로 테스트 계정·bounce 를 남기지 않는다.

    $AuthUiUrl = $envVars['AUTH_UI_URL']
    Write-Host "✉️  mail-link smoke test..." -ForegroundColor Yellow

    if (-not $AuthUiUrl) {
        Write-Host "  ❌ AUTH_UI_URL 이 .env 에 없음" -ForegroundColor Red
        throw "mail-link smoke 검증 실패: AUTH_UI_URL 미설정"
    }

    try {
        $discovery = Invoke-RestMethod -Uri "$ApiUrl/api/v1/config" -Method Get -TimeoutSec 10 -ErrorAction Stop
    } catch {
        throw "mail-link smoke 검증 실패: /api/v1/config 조회 불가 ($_)"
    }

    if ($discovery.auth_ui -ne $AuthUiUrl) {
        Write-Host "  ❌ 배포된 auth_ui = '$($discovery.auth_ui)' (기대 '$AuthUiUrl')" -ForegroundColor Red
        Write-Host "     AUTHWAY_APP_FRONTEND_URL 주입이 누락됐거나 빈 값입니다." -ForegroundColor Yellow
        Write-Host "     빈 값은 viper 가 무시하고 기본값(localhost)으로 되돌립니다." -ForegroundColor Yellow
        throw "mail-link smoke 검증 실패: auth_ui 불일치"
    }
    Write-Host "  ✓ auth_ui = $AuthUiUrl (일치)" -ForegroundColor Green

    # 메일 템플릿이 만드는 경로 전부. 하나라도 404 면 그 메일은 도착해도 무용지물이다.
    $linkPaths = @(
        '/invitation/accept?token=deploy-probe',
        '/magic-link?token=deploy-probe',
        '/verify-email?token=deploy-probe',
        '/reset-password?token=deploy-probe'
    )
    $linkFailures = @()
    foreach ($path in $linkPaths) {
        try {
            $resp = Invoke-WebRequest -Uri "$AuthUiUrl$path" -Method Get -TimeoutSec 15 `
                -SkipHttpErrorCheck -ErrorAction Stop
            $code = [int]$resp.StatusCode
        } catch {
            $code = 0
            if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        }
        if ($code -eq 200) {
            Write-Host "  ✓ $path → 200" -ForegroundColor Green
        } else {
            Write-Host "  ❌ $path → $code (200 기대)" -ForegroundColor Red
            $linkFailures += "$path=$code"
        }
    }

    if ($linkFailures.Count -gt 0) {
        Write-Host ""
        Write-Host "❌ mail-link smoke 실패: $($linkFailures -join ', ')" -ForegroundColor Red
        Write-Host "   auth UI 가 배포되지 않았거나, SPA 딥링크 fallback(_redirects 200 rewrite)이" -ForegroundColor Yellow
        Write-Host "   설정되지 않았습니다. 이 상태로는 초대 메일을 받은 사용자가 전원 404 를 봅니다." -ForegroundColor Yellow
        throw "mail-link smoke 검증 실패"
    }

    Write-Host ""
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
    Write-Host "  ✅ Central API 배포 완료 (검증 통과)" -ForegroundColor Green
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
    Write-Host ""
    Write-Host "🌐 API URL: $ApiUrl" -ForegroundColor Cyan
    Write-Host "🏷  배포 버전: $ImageTag" -ForegroundColor Cyan
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
