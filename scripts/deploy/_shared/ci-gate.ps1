# ============================================================
# 배포 전 CI 확인 — 빨간 CI 위에서는 배포하지 않는다
# ============================================================
# 2026-08 에 red CI 위에서 배포가 나간 적이 있다(로드맵 «배포가 CI 상태를 게이트로»).
# 이 검사는 «막는 것»을 하나로 좁힌다: 배포하려는 커밋(HEAD)의 CI 가 **실패**로 끝났으면 거절.
# 아직 push 하지 않은 커밋(CI 기록 없음)·진행 중인 CI 는 막지 않고 말만 한다 —
# staging 은 push 전 로컬 커밋으로도 배포하고, prod 는 staging 에서 검증된 이미지로 나간다.
# 끄기: 환경 변수 AUTHWAY_SKIP_CI_CHECK=1 (이유를 배포 기록에 남길 것).
# 사용: . (Join-Path $SharedDir "ci-gate.ps1"); Assert-CiNotRed -ProjectRoot $ProjectRoot
# ============================================================

# 실행 목록 → 평결. green: 전부 성공(건너뜀 포함) · red: 하나라도 실패·시간 초과 ·
# pending: 끝나지 않은 것이 있음 · unknown: 기록 없음.
function Get-CiVerdictFromRuns {
    param([object[]]$Runs)

    # Dependabot's own runs (event "dynamic") attach to the default branch's head
    # commit too; they say nothing about whether the code builds.
    $Runs = @($Runs | Where-Object { $_ -and $_.event -ne 'dynamic' })
    if ($Runs.Count -eq 0) { return 'unknown' }

    $failed = @($Runs | Where-Object { $_.status -eq 'completed' -and $_.conclusion -in @('failure', 'timed_out', 'startup_failure') })
    if ($failed.Count -gt 0) { return 'red' }

    $open = @($Runs | Where-Object { $_.status -ne 'completed' })
    if ($open.Count -gt 0) { return 'pending' }

    return 'green'
}

function Get-CiVerdict {
    param(
        [Parameter(Mandatory = $true)][string]$Sha,
        [string]$Repo = 'iyulab/Authway'
    )

    if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { return 'unknown' }
    $json = & gh run list --repo $Repo --commit $Sha --json status,conclusion,workflowName,event --limit 50 2>$null
    if ($LASTEXITCODE -ne 0 -or -not $json) { return 'unknown' }
    try { $runs = $json | ConvertFrom-Json } catch { return 'unknown' }
    return Get-CiVerdictFromRuns -Runs $runs
}

function Assert-CiNotRed {
    param(
        [Parameter(Mandatory = $true)][string]$ProjectRoot,
        [string]$Repo = 'iyulab/Authway'
    )

    if ($env:AUTHWAY_SKIP_CI_CHECK -eq '1') {
        Write-Host "⚠️  CI 확인을 건너뜁니다 (AUTHWAY_SKIP_CI_CHECK=1)" -ForegroundColor Yellow
        return
    }

    $sha = (& git -C $ProjectRoot rev-parse HEAD 2>$null)
    if ($LASTEXITCODE -ne 0 -or -not $sha) {
        Write-Host "⚠️  HEAD 커밋을 읽지 못해 CI 확인을 건너뜁니다" -ForegroundColor Yellow
        return
    }
    $sha = $sha.Trim()
    $short = $sha.Substring(0, 7)

    switch (Get-CiVerdict -Sha $sha -Repo $Repo) {
        'green'   { Write-Host "✓ CI 초록 ($short)" -ForegroundColor Green }
        'pending' { Write-Host "⚠️  $short 의 CI 가 아직 진행 중입니다 — 끝난 뒤의 결과는 확인되지 않았습니다" -ForegroundColor Yellow }
        'unknown' { Write-Host "⚠️  $short 의 CI 기록이 없습니다 (push 전 커밋이거나 gh 를 쓸 수 없음)" -ForegroundColor Yellow }
        'red'     {
            throw "CI 가 빨간 커밋($short) 위에서는 배포하지 않습니다. 고친 뒤 다시 하거나, 알고 넘어가려면 AUTHWAY_SKIP_CI_CHECK=1 (사유를 기록)."
        }
    }
}
