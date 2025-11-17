# 추적 테이블 직접 확인
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

# PostgreSQL 환경 변수 설정
$env:PGHOST = $envVars['AUTHWAY_DATABASE_HOST']
$env:PGPORT = "5432"
$env:PGDATABASE = $envVars['AUTHWAY_DATABASE_NAME']
$env:PGUSER = $envVars['AUTHWAY_DATABASE_USER']
$env:PGPASSWORD = $envVars['AUTHWAY_DATABASE_PASSWORD']
$env:PGSSLMODE = "require"

Write-Host "=== 추적 테이블 내용 확인 ===" -ForegroundColor Cyan
Write-Host ""

# BOM 없는 UTF-8로 임시 파일 생성
$query = "SELECT version, name, success, executed_at, execution_time_ms FROM schema_migrations ORDER BY version;"
$tempFile = [System.IO.Path]::GetTempFileName()
$utf8NoBom = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllText($tempFile, $query, $utf8NoBom)

try {
    Write-Host "실행 쿼리: $query" -ForegroundColor Yellow
    Write-Host ""

    $result = psql -f $tempFile

    Write-Host ""
    Write-Host "Exit Code: $LASTEXITCODE" -ForegroundColor $(if ($LASTEXITCODE -eq 0) { "Green" } else { "Red" })
} finally {
    Remove-Item $tempFile -ErrorAction SilentlyContinue

    # 환경 변수 정리
    Remove-Item Env:PGHOST -ErrorAction SilentlyContinue
    Remove-Item Env:PGPORT -ErrorAction SilentlyContinue
    Remove-Item Env:PGDATABASE -ErrorAction SilentlyContinue
    Remove-Item Env:PGUSER -ErrorAction SilentlyContinue
    Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    Remove-Item Env:PGSSLMODE -ErrorAction SilentlyContinue
}
