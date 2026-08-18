# ============================================================
# Hydra Container Apps env 전달 검증 (POST-DEPLOY-VERIFY.md §5)
# ============================================================
# publish-hydra.core.ps1이 $TokenEnv 배열을 `az containerapp update
# --set-env-vars`로 넘긴다. PowerShell 쪽 인자 구성은 로컬에서 실측 확인됐지만,
# az 자체가 그 인자들(특히 값에 쉼표가 든 HYDRA_ALLOWED_TOP_LEVEL_CLAIMS)을
# 실제로 받아들였는지는 배포된 리소스를 조회해야만 확인 가능 — 로컬로 닫을 수
# 없는 배포-전용 검증이라 사람 손으로 매 배포마다 반복돼 왔다.
#
# 확인 대상: public·admin 두 Container App 모두, STRATEGIES_ACCESS_TOKEN이
# 실제로 "opaque"로 반영됐는지(전역 전략 회귀 시 모든 소비자의 토큰 형식이
# 바뀐다 — 가장 파급이 큰 값이라 이것만 자동 검증한다). 커스텀 클레임
# 미러링(§4)은 배포마다 값이 다를 수 있어 여기서는 다루지 않는다 — 필요하면
# 이 스크립트가 출력하는 전체 env 목록에서 사람이 직접 확인.
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

foreach ($app in $apps) {
    if ([string]::IsNullOrWhiteSpace($app.Name)) {
        Write-Host "❌ $($app.Label): CONTAINER_APP_HYDRA$(if ($app.Label -eq 'admin') { '_ADMIN' }) 미설정" -ForegroundColor Red
        $failed = $true
        continue
    }

    Write-Host "🔍 $($app.Label) ($($app.Name)) — STRATEGIES_ACCESS_TOKEN" -ForegroundColor Gray
    $raw = az containerapp show -n $app.Name -g $resourceGroup `
        --query "properties.template.containers[0].env[?name=='STRATEGIES_ACCESS_TOKEN']" `
        -o json 2>&1

    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ $($app.Label): az containerapp show 실패 — $raw" -ForegroundColor Red
        $failed = $true
        continue
    }

    $parsed = $raw | ConvertFrom-Json
    if (-not $parsed -or $parsed.Count -eq 0) {
        Write-Host "❌ $($app.Label): STRATEGIES_ACCESS_TOKEN env 자체가 없음 — 배열 전개 실패 가능성" -ForegroundColor Red
        $failed = $true
        continue
    }

    $value = $parsed[0].value
    if ($value -ne "opaque") {
        Write-Host "❌ $($app.Label): STRATEGIES_ACCESS_TOKEN = '$value' (기대: 'opaque')" -ForegroundColor Red
        $failed = $true
        continue
    }

    Write-Host "✅ $($app.Label): opaque 확인" -ForegroundColor Green
}

Write-Host ""
if ($failed) {
    Write-Host "❌ Hydra env 검증 실패 — 위 항목 참조. publish-hydra.core.ps1의 배열 전개를 재확인할 것." -ForegroundColor Red
    exit 1
}

Write-Host "✅ Hydra env 검증 통과 (public·admin 양쪽 STRATEGIES_ACCESS_TOKEN=opaque)" -ForegroundColor Green
exit 0
