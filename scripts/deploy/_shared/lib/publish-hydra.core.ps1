# ============================================================
# Authway Hydra 배포 코어 로직 (타겟 무관)
# ============================================================
# Ory Hydra를 Azure Container App에 배포. 공식 이미지 사용 (빌드 없음).
# ============================================================

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("prod", "staging")]
    [string]$Target,

    [switch]$UpdateEnvOnly
)

Write-Host "🚀 Authway Hydra 배포 시작... (target=$Target)" -ForegroundColor Cyan
Write-Host ""

$LibDir = $PSScriptRoot
$SharedDir = Split-Path -Parent $LibDir

. (Join-Path $SharedDir "load-env.ps1")

try {
    $envVars = Get-DeployEnv -Target $Target
    Set-AzureSubscription -EnvVars $envVars
} catch {
    Write-Host "❌ preflight 실패: $_" -ForegroundColor Red
    exit 1
}

$RESOURCE_GROUP = $envVars['RESOURCE_GROUP']
$CONTAINER_APP_HYDRA = $envVars['CONTAINER_APP_HYDRA']

Write-Host "✓ 환경 변수 로드 완료" -ForegroundColor Green
Write-Host ""

try {
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host "  📦 Hydra Container App 배포" -ForegroundColor Cyan
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""

    Write-Host "📍 Resource Group: $RESOURCE_GROUP" -ForegroundColor Gray
    Write-Host "📦 Container App: $CONTAINER_APP_HYDRA" -ForegroundColor Gray
    Write-Host ""

    if ($UpdateEnvOnly) {
        Write-Host "🔧 환경 변수만 업데이트합니다..." -ForegroundColor Yellow
        Write-Host ""

        az containerapp update `
            --name $CONTAINER_APP_HYDRA `
            --resource-group $RESOURCE_GROUP `
            --set-env-vars `
                "URLS_SELF_ISSUER=$($envVars['HYDRA_ISSUER'])" `
                "URLS_LOGIN=$($envVars['LOGIN_URL'])" `
                "URLS_CONSENT=$($envVars['CONSENT_URL'])" `
                "URLS_ERROR=$($envVars['ERROR_URL'])" `
                "SERVE_COOKIES_SAME_SITE_MODE=Lax" `
                "SERVE_PUBLIC_CORS_ENABLED=true" `
                "SERVE_PUBLIC_CORS_ALLOWED_ORIGINS=$($envVars['CORS_ALLOWED_ORIGINS'])" `
                "LOG_LEVEL=$($envVars['LOG_LEVEL'])"

        if ($LASTEXITCODE -eq 0) {
            Write-Host ""
            Write-Host "✅ 환경 변수 업데이트 완료!" -ForegroundColor Green
        } else {
            throw "환경 변수 업데이트 실패"
        }
    } else {
        Write-Host "📝 Hydra Container App 업데이트 중..." -ForegroundColor Yellow
        Write-Host "   이미지: oryd/hydra:v26.2.0" -ForegroundColor Gray
        Write-Host ""

        $dsnUser = $envVars['AUTHWAY_DATABASE_USER']
        $dsnPassword = $envVars['AUTHWAY_DATABASE_PASSWORD']
        $dsnHost = $envVars['AUTHWAY_DATABASE_HOST']
        $dsnPort = $envVars['AUTHWAY_DATABASE_PORT']
        $dsnName = $envVars['AUTHWAY_DATABASE_NAME']
        $DSN = "postgres://${dsnUser}:${dsnPassword}@${dsnHost}:${dsnPort}/${dsnName}?sslmode=require"

        # Hydra entrypoint를 매 배포마다 명시: 일부 환경에서 인자 토큰화가 어긋나
        # `serve all --dev` 가 단일 토큰으로 들어가면 Hydra가 unknown command로 거절함.
        # prod 패턴(command=/bin/sh, args=-c "hydra serve all --dev")을 강제 일관화.
        az containerapp update `
            --name $CONTAINER_APP_HYDRA `
            --resource-group $RESOURCE_GROUP `
            --image "oryd/hydra:v26.2.0" `
            --command "/bin/sh" `
            --args "-c" "hydra serve all --dev" `
            --set-env-vars `
                "DSN=$DSN" `
                "SECRETS_SYSTEM=$($envVars['JWT_ACCESS_SECRET'])" `
                "URLS_SELF_ISSUER=$($envVars['HYDRA_ISSUER'])" `
                "URLS_LOGIN=$($envVars['LOGIN_URL'])" `
                "URLS_CONSENT=$($envVars['CONSENT_URL'])" `
                "URLS_ERROR=$($envVars['ERROR_URL'])" `
                "SERVE_COOKIES_SAME_SITE_MODE=Lax" `
                "SERVE_PUBLIC_CORS_ENABLED=true" `
                "SERVE_PUBLIC_CORS_ALLOWED_ORIGINS=$($envVars['CORS_ALLOWED_ORIGINS'])" `
                "LOG_LEVEL=$($envVars['LOG_LEVEL'])"

        if ($LASTEXITCODE -eq 0) {
            Write-Host ""
            Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
            Write-Host "  ✅ Hydra 배포 완료!" -ForegroundColor Green
            Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
            Write-Host ""
            Write-Host "🌐 Issuer URL: $($envVars['HYDRA_ISSUER'])" -ForegroundColor Cyan
            Write-Host ""
            Write-Host "🔍 헬스 체크:" -ForegroundColor Yellow
            Write-Host "   - OIDC Discovery: $($envVars['HYDRA_ISSUER'])/.well-known/openid-configuration" -ForegroundColor Gray
            Write-Host "   - Health: $($envVars['HYDRA_ISSUER'])/health/ready" -ForegroundColor Gray
            Write-Host ""
        } else {
            throw "Container App 업데이트 실패"
        }
    }

} catch {
    Write-Host ""
    Write-Host "❌ 배포 실패: $_" -ForegroundColor Red
    Write-Host ""
    exit 1
}
