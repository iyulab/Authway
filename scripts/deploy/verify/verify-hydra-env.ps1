# ============================================================
# Hydra Container Apps env 전달 검증 (POST-DEPLOY-VERIFY.md §1-1)
# ============================================================
# publish-hydra.core.ps1 이 공유 설정(configs/hydra.shared.env)과 흐름 URL 을
# `az containerapp update --set-env-vars` 로 넘긴다. az 가 그 인자들(특히 값에
# 쉼표가 든 것)을 실제로 받아들였는지는 배포된 리소스를 조회해야만 알 수 있다.
#
# 확인 대상:
#   - public·admin 두 Container App 모두: 공유 설정의 모든 키가 파일의 값과 같다
#     (OAUTH2_ALLOWED_TOP_LEVEL_CLAIMS 는 배포의 HYDRA_ALLOWED_TOP_LEVEL_CLAIMS 가
#     더해진 값).
#   - public: 흐름 URL 4 종.
#   - public·admin 둘 다: 비밀(DSN·SECRETS_SYSTEM 등)이 평문 값이 아니라 secretref.
#
# 사용:
#   verify/verify-hydra-env.ps1 -Target staging
#   verify/verify-hydra-env.ps1 -Target prod
# ============================================================

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("prod", "staging")]
    [string]$Target
)

$VerifyDir = $PSScriptRoot
if (-not $VerifyDir) { $VerifyDir = Split-Path -Parent $MyInvocation.MyCommand.Path }
$SharedDir = Join-Path (Split-Path -Parent $VerifyDir) "_shared"

. (Join-Path $SharedDir "load-env.ps1")
. (Join-Path $SharedDir "hydra-flow-urls.ps1")
. (Join-Path $SharedDir "hydra-shared-env.ps1")
. (Join-Path $SharedDir "app-secrets.ps1")

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🔧 Hydra Container App env 검증 (target=$Target)" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

try {
    $envVars = Get-DeployEnv -Target $Target
} catch {
    Write-Host "❌ env 로드 실패: $_" -ForegroundColor Red
    exit 1
}

try {
    Set-AzureSubscription -EnvVars $envVars
} catch {
    Write-Host "❌ Azure 구독 고정 실패: $_" -ForegroundColor Red
    exit 1
}

$resourceGroup = $envVars['RESOURCE_GROUP']
$apps = @(
    @{ Label = "public"; Name = $envVars['CONTAINER_APP_HYDRA'] },
    @{ Label = "admin"; Name = $envVars['CONTAINER_APP_HYDRA_ADMIN'] }
)

$failed = $false

try {
    $shared = Get-HydraSharedEnv -EnvVars $envVars
} catch {
    Write-Host "❌ 공유 설정 로드 실패: $_" -ForegroundColor Red
    exit 1
}

foreach ($app in $apps) {
    if ([string]::IsNullOrWhiteSpace($app.Name)) {
        Write-Host "❌ $($app.Label): CONTAINER_APP_HYDRA$(if ($app.Label -eq 'admin') { '_ADMIN' }) 미설정" -ForegroundColor Red
        $failed = $true
        continue
    }

    Write-Host "🔍 $($app.Label) ($($app.Name)) — 공유 설정 $($shared.Count) 개" -ForegroundColor Gray
    $raw = az containerapp show -n $app.Name -g $resourceGroup `
        --query "properties.template.containers[0].env" `
        -o json --only-show-errors 2>&1

    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ $($app.Label): az containerapp show 실패 — $raw" -ForegroundColor Red
        $failed = $true
        continue
    }

    try {
        Assert-NoPlaintextSecretEnv -App $app.Name -ResourceGroup $resourceGroup
    } catch {
        $failed = $true
    }

    $actual = @{}
    foreach ($e in ($raw | ConvertFrom-Json)) { $actual[$e.name] = $e.value }
    foreach ($name in $shared.Keys) {
        if (-not $actual.ContainsKey($name)) {
            Write-Host "❌ $($app.Label): $name 이 없음 (기대: '$($shared[$name])')" -ForegroundColor Red
            $failed = $true
        } elseif ($actual[$name] -ne $shared[$name]) {
            Write-Host "❌ $($app.Label): $name = '$($actual[$name])' (기대: '$($shared[$name])')" -ForegroundColor Red
            $failed = $true
        } else {
            Write-Host "✅ $($app.Label): $name = $($shared[$name])" -ForegroundColor Green
        }
    }
}

# Flow URLs on the public app: where Hydra sends the browser during sign-in.
$publicApp = $envVars['CONTAINER_APP_HYDRA']
if (-not [string]::IsNullOrWhiteSpace($publicApp)) {
    Write-Host "🔍 public ($publicApp) — URLS_LOGIN/CONSENT/LOGOUT/ERROR" -ForegroundColor Gray
    $expected = Get-HydraFlowUrls -EnvVars $envVars
    $raw = az containerapp show -n $publicApp -g $resourceGroup `
        --query "properties.template.containers[0].env[?starts_with(name, 'URLS_')]" `
        -o json --only-show-errors 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ public: az containerapp show 실패 — $raw" -ForegroundColor Red
        $failed = $true
    } else {
        $actual = @{}
        foreach ($e in ($raw | ConvertFrom-Json)) { $actual[$e.name] = $e.value }
        foreach ($name in $expected.Keys) {
            if ($actual[$name] -ne $expected[$name]) {
                Write-Host "❌ public: $name = '$($actual[$name])' (기대: '$($expected[$name])')" -ForegroundColor Red
                $failed = $true
            } else {
                Write-Host "✅ public: $name = $($expected[$name])" -ForegroundColor Green
            }
        }
    }
}

Write-Host ""
if ($failed) {
    Write-Host "❌ Hydra env 검증 실패 — 위 항목 참조. 배포가 configs/hydra.shared.env 보다 앞서 나갔거나(재배포 필요) 인자 전개가 깨진 것." -ForegroundColor Red
    exit 1
}

Write-Host "✅ Hydra env 검증 통과 (public·admin 양쪽 공유 설정 전부·비밀은 secretref, public 흐름 URL 4종)" -ForegroundColor Green
exit 0
