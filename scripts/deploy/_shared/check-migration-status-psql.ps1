# ============================================================
# Migration Status Checker (psql 버전)
# ============================================================
# Check the status of all database migrations using psql
# Shows applied, pending, and failed migrations
# ============================================================

param(
    [switch]$Verbose
)

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  📊 Migration Status Checker (psql)" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

# Load environment variables
$ScriptDir = $PSScriptRoot
$EnvFile = Join-Path $ScriptDir ".env"

if (-not (Test-Path $EnvFile)) {
    Write-Host "❌ .env 파일을 찾을 수 없습니다: $EnvFile" -ForegroundColor Red
    exit 1
}

$envVars = @{}
Get-Content $EnvFile | ForEach-Object {
    if ($_ -match '^([^#][^=]+)=(.+)$') {
        $envVars[$matches[1].Trim()] = $matches[2].Trim()
    }
}

Write-Host "🗄️  데이터베이스: $($envVars['AUTHWAY_DATABASE_NAME'])" -ForegroundColor White
Write-Host "🌐 호스트: $($envVars['AUTHWAY_DATABASE_HOST'])" -ForegroundColor White
Write-Host ""

# Check if psql is installed
$psqlPath = Get-Command psql -ErrorAction SilentlyContinue

if (-not $psqlPath) {
    # Try to find psql in common installation paths
    $possiblePaths = @(
        "C:\Program Files\PostgreSQL\18\bin\psql.exe",
        "C:\Program Files\PostgreSQL\17\bin\psql.exe",
        "C:\Program Files\PostgreSQL\16\bin\psql.exe",
        "C:\Program Files\PostgreSQL\15\bin\psql.exe",
        "C:\Program Files (x86)\PostgreSQL\18\bin\psql.exe",
        "C:\Program Files (x86)\PostgreSQL\17\bin\psql.exe",
        "C:\ProgramData\chocolatey\lib\postgresql\tools\bin\psql.exe"
    )

    foreach ($path in $possiblePaths) {
        if (Test-Path $path) {
            $psqlPath = @{ Source = $path }
            Write-Host "✅ psql 찾음: $path" -ForegroundColor Green

            # Add to PATH for this session
            $pgBinDir = Split-Path $path -Parent
            $env:Path = "$pgBinDir;$env:Path"
            break
        }
    }
}

if (-not $psqlPath) {
    Write-Host "❌ psql을 찾을 수 없습니다." -ForegroundColor Red
    Write-Host ""
    Write-Host "PostgreSQL 클라이언트를 설치해주세요:" -ForegroundColor Yellow
    Write-Host "  choco install postgresql" -ForegroundColor Gray
    Write-Host "  https://www.postgresql.org/download/" -ForegroundColor Gray
    Write-Host ""
    Write-Host "설치 후 다음 중 하나를 수행하세요:" -ForegroundColor Yellow
    Write-Host "  1. VSCode를 재시작하거나" -ForegroundColor Gray
    Write-Host "  2. 새 PowerShell 터미널을 열거나" -ForegroundColor Gray
    Write-Host "  3. 다음 명령으로 PATH 새로고침:" -ForegroundColor Gray
    Write-Host "     `$env:Path = [System.Environment]::GetEnvironmentVariable('Path','Machine') + ';' + [System.Environment]::GetEnvironmentVariable('Path','User')" -ForegroundColor DarkGray
    Write-Host ""
    exit 1
}

# Set PostgreSQL environment variables for connection
$env:PGHOST = $envVars['AUTHWAY_DATABASE_HOST']
$env:PGPORT = "5432"
$env:PGDATABASE = $envVars['AUTHWAY_DATABASE_NAME']
$env:PGUSER = $envVars['AUTHWAY_DATABASE_USER']
$env:PGPASSWORD = $envVars['AUTHWAY_DATABASE_PASSWORD']
$env:PGSSLMODE = "require"

# Function to execute SQL query using psql
function Invoke-PsqlQuery {
    param([string]$Query)

    # BOM 없는 UTF-8로 임시 파일 생성
    $tempFile = [System.IO.Path]::GetTempFileName()
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($tempFile, $Query, $utf8NoBom)

    try {
        $result = psql -t -A -q -f $tempFile 2>&1

        return @{
            Success = ($LASTEXITCODE -eq 0)
            Output = if ($result -is [array]) { $result -join "`n" } else { $result }
        }
    } finally {
        Remove-Item $tempFile -ErrorAction SilentlyContinue
    }
}

# Check if tracking table exists
Write-Host "🔍 마이그레이션 추적 테이블 확인 중..." -ForegroundColor Yellow

$checkTableQuery = @"
SELECT EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public'
    AND table_name = 'schema_migrations'
);
"@

$tableResult = Invoke-PsqlQuery -Query $checkTableQuery

if (-not $tableResult.Success) {
    Write-Host "❌ 데이터베이스 연결 실패" -ForegroundColor Red
    Write-Host $tableResult.Output -ForegroundColor Gray
    exit 1
}

if ($Verbose) {
    Write-Host "🐛 디버그: 테이블 존재 확인 결과 = '$($tableResult.Output)'" -ForegroundColor Gray
}

