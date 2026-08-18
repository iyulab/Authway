# ============================================================
# Migration Helper Functions
# ============================================================
# 배포 스크립트에서 사용할 공통 마이그레이션 함수들
# 지능적으로 보류 중인 마이그레이션만 실행하여 지연시간 최소화
# ============================================================

# Script-level variable for psql path
$script:PsqlExecutable = "psql"

# psql 경로 찾기 및 설정
function Initialize-PsqlPath {
    $psqlCmd = Get-Command psql -ErrorAction SilentlyContinue

    if ($psqlCmd) {
        $script:PsqlExecutable = $psqlCmd.Source
        return $true
    }

    # 일반적인 설치 경로에서 psql 찾기
    $possiblePaths = @(
        "C:\Program Files\PostgreSQL\18\bin\psql.exe",
        "C:\Program Files\PostgreSQL\17\bin\psql.exe",
        "C:\Program Files\PostgreSQL\16\bin\psql.exe",
        "C:\Program Files\PostgreSQL\15\bin\psql.exe",
        "C:\Program Files (x86)\PostgreSQL\18\bin\psql.exe",
        "C:\Program Files (x86)\PostgreSQL\17\bin\psql.exe"
    )

    foreach ($path in $possiblePaths) {
        if (Test-Path $path) {
            # PATH에도 추가 (다른 명령어를 위해)
            $pgBinDir = Split-Path $path -Parent
            $env:Path = "$pgBinDir;$env:Path"
            $script:PsqlExecutable = $path
            return $true
        }
    }

    return $false
}

# psql로 빠른 쿼리 실행
function Invoke-FastQuery {
    param(
        [string]$Query,
        [hashtable]$EnvVars
    )

    $PsqlPath = $script:PsqlExecutable

    # 필수 환경 변수 검증
    $requiredVars = @('AUTHWAY_DATABASE_HOST', 'AUTHWAY_DATABASE_NAME', 'AUTHWAY_DATABASE_USER', 'AUTHWAY_DATABASE_PASSWORD')
    foreach ($varName in $requiredVars) {
        if (-not $EnvVars.ContainsKey($varName) -or [string]::IsNullOrWhiteSpace($EnvVars[$varName])) {
            return @{
                Success = $false
                Output = "필수 환경 변수가 설정되지 않았습니다: $varName"
            }
        }
    }

    # PostgreSQL 환경 변수 설정
    $env:PGHOST = $EnvVars['AUTHWAY_DATABASE_HOST']
    $env:PGPORT = if ($EnvVars['AUTHWAY_DATABASE_PORT']) { $EnvVars['AUTHWAY_DATABASE_PORT'] } else { "5432" }
    $env:PGDATABASE = $EnvVars['AUTHWAY_DATABASE_NAME']
    $env:PGUSER = $EnvVars['AUTHWAY_DATABASE_USER']
    $env:PGPASSWORD = $EnvVars['AUTHWAY_DATABASE_PASSWORD']
    $env:PGSSLMODE = if ($EnvVars['AUTHWAY_DATABASE_SSL_MODE']) { $EnvVars['AUTHWAY_DATABASE_SSL_MODE'] } else { "require" }

    try {
        # BOM 없는 UTF-8로 임시 파일 생성
        $tempFile = [System.IO.Path]::GetTempFileName()
        $utf8NoBom = New-Object System.Text.UTF8Encoding $false
        [System.IO.File]::WriteAllText($tempFile, $Query, $utf8NoBom)

        try {
            # psql 직접 호출 (PATH에 있으므로)
            $result = psql -t -A -q -f $tempFile 2>&1
            $success = ($LASTEXITCODE -eq 0)

            return @{
                Success = $success
                Output = if ($result -is [array]) { $result -join "`n" } else { $result }
            }
        } finally {
            Remove-Item $tempFile -ErrorAction SilentlyContinue
        }
    } finally {
        # 환경 변수 정리
        Remove-Item Env:PGHOST -ErrorAction SilentlyContinue
        Remove-Item Env:PGPORT -ErrorAction SilentlyContinue
        Remove-Item Env:PGDATABASE -ErrorAction SilentlyContinue
        Remove-Item Env:PGUSER -ErrorAction SilentlyContinue
        Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
        Remove-Item Env:PGSSLMODE -ErrorAction SilentlyContinue
    }
}

