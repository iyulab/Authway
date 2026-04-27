# ============================================================
# Authway 전체 배포 코어 로직 (타겟 무관)
# ============================================================
# 모든 서비스를 순차적으로 배포하고 헬스 체크 수행.
# 내부적으로 publish-*.core.ps1 들을 -Target 전달하여 호출.
# ============================================================

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("prod", "staging")]
    [string]$Target,

    [switch]$SkipBuild,
    [switch]$SkipHealthCheck,
    [switch]$SkipMigration,
    [switch]$ForceMigration,
    [string[]]$Services = @("hydra", "api", "auth-api", "admin", "auth-ui")
)

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🚀 Authway 전체 배포 시작 (target=$Target)" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

$LibDir = $PSScriptRoot
$SharedDir = Split-Path -Parent $LibDir
$StartTime = Get-Date

. (Join-Path $SharedDir "load-env.ps1")

try {
    $envVars = Get-DeployEnv -Target $Target
} catch {
    Write-Host "❌ env 로드 실패: $_" -ForegroundColor Red
    exit 1
}

$deploymentResults = @{}
$totalServices = $Services.Count
$successCount = 0
$failCount = 0

function Deploy-Service {
    param(
        [string]$ServiceName,
        [string]$ScriptPath,
        [hashtable]$Params = @{}
    )

    Write-Host ""
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Yellow
    Write-Host "  📦 $ServiceName 배포 중... ($($successCount + $failCount + 1)/$totalServices)" -ForegroundColor Yellow
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Yellow
    Write-Host ""

    $serviceStart = Get-Date

    try {
        # splat core script with -Target + pass-through params
        $invokeParams = @{ Target = $Target }
        foreach ($k in $Params.Keys) { $invokeParams[$k] = $Params[$k] }
        & $ScriptPath @invokeParams

        if ($LASTEXITCODE -eq 0) {
            $duration = [math]::Round(((Get-Date) - $serviceStart).TotalSeconds, 1)
            Write-Host "✅ $ServiceName 배포 완료! (소요시간: $duration 초)" -ForegroundColor Green
            $script:successCount++
            $deploymentResults[$ServiceName] = @{
                Status = "Success"
                Duration = $duration
            }
            return $true
        } else {
            throw "배포 스크립트 실행 실패 (exit=$LASTEXITCODE)"
        }
    } catch {
        $duration = [math]::Round(((Get-Date) - $serviceStart).TotalSeconds, 1)
        Write-Host ""
        Write-Host "❌ $ServiceName 배포 실패: $_" -ForegroundColor Red
        $script:failCount++
        $deploymentResults[$ServiceName] = @{
            Status = "Failed"
            Duration = $duration
            Error = $_.ToString()
        }
        return $false
    }
}

function Test-HealthEndpoint {
    param([string]$Name, [string]$Url)
    Write-Host "  🔍 $Name 헬스 체크: " -NoNewline -ForegroundColor Yellow
    try {
        $response = Invoke-WebRequest -Uri $Url -Method Get -TimeoutSec 10 -UseBasicParsing
        if ($response.StatusCode -eq 200) {
            Write-Host "✓" -ForegroundColor Green
            return $true
        } else {
            Write-Host "❌ (Status: $($response.StatusCode))" -ForegroundColor Red
            return $false
        }
    } catch {
        Write-Host "❌ (Error: $_)" -ForegroundColor Red
        return $false
    }
}

function Test-AdminAuthSmoke {
    param([string]$ApiUrl)
    Write-Host "  🔒 Central API adminAuth smoke: " -NoNewline -ForegroundColor Yellow
    $endpoints = @(
        @{Method='GET';  Path='/api/v1/clients'},
        @{Method='GET';  Path='/api/v1/users'},
        @{Method='PUT';  Path='/api/v1/clients/00000000-0000-0000-0000-000000000000'; Body='{}'},
        @{Method='POST'; Path='/api/v1/clients/00000000-0000-0000-0000-000000000000/regenerate-secret'}
    )
    $failed = @()
    foreach ($ep in $endpoints) {
        try {
            $params = @{
                Uri = "$ApiUrl$($ep.Path)"
                Method = $ep.Method
                TimeoutSec = 10
                SkipHttpErrorCheck = $true
                ErrorAction = 'Stop'
            }
            if ($ep.Body) { $params.Body = $ep.Body; $params.ContentType = 'application/json' }
            $resp = Invoke-WebRequest @params
            $code = [int]$resp.StatusCode
        } catch {
            $code = 0
            if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        }
        if ($code -ne 401 -and $code -ne 503) {
            $failed += "$($ep.Method) $($ep.Path)=$code"
        }
    }
    if ($failed.Count -eq 0) {
        Write-Host "✓ (모든 admin 라우트 401/503)" -ForegroundColor Green
        return $true
    } else {
        Write-Host "❌ ($($failed -join ', '))" -ForegroundColor Red
        return $false
    }
}

