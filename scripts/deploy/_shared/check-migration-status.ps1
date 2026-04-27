# ============================================================
# Migration Status Checker
# ============================================================
# Check the status of all database migrations
# Shows applied, pending, and failed migrations
# ============================================================

param(
    [switch]$Verbose
)

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  📊 Migration Status Checker" -ForegroundColor Cyan
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

# Check if Azure CLI is installed
$azCliPath = Get-Command az -ErrorAction SilentlyContinue
if (-not $azCliPath) {
    Write-Host "❌ Azure CLI를 찾을 수 없습니다." -ForegroundColor Red
    exit 1
}

# Check if logged in to Azure
$azAccount = az account show 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Azure에 로그인되어 있지 않습니다." -ForegroundColor Red
    Write-Host "다음 명령으로 로그인해주세요: az login" -ForegroundColor Yellow
    exit 1
}

# Extract server name from host
$serverName = $envVars['AUTHWAY_DATABASE_HOST'] -replace '\.postgres\.database\.azure\.com$', ''

# Function to execute SQL query
function Invoke-AzurePostgresQuery {
    param([string]$Query)

    $tempFile = [System.IO.Path]::GetTempFileName() + ".sql"
    try {
        $Query | Out-File -FilePath $tempFile -Encoding UTF8 -NoNewline

        $result = az postgres flexible-server execute `
            --name $serverName `
            --admin-user $envVars['AUTHWAY_DATABASE_USER'] `
            --admin-password $envVars['AUTHWAY_DATABASE_PASSWORD'] `
            --database-name $envVars['AUTHWAY_DATABASE_NAME'] `
            --file-path $tempFile `
            2>&1

        # Convert result to string to handle both success and error cases
        $outputString = if ($result -is [array]) {
            $result -join "`n"
        } else {
            $result.ToString()
        }

        # Debug: Show raw output
        if ($Verbose) {
            Write-Host "🐛 원본 출력:" -ForegroundColor Gray
            Write-Host $outputString -ForegroundColor DarkGray
            Write-Host "🐛 ---" -ForegroundColor Gray
        }

        # Filter out Azure CLI WARNING messages
        $cleanOutput = $outputString -split "`n" |
            Where-Object { $_ -notmatch '^WARNING:' -and $_.Trim() -ne '' } |
            Select-Object -First 1

        return @{
            Success = ($LASTEXITCODE -eq 0)
            Output = if ($cleanOutput) { $cleanOutput.Trim() } else { "" }
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
) AS tracking_exists;
"@

$tableResult = Invoke-AzurePostgresQuery -Query $checkTableQuery

if (-not $tableResult.Success) {
    Write-Host "❌ 데이터베이스 연결 실패" -ForegroundColor Red
    Write-Host $tableResult.Output -ForegroundColor Gray
    exit 1
}

# Debug output
if ($Verbose) {
    Write-Host "🐛 디버그: 테이블 존재 확인 결과 = '$($tableResult.Output)'" -ForegroundColor Gray
}

if ($tableResult.Output -notmatch "t|true") {
    Write-Host "⚠️  schema_migrations 테이블이 존재하지 않습니다" -ForegroundColor Yellow
    Write-Host "첫 마이그레이션을 실행하면 자동으로 생성됩니다" -ForegroundColor Gray
    if ($Verbose) {
        Write-Host "🐛 디버그: 예상값 't' 또는 'true', 실제값 '$($tableResult.Output)'" -ForegroundColor Gray
    }
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
    success,
    CASE WHEN error_message IS NOT NULL THEN LEFT(error_message, 100) ELSE '' END as error_message
FROM schema_migrations
ORDER BY version ASC;
"@

$migrationsResult = Invoke-AzurePostgresQuery -Query $allMigrationsQuery

if ($migrationsResult.Success) {
    # Parse TSV output
    $lines = $migrationsResult.Output -split "`n"

    if ($lines.Count -le 1) {
        Write-Host "  ℹ️  적용된 마이그레이션이 없습니다" -ForegroundColor Gray
    } else {
        # Print header
        Write-Host "  Version  | Name                         | Status    | Executed At        | Time (ms)" -ForegroundColor White
        Write-Host "  ─────────┼──────────────────────────────┼───────────┼────────────────────┼──────────" -ForegroundColor Gray

        foreach ($line in $lines) {
            if ([string]::IsNullOrWhiteSpace($line)) { continue }

            $fields = $line -split "`t"
            if ($fields.Count -ge 5) {
                $version = $fields[0].PadRight(8)
                $name = $fields[1].PadRight(28).Substring(0, 28)
                $executedAt = $fields[2].PadRight(18)
                $execTime = if ($fields[3]) { $fields[3].PadLeft(8) } else { "       -" }
                $success = $fields[4]

                $statusIcon = if ($success -eq "t" -or $success -eq "true") { "✅" } else { "❌" }
                $statusText = if ($success -eq "t" -or $success -eq "true") { "SUCCESS  " } else { "FAILED   " }
                $color = if ($success -eq "t" -or $success -eq "true") { "Green" } else { "Red" }

                Write-Host "  $version | $name | " -NoNewline
                Write-Host $statusIcon -NoNewline -ForegroundColor $color
                Write-Host " $statusText | $executedAt | $execTime" -ForegroundColor $color

                # Show error message if failed and verbose
                if (($success -eq "f" -or $success -eq "false") -and $Verbose -and $fields.Count -ge 6 -and $fields[5]) {
                    Write-Host "    ⚠️  Error: $($fields[5])" -ForegroundColor Yellow
                }
            }
        }
    }
} else {
    Write-Host "❌ 마이그레이션 목록 조회 실패" -ForegroundColor Red
    Write-Host $migrationsResult.Output -ForegroundColor Gray
}

Write-Host ""

# Get pending migrations
Write-Host "📁 보류 중인 마이그레이션:" -ForegroundColor Cyan
Write-Host ""

$MigrationsDir = Join-Path (Split-Path $ScriptDir -Parent) "migrations"
$migrationFiles = Get-ChildItem -Path $MigrationsDir -Filter "*.sql" | Sort-Object Name

$pendingCount = 0

foreach ($file in $migrationFiles) {
    if ($file.Name -match '^(\d+)_') {
        $version = $matches[1]

        # Check if this version is applied
        $checkQuery = "SELECT success FROM schema_migrations WHERE version = '$version' AND success = true;"
        $checkResult = Invoke-AzurePostgresQuery -Query $checkQuery

        if ($checkResult.Success -and ($checkResult.Output -match "t|true")) {
            # Skip applied migrations
            continue
        }

        Write-Host "  📄 $($file.Name)" -ForegroundColor Yellow
        $pendingCount++
    }
}

if ($pendingCount -eq 0) {
    Write-Host "  ✅ 모든 마이그레이션이 적용되었습니다" -ForegroundColor Green
}

Write-Host ""

# Summary
$successQuery = "SELECT COUNT(*) FROM schema_migrations WHERE success = true;"
$failedQuery = "SELECT COUNT(*) FROM schema_migrations WHERE success = false;"

$successResult = Invoke-AzurePostgresQuery -Query $successQuery
$failedResult = Invoke-AzurePostgresQuery -Query $failedQuery

Write-Host "📊 요약:" -ForegroundColor Cyan
if ($successResult.Success) {
    $successCount = $successResult.Output.Trim()
    Write-Host "  ✅ 성공: ${successCount}개" -ForegroundColor Green
} else {
    Write-Host "  ⚠️  성공 카운트 조회 실패" -ForegroundColor Yellow
    if ($Verbose) {
        Write-Host "     오류: $($successResult.Output)" -ForegroundColor Gray
    }
}
if ($failedResult.Success) {
    $failedCount = $failedResult.Output.Trim()
    if ([int]$failedCount -gt 0) {
        Write-Host "  ❌ 실패: ${failedCount}개" -ForegroundColor Red
    }
} else {
    Write-Host "  ⚠️  실패 카운트 조회 실패" -ForegroundColor Yellow
    if ($Verbose) {
        Write-Host "     오류: $($failedResult.Output)" -ForegroundColor Gray
    }
}
Write-Host "  ⏳ 보류: ${pendingCount}개" -ForegroundColor Yellow
Write-Host ""
