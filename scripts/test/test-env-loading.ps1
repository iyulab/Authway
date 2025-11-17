# 환경 변수 로딩 테스트
$ProjectRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$ScriptDir = Join-Path $ProjectRoot "scripts\deploy"
$EnvFile = Join-Path $ScriptDir ".env"

Write-Host "=== 환경 변수 로딩 테스트 ===" -ForegroundColor Cyan
Write-Host ""
Write-Host ".env 파일 경로: $EnvFile" -ForegroundColor Yellow
Write-Host ".env 파일 존재: $(Test-Path $EnvFile)" -ForegroundColor Yellow
Write-Host ""

if (-not (Test-Path $EnvFile)) {
    Write-Host "❌ .env 파일을 찾을 수 없습니다!" -ForegroundColor Red
    exit 1
}

$envVars = @{}
Get-Content $EnvFile | ForEach-Object {
    if ($_ -match '^([^#][^=]+)=(.+)$') {
        $key = $matches[1].Trim()
        $value = $matches[2].Trim()
        $envVars[$key] = $value

        # 비밀번호는 일부만 표시
        $displayValue = if ($key -match 'PASSWORD') {
            if ($value.Length -gt 4) {
                $value.Substring(0, 3) + "***"
            } else {
                "***"
            }
        } else {
            $value
        }

        Write-Host "  ✓ $key = $displayValue" -ForegroundColor Green
    }
}

Write-Host ""
Write-Host "=== 필수 변수 체크 ===" -ForegroundColor Cyan
$requiredVars = @('AUTHWAY_DATABASE_HOST', 'AUTHWAY_DATABASE_NAME', 'AUTHWAY_DATABASE_USER', 'AUTHWAY_DATABASE_PASSWORD')

$allPresent = $true
foreach ($varName in $requiredVars) {
    $exists = $envVars.ContainsKey($varName)
    $notEmpty = -not [string]::IsNullOrWhiteSpace($envVars[$varName])

    if ($exists -and $notEmpty) {
        Write-Host "  ✅ ${varName}: OK" -ForegroundColor Green
    } elseif ($exists) {
        Write-Host "  ⚠️  ${varName}: 존재하지만 비어있음" -ForegroundColor Yellow
        $allPresent = $false
    } else {
        Write-Host "  ❌ ${varName}: 없음" -ForegroundColor Red
        $allPresent = $false
    }
}

Write-Host ""
if ($allPresent) {
    Write-Host "✅ 모든 필수 환경 변수가 설정되었습니다!" -ForegroundColor Green
} else {
    Write-Host "❌ 일부 필수 환경 변수가 누락되었습니다." -ForegroundColor Red
}
