# ============================================================
# Authway audit_logs 배포 smoke (타겟 무관)
# ============================================================
# 배포 직후 audit_logs 에 행이 실제 생성되는지 확인 — fail-closed.
# 0건이면 audit emit 회귀가 prod/staging 에 섞여 들어간 가능성 → 배포 실패 처리.
#
# 근거: ISSUE-Authway-20260415-audit-smoke-staging-automation.md
#   배포 후 audit 배선 검증을 사람 눈이 아닌 스크립트로 강제.
#
# 사용:
#   _shared/smoke-audit.ps1 -Target prod
#   _shared/smoke-audit.ps1 -Target staging -WindowMinutes 10
# ============================================================

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("prod", "staging")]
    [string]$Target,

    # 얼마나 직전 구간을 볼지. 배포 직후 호출 기준.
    [int]$WindowMinutes = 5,

    # 0건 시 fail-closed 대신 경고로만 처리 (개발 중 idle 환경 대응용).
    [switch]$WarnOnly
)

$SharedDir = $PSScriptRoot
if (-not $SharedDir) { $SharedDir = Split-Path -Parent $MyInvocation.MyCommand.Path }

. (Join-Path $SharedDir "load-env.ps1")
. (Join-Path $SharedDir "migration-helpers.ps1")

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🧾 audit_logs smoke (target=$Target)" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

try {
    $envVars = Get-DeployEnv -Target $Target
} catch {
    Write-Host "❌ env 로드 실패: $_" -ForegroundColor Red
    exit 1
}

# psql 경로 초기화 (migration-helpers.ps1 내부 상태)
if (-not (Initialize-PsqlPath)) {
    Write-Host "❌ psql 미발견 (PATH / 기본 설치경로 모두 없음)" -ForegroundColor Red
    Write-Host "   설치: choco install postgresql  또는  winget install PostgreSQL.PostgreSQL" -ForegroundColor Yellow
    exit 1
}

# 대표 action 후보군. 실제 배포 흐름에서 최소 하나는 발생해야 함.
# (deploy-all.core.ps1 의 sync-hydra 호출이 client.* 계열 emit 가능성 있음.)
$query = @"
SELECT COUNT(*) FROM audit_logs
WHERE created_at > NOW() - INTERVAL '$WindowMinutes minute'
"@

Write-Host "🔍 쿼리: 최근 $WindowMinutes 분간 audit_logs 행 수" -ForegroundColor Gray
Write-Host "   DB: $($envVars['AUTHWAY_DATABASE_HOST']) / $($envVars['AUTHWAY_DATABASE_NAME'])" -ForegroundColor Gray
Write-Host ""

$result = Invoke-FastQuery -Query $query -EnvVars $envVars
if (-not $result.Success) {
    Write-Host "❌ 쿼리 실패: $($result.Output)" -ForegroundColor Red
    exit 1
}

$count = 0
if ([int]::TryParse(($result.Output -replace '\s', ''), [ref]$count)) {
    # parsed
} else {
    Write-Host "❌ 쿼리 결과 파싱 실패: '$($result.Output)'" -ForegroundColor Red
    exit 1
}

Write-Host "   최근 $WindowMinutes 분 audit_logs 행 수: $count" -ForegroundColor Gray
Write-Host ""

if ($count -eq 0) {
    if ($WarnOnly) {
        Write-Host "⚠️  audit_logs smoke: 0건 (WarnOnly 모드, fail-closed 생략)" -ForegroundColor Yellow
        Write-Host "   idle 환경이거나 audit 배선 회귀 가능성. 트래픽 있는 환경에서 재실행 권장." -ForegroundColor Yellow
        exit 0
    }
    Write-Host "❌ audit_logs smoke 실패: 최근 $WindowMinutes 분간 0건" -ForegroundColor Red
    Write-Host "   원인 후보:" -ForegroundColor Yellow
    Write-Host "   1) audit 배선이 신규 배포에서 회귀 (emit 누락)" -ForegroundColor Yellow
    Write-Host "   2) LogAsync 버퍼 드롭 (채널 capacity 초과)" -ForegroundColor Yellow
    Write-Host "   3) DB 연결 실패 (앱은 fail-closed지만 smoke 도달 시 가능성 낮음)" -ForegroundColor Yellow
    Write-Host "   4) idle 환경에서 smoke 실행 (트래픽 없음) — -WarnOnly 고려" -ForegroundColor Yellow
    exit 1
}

# 샘플 action 분포 조회 (최근 3건)
$sampleQuery = @"
SELECT action, actor_type, created_at FROM audit_logs
WHERE created_at > NOW() - INTERVAL '$WindowMinutes minute'
ORDER BY created_at DESC LIMIT 3
"@
$sampleResult = Invoke-FastQuery -Query $sampleQuery -EnvVars $envVars
if ($sampleResult.Success -and $sampleResult.Output) {
    Write-Host "📋 최근 샘플:" -ForegroundColor Gray
    $sampleResult.Output -split "`n" | ForEach-Object {
        if ($_.Trim()) { Write-Host "   $_" -ForegroundColor DarkGray }
    }
    Write-Host ""
}

Write-Host "✅ audit_logs smoke 통과: $count 건 기록됨" -ForegroundColor Green
exit 0