try {
    # ============================================================
    # 0. DB 마이그레이션
    # ============================================================
    if (-not $SkipMigration) {
        Write-Host ""
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
        Write-Host "  🗄️  데이터베이스 마이그레이션" -ForegroundColor Magenta
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
        Write-Host ""

        $migrationHelperPath = Join-Path $SharedDir "migration-helpers.ps1"
        if (Test-Path $migrationHelperPath) {
            . $migrationHelperPath

            $migrationResult = Invoke-AutoMigration `
                -EnvVars $envVars `
                -ScriptDir $SharedDir `
                -Force:$ForceMigration

            if ($migrationResult.Skipped) {
                if ($migrationResult.Applied -eq 0) {
                    Write-Verbose "마이그레이션 확인 완료 (보류 없음)"
                } else {
                    Write-Host "⏭️  마이그레이션 스킵: $($migrationResult.Message)" -ForegroundColor Yellow
                }
                Write-Host ""
            } elseif (-not $migrationResult.Success) {
                Write-Host ""
                Write-Host "❌ 마이그레이션 실패!" -ForegroundColor Red
                Write-Host ""
                $continue = Read-Host "배포를 계속하시겠습니까? (yes/no)"
                if ($continue -ne "yes" -and $continue -ne "y") {
                    exit 1
                }
                Write-Host ""
            } else {
                Write-Host ""
                Write-Host "✅ 마이그레이션 완료: $($migrationResult.Applied)개 적용" -ForegroundColor Green
                Write-Host ""
            }
        } else {
            Write-Warning "⚠️  마이그레이션 헬퍼를 찾을 수 없습니다: $migrationHelperPath"
            Write-Host ""
        }
    } else {
        Write-Host "⏭️  마이그레이션을 건너뜁니다 (-SkipMigration)" -ForegroundColor Yellow
        Write-Host ""
    }

    # ============================================================
    # 1. Container Apps 배포 (의존성 순서대로)
    # ============================================================
    if ($Services -contains "hydra") {
        Deploy-Service -ServiceName "Hydra (OAuth Server)" `
            -ScriptPath (Join-Path $LibDir "publish-hydra.core.ps1")
    }

    if ($Services -contains "api") {
        $params = @{}
        if ($SkipBuild) { $params["SkipBuild"] = $true }
        Deploy-Service -ServiceName "Central API" `
            -ScriptPath (Join-Path $LibDir "publish-api.core.ps1") `
            -Params $params
    }

    if ($Services -contains "auth-api") {
        $params = @{}
        if ($SkipBuild) { $params["SkipBuild"] = $true }
        Deploy-Service -ServiceName "Auth Backend" `
            -ScriptPath (Join-Path $LibDir "publish-auth-api.core.ps1") `
            -Params $params
    }

    # ============================================================
    # 2. Static Web Apps 배포
    # ============================================================
    if ($Services -contains "admin") {
        Deploy-Service -ServiceName "Admin Dashboard" `
            -ScriptPath (Join-Path $LibDir "publish-admin.core.ps1")
    }

    if ($Services -contains "auth-ui") {
        Deploy-Service -ServiceName "Auth UI" `
            -ScriptPath (Join-Path $LibDir "publish-auth-ui.core.ps1")
    }

    # ============================================================
    # 3. Hydra 클라이언트 동기화
    # ============================================================
    if ($Services -contains "api" -and $deploymentResults["Central API"].Status -eq "Success") {
        Write-Host ""
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
        Write-Host "  🔄 Hydra 클라이언트 동기화" -ForegroundColor Magenta
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Magenta
        Write-Host ""
        Write-Host "⏳ API 서버 시작 대기 (15초)..." -ForegroundColor Yellow
        Start-Sleep -Seconds 15

        try {
            $syncUrl = "$($envVars['API_URL'])/api/v1/clients/sync-hydra"
            Write-Host "📡 동기화 요청: $syncUrl" -ForegroundColor Gray

            $syncResponse = Invoke-RestMethod -Uri $syncUrl -Method Post -ContentType "application/json" -TimeoutSec 60

            Write-Host ""
            Write-Host "✅ Hydra 동기화 완료!" -ForegroundColor Green
            Write-Host "   - 동기화 성공: $($syncResponse.synced) 클라이언트" -ForegroundColor Gray
            Write-Host "   - 동기화 실패: $($syncResponse.failed) 클라이언트" -ForegroundColor Gray
            Write-Host ""
        } catch {
            Write-Host ""
            Write-Host "⚠️  Hydra 동기화 실패 (수동으로 실행 가능): $_" -ForegroundColor Yellow
            Write-Host "   curl -X POST $syncUrl" -ForegroundColor Gray
            Write-Host ""
        }
    }

    # ============================================================
    # 4. 헬스 체크
    # ============================================================
    if (-not $SkipHealthCheck -and $successCount -gt 0) {
        Write-Host ""
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
        Write-Host "  🏥 헬스 체크 수행" -ForegroundColor Cyan
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
        Write-Host ""
        Write-Host "⏳ Container Apps가 시작될 때까지 30초 대기..." -ForegroundColor Yellow
        Start-Sleep -Seconds 30
        Write-Host ""

        $healthResults = @()

        if ($Services -contains "hydra") {
            $healthResults += Test-HealthEndpoint `
                -Name "Hydra OIDC" `
                -Url "$($envVars['HYDRA_ISSUER'])/.well-known/openid-configuration"
        }

        if ($Services -contains "api") {
            $healthResults += Test-HealthEndpoint -Name "Central API" -Url "$($envVars['API_URL'])/health"
            $healthResults += Test-AdminAuthSmoke -ApiUrl $envVars['API_URL']
        }

        if ($Services -contains "auth-api") {
            $healthResults += Test-HealthEndpoint -Name "Auth Backend" -Url "$($envVars['AUTH_API_URL'])/health"
        }

        if ($Services -contains "admin") {
            $healthResults += Test-HealthEndpoint -Name "Admin Dashboard" -Url $envVars['ADMIN_URL']
        }

        if ($Services -contains "auth-ui") {
            $healthResults += Test-HealthEndpoint -Name "Auth UI" -Url $envVars['LOGIN_URL']
        }

        $healthyCount = ($healthResults | Where-Object { $_ -eq $true }).Count
        Write-Host ""
        Write-Host "  헬스 체크 결과: $healthyCount/$($healthResults.Count) 정상" -ForegroundColor $(if ($healthyCount -eq $healthResults.Count) { "Green" } else { "Yellow" })
    }

    # ============================================================
    # 5. 배포 smoke — audit_logs 자동 검증 (staging/prod 공통)
    # ============================================================
    if (-not $SkipHealthCheck -and $successCount -gt 0 -and ($Services -contains "api")) {
        Write-Host ""
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
        Write-Host "  🧾 Audit 로그 smoke" -ForegroundColor Cyan
        Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
        Write-Host ""

        $smokeAuditPath = Join-Path $SharedDir "smoke-audit.ps1"
        if (Test-Path $smokeAuditPath) {
            & $smokeAuditPath -Target $Target -WindowMinutes 10
            if ($LASTEXITCODE -ne 0) {
                Write-Host "⚠️  audit smoke 실패 — audit 배선 회귀 의심" -ForegroundColor Yellow
            }
        } else {
            Write-Host "⏭️  smoke-audit.ps1 미존재 — 건너뜀" -ForegroundColor Gray
        }
    }

    # ============================================================
    # 6. 요약
    # ============================================================
    $totalDuration = [math]::Round(((Get-Date) - $StartTime).TotalMinutes, 1)

    Write-Host ""
    Write-Host "═══════════════════════════════════════════" -ForegroundColor $(if ($failCount -eq 0) { "Green" } else { "Yellow" })
    Write-Host "  📊 배포 완료 요약 (target=$Target)" -ForegroundColor $(if ($failCount -eq 0) { "Green" } else { "Yellow" })
    Write-Host "═══════════════════════════════════════════" -ForegroundColor $(if ($failCount -eq 0) { "Green" } else { "Yellow" })
    Write-Host ""
    Write-Host "  총 배포 시간: $totalDuration 분" -ForegroundColor White
    Write-Host "  성공: $successCount / 실패: $failCount / 전체: $totalServices" -ForegroundColor White
    Write-Host ""

    Write-Host "  📦 서비스별 결과:" -ForegroundColor Cyan
    foreach ($service in $deploymentResults.Keys) {
        $result = $deploymentResults[$service]
        $statusIcon = if ($result.Status -eq "Success") { "✅" } else { "❌" }
        $statusColor = if ($result.Status -eq "Success") { "Green" } else { "Red" }
        Write-Host "    $statusIcon $service : " -NoNewline -ForegroundColor $statusColor
        Write-Host "$($result.Status) ($($result.Duration)초)" -ForegroundColor Gray
        if ($result.Error) {
            Write-Host "       오류: $($result.Error)" -ForegroundColor Red
        }
    }

    Write-Host ""
    Write-Host "  🌐 서비스 URL:" -ForegroundColor Cyan
    Write-Host "    - Admin Dashboard: $($envVars['ADMIN_URL'])" -ForegroundColor Gray
    Write-Host "    - Auth UI: $($envVars['LOGIN_URL'])" -ForegroundColor Gray
    Write-Host "    - Hydra (OAuth): $($envVars['HYDRA_ISSUER'])" -ForegroundColor Gray
    Write-Host "    - Central API: $($envVars['API_URL'])" -ForegroundColor Gray
    Write-Host "    - Auth Backend: $($envVars['AUTH_API_URL'])" -ForegroundColor Gray
    Write-Host ""

    if ($failCount -eq 0) {
        Write-Host "  ✅ 모든 서비스가 성공적으로 배포되었습니다!" -ForegroundColor Green
        Write-Host ""
        exit 0
    } else {
        Write-Host "  ⚠️  일부 서비스 배포에 실패했습니다." -ForegroundColor Yellow
        Write-Host "     실패한 서비스를 개별적으로 재배포해주세요." -ForegroundColor Yellow
        Write-Host ""
        exit 1
    }

} catch {
    Write-Host ""
    Write-Host "❌ 배포 프로세스 오류: $_" -ForegroundColor Red
    Write-Host ""
    Write-Host "스택 트레이스:" -ForegroundColor Yellow
    Write-Host $_.ScriptStackTrace -ForegroundColor Gray
    exit 1
}
