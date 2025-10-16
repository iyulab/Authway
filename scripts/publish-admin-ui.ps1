# ============================================================
# Authway Admin Dashboard 배포 스크립트
# ============================================================
# Azure Static Web Apps에 Admin Dashboard를 배포합니다.
# ============================================================

param(
    [string]$DeploymentToken
)

Write-Host "🚀 Authway Admin Dashboard 배포 시작..." -ForegroundColor Cyan
Write-Host ""

# 스크립트 디렉토리
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ScriptRoot = Split-Path -Parent $ScriptDir
$AdminPath = Join-Path $ScriptRoot "packages\web\admin-dashboard"

# .env 파일에서 환경변수 로드
$EnvFile = Join-Path $ScriptDir ".env"
if (Test-Path $EnvFile) {
    Write-Host "📄 .env 파일 로드 중..." -ForegroundColor Gray
    Get-Content $EnvFile | ForEach-Object {
        $pattern = '^([^#=]+)=(.*)$'
        if ($_ -match $pattern) {
            $name = $matches[1].Trim()
            $value = $matches[2].Trim()
            # 따옴표 제거
            $value = $value -replace '^["'']|["'']$', ''
            [Environment]::SetEnvironmentVariable($name, $value, "Process")
        }
    }
}

# 배포 토큰 우선순위: 파라미터 > 환경변수 > .env 파일
if (-not $DeploymentToken) {
    $DeploymentToken = $env:ADMIN_DEPLOYMENT_TOKEN
}

# 배포 토큰 확인
if (-not $DeploymentToken) {
    Write-Host "❌ 배포 토큰이 필요합니다." -ForegroundColor Red
    Write-Host ""
    Write-Host "다음 중 하나의 방법으로 토큰을 제공하세요:" -ForegroundColor Yellow
    Write-Host "  1. scripts\.env 파일: ADMIN_DEPLOYMENT_TOKEN=your-token" -ForegroundColor Gray
    Write-Host "  2. 환경변수: `$env:ADMIN_DEPLOYMENT_TOKEN = 'your-token'" -ForegroundColor Gray
    Write-Host "  3. 파라미터: .\publish-admin-ui.ps1 -DeploymentToken 'your-token'" -ForegroundColor Gray
    Write-Host ""
    Write-Host "배포 토큰 확인 방법:" -ForegroundColor Yellow
    Write-Host "  Azure Portal → Static Web Apps → authway-admin → Manage deployment token" -ForegroundColor Gray
    exit 1
}

# Admin Dashboard 디렉토리 확인
if (-not (Test-Path $AdminPath)) {
    Write-Host "❌ Admin Dashboard 디렉토리를 찾을 수 없습니다: $AdminPath" -ForegroundColor Red
    exit 1
}

# Admin Dashboard로 이동
Push-Location $AdminPath

try {
    # 의존성 확인
    Write-Host "📦 의존성 확인 중..." -ForegroundColor Yellow
    if (-not (Test-Path "node_modules")) {
        Write-Host "  node_modules가 없습니다. 설치 중..." -ForegroundColor Gray
        npm install
        if ($LASTEXITCODE -ne 0) {
            throw "npm install 실패"
        }
    }

    # .env.production 파일 확인
    if (-not (Test-Path ".env.production")) {
        Write-Host "⚠️  .env.production 파일이 없습니다." -ForegroundColor Yellow
        Write-Host "  기본 .env 파일을 사용합니다." -ForegroundColor Gray
    } else {
        Write-Host "✓ .env.production 파일 확인됨" -ForegroundColor Green
    }

    # 프로덕션 빌드
    Write-Host ""
    Write-Host "🔨 프로덕션 빌드 시작..." -ForegroundColor Yellow
    npm run build
    if ($LASTEXITCODE -ne 0) {
        throw "빌드 실패"
    }
    Write-Host "✓ 빌드 완료" -ForegroundColor Green

    # 빌드 결과 확인
    if (-not (Test-Path "dist")) {
        throw "빌드 디렉토리(dist)를 찾을 수 없습니다."
    }

    # Azure Static Web Apps 배포
    Write-Host ""
    Write-Host "☁️  Azure Static Web Apps에 배포 중..." -ForegroundColor Yellow
    Write-Host "  대상: https://authway-admin.iyulab.com" -ForegroundColor Gray

    npx @azure/static-web-apps-cli deploy ./dist `
        --env production `
        --deployment-token $DeploymentToken `
        --no-use-keychain

    if ($LASTEXITCODE -ne 0) {
        throw "Azure Static Web Apps 배포 실패"
    }

    Write-Host ""
    Write-Host "✅ Admin Dashboard 배포 완료!" -ForegroundColor Green
    Write-Host ""
    Write-Host "📌 접속 URL:" -ForegroundColor Cyan
    Write-Host "   https://authway-admin.iyulab.com" -ForegroundColor White
    Write-Host ""
    Write-Host "💡 팁: 브라우저에서 Ctrl+Shift+R로 캐시를 지우고 새로고침하세요." -ForegroundColor Yellow
    Write-Host ""

} catch {
    Write-Host ""
    Write-Host "❌ 배포 실패: $_" -ForegroundColor Red
    Write-Host ""
    exit 1
} finally {
    # 원래 디렉토리로 복귀
    Pop-Location
}
