# ============================================================
# Invoke-AzRetry — 멱등 az 쓰기 명령을 일시 오류에 한해 다시 시도
# ============================================================
# az 의 secret/registry 설정은 같은 값을 다시 써도 결과가 같다(멱등). 그런데
# 관리 API 가 가끔 일시 오류로 실패해 배포 전체가 멈췄다(2026-10-09·10 각 1 회,
# 「Google client secret 설정 실패」). 이런 명령만 짧게 다시 시도한다 — 결과가
# 누적되는 명령(이미지 업데이트 등)에는 쓰지 않는다.
# ============================================================

function Invoke-AzRetry {
    param(
        [Parameter(Mandatory = $true)][scriptblock]$Command,
        [Parameter(Mandatory = $true)][string]$What,
        [int]$Attempts = 3,
        [int]$DelaySeconds = 10
    )
    for ($i = 1; $i -le $Attempts; $i++) {
        & $Command
        if ($LASTEXITCODE -eq 0) { return }
        if ($i -lt $Attempts) {
            Write-Host "   ⚠ $What 실패(시도 $i/$Attempts) — ${DelaySeconds}초 뒤 다시 시도" -ForegroundColor Yellow
            Start-Sleep -Seconds $DelaySeconds
        }
    }
    throw "$What 실패($Attempts 회 시도)"
}