if ($tableResult.Output -notmatch "t|true") {
    Write-Host "⚠️  schema_migrations 테이블이 존재하지 않습니다" -ForegroundColor Yellow
    Write-Host "첫 마이그레이션을 실행하면 자동으로 생성됩니다" -ForegroundColor Gray
    Write-Host ""
    exit 0
}

Write-Host "✅ 추적 테이블 확인됨" -ForegroundColor Green
Write-Host ""

# Get all applied migrations
Write-Host "📋 적용된 마이그레이션:" -ForegroundColor Cyan
Write-Host ""

$allMigrationsQuery = @"
SELECT
    version,
    name,
    TO_CHAR(executed_at, 'YYYY-MM-DD HH24:MI:SS') as executed_at,
    execution_time_ms,
    CASE WHEN success THEN '✅' ELSE '❌' END as status
FROM schema_migrations
ORDER BY version;
"@

$migrationsResult = Invoke-PsqlQuery -Query $allMigrationsQuery

if ($migrationsResult.Success) {
    # Format output as table
    Write-Host "  Version  | Name                         | Status    | Executed At        | Time (ms)" -ForegroundColor White
    Write-Host "  ─────────┼──────────────────────────────┼───────────┼────────────────────┼──────────" -ForegroundColor Gray

    if ($migrationsResult.Output.Trim() -ne '') {
        $migrationsResult.Output -split "`n" | ForEach-Object {
            if ($_.Trim() -ne '') {
                $parts = $_ -split '\|'
                if ($parts.Length -eq 5) {
                    $version = $parts[0].Trim().PadRight(8)
                    $name = $parts[1].Trim().PadRight(28)
                    $status = $parts[4].Trim().PadRight(9)
                    $executedAt = $parts[2].Trim().PadRight(18)
                    $timeMs = $parts[3].Trim().PadLeft(9)

                    Write-Host "  $version | $name | $status | $executedAt | $timeMs" -ForegroundColor White
                }
            }
        }
    } else {
        Write-Host "  (마이그레이션 기록 없음)" -ForegroundColor Gray
    }
} else {
    Write-Host "  ❌ 마이그레이션 기록 조회 실패" -ForegroundColor Red
}

Write-Host ""

# Get pending migrations
Write-Host "📁 보류 중인 마이그레이션:" -ForegroundColor Cyan
Write-Host ""

$MigrationsDir = Join-Path (Split-Path $ScriptDir -Parent) "migrations"
$migrationFiles = Get-ChildItem -Path $MigrationsDir -Filter "*.sql" |
    Where-Object { $_.Name -match '^\d+_' -and $_.Name -notmatch '_rollback\.sql$' } |
    Sort-Object Name

$appliedVersionsQuery = "SELECT version FROM schema_migrations WHERE success = true;"
$appliedResult = Invoke-PsqlQuery -Query $appliedVersionsQuery
$appliedVersions = if ($appliedResult.Success) {
    ($appliedResult.Output -split "`n" | Where-Object { $_.Trim() -ne '' })
} else {
    @()
}

$pendingCount = 0
foreach ($file in $migrationFiles) {
    if ($file.Name -match '^(\d+)_') {
        $version = $matches[1]
        if ($version -notin $appliedVersions) {
            Write-Host "  📄 $($file.Name)" -ForegroundColor Yellow
            $pendingCount++
        }
    }
}

if ($pendingCount -eq 0) {
    Write-Host "  ✅ 모든 마이그레이션이 적용되었습니다" -ForegroundColor Green
}

Write-Host ""

# Summary
$successQuery = "SELECT COUNT(*) FROM schema_migrations WHERE success = true;"
$failedQuery = "SELECT COUNT(*) FROM schema_migrations WHERE success = false;"

$successResult = Invoke-PsqlQuery -Query $successQuery
$failedResult = Invoke-PsqlQuery -Query $failedQuery

Write-Host "📊 요약:" -ForegroundColor Cyan
if ($successResult.Success) {
    $successCount = $successResult.Output.Trim()
    Write-Host "  ✅ 성공: ${successCount}개" -ForegroundColor Green
} else {
    Write-Host "  ⚠️  성공 카운트 조회 실패" -ForegroundColor Yellow
}

if ($failedResult.Success) {
    $failedCount = $failedResult.Output.Trim()
    if ([int]$failedCount -gt 0) {
        Write-Host "  ❌ 실패: ${failedCount}개" -ForegroundColor Red
    }
} else {
    Write-Host "  ⚠️  실패 카운트 조회 실패" -ForegroundColor Yellow
}

Write-Host "  ⏳ 보류: ${pendingCount}개" -ForegroundColor Yellow
Write-Host ""

# Clean up environment variables
Remove-Item Env:PGHOST -ErrorAction SilentlyContinue
Remove-Item Env:PGPORT -ErrorAction SilentlyContinue
Remove-Item Env:PGDATABASE -ErrorAction SilentlyContinue
Remove-Item Env:PGUSER -ErrorAction SilentlyContinue
Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
Remove-Item Env:PGSSLMODE -ErrorAction SilentlyContinue
