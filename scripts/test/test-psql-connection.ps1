# psql 직접 연결 테스트
$ProjectRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$ScriptDir = Join-Path $ProjectRoot "scripts\deploy"
$EnvFile = Join-Path $ScriptDir ".env"

# 환경 변수 로드
$envVars = @{}
Get-Content $EnvFile | ForEach-Object {
    if ($_ -match '^([^#][^=]+)=(.+)$') {
        $envVars[$matches[1].Trim()] = $matches[2].Trim()
    }
}

Write-Host "=== psql 연결 테스트 ===" -ForegroundColor Cyan
Write-Host ""

# PostgreSQL 환경 변수 설정
$env:PGHOST = $envVars['AUTHWAY_DATABASE_HOST']
$env:PGPORT = "5432"
$env:PGDATABASE = $envVars['AUTHWAY_DATABASE_NAME']
$env:PGUSER = $envVars['AUTHWAY_DATABASE_USER']
$env:PGPASSWORD = $envVars['AUTHWAY_DATABASE_PASSWORD']
$env:PGSSLMODE = "require"

Write-Host "연결 정보:" -ForegroundColor Yellow
Write-Host "  Host: $env:PGHOST" -ForegroundColor Gray
Write-Host "  Database: $env:PGDATABASE" -ForegroundColor Gray
Write-Host "  User: $env:PGUSER" -ForegroundColor Gray
Write-Host "  SSL Mode: $env:PGSSLMODE" -ForegroundColor Gray
Write-Host ""

# 간단한 쿼리 실행
Write-Host "쿼리 실행 중: SELECT 1" -ForegroundColor Yellow

try {
    $result = psql -t -A -c "SELECT 1;" 2>&1
    $exitCode = $LASTEXITCODE

    Write-Host ""
    Write-Host "Exit Code: $exitCode" -ForegroundColor $(if ($exitCode -eq 0) { "Green" } else { "Red" })
    Write-Host "결과:" -ForegroundColor Yellow
    Write-Host $result -ForegroundColor Gray
    Write-Host ""

    if ($exitCode -eq 0) {
        Write-Host "✅ psql 연결 성공!" -ForegroundColor Green
    } else {
        Write-Host "❌ psql 연결 실패!" -ForegroundColor Red
    }
} catch {
    Write-Host "❌ 예외 발생: $_" -ForegroundColor Red
} finally {
    # 환경 변수 정리
    Remove-Item Env:PGHOST -ErrorAction SilentlyContinue
    Remove-Item Env:PGPORT -ErrorAction SilentlyContinue
    Remove-Item Env:PGDATABASE -ErrorAction SilentlyContinue
    Remove-Item Env:PGUSER -ErrorAction SilentlyContinue
    Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    Remove-Item Env:PGSSLMODE -ErrorAction SilentlyContinue
}