# 보류 중인 마이그레이션 확인 (초고속)
function Get-PendingMigrations {
    param(
        [hashtable]$EnvVars,
        [string]$MigrationsDir
    )

    # 1. 추적 테이블 존재 확인
    $checkTableQuery = @"
SELECT EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public'
    AND table_name = 'schema_migrations'
);
"@

    $tableResult = Invoke-FastQuery -Query $checkTableQuery -EnvVars $EnvVars

    if (-not $tableResult.Success) {
        Write-Warning "데이터베이스 연결 실패: $($tableResult.Output)"
        return $null
    }

    $trackingTableExists = $tableResult.Output -match "t|true"

    # 2. 적용된 마이그레이션 버전 가져오기
    $appliedVersions = @()
    if ($trackingTableExists) {
        $versionsQuery = "SELECT version FROM schema_migrations WHERE success = true;"
        $versionsResult = Invoke-FastQuery -Query $versionsQuery -EnvVars $EnvVars

        if (-not $versionsResult.Success) {
            Write-Warning "적용된 마이그레이션 버전을 가져올 수 없습니다: $($versionsResult.Output)"
            return $null
        }

        if ($versionsResult.Output.Trim() -ne '') {
            $appliedVersions = $versionsResult.Output -split "`n" | Where-Object { $_.Trim() -ne '' }
        }
    }

    # 3. 마이그레이션 파일 목록 가져오기
    $migrationFiles = Get-ChildItem -Path $MigrationsDir -Filter "*.sql" -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^\d+_' -and $_.Name -notmatch '_rollback\.sql$' } |
        Sort-Object Name

    # 4. 보류 중인 마이그레이션 찾기
    $pendingMigrations = @()
    foreach ($file in $migrationFiles) {
        if ($file.Name -match '^(\d+)_') {
            $version = $matches[1]
            if ($version -notin $appliedVersions) {
                $pendingMigrations += @{
                    Version = $version
                    Name = $file.Name
                    Path = $file.FullName
                }
            }
        }
    }

    # PowerShell이 빈 배열을 $null로 변환하지 않도록 명시적으로 배열로 반환
    return , $pendingMigrations
}

# 마이그레이션 락 획득 (병렬 배포 방지)
function Get-MigrationLock {
    param(
        [hashtable]$EnvVars,
        [int]$TimeoutSeconds = 30
    )

    $lockId = 999999  # 마이그레이션용 고유 락 ID
    $lockQuery = "SELECT pg_try_advisory_lock($lockId);"

    $startTime = Get-Date
    while (((Get-Date) - $startTime).TotalSeconds -lt $TimeoutSeconds) {
        $result = Invoke-FastQuery -Query $lockQuery -EnvVars $EnvVars

        if ($result.Success -and $result.Output -match "t|true") {
            return $true
        }

        Start-Sleep -Milliseconds 500
    }

    return $false
}

# 마이그레이션 락 해제
function Release-MigrationLock {
    param([hashtable]$EnvVars)

    $lockId = 999999
    $unlockQuery = "SELECT pg_advisory_unlock($lockId);"

    Invoke-FastQuery -Query $unlockQuery -EnvVars $EnvVars | Out-Null
}

# 단일 마이그레이션 실행
function Invoke-Migration {
    param(
        [string]$MigrationPath,
        [string]$Version,
        [string]$Name,
        [hashtable]$EnvVars
    )

    Write-Host "  🔄 마이그레이션 실행 중: $Name" -ForegroundColor Yellow

    # SQL 파일 읽기 (BOM 제거)
    $sqlBytes = [System.IO.File]::ReadAllBytes($MigrationPath)
    $sql = [System.Text.Encoding]::UTF8.GetString($sqlBytes).TrimStart([char]0xFEFF)

    # 시작 시간 기록
    $startTime = Get-Date

    # 마이그레이션 SQL + 추적 INSERT를 하나의 트랜잭션으로 결합
    $combinedSql = @"
BEGIN;

-- 마이그레이션 SQL 실행
$sql

-- 추적 테이블에 기록 (트랜잭션 내에서)
INSERT INTO schema_migrations (version, name, execution_time_ms, success)
VALUES ('$Version', '$Name', 0, true)
ON CONFLICT (version) DO UPDATE
SET executed_at = CURRENT_TIMESTAMP,
    execution_time_ms = 0,
    success = true;

COMMIT;
"@

    # 트랜잭션으로 실행
    $result = Invoke-FastQuery -Query $combinedSql -EnvVars $EnvVars

    # 종료 시간 및 실행 시간 계산
    $endTime = Get-Date
    $durationMs = [math]::Round(($endTime - $startTime).TotalMilliseconds)

    if ($result.Success) {
        # 실행 시간 업데이트
        $updateQuery = @"
UPDATE schema_migrations
SET execution_time_ms = $durationMs
WHERE version = '$Version';
"@
        Invoke-FastQuery -Query $updateQuery -EnvVars $EnvVars | Out-Null

        Write-Host "  ✅ 완료: $Name (${durationMs}ms)" -ForegroundColor Green
        return $true
    } else {
        # 실패 기록 (별도 트랜잭션)
        $errorMsg = ($result.Output -replace "'", "''").Substring(0, [Math]::Min(500, $result.Output.Length))  # 500자로 제한
        $recordQuery = @"
INSERT INTO schema_migrations (version, name, execution_time_ms, success, error_message)
VALUES ('$Version', '$Name', $durationMs, false, '$errorMsg')
ON CONFLICT (version) DO UPDATE
SET executed_at = CURRENT_TIMESTAMP,
    execution_time_ms = $durationMs,
    success = false,
    error_message = '$errorMsg';
"@

        Invoke-FastQuery -Query $recordQuery -EnvVars $EnvVars | Out-Null

        Write-Host "  ❌ 실패: $Name" -ForegroundColor Red
        Write-Host "  오류: $($result.Output)" -ForegroundColor Gray

        return $false
    }
}

