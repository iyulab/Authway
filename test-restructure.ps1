# Authway 재구조화 검증 스크립트
# 새로운 디렉토리 구조가 제대로 작동하는지 테스트합니다

Write-Host "🧪 Authway 재구조화 검증 시작..." -ForegroundColor Cyan
Write-Host ""

$ErrorCount = 0
$WarningCount = 0

# 함수: 디렉토리 존재 확인
function Test-Directory {
    param([string]$Path, [string]$Name)

    if (Test-Path $Path) {
        Write-Host "✓ $Name 디렉토리 존재" -ForegroundColor Green
        return $true
    } else {
        Write-Host "❌ $Name 디렉토리 없음: $Path" -ForegroundColor Red
        $script:ErrorCount++
        return $false
    }
}

# 함수: 파일 존재 확인
function Test-File {
    param([string]$Path, [string]$Name)

    if (Test-Path $Path) {
        Write-Host "✓ $Name 파일 존재" -ForegroundColor Green
        return $true
    } else {
        Write-Host "❌ $Name 파일 없음: $Path" -ForegroundColor Red
        $script:ErrorCount++
        return $false
    }
}

# 함수: Go 빌드 테스트
function Test-GoBuild {
    param([string]$Path, [string]$Name)

    Write-Host "🔨 $Name Go 빌드 테스트 중..." -ForegroundColor Yellow
    Push-Location $Path

    try {
        $output = go build -o test.exe ./cmd/main.go 2>&1
        if ($LASTEXITCODE -eq 0 -and (Test-Path "test.exe")) {
            Write-Host "✓ $Name 빌드 성공" -ForegroundColor Green
            Remove-Item test.exe -ErrorAction SilentlyContinue
            Pop-Location
            return $true
        } else {
            Write-Host "❌ $Name 빌드 실패" -ForegroundColor Red
            Write-Host "   에러: $output" -ForegroundColor Gray
            $script:ErrorCount++
            Pop-Location
            return $false
        }
    } catch {
        Write-Host "❌ $Name 빌드 실패: $($_.Exception.Message)" -ForegroundColor Red
        $script:ErrorCount++
        Pop-Location
        return $false
    }
}

# 1. 새 디렉토리 구조 확인
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  1️⃣ 새 디렉토리 구조 확인" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

Test-Directory "apps\central\api" "Central API"
Test-Directory "apps\central\admin" "Central Admin"
Test-Directory "apps\branding\auth-api" "Branding Auth API"

Write-Host ""

# 2. 중요 파일 존재 확인
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  2️⃣ 중요 파일 존재 확인" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

Test-File "apps\central\api\cmd\main.go" "Central API main.go"
Test-File "apps\central\api\go.mod" "Central API go.mod (루트 공유)"
Test-File "apps\central\admin\package.json" "Admin package.json"
Test-File "apps\branding\auth-api\cmd\main.go" "Auth API main.go"
Test-File "apps\branding\auth-api\go.mod" "Auth API go.mod"

Write-Host ""

# 3. Go 모듈 경로 확인
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  3️⃣ Go 모듈 경로 확인" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

# Auth API go.mod 확인
$authApiGoMod = Get-Content "apps\branding\auth-api\go.mod" -Raw
if ($authApiGoMod -match "module authway/apps/branding/auth-api") {
    Write-Host "✓ Auth API go.mod 모듈명 올바름" -ForegroundColor Green
} else {
    Write-Host "❌ Auth API go.mod 모듈명 잘못됨" -ForegroundColor Red
    $ErrorCount++
}

# Central API import 경로 확인
$centralApiMain = Get-Content "apps\central\api\cmd\main.go" -Raw
if ($centralApiMain -match "authway/apps/central/api") {
    Write-Host "✓ Central API import 경로 올바름" -ForegroundColor Green
} else {
    Write-Host "❌ Central API import 경로 잘못됨 (authway/apps/central/api 없음)" -ForegroundColor Red
    $ErrorCount++
}

# Auth API import 경로 확인
$authApiMain = Get-Content "apps\branding\auth-api\cmd\main.go" -Raw
if ($authApiMain -match "authway/apps/branding/auth-api") {
    Write-Host "✓ Auth API import 경로 올바름" -ForegroundColor Green
} else {
    Write-Host "❌ Auth API import 경로 잘못됨 (authway/apps/branding/auth-api 없음)" -ForegroundColor Red
    $ErrorCount++
}

