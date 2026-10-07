# ============================================================
# psql Helper Functions
# ============================================================
# 배포 검증 스크립트가 대상 DB에 짧은 쿼리를 보낼 때 쓰는 공통 함수.
# 접속 정보는 Get-DeployEnv(load-env.ps1)가 돌려준 hashtable 에서 읽는다.
# 마이그레이션 적용은 여기서 하지 않는다 — API 기동 시 Go migrator 몫.
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
