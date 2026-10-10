# ============================================================
# 로그인 → consent → 콜백 회귀 스모크 (POST-DEPLOY-VERIFY.md §1-3)
# ============================================================
# authorization_code + password 로그인의 전 구간을 실제로 구동한다 —
# Hydra 로그인 챌린지 발급부터 access token 교환까지. 이 구간은 로컬에서
# 선검증할 수 없다(배포된 실제 Hydra·central API가 서로 맞물려
# 동작하는지가 검증 대상 자체이므로), 그래서 지금까지 사람이 매 배포마다
# 브라우저로 반복해 왔다.
#
# 검증용 client·user는 이 스크립트가 실행마다 새로 만들고 끝에 항상
# 삭제한다(성공/실패 무관, finally). 검증용 비밀번호는 일부러 고정값이다 —
# 매 실행 즉시 폐기되는 행 하나의 자격증명이라 회전할 이유가 없고, 고정해
# 두면 이 스크립트 자체를 실 자격증명 없이도 읽을 수 있다.
#
# 로그아웃 회귀는 이번 자동화 범위 밖이다 — Hydra RP-initiated logout의
# 정확한 파라미터 계약을 자동화하지 않았고, 잘못 만든 자동화가
# "통과"를 잘못 보고하는 편이 사람이 매번 확인하는 것보다 나쁘다고 판단했다.
# 계속 사람이 확인한다(POST-DEPLOY-VERIFY.md §2 참조).
#
# 사용:
#   verify/verify-oauth-smoke.ps1 -Target staging -Tenant <검증용 테넌트 UUID>
#   verify/verify-oauth-smoke.ps1 -Target prod -Tenant <검증용 테넌트 UUID> -SkipAuditSmoke
# ============================================================

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("prod", "staging")]
    [string]$Target,

    [Parameter(Mandatory = $true)]
    [string]$Tenant,

    # audit_logs smoke(_shared/smoke-audit.ps1)를 이어서 실행하지 않으려면.
    [switch]$SkipAuditSmoke
)

$ErrorActionPreference = "Stop"

$VerifyDir = $PSScriptRoot
if (-not $VerifyDir) { $VerifyDir = Split-Path -Parent $MyInvocation.MyCommand.Path }
$SharedDir = Join-Path (Split-Path -Parent $VerifyDir) "_shared"

. (Join-Path $SharedDir "load-env.ps1")
. (Join-Path $SharedDir "psql-helpers.ps1")

# 이 스크립트가 실행마다 만들고 지우는 검증용 계정의 고정 비밀번호/해시.
# 행이 즉시 삭제되는 일회성 검증용이라 회전할 필요가 없다(스크립트 헤더 참조).
$VerifyPassword = "PostDeployVerify-2026-Rotate"
$VerifyPasswordHash = '$2a$10$F3sj/nsMgz0Zs4CiCJOYKe0XB87PC25rRv00Mn5GVRgvdgC6znkWK'

function New-VerifySuffix {
    -join ((48..57) + (97..122) | Get-Random -Count 8 | ForEach-Object { [char]$_ })
}

function Invoke-CurlCapture {
    param(
        [string[]]$CurlArgs,
        [string]$CookieJar
    )
    $headerFile = [System.IO.Path]::GetTempFileName()
    $bodyFile = [System.IO.Path]::GetTempFileName()
    try {
        $args = @("-s", "-D", $headerFile, "-o", $bodyFile, "-c", $CookieJar, "-b", $CookieJar) + $CurlArgs
        & curl.exe @args | Out-Null
        $headers = Get-Content $headerFile -Raw -ErrorAction SilentlyContinue
        $body = Get-Content $bodyFile -Raw -ErrorAction SilentlyContinue
        $statusLine = ($headers -split "`r`n" | Select-Object -First 1)
        $status = if ($statusLine -match '\s(\d{3})\s') { [int]$matches[1] } else { 0 }
        $location = $null
        foreach ($line in ($headers -split "`r`n")) {
            if ($line -match '^[Ll]ocation:\s*(.+)$') { $location = $matches[1].Trim() }
        }
        return @{ Status = $status; Location = $location; Body = $body; Headers = $headers }
    } finally {
        Remove-Item $headerFile, $bodyFile -ErrorAction SilentlyContinue
    }
}

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🔐 OAuth 회귀 스모크 (target=$Target)" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

$envVars = Get-DeployEnv -Target $Target
$issuer = $envVars['HYDRA_ISSUER']
$apiUrl = $envVars['API_URL']
$adminKey = $envVars['ADMIN_API_KEY']

foreach ($pair in @(@{n='HYDRA_ISSUER';v=$issuer}, @{n='API_URL';v=$apiUrl}, @{n='ADMIN_API_KEY';v=$adminKey})) {
    if ([string]::IsNullOrWhiteSpace($pair.v)) {
        Write-Host "❌ $($pair.n) 미설정 ($($envVars['__ENV_FILE__']))" -ForegroundColor Red
        exit 1
    }
}