Write-Host ""

# 4. Go 빌드 테스트
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  4️⃣ Go 빌드 테스트" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

Test-GoBuild "apps\central\api" "Central API"
Test-GoBuild "apps\branding\auth-api" "Auth API"

Write-Host ""

# 5. 스크립트 경로 확인
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  5️⃣ 스크립트 경로 확인" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

# start-dev.ps1 경로 확인
$startDev = Get-Content "start-dev.ps1" -Raw
if ($startDev -match 'apps\\central\\api') {
    Write-Host "✓ start-dev.ps1 Central API 경로 올바름" -ForegroundColor Green
} else {
    Write-Host "❌ start-dev.ps1 Central API 경로 잘못됨" -ForegroundColor Red
    $ErrorCount++
}

if ($startDev -match 'apps\\central\\admin') {
    Write-Host "✓ start-dev.ps1 Admin 경로 올바름" -ForegroundColor Green
} else {
    Write-Host "❌ start-dev.ps1 Admin 경로 잘못됨" -ForegroundColor Red
    $ErrorCount++
}

if ($startDev -match 'apps\\branding\\auth-api') {
    Write-Host "✓ start-dev.ps1 Auth API 경로 올바름" -ForegroundColor Green
} else {
    Write-Host "❌ start-dev.ps1 Auth API 경로 잘못됨" -ForegroundColor Red
    $ErrorCount++
}

Write-Host ""

# 6. 오래된 디렉토리 확인
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  6️⃣ 오래된 디렉토리 확인 (정리 대기)" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

if (Test-Path "src\server") {
    Write-Host "⚠️  src\server 디렉토리 존재 (정리 필요)" -ForegroundColor Yellow
    $WarningCount++
} else {
    Write-Host "✓ src\server 디렉토리 이미 정리됨" -ForegroundColor Green
}

if (Test-Path "packages\web") {
    Write-Host "⚠️  packages\web 디렉토리 존재 (정리 필요)" -ForegroundColor Yellow
    $WarningCount++
} else {
    Write-Host "✓ packages\web 디렉토리 이미 정리됨" -ForegroundColor Green
}

if (Test-Path "apps\auth-backend") {
    Write-Host "⚠️  apps\auth-backend 디렉토리 존재 (정리 필요)" -ForegroundColor Yellow
    $WarningCount++
} else {
    Write-Host "✓ apps\auth-backend 디렉토리 이미 정리됨" -ForegroundColor Green
}

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  📊 검증 결과" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

if ($ErrorCount -eq 0 -and $WarningCount -eq 0) {
    Write-Host "✅ 모든 검증 통과!" -ForegroundColor Green
    Write-Host "   재구조화가 성공적으로 완료되었습니다." -ForegroundColor Green
    Write-Host ""
} elseif ($ErrorCount -eq 0) {
    Write-Host "⚠️  경고 $WarningCount 개" -ForegroundColor Yellow
    Write-Host "   재구조화는 성공했지만, 오래된 디렉토리를 정리해야 합니다." -ForegroundColor Yellow
    Write-Host ""
    Write-Host "📝 정리 명령어:" -ForegroundColor Cyan
    Write-Host "   rm -r src\server" -ForegroundColor Gray
    Write-Host "   rm -r packages\web" -ForegroundColor Gray
    Write-Host "   rm -r apps\auth-backend" -ForegroundColor Gray
    Write-Host ""
} else {
    Write-Host "❌ 에러 $ErrorCount 개, 경고 $WarningCount 개" -ForegroundColor Red
    Write-Host "   위의 에러를 수정해야 합니다." -ForegroundColor Red
    Write-Host ""
    exit 1
}

Write-Host "🎯 다음 단계:" -ForegroundColor Cyan
Write-Host "   1. 실제 개발 환경 테스트: .\start-dev.ps1" -ForegroundColor White
Write-Host "   2. ASP.NET Sample 테스트: cd samples\asp-sample && .\setup-client-local.ps1" -ForegroundColor White
Write-Host "   3. 빌드 확인 후 오래된 디렉토리 정리" -ForegroundColor White
Write-Host ""