# 메인 마이그레이션 실행 함수 (배포 스크립트에서 호출)
# DEPRECATED (v0.4.0): 마이그레이션은 Go startup migrator(migrate.go)가 처리한다.
# deploy-all.core.ps1은 이 함수를 더 이상 호출하지 않는다. 직접 호출 시 오류 발생.
function Invoke-AutoMigration {
    param(
        [hashtable]$EnvVars,
        [string]$ScriptDir,
        [switch]$Force
    )

    throw "DEPRECATED: Invoke-AutoMigration은 v0.4.0에서 제거되었습니다. 마이그레이션은 Go startup migrator가 자동 처리합니다."

    # Central API의 embedded migrations 디렉토리 사용
    # $ScriptDir = scripts/deploy/_shared → 3-level Split-Path-Parent → repo root
    $ProjectRoot = Split-Path (Split-Path (Split-Path $ScriptDir -Parent) -Parent) -Parent
    $MigrationsDir = Join-Path $ProjectRoot "apps\central\api\internal\database\migrations"

    # 마이그레이션 디렉토리 확인
    if (-not (Test-Path $MigrationsDir)) {
        Write-Verbose "마이그레이션 디렉토리가 없습니다: $MigrationsDir"
        return @{ Success = $true; Skipped = $true; Message = "마이그레이션 디렉토리 없음" }
    }

    # psql 초기화
    if (-not (Initialize-PsqlPath)) {
        Write-Warning "⚠️  psql을 찾을 수 없어 마이그레이션을 건너뜁니다."
        Write-Warning "   배포 후 수동으로 마이그레이션을 실행하세요: .\scripts\deploy\run-migration-azure.ps1"
        return @{ Success = $true; Skipped = $true; Message = "psql 없음" }
    }

    # 보류 중인 마이그레이션 확인 (초고속 체크)
    $pendingMigrations = Get-PendingMigrations -EnvVars $EnvVars -MigrationsDir $MigrationsDir

    if ($null -eq $pendingMigrations) {
        Write-Warning "⚠️  데이터베이스 연결 실패, 마이그레이션을 건너뜁니다."
        return @{ Success = $true; Skipped = $true; Message = "DB 연결 실패" }
    }

    if ($pendingMigrations.Count -eq 0) {
        Write-Host "✅ 모든 마이그레이션이 적용되었습니다 (확인 완료)" -ForegroundColor Green
        return @{ Success = $true; Skipped = $true; Applied = 0 }
    }

    # 보류 중인 마이그레이션 발견
    Write-Host ""
    Write-Host "📋 보류 중인 마이그레이션 $($pendingMigrations.Count)개 발견" -ForegroundColor Cyan
    foreach ($migration in $pendingMigrations) {
        Write-Host "   • $($migration.Name)" -ForegroundColor Yellow
    }
    Write-Host ""

    # 사용자 확인 (Force 플래그가 없으면)
    if (-not $Force) {
        $response = Read-Host "마이그레이션을 실행하시겠습니까? (yes/no)"
        if ($response -ne 'yes' -and $response -ne 'y') {
            Write-Host "⏭️  마이그레이션을 건너뜁니다." -ForegroundColor Yellow
            return @{ Success = $true; Skipped = $true; Message = "사용자 취소" }
        }
    }

    # 락 획득
    Write-Host "🔒 마이그레이션 락 획득 중..." -ForegroundColor Yellow
    if (-not (Get-MigrationLock -EnvVars $EnvVars)) {
        Write-Warning "⚠️  마이그레이션 락을 획득할 수 없습니다 (다른 프로세스가 실행 중)"
        return @{ Success = $false; Error = "락 획득 실패" }
    }

    try {
        Write-Host "✅ 락 획득 완료" -ForegroundColor Green
        Write-Host ""
        Write-Host "🔄 마이그레이션 실행 중..." -ForegroundColor Cyan
        Write-Host ""

        $successCount = 0
        $failCount = 0

        foreach ($migration in $pendingMigrations) {
            $result = Invoke-Migration `
                -MigrationPath $migration.Path `
                -Version $migration.Version `
                -Name $migration.Name `
                -EnvVars $EnvVars

            if ($result) {
                $successCount++
            } else {
                $failCount++
                # 실패 시 중단
                break
            }
        }

        Write-Host ""
        if ($failCount -eq 0) {
            Write-Host "✅ 마이그레이션 완료: $successCount 개 성공" -ForegroundColor Green
            return @{ Success = $true; Applied = $successCount; Failed = 0 }
        } else {
            Write-Host "❌ 마이그레이션 실패: $successCount 개 성공, $failCount 개 실패" -ForegroundColor Red
            return @{ Success = $false; Applied = $successCount; Failed = $failCount }
        }
    } finally {
        # 락 해제
        Release-MigrationLock -EnvVars $EnvVars
        Write-Verbose "🔓 마이그레이션 락 해제됨"
    }
}

# 함수들이 정의되어 dot-sourcing으로 사용 가능
# Export-ModuleMember는 .psm1 모듈에서만 사용 가능하므로 제거
