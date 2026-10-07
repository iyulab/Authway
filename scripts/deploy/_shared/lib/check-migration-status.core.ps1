# ============================================================
# Migration status — 대상 DB의 schema_migrations 와 저장소의 마이그레이션 파일 대조
# ============================================================
# 마이그레이션은 API 기동 시 Go migrator(apps/central/api/internal/database)가
# 한 트랜잭션으로 적용한다. 이 스크립트는 읽기 전용으로 결과만 확인한다:
#   - 적용된 버전과 최신 버전
#   - 저장소에는 있으나 DB에 없는 버전(보류) — 배포 전이면 정상, 배포 후면 결함
#   - success=false 로 남은 기록
#   - DB에는 있으나 저장소에 없는 버전(알 수 없는 기록)
#
# 사용: prod/check-migration-status.ps1 · staging/check-migration-status.ps1
# 종료 코드: 0 = 보류·실패 없음, 1 = 접속/조회 실패, 2 = 보류 또는 실패 기록 있음
# ============================================================

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("prod", "staging")]
    [string]$Target
)

$LibDir = $PSScriptRoot
$SharedDir = Split-Path -Parent $LibDir
$RepoRoot = Split-Path -Parent (Split-Path -Parent (Split-Path -Parent $SharedDir))
$MigrationsDir = Join-Path $RepoRoot "apps\central\api\internal\database\migrations"

. (Join-Path $SharedDir "load-env.ps1")
. (Join-Path $SharedDir "psql-helpers.ps1")

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  📊 Migration status (target=$Target)" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

try {
    $envVars = Get-DeployEnv -Target $Target
} catch {
    Write-Host "❌ env 로드 실패: $_" -ForegroundColor Red
    exit 1
}

if (-not (Initialize-PsqlPath)) {
    Write-Host "❌ psql 미발견 (PATH / 기본 설치경로 모두 없음)" -ForegroundColor Red
    Write-Host "   설치: winget install PostgreSQL.PostgreSQL" -ForegroundColor Yellow
    exit 1
}

# Go migrator 와 같은 규칙: ^(\d+)_(.+)\.sql$
$repoVersions = [ordered]@{}
Get-ChildItem -Path $MigrationsDir -Filter "*.sql" | Sort-Object Name | ForEach-Object {
    if ($_.Name -match '^(\d+)_(.+)\.sql$') {
        $repoVersions[$matches[1]] = $matches[2]
    }
}
if ($repoVersions.Count -eq 0) {
    Write-Host "❌ 마이그레이션 파일을 찾지 못함: $MigrationsDir" -ForegroundColor Red
    exit 1
}

$query = @"
SELECT version, name, success, TO_CHAR(executed_at, 'YYYY-MM-DD HH24:MI:SS')
FROM schema_migrations
ORDER BY version;
"@

$result = Invoke-FastQuery -Query $query -EnvVars $envVars
if (-not $result.Success) {
    Write-Host "❌ schema_migrations 조회 실패: $($result.Output)" -ForegroundColor Red
    exit 1
}

$applied = @{}
$failed = @()
$rows = @()
foreach ($line in ($result.Output -split "`n")) {
    if ($line.Trim() -eq '') { continue }
    $parts = $line -split '\|'
    if ($parts.Length -ne 4) { continue }
    $row = [pscustomobject]@{
        Version    = $parts[0].Trim()
        Name       = $parts[1].Trim()
        Success    = ($parts[2].Trim() -eq 't')
        ExecutedAt = $parts[3].Trim()
    }
    $rows += $row
    if ($row.Success) { $applied[$row.Version] = $true } else { $failed += $row }
}

Write-Host "📋 적용 기록 (schema_migrations)" -ForegroundColor Cyan
$rows | Format-Table -AutoSize Version, Name, Success, ExecutedAt | Out-String -Width 200 | Write-Host

$pending = @($repoVersions.Keys | Where-Object { -not $applied.ContainsKey($_) })
$unknown = @($rows | Where-Object { -not $repoVersions.Contains($_.Version) })
$latestRepo = @($repoVersions.Keys)[-1]
$latestApplied = @($applied.Keys | Sort-Object)[-1]

Write-Host "📊 요약" -ForegroundColor Cyan
Write-Host "  저장소 최신 버전 : $latestRepo"
Write-Host "  DB 최신 적용 버전: $latestApplied"
Write-Host "  적용: $($applied.Count) / 저장소 파일: $($repoVersions.Count)"

if ($pending.Count -gt 0) {
    Write-Host "  ⏳ 보류: $($pending.Count)개" -ForegroundColor Yellow
    foreach ($v in $pending) { Write-Host "     $v`_$($repoVersions[$v]).sql" -ForegroundColor Yellow }
} else {
    Write-Host "  ✅ 보류 없음" -ForegroundColor Green
}

if ($failed.Count -gt 0) {
    Write-Host "  ❌ 실패 기록: $($failed.Count)개" -ForegroundColor Red
    foreach ($r in $failed) { Write-Host "     $($r.Version) $($r.Name)" -ForegroundColor Red }
}

if ($unknown.Count -gt 0) {
    # 예: 옛 마이그레이션 시스템이 남긴 기록. 결함은 아니므로 종료 코드에 반영하지 않는다.
    Write-Host "  ℹ️  저장소에 없는 기록: $(($unknown | ForEach-Object { "$($_.Version) $($_.Name)" }) -join ', ')" -ForegroundColor Gray
}
Write-Host ""

if ($pending.Count -gt 0 -or $failed.Count -gt 0) { exit 2 }
exit 0
