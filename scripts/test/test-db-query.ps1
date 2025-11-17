# 간단한 DB 쿼리 테스트
$ScriptDir = "D:\data\Authway\scripts\deploy"
$EnvFile = Join-Path $ScriptDir ".env"

$envVars = @{}
Get-Content $EnvFile | ForEach-Object {
    if ($_ -match '^([^#][^=]+)=(.+)$') {
        $envVars[$matches[1].Trim()] = $matches[2].Trim()
    }
}

$env:PGHOST = $envVars['AUTHWAY_DATABASE_HOST']
$env:PGPORT = "5432"
$env:PGDATABASE = $envVars['AUTHWAY_DATABASE_NAME']
$env:PGUSER = $envVars['AUTHWAY_DATABASE_USER']
$env:PGPASSWORD = $envVars['AUTHWAY_DATABASE_PASSWORD']
$env:PGSSLMODE = "require"

Write-Host "=== 추적 테이블 내용 ===" -ForegroundColor Cyan
psql -c "SELECT * FROM schema_migrations ORDER BY version;"

Write-Host ""
Write-Host "=== 카운트 ===" -ForegroundColor Cyan
psql -c "SELECT COUNT(*) as total FROM schema_migrations;"