if (-not (Initialize-PsqlPath)) {
    Write-Host "❌ psql 미발견 — 검증용 유저 생성/삭제에 필요" -ForegroundColor Red
    exit 1
}

$suffix = New-VerifySuffix
$verifyEmail = "verify-oauth-smoke-$suffix@authway.invalid"
$cookieJar = [System.IO.Path]::GetTempFileName()
$clientId = $null
$clientDbId = $null
$userCreated = $false
$failed = $false

try {
    # --- 1. 검증용 client 생성 ---
    Write-Host "1️⃣  검증용 client 생성" -ForegroundColor Yellow
    $createClientBody = @{
        tenant_id       = $Tenant
        name            = "verify-oauth-smoke-$suffix"
        public          = $true
        redirect_uris   = @("http://localhost:9999/verify-callback")
        allowed_origins = @("http://localhost:9999")
        grant_types     = @("authorization_code")
        scopes          = @("openid")
        # JWT 액세스 토큰 — 아래에서 클레임 형태(계약의 TokenClaims)를 읽는다.
        access_token_strategy = "jwt"
    } | ConvertTo-Json -Compress

    $tmpBody = [System.IO.Path]::GetTempFileName()
    Set-Content -Path $tmpBody -Value $createClientBody -NoNewline -Encoding UTF8
    $createResp = & curl.exe -s -X POST "$apiUrl/api/v1/clients" `
        -H "Content-Type: application/json" -H "Authorization: Bearer $adminKey" `
        --data "@$tmpBody"
    Remove-Item $tmpBody -ErrorAction SilentlyContinue

    $createParsed = $createResp | ConvertFrom-Json
    if (-not $createParsed.client -or -not $createParsed.client.client_id) {
        Write-Host "❌ client 생성 실패: $createResp" -ForegroundColor Red
        exit 1
    }
    $clientId = $createParsed.client.client_id
    $clientDbId = $createParsed.client.id
    Write-Host "   ✅ client_id=$clientId" -ForegroundColor Green

    # --- 2. 검증용 user 직접 생성 (admin API에 create 엔드포인트가 없음 —
    #     invitation-accept/social 로그인으로만 유저가 생기는 구조라 기존
    #     dogfooding 세션과 동일하게 DB 직접 INSERT로 우회) ---
    Write-Host "2️⃣  검증용 user 생성 ($verifyEmail)" -ForegroundColor Yellow
    $insertUserQuery = @"
INSERT INTO users (id, tenant_id, email, password_hash, name, email_verified, active, provider)
VALUES (gen_random_uuid(), '$Tenant', '$verifyEmail', '$VerifyPasswordHash', 'post-deploy-verify', true, true, 'local');
"@
    $insertResult = Invoke-FastQuery -Query $insertUserQuery -EnvVars $envVars
    if (-not $insertResult.Success) {
        Write-Host "❌ user 생성 실패: $($insertResult.Output)" -ForegroundColor Red
        exit 1
    }
    $userCreated = $true
    Write-Host "   ✅ 생성 완료" -ForegroundColor Green

    # --- 3. authorization_code 플로우 시작 → 로그인 flow id ---
    # Hydra 는 브라우저를 API 의 /login?login_challenge= 로 보내고, API 는 로그인 UI 의
    # ?flow=<id> 로 보낸다(docs/api/login-flows.md). 화면이 하는 일을 그대로 따라간다.
    Write-Host "3️⃣  OAuth authorize → 로그인 flow" -ForegroundColor Yellow
    $state = New-VerifySuffix
    $authUrl = "$issuer/oauth2/auth?client_id=$clientId&response_type=code&scope=openid&redirect_uri=http%3A%2F%2Flocalhost%3A9999%2Fverify-callback&state=verify$state"
    $r1 = Invoke-CurlCapture -CookieJar $cookieJar -CurlArgs @($authUrl)
    if (-not $r1.Location -or $r1.Location -notmatch 'login_challenge=') {
        Write-Host "❌ login_challenge를 얻지 못함 — status=$($r1.Status) location=$($r1.Location)" -ForegroundColor Red
        exit 1
    }
    $r1b = Invoke-CurlCapture -CookieJar $cookieJar -CurlArgs @($r1.Location)
    if (-not $r1b.Location -or $r1b.Location -notmatch '[?&]flow=([^&]+)') {
        Write-Host "❌ 로그인 flow id를 얻지 못함 — status=$($r1b.Status) location=$($r1b.Location)" -ForegroundColor Red
        exit 1
    }
    $loginFlow = [uri]::UnescapeDataString($matches[1])
    Write-Host "   ✅ 로그인 flow 획득" -ForegroundColor Green

    # --- 4. 비밀번호 로그인 제출 ---
    Write-Host "4️⃣  비밀번호 로그인 제출" -ForegroundColor Yellow
    $loginBody = @{ email = $verifyEmail; password = $VerifyPassword } | ConvertTo-Json -Compress
    $tmpBody = [System.IO.Path]::GetTempFileName()
    Set-Content -Path $tmpBody -Value $loginBody -NoNewline -Encoding UTF8
    $loginResp = & curl.exe -s -c $cookieJar -b $cookieJar -X POST "$apiUrl/api/v1/login-flows/$([uri]::EscapeDataString($loginFlow))/password" `
        -H "Content-Type: application/json" --data "@$tmpBody"
    Remove-Item $tmpBody -ErrorAction SilentlyContinue
    $loginJson = $loginResp | ConvertFrom-Json
    if ($loginJson.next -ne 'redirect' -or -not $loginJson.redirect_to) {
        Write-Host "❌ 로그인 실패: $loginResp" -ForegroundColor Red
        exit 1
    }
    Write-Host "   ✅ 로그인 accept됨" -ForegroundColor Green

    # --- 5. Hydra accept 리다이렉트를 따라가 동의 flow id 획득 ---
    Write-Host "5️⃣  로그인 accept → 동의 flow" -ForegroundColor Yellow
    $r2 = Invoke-CurlCapture -CookieJar $cookieJar -CurlArgs @($loginJson.redirect_to)
    if (-not $r2.Location -or $r2.Location -notmatch 'consent_challenge=') {
        Write-Host "❌ consent_challenge를 얻지 못함 — status=$($r2.Status) location=$($r2.Location)" -ForegroundColor Red
        exit 1
    }
    $r2b = Invoke-CurlCapture -CookieJar $cookieJar -CurlArgs @($r2.Location)
    if (-not $r2b.Location -or $r2b.Location -notmatch '[?&]flow=([^&]+)') {
        Write-Host "❌ 동의 flow id를 얻지 못함 — status=$($r2b.Status) location=$($r2b.Location)" -ForegroundColor Red
        exit 1
    }
    $consentFlow = [uri]::UnescapeDataString($matches[1])
    Write-Host "   ✅ 동의 flow 획득" -ForegroundColor Green

    # --- 6. consent accept ---
    Write-Host "6️⃣  consent accept" -ForegroundColor Yellow
    $consentBody = @{ grant_scope = @("openid") } | ConvertTo-Json -Compress
    $tmpBody = [System.IO.Path]::GetTempFileName()
    Set-Content -Path $tmpBody -Value $consentBody -NoNewline -Encoding UTF8
    $consentResp = & curl.exe -s -c $cookieJar -b $cookieJar -X POST "$apiUrl/api/v1/consent-flows/$([uri]::EscapeDataString($consentFlow))/accept" `
        -H "Content-Type: application/json" --data "@$tmpBody"
    Remove-Item $tmpBody -ErrorAction SilentlyContinue
    $consentJson = $consentResp | ConvertFrom-Json
    if ($consentJson.next -ne 'redirect' -or -not $consentJson.redirect_to) {
        Write-Host "❌ consent accept 실패: $consentResp" -ForegroundColor Red
        exit 1
    }

    # --- 7. 최종 콜백에서 authorization code 추출 ---
    $r3 = Invoke-CurlCapture -CookieJar $cookieJar -CurlArgs @($consentJson.redirect_to)
    if (-not $r3.Location -or $r3.Location -notmatch 'code=([^&]+)') {
        Write-Host "❌ authorization code를 얻지 못함 — status=$($r3.Status) location=$($r3.Location)" -ForegroundColor Red
        exit 1
    }
    $code = [uri]::UnescapeDataString($matches[1])
    Write-Host "   ✅ authorization code 발급 확인" -ForegroundColor Green

    # --- 8. 토큰 교환 ---
    Write-Host "7️⃣  authorization code → access token 교환" -ForegroundColor Yellow
    $tokenResp = & curl.exe -s -X POST "$issuer/oauth2/token" `
        -d "grant_type=authorization_code" -d "code=$code" `
        -d "redirect_uri=http://localhost:9999/verify-callback" -d "client_id=$clientId"
    $tokenJson = $tokenResp | ConvertFrom-Json
    if (-not $tokenJson.access_token) {
        Write-Host "❌ 토큰 교환 실패: $tokenResp" -ForegroundColor Red
        exit 1
    }
    Write-Host "   ✅ access_token 발급 확인 — 로그인→consent→콜백 전 구간 정상" -ForegroundColor Green

    # --- 8-1. 액세스 토큰의 클레임 형태 ---
    # Authway 자체 클레임은 최상위에 있어야 한다(계약 TokenClaims). 인가 서버 설정
    # (OAUTH2_ALLOWED_TOP_LEVEL_CLAIMS)이 빠진 채 배포되면 ext 안에만 남는다.
    $jwtParts = $tokenJson.access_token.Split('.')
    if ($jwtParts.Count -ne 3) {
        Write-Host "❌ 액세스 토큰이 JWT 가 아님 — client 의 access_token_strategy=jwt 가 반영되지 않음" -ForegroundColor Red
        exit 1
    }
    $b64 = $jwtParts[1].Replace('-', '+').Replace('_', '/')
    $b64 += '=' * ((4 - $b64.Length % 4) % 4)
    $claims = [System.Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($b64)) | ConvertFrom-Json
    if ($claims.tenant_id -ne $Tenant -or -not $claims.email -or -not $claims.auth_time) {
        Write-Host "❌ 액세스 토큰 최상위에 tenant_id/email/auth_time 이 없음 (tenant_id='$($claims.tenant_id)', auth_time='$($claims.auth_time)') — 인가 서버의 top-level 클레임 설정 확인" -ForegroundColor Red
        exit 1
    }
    Write-Host "   ✅ 액세스 토큰 최상위 클레임 확인 (tenant_id, email, auth_time)" -ForegroundColor Green

    # --- 9. 방금 로그인한 토큰으로 본인 계정 삭제 ---
    # 토큰의 auth_time(로그인 → 동의 → 토큰으로 이어지는 값)이 살아 있어야 200 이
    # 나온다 — 그 사슬 전체의 검증이다. 삭제는 인가 서버의 세션·토큰도 끝내므로
    # 검증용 user 의 정리도 겸한다(아래 finally 의 DB 삭제는 실패 시의 안전망).
    Write-Host "8️⃣  본인 계정 삭제 (DELETE /api/v1/profile/me)" -ForegroundColor Yellow
    $delMe = & curl.exe -s -o NUL -w "%{http_code}" -X DELETE "$apiUrl/api/v1/profile/me" -H "Authorization: Bearer $($tokenJson.access_token)"
    if ($delMe -ne '200') {
        Write-Host "❌ 본인 계정 삭제 실패(HTTP $delMe) — 방금 로그인한 토큰인데 거부됨" -ForegroundColor Red
        exit 1
    }
    $afterDelete = & curl.exe -s -o NUL -w "%{http_code}" "$apiUrl/api/v1/profile/me" -H "Authorization: Bearer $($tokenJson.access_token)"
    if ($afterDelete -ne '401') {
        Write-Host "❌ 삭제된 계정의 토큰이 아직 받아들여짐(HTTP $afterDelete)" -ForegroundColor Red
        exit 1
    }
    Write-Host "   ✅ 계정 삭제 200, 그 토큰은 이후 401" -ForegroundColor Green

} catch {
    Write-Host "❌ 예외 발생: $_" -ForegroundColor Red
    $failed = $true
} finally {
    Write-Host ""
    Write-Host "🧹 정리 중..." -ForegroundColor Gray
    if ($userCreated) {
        $deleteUserQuery = "DELETE FROM users WHERE email = '$verifyEmail';"
        $delResult = Invoke-FastQuery -Query $deleteUserQuery -EnvVars $envVars
        if (-not $delResult.Success) {
            Write-Host "⚠️  검증용 user 삭제 실패 — 수동 확인 필요: $verifyEmail ($($delResult.Output))" -ForegroundColor Yellow
        }
    }
    if ($clientDbId) {
        # PowerShell drops a $null argument to a native command, so "-o $null"
        # would make curl read "-X" as the output file and send a GET instead.
        $delCode = & curl.exe -s -o NUL -w "%{http_code}" -X DELETE "$apiUrl/api/v1/clients/$clientDbId" -H "Authorization: Bearer $adminKey"
        if ($delCode -notin @('200', '204')) {
            Write-Host "⚠️  검증용 client 삭제 실패(HTTP $delCode) — 수동 확인 필요: $clientId" -ForegroundColor Yellow
        }
    }
    Remove-Item $cookieJar -ErrorAction SilentlyContinue
    Write-Host "   완료" -ForegroundColor Gray
}

if ($failed) {
    exit 1
}

if (-not $SkipAuditSmoke) {
    Write-Host ""
    Write-Host "9️⃣  audit_logs smoke 이어서 실행" -ForegroundColor Yellow
    & (Join-Path $SharedDir "smoke-audit.ps1") -Target $Target -WindowMinutes 5 -WarnOnly
}

Write-Host ""
Write-Host "✅ OAuth 회귀 스모크 통과" -ForegroundColor Green
Write-Host "   (로그아웃 회귀는 자동화 범위 밖 — 사람이 확인. POST-DEPLOY-VERIFY.md §2 참조)" -ForegroundColor Gray
exit 0
