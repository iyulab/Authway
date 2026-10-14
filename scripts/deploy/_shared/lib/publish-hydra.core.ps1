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
. (Join-Path $SharedDir "az-retry.ps1")
. (Join-Path $SharedDir "hydra-flow-urls.ps1")
. (Join-Path $SharedDir "hydra-shared-env.ps1")
. (Join-Path $SharedDir "app-secrets.ps1")

try {
    $envVars = Get-DeployEnv -Target $Target
    Test-DeploySecrets -EnvVars $envVars -RequiredKeys @('HYDRA_SECRETS_SYSTEM', 'ACR_USERNAME', 'ACR_PASSWORD', 'AUTHWAY_DATABASE_PASSWORD')
    Set-AzureSubscription -EnvVars $envVars
} catch {
    Write-Host "❌ preflight 실패: $_" -ForegroundColor Red
    exit 1
}

$RESOURCE_GROUP = $envVars['RESOURCE_GROUP']
$CONTAINER_APP_HYDRA = $envVars['CONTAINER_APP_HYDRA']
$CONTAINER_APP_HYDRA_ADMIN = $envVars['CONTAINER_APP_HYDRA_ADMIN']

# Where Hydra sends the browser during sign-in (see hydra-flow-urls.ps1), and
# the settings every deployment shares (configs/hydra.shared.env, see
# hydra-shared-env.ps1) — token shape, what discovery advertises, lifetimes.
try {
    $FlowUrlEnv = @((Get-HydraFlowUrls -EnvVars $envVars).GetEnumerator() | ForEach-Object { "$($_.Key)=$($_.Value)" })
    $SharedEnv = @((Get-HydraSharedEnv -EnvVars $envVars).GetEnumerator() | ForEach-Object { "$($_.Key)=$($_.Value)" })
} catch {
    Write-Host "❌ preflight 실패: $_" -ForegroundColor Red
    exit 1
}

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
                "SERVE_COOKIES_SAME_SITE_MODE=Lax" `
                "SERVE_PUBLIC_CORS_ENABLED=true" `
                "SERVE_PUBLIC_CORS_ALLOWED_ORIGINS=$($envVars['CORS_ALLOWED_ORIGINS'])" `
                "LOG_LEVEL=$($envVars['LOG_LEVEL'])" `
                $FlowUrlEnv `
                $SharedEnv

        if ($LASTEXITCODE -eq 0) {
            Write-Host ""
            Write-Host "✅ 환경 변수 업데이트 완료!" -ForegroundColor Green
        } else {
            throw "환경 변수 업데이트 실패"
        }
        Assert-NoPlaintextSecretEnv -App $CONTAINER_APP_HYDRA -ResourceGroup $RESOURCE_GROUP
    } else {
        Write-Host "📝 Hydra Container App 업데이트 중..." -ForegroundColor Yellow
        Write-Host "   이미지: iyulabimages.azurecr.io/hydra:v26.2.0" -ForegroundColor Gray
        Write-Host ""

        # ACR 레지스트리 자격증명 등록 (idempotent — 이미 있어도 덮어쓰기)
        Write-Host "🔑 ACR 레지스트리 인증 설정 중..." -ForegroundColor Yellow
        Invoke-AzRetry -What "ACR 레지스트리 설정" -Command {
            az containerapp registry set `
                --name $CONTAINER_APP_HYDRA `
                --resource-group $RESOURCE_GROUP `
                --server "iyulabimages.azurecr.io" `
                --username $envVars['ACR_USERNAME'] `
                --password $envVars['ACR_PASSWORD'] | Out-Null
        }
        Write-Host "   ✓ ACR 레지스트리 인증 완료" -ForegroundColor Green
        Write-Host ""

        $dsnUser = $envVars['AUTHWAY_DATABASE_USER']
        $dsnPassword = $envVars['AUTHWAY_DATABASE_PASSWORD']
        $dsnHost = $envVars['AUTHWAY_DATABASE_HOST']
        $dsnPort = $envVars['AUTHWAY_DATABASE_PORT']
        $dsnName = $envVars['AUTHWAY_DATABASE_NAME']
        $DSN = "postgres://${dsnUser}:${dsnPassword}@${dsnHost}:${dsnPort}/${dsnName}?sslmode=require"

        # The DSN embeds the database password and SECRETS_SYSTEM encrypts
        # what Hydra stores, so both go in as Container App secrets and the
        # environment only names them (app-secrets.ps1). Each app holds its
        # own copy: secrets belong to an app, not to the environment.
        $hydraSecrets = [ordered]@{
            'dsn'            = $DSN
            'secrets-system' = $envVars['HYDRA_SECRETS_SYSTEM']
        }
        Write-Host "🔐 Container App 비밀 반영 중: $($hydraSecrets.Keys -join ', ')" -ForegroundColor Yellow
        $revisionBefore = Get-LatestRevisionName -App $CONTAINER_APP_HYDRA -ResourceGroup $RESOURCE_GROUP
        Set-ContainerAppSecrets -App $CONTAINER_APP_HYDRA -ResourceGroup $RESOURCE_GROUP -Secrets $hydraSecrets

        # Hydra entrypoint를 매 배포마다 명시. args를 토큰 단위로 분리하여 az CLI parsing bug 회피
        # (--args "-c" 패턴은 az CLI가 -c를 자신의 옵션으로 파싱하여 실패함).
        # /bin/sh 래퍼 제거: hydra serve all 직접 토큰화. production에서 --dev 사용 불가.
        az containerapp update `
            --name $CONTAINER_APP_HYDRA `
            --resource-group $RESOURCE_GROUP `
            --image "iyulabimages.azurecr.io/hydra:v26.2.0" `
            --command "hydra" `
            --args "serve" "all" `
            --set-env-vars `
                "DSN=secretref:dsn" `
                "SECRETS_SYSTEM=secretref:secrets-system" `
                "URLS_SELF_ISSUER=$($envVars['HYDRA_ISSUER'])" `
                "SERVE_COOKIES_SAME_SITE_MODE=Lax" `
                "SERVE_PUBLIC_CORS_ENABLED=true" `
                "SERVE_PUBLIC_CORS_ALLOWED_ORIGINS=$($envVars['CORS_ALLOWED_ORIGINS'])" `
                "LOG_LEVEL=$($envVars['LOG_LEVEL'])" `
                $FlowUrlEnv `
                $SharedEnv

        if ($LASTEXITCODE -ne 0) { throw "Hydra public 업데이트 실패" }
        Restart-IfNoNewRevision -App $CONTAINER_APP_HYDRA -ResourceGroup $RESOURCE_GROUP -RevisionBefore $revisionBefore
        Assert-NoPlaintextSecretEnv -App $CONTAINER_APP_HYDRA -ResourceGroup $RESOURCE_GROUP

        if ($CONTAINER_APP_HYDRA_ADMIN) {
            Write-Host "🔄 Hydra Admin Container App 업데이트 중..." -ForegroundColor Yellow
            Write-Host "   Container App: $CONTAINER_APP_HYDRA_ADMIN" -ForegroundColor Gray

            Invoke-AzRetry -What "Hydra admin ACR 설정" -Command {
                az containerapp registry set `
                    --name $CONTAINER_APP_HYDRA_ADMIN `
                    --resource-group $RESOURCE_GROUP `
                    --server "iyulabimages.azurecr.io" `
                    --username $envVars['ACR_USERNAME'] `
                    --password $envVars['ACR_PASSWORD'] | Out-Null
            }
            $adminRevisionBefore = Get-LatestRevisionName -App $CONTAINER_APP_HYDRA_ADMIN -ResourceGroup $RESOURCE_GROUP
            Set-ContainerAppSecrets -App $CONTAINER_APP_HYDRA_ADMIN -ResourceGroup $RESOURCE_GROUP -Secrets $hydraSecrets

            az containerapp update `
                --name $CONTAINER_APP_HYDRA_ADMIN `
                --resource-group $RESOURCE_GROUP `
                --image "iyulabimages.azurecr.io/hydra:v26.2.0" `
                --command "hydra" `
                --args "serve" "admin" `
                --set-env-vars `
                    "DSN=secretref:dsn" `
                    "SECRETS_SYSTEM=secretref:secrets-system" `
                    "URLS_SELF_ISSUER=$($envVars['HYDRA_ISSUER'])" `
                    "LOG_LEVEL=$($envVars['LOG_LEVEL'])" `
                    $SharedEnv | Out-Null
            if ($LASTEXITCODE -ne 0) { throw "Hydra admin 업데이트 실패" }
            Restart-IfNoNewRevision -App $CONTAINER_APP_HYDRA_ADMIN -ResourceGroup $RESOURCE_GROUP -RevisionBefore $adminRevisionBefore
            Assert-NoPlaintextSecretEnv -App $CONTAINER_APP_HYDRA_ADMIN -ResourceGroup $RESOURCE_GROUP
            Write-Host "   ✓ Hydra Admin 업데이트 완료" -ForegroundColor Green
            Write-Host ""
        }

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
    }

} catch {
    Write-Host ""
    Write-Host "❌ 배포 실패: $_" -ForegroundColor Red
    Write-Host ""
    exit 1
}
