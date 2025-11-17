# 자동 마이그레이션 시스템 독립 테스트
# 이 스크립트는 migration-helpers.ps1의 기능을 독립적으로 테스트합니다

$ErrorActionPreference = "Stop"

Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🧪 자동 마이그레이션 시스템 테스트" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

# 프로젝트 루트 디렉토리 (scripts/test에서 두 단계 위)
$ProjectRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$ScriptDir = Join-Path $ProjectRoot "scripts\deploy"
$MigrationsDir = Join-Path $ProjectRoot "migrations"

# 환경 변수 로드
$EnvFile = Join-Path $ScriptDir ".env"
Write-Host "📋 환경 변수 로드 중..." -ForegroundColor Yellow
Write-Host "   파일: $EnvFile" -ForegroundColor Gray

if (-not (Test-Path $EnvFile)) {
    Write-Host "❌ .env 파일을 찾을 수 없습니다!" -ForegroundColor Red
    exit 1
}

$envVars = @{}
Get-Content $EnvFile | ForEach-Object {
    if ($_ -match '^([^#][^=]+)=(.+)$') {
        $envVars[$matches[1].Trim()] = $matches[2].Trim()
    }
}

Write-Host "   ✅ 환경 변수 로드 완료: $($envVars.Keys.Count)개" -ForegroundColor Green
Write-Host ""

# migration-helpers.ps1 로드
$HelperPath = Join-Path $ScriptDir "migration-helpers.ps1"
Write-Host "📥 Helper 모듈 로드 중..." -ForegroundColor Yellow
Write-Host "   파일: $HelperPath" -ForegroundColor Gray

if (-not (Test-Path $HelperPath)) {
    Write-Host "❌ migration-helpers.ps1을 찾을 수 없습니다!" -ForegroundColor Red
    exit 1
}

. $HelperPath
Write-Host "   ✅ Helper 모듈 로드 완료" -ForegroundColor Green
Write-Host ""

# psql 초기화 테스트
Write-Host "🔧 psql 초기화 테스트" -ForegroundColor Yellow
$psqlInitResult = Initialize-PsqlPath
Write-Host "   결과: $(if ($psqlInitResult) { '✅ 성공' } else { '❌ 실패' })" -ForegroundColor $(if ($psqlInitResult) { 'Green' } else { 'Red' })
if ($psqlInitResult) {
    Write-Host "   psql 경로: $script:PsqlExecutable" -ForegroundColor Gray
}
Write-Host ""

# Get-PendingMigrations 테스트
Write-Host "🔍 Get-PendingMigrations 테스트" -ForegroundColor Yellow
Write-Host "   환경 변수: $($envVars.Keys.Count)개" -ForegroundColor Gray
Write-Host "   마이그레이션 디렉토리: $MigrationsDir" -ForegroundColor Gray
Write-Host ""

try {
    $result = Get-PendingMigrations -EnvVars $envVars -MigrationsDir $MigrationsDir

    Write-Host ""
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
    Write-Host "  ✅ 함수 실행 완료" -ForegroundColor Green
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
    Write-Host ""

    # 반환값 상세 분석
    Write-Host "📊 반환값 분석:" -ForegroundColor Cyan
    Write-Host "   타입: $($result.GetType().FullName)" -ForegroundColor Gray
    Write-Host "   null 체크: $(if ($null -eq $result) { '❌ NULL' } else { '✅ NOT NULL' })" -ForegroundColor Gray

    if ($null -ne $result) {
        Write-Host "   IsArray: $(if ($result -is [Array]) { 'Yes' } else { 'No' })" -ForegroundColor Gray
        Write-Host "   Count: $($result.Count)" -ForegroundColor Gray
        Write-Host "   Length: $($result.Length)" -ForegroundColor Gray

        if ($result.Count -gt 0) {
            Write-Host ""
            Write-Host "📋 보류 중인 마이그레이션:" -ForegroundColor Yellow
            foreach ($migration in $result) {
                Write-Host "   • $($migration.Name) (버전: $($migration.Version))" -ForegroundColor Cyan
            }
        } else {
            Write-Host ""
            Write-Host "✅ 보류 중인 마이그레이션 없음" -ForegroundColor Green
        }
    } else {
        Write-Host "   ❌ 함수가 NULL을 반환했습니다!" -ForegroundColor Red
    }

} catch {
    Write-Host ""
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Red
    Write-Host "  ❌ 오류 발생" -ForegroundColor Red
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Red
    Write-Host ""
    Write-Host "오류 메시지: $_" -ForegroundColor Red
    Write-Host "오류 타입: $($_.Exception.GetType().FullName)" -ForegroundColor Red
    Write-Host ""
    Write-Host "스택 트레이스:" -ForegroundColor Yellow
    Write-Host $_.ScriptStackTrace -ForegroundColor Gray
}

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🏁 테스트 완료" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
