# ============================================================
# Authway 배포 환경 변수 로더 (공유 헬퍼)
# ============================================================
# 경로 규약 (2026-04-15 재구성):
#   scripts/deploy/
#   ├── _shared/           ← 이 파일 위치
#   │   ├── load-env.ps1
#   │   └── lib/*.core.ps1
#   ├── prod/.env          ← Target=prod → 여기서 로드
#   └── staging/.env       ← Target=staging → 여기서 로드
#
# 이전 규약 (deprecated):
#   scripts/deploy/.env         (→ prod/.env)
#   scripts/deploy/.env.staging (→ staging/.env)
#
# 사용:
#   . (Join-Path $SharedDir "load-env.ps1")
#   $envVars = Get-DeployEnv -Target "prod"
#
# -Target 값:
#   - "" / "prod"    → prod/.env
#   - "staging"      → staging/.env
#   - "<name>"       → <name>/.env (canary 등 확장용. 디렉터리 생성 필요)
# ============================================================

function Get-DeployEnv {
    param(
        [Parameter(Mandatory = $false)]
        [string]$Target = ""
    )

    $SharedDir = $PSScriptRoot
    if (-not $SharedDir) { $SharedDir = Split-Path -Parent $MyInvocation.MyCommand.Path }
    $DeployRoot = Split-Path -Parent $SharedDir

    $TargetName = if ([string]::IsNullOrWhiteSpace($Target)) { "prod" } else { $Target }
    $TargetDir = Join-Path $DeployRoot $TargetName
    $EnvFile = Join-Path $TargetDir ".env"

    if (-not (Test-Path $EnvFile)) {
        Write-Host "❌ 환경 변수 파일을 찾을 수 없습니다: $EnvFile" -ForegroundColor Red
        $ExampleFile = Join-Path $TargetDir ".env.example"
        if (Test-Path $ExampleFile) {
            Write-Host "   템플릿 참고: $ExampleFile → $EnvFile 으로 복사 후 값을 채우세요." -ForegroundColor Yellow
        } else {
            Write-Host "   디렉터리/템플릿 누락: $TargetDir" -ForegroundColor Yellow
        }
        throw "env file missing: $EnvFile"
    }

    Write-Host "📋 환경 변수 로드 중: $TargetName/.env" -ForegroundColor Yellow

    $envVars = @{}
    Get-Content $EnvFile | ForEach-Object {
        if ($_ -match '^([^#][^=]+)=(.+)$') {
            $envVars[$matches[1].Trim()] = $matches[2].Trim()
        }
    }

    $envVars['__TARGET__'] = $TargetName
    $envVars['__ENV_FILE__'] = $EnvFile
    $envVars['__TARGET_DIR__'] = $TargetDir

    return $envVars
}

# ============================================================
# Preflight: required secrets + placeholder 거부
# ============================================================
# placeholder 패턴 (`your-`, `changeme`, `<`, `TODO`)이 하나라도 남아있으면
# fail-closed (throw). 2026-04 prod 사고 (ADMIN_API_KEY 미주입)의 재발 방지.
# ============================================================
function Test-DeploySecrets {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable]$EnvVars,

        [Parameter(Mandatory = $true)]
        [string[]]$RequiredKeys
    )

    $missing = @()
    foreach ($k in $RequiredKeys) {
        $v = $EnvVars[$k]
        if (-not $v -or $v -match '^(your-|changeme|<|TODO)') {
            $missing += $k
        }
    }
    if ($missing.Count -gt 0) {
        Write-Host "❌ 필수 secret 누락/플레이스홀더: $($missing -join ', ')" -ForegroundColor Red
        Write-Host "   $($EnvVars['__ENV_FILE__']) 을 채워야 합니다." -ForegroundColor Yellow
        Write-Host "   키 생성: openssl rand -base64 48" -ForegroundColor Yellow
        throw "missing required secrets: $($missing -join ', ')"
    }
}

# ============================================================
# Preflight: Azure subscription pinning
# ============================================================
# .env의 AZURE_SUBSCRIPTION_ID 로 항상 고정. 다계정 개발자 환경에서
# staging 배포가 실수로 prod 구독에 리소스를 만드는 것을 방지.
# ============================================================
function Set-AzureSubscription {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable]$EnvVars
    )

    $AZURE_SUBSCRIPTION_ID = $EnvVars['AZURE_SUBSCRIPTION_ID']
    if (-not $AZURE_SUBSCRIPTION_ID -or $AZURE_SUBSCRIPTION_ID -match '^(your-|changeme|<|TODO)') {
        Write-Host "❌ AZURE_SUBSCRIPTION_ID 누락/플레이스홀더" -ForegroundColor Red
        Write-Host "   확인: az account list --query '[].{name:name,id:id}' -o table" -ForegroundColor Yellow
        throw "AZURE_SUBSCRIPTION_ID not set"
    }

    Write-Host "🔧 Azure 구독 고정 중: $AZURE_SUBSCRIPTION_ID ($($EnvVars['__TARGET__']))" -ForegroundColor Yellow
    az account set -s $AZURE_SUBSCRIPTION_ID
    if ($LASTEXITCODE -ne 0) {
        throw "az account set failed — subscription ID invalid or 'az login' required"
    }

    $activeSub = az account show --query id -o tsv 2>$null
    if ($activeSub -ne $AZURE_SUBSCRIPTION_ID) {
        throw "active subscription mismatch: expected=$AZURE_SUBSCRIPTION_ID actual=$activeSub"
    }
}
