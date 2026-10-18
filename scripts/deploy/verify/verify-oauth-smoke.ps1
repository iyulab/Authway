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
#   verify/verify-oauth-smoke.ps1 -Target staging [-WithTerms]
#     — 약관 단계까지(-Tenant 를 주지 않으면 이것이 기본): 임시 테넌트를 만들어(실제 사용자가 있는 테넌트에 문서를 올리지
#       않으려고) 문서 둘을 올리고, 로그인이 `next: terms` 로 멈추는지 → 동의 →
#       동의 기록 → 나머지 전 구간. 끝나면 테넌트까지 지운다. -Tenant 는 무시된다.
#   verify/verify-oauth-smoke.ps1 -Target staging -WithDelegation
#     — 위임까지(docs/api/delegation.md): 검증용 client 를 기밀·token exchange·delegation
#       scope 로 만들고, 동의 → ID 토큰의 delegation_grant → discovery 의 token_endpoint 에서
#       교환 → 액세스 토큰의 sub·act·tenant_id·수명·리프레시 없음 → 계정 삭제 뒤 같은 handle 은
#       invalid_grant. 인가 서버의 토큰 훅(판정)과 클라이언트별 신뢰 등록까지 배포 환경에서 탄다.
#       배포가 승인 요청을 제공하면(capabilities.approval_requests) 승인 요청 → 같은 세션으로 계정 앱
#       로그인(PKCE) → 승인 → 폴링 토큰의 approval.id·act·authorization_details 까지.
#   verify/verify-oauth-smoke.ps1 -Target staging -Tenant <UUID> -Native [-Audience <API 식별자>]
#     — 데스크톱·CLI 앱의 모양(docs/api/client-management.md, RFC 8252): 공개 client 를 루프백
#       `http://127.0.0.1/callback` · authorization_code+refresh_token · offline_access · audience 로 만들고,
#       인가 요청은 운영체제가 고른 포트 + PKCE S256 → 액세스 토큰의 aud · 최상위 client_id → 리프레시가
#       새 리프레시 토큰을 주고 옛 것의 두 번째 사용은 거절(회전). 클레임 요약을 출력한다(리소스 서버에 건넬 모양).
# ============================================================

param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("prod", "staging")]
    [string]$Target,

    [string]$Tenant,

    # 약관 단계(docs/api/login-flows.md 「Terms」)까지 검증. 임시 테넌트를 쓴다.
    [switch]$WithTerms,

    # audit_logs smoke(_shared/smoke-audit.ps1)를 이어서 실행하지 않으려면.
    [switch]$SkipAuditSmoke,

    # 위임 교환(RFC 8693)까지 검증.
    [switch]$WithDelegation,

    # 데스크톱·CLI 앱 모양(공개 · 루프백 · PKCE · 리프레시 회전 · audience)으로 검증.
    [switch]$Native,

    # -Native 의 client 가 등록할 audience(리소스 서버 식별자).
    [string]$Audience = "https://api.verify.invalid"
)

$ErrorActionPreference = "Stop"

if ($Native -and $WithDelegation) {
    Write-Host "❌ -Native 와 -WithDelegation 은 함께 쓰지 않는다(위임 client 는 기밀이다)" -ForegroundColor Red
    exit 1
}

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

# 테넌트를 주지 않으면 임시 테넌트로(-WithTerms) — 실제 사용자가 있는 테넌트에 검증용 행을
# 만들지 않고, 끝나면 테넌트째 지운다.
if (-not $Tenant) { $WithTerms = $true }

# 이전 실행이 끊겨 finally 정리가 돌지 않았으면 검증용 자원이 남는다. 지우지는 않고 알린다 —
# 같은 이름 꼴의 자원이 다른 실행에서 진행 중일 수 있다.
$leftoverQuery = @"
SELECT 'tenants', count(*) FROM tenants WHERE slug LIKE 'verify-terms-%' AND deleted_at IS NULL
UNION ALL SELECT 'users', count(*) FROM users WHERE email LIKE 'verify-oauth-smoke-%@authway.invalid' AND deleted_at IS NULL
UNION ALL SELECT 'clients', count(*) FROM clients WHERE name LIKE 'verify-oauth-smoke-%' AND deleted_at IS NULL;
"@
$leftover = Invoke-FastQuery -Query $leftoverQuery -EnvVars $envVars
if (-not $leftover.Success) {
    Write-Host "⚠️  남은 검증 자원 조회 실패: $($leftover.Output)" -ForegroundColor Yellow
} else {
    $found = @()
    foreach ($line in ($leftover.Output -split "`n")) {
        $parts = $line.Trim() -split '\|'
        if ($parts.Length -eq 2 -and [int]$parts[1] -gt 0) { $found += "$($parts[0]) $($parts[1])" }
    }
    if ($found.Count -gt 0) {
        Write-Host "⚠️  이전 실행이 남긴 검증 자원: $($found -join ', ')" -ForegroundColor Yellow
        Write-Host "   (테넌트 slug 'verify-terms-*', 사용자 'verify-oauth-smoke-*@authway.invalid', 클라이언트 이름 'verify-oauth-smoke-*' — 진행 중인 다른 실행이 없으면 지울 것)" -ForegroundColor Yellow
    }
}

# 관리 API 호출(JSON 본문은 파일로 — Windows curl 의 따옴표 처리 회피).
function Invoke-AdminJson {
    param([string]$Method, [string]$Path, $Body)
    $curlArgs = @('-s', '-w', "`n%{http_code}", '-X', $Method, "$apiUrl$Path", '-H', "Authorization: Bearer $adminKey")
    $tmp = $null
    if ($null -ne $Body) {
        $tmp = [System.IO.Path]::GetTempFileName()
        Set-Content -Path $tmp -Value ($Body | ConvertTo-Json -Compress -Depth 5) -NoNewline -Encoding UTF8
        $curlArgs += @('-H', 'Content-Type: application/json', '--data', "@$tmp")
    }
    $out = & curl.exe @curlArgs
    if ($tmp) { Remove-Item $tmp -ErrorAction SilentlyContinue }
    $lines = ($out -join "`n") -split "`n"
    $status = $lines[-1]
    $text = ($lines[0..($lines.Count - 2)] -join "`n")
    $json = $null
    if ($text) { try { $json = $text | ConvertFrom-Json } catch { } }
    return @{ Status = $status; Text = $text; Json = $json }
}

$suffix = New-VerifySuffix
$termsTenantCreated = $false
$verifyEmail = "verify-oauth-smoke-$suffix@authway.invalid"
$cookieJar = [System.IO.Path]::GetTempFileName()
$clientId = $null
$clientDbId = $null
$clientSecret = $null
# 요청·동의하는 scope: 위임까지 보면 delegation 을 더한다.
# @(...) 로 감싼다: if 식은 원소 하나짜리 배열을 문자열로 풀어 scopes 가 JSON 문자열로 나간다.
$scopes = @(if ($WithDelegation) { "openid", "delegation" } elseif ($Native) { "openid", "profile", "email", "offline_access" } else { "openid" })
# 앱이 돌아올 주소. 데스크톱 앱은 포트 없는 루프백을 등록하고, 요청 때 운영체제가 고른 포트를 쓴다(RFC 8252 §7.3).
$redirectUri = "http://localhost:9999/verify-callback"
$codeVerifier = $null
if ($Native) {
    $redirectUri = "http://127.0.0.1:$(Get-Random -Minimum 49152 -Maximum 65535)/callback"
    # RFC 7636 §4.1: 43~128 자.
    $codeVerifier = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
}
$userCreated = $false
$failed = $false

try {
    # 배포 직후에는 첫 요청이 응답 없이 끊기는 일이 있다(리비전 전환). 검증용 자원을
    # 만들기 전에 API 가 답하는지부터 본다 — 생성 요청 자체는 다시 보내지 않는다.
    foreach ($attempt in 1..5) {
        $healthCode = & curl.exe -s -o NUL -w "%{http_code}" --max-time 10 "$apiUrl/health"
        if ($healthCode -eq '200') { break }
        Start-Sleep -Seconds 3
    }

    # --- 0. (-WithTerms) 임시 테넌트 + 문서 둘 ---
    if ($WithTerms) {
        Write-Host "0️⃣  임시 테넌트 + 약관 문서 둘" -ForegroundColor Yellow
        $t = Invoke-AdminJson -Method POST -Path "/api/v1/tenants" -Body @{ name = "verify-terms-$suffix"; slug = "verify-terms-$suffix" }
        if ($t.Status -ne '201' -or -not $t.Json.id) {
            Write-Host "❌ 임시 테넌트 생성 실패(HTTP $($t.Status)): $($t.Text)" -ForegroundColor Red
            exit 1
        }
        $Tenant = $t.Json.id
        $termsTenantCreated = $true
        foreach ($doc in @(@{ kind = 'terms_of_service'; version = "v-$suffix" }, @{ kind = 'privacy_policy'; version = "p-$suffix" })) {
            $p = Invoke-AdminJson -Method PUT -Path "/api/v1/tenants/$Tenant/terms/$($doc.kind)" -Body @{ version = $doc.version; url = "https://example.com/$($doc.kind)" }
            if ($p.Status -ne '201') {
                Write-Host "❌ 문서 올리기 실패($($doc.kind), HTTP $($p.Status)): $($p.Text)" -ForegroundColor Red
                exit 1
            }
        }
        Write-Host "   ✅ tenant=$Tenant, 문서 2" -ForegroundColor Green
    }

    # --- 1. 검증용 client 생성 ---
    Write-Host "1️⃣  검증용 client 생성" -ForegroundColor Yellow
    $createClientBody = @{
        tenant_id       = $Tenant
        name            = "verify-oauth-smoke-$suffix"
        public          = $true
        redirect_uris   = @("http://localhost:9999/verify-callback")
        allowed_origins = @("http://localhost:9999")
        grant_types     = @("authorization_code")
        scopes          = $scopes
        # JWT 액세스 토큰 — 아래에서 클레임 형태(계약의 TokenClaims)를 읽는다.
        access_token_strategy = "jwt"
    }
    if ($Native) {
        $createClientBody.redirect_uris = @("http://127.0.0.1/callback")
        $createClientBody.Remove('allowed_origins')
        $createClientBody.grant_types = @("authorization_code", "refresh_token")
        $createClientBody.audience = @($Audience)
    }
    if ($WithDelegation) {
        # 위임을 교환하는 client 는 기밀이다(자기 자격증명으로 교환한다).
        $createClientBody.public = $false
        $createClientBody.Remove('allowed_origins')
        $createClientBody.grant_types = @("authorization_code", "urn:ietf:params:oauth:grant-type:token-exchange")
    }
    $createClientBody = $createClientBody | ConvertTo-Json -Compress

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
    $clientSecret = $createParsed.credentials.client_secret
    Write-Host "   ✅ client_id=$clientId" -ForegroundColor Green

    # --- 2. 검증용 user 직접 생성 (admin API에 create 엔드포인트가 없음 —
    #     invitation-accept/social 로그인으로만 유저가 생기는 구조라 기존
    #     dogfooding 세션과 동일하게 DB 직접 INSERT로 우회) ---
    Write-Host "2️⃣  검증용 user 생성 ($verifyEmail)" -ForegroundColor Yellow
    $insertUserQuery = @"
INSERT INTO users (id, tenant_id, email, password_hash, name, email_verified, active)
VALUES (gen_random_uuid(), '$Tenant', '$verifyEmail', '$VerifyPasswordHash', 'post-deploy-verify', true, true);
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
    $authUrl = "$issuer/oauth2/auth?client_id=$clientId&response_type=code&scope=$($scopes -join '%20')&redirect_uri=$([uri]::EscapeDataString($redirectUri))&state=verify$state"
    if ($codeVerifier) {
        $sha = [System.Security.Cryptography.SHA256]::HashData([System.Text.Encoding]::ASCII.GetBytes($codeVerifier))
        $challenge = [Convert]::ToBase64String($sha).TrimEnd('=').Replace('+', '-').Replace('/', '_')
        $authUrl += "&code_challenge=$challenge&code_challenge_method=S256"
    }
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
    if ($WithTerms) {
        # 모든 요소 뒤, 수락 전에 약관 단계로 멈춰야 한다.
        if ($loginJson.next -ne 'terms' -or -not $loginJson.terms_challenge -or @($loginJson.terms).Count -ne 2) {
            Write-Host "❌ 약관 단계가 아님(문서 2 개를 기대): $loginResp" -ForegroundColor Red
            exit 1
        }
        Write-Host "   ✅ next=terms, 문서 $(@($loginJson.terms).Count) 개" -ForegroundColor Green
        $accepted = @($loginJson.terms | ForEach-Object { @{ kind = $_.kind; version = $_.version } })
        $acceptBody = @{ terms_challenge = $loginJson.terms_challenge; accepted = $accepted } | ConvertTo-Json -Compress -Depth 5
        $tmpBody = [System.IO.Path]::GetTempFileName()
        Set-Content -Path $tmpBody -Value $acceptBody -NoNewline -Encoding UTF8
        $loginResp = & curl.exe -s -c $cookieJar -b $cookieJar -X POST "$apiUrl/api/v1/login-flows/$([uri]::EscapeDataString($loginFlow))/terms" `
            -H "Content-Type: application/json" --data "@$tmpBody"
        Remove-Item $tmpBody -ErrorAction SilentlyContinue
        $loginJson = $loginResp | ConvertFrom-Json
        $users = Invoke-AdminJson -Method GET -Path "/api/v1/users?tenant_id=$Tenant&limit=10"
        $verifyUserId = @($users.Json.users | Where-Object { $_.email -eq $verifyEmail })[0].id
        $acc = Invoke-AdminJson -Method GET -Path "/api/v1/users/$verifyUserId/terms-acceptances"
        if ($acc.Status -ne '200' -or @($acc.Json.acceptances).Count -ne 2) {
            Write-Host "❌ 동의 기록이 둘이 아님(HTTP $($acc.Status)): $($acc.Text)" -ForegroundColor Red
            exit 1
        }
        Write-Host "   ✅ 약관 동의 → 기록 2" -ForegroundColor Green
    }
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
    # remember: 아래 8-2 의 prompt=none 이 code 로 답하려면 기억된 동의가 있어야 한다.
    $consentBody = @{ grant_scope = $scopes; remember = $true } | ConvertTo-Json -Compress
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
    # At the token endpoint discovery names, as a client finds it — the API's
    # front, which passes the grant on to the authorization server.
    $tokenEndpoint = (& curl.exe -s "$issuer/.well-known/openid-configuration" | ConvertFrom-Json).token_endpoint
    if (-not $tokenEndpoint) {
        Write-Host "❌ discovery 에 token_endpoint 가 없음" -ForegroundColor Red
        exit 1
    }
    Write-Host "   token_endpoint: $tokenEndpoint" -ForegroundColor Gray
    $codeArgs = @('-s', '-X', 'POST', $tokenEndpoint,
        '-d', 'grant_type=authorization_code', '-d', "code=$code",
        '--data-urlencode', "redirect_uri=$redirectUri", '-d', "client_id=$clientId")
    if ($codeVerifier) { $codeArgs += @('-d', "code_verifier=$codeVerifier") }
    if ($clientSecret) { $codeArgs += @('--data-urlencode', "client_secret=$clientSecret") }
    $tokenResp = & curl.exe @codeArgs
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

    # 서명 키가 발급자의 JWKS 에 공개돼 있어야 리소스 서버가 검증한다. 키를 교체한 뒤에는
    # 새 키로 서명하는지(kid), 옛 키를 지운 뒤에는 쓰는 키를 지우지 않았는지를 여기서 본다.
    $hb64 = $jwtParts[0].Replace('-', '+').Replace('_', '/')
    $hb64 += '=' * ((4 - $hb64.Length % 4) % 4)
    $tokenKid = ([System.Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($hb64)) | ConvertFrom-Json).kid
    $publishedKids = @((& curl.exe -s "$issuer/.well-known/jwks.json" | ConvertFrom-Json).keys | ForEach-Object { $_.kid })
    if ($publishedKids -notcontains $tokenKid) {
        Write-Host "❌ 액세스 토큰의 서명 키 kid=$tokenKid 가 JWKS 에 없음 (공개된 kid: $($publishedKids -join ', '))" -ForegroundColor Red
        exit 1
    }
    Write-Host "   ✅ 서명 키 kid=$tokenKid — JWKS 에 공개됨 (공개된 kid $($publishedKids.Count) 개)" -ForegroundColor Green

    # --- 8-1b. (-Native) 리소스 서버가 보는 것 · 리프레시 회전 ---
    if ($Native) {
        Write-Host "7️⃣-1b 데스크톱 앱 모양 — aud · client_id · 리프레시 회전" -ForegroundColor Yellow
        $tokenAlg = ([System.Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($hb64)) | ConvertFrom-Json).alg
        if (@($claims.aud) -notcontains $Audience) {
            Write-Host "❌ 액세스 토큰 aud 에 $Audience 가 없음 (aud='$(@($claims.aud) -join ',')') — 등록한 audience 가 실리지 않음" -ForegroundColor Red
            exit 1
        }
        if ($claims.client_id -ne $clientId) {
            Write-Host "❌ 액세스 토큰 최상위 client_id 가 '$($claims.client_id)' — '$clientId' 여야 함" -ForegroundColor Red
            exit 1
        }
        if (-not $tokenJson.refresh_token) {
            Write-Host "❌ offline_access 를 동의했는데 리프레시 토큰이 없음" -ForegroundColor Red
            exit 1
        }
        $refreshOnce = { param($rt) & curl.exe -s -X POST $tokenEndpoint -d 'grant_type=refresh_token' --data-urlencode "refresh_token=$rt" -d "client_id=$clientId" | ConvertFrom-Json }
        $renewed = & $refreshOnce $tokenJson.refresh_token
        if (-not $renewed.access_token -or -not $renewed.refresh_token -or $renewed.refresh_token -eq $tokenJson.refresh_token) {
            Write-Host "❌ 리프레시가 새 액세스·리프레시 토큰을 주지 않음: $($renewed | ConvertTo-Json -Compress)" -ForegroundColor Red
            exit 1
        }
        $reused = & $refreshOnce $tokenJson.refresh_token
        if ($reused.error -ne 'invalid_grant') {
            Write-Host "❌ 이미 쓴 리프레시 토큰의 두 번째 사용이 거절되지 않음: $($reused | ConvertTo-Json -Compress)" -ForegroundColor Red
            exit 1
        }
        # 재사용을 탈취로 보고 그 계열을 끝낸다 — 방금 받은 새 액세스 토큰도 더는 받아들여지지 않아야 한다.
        $renewedNow = & curl.exe -s -o NUL -w "%{http_code}" "$apiUrl/api/v1/profile/me" -H "Authorization: Bearer $($renewed.access_token)"
        if ($renewedNow -ne '401') {
            Write-Host "❌ 재사용 감지 뒤에도 그 계열의 새 액세스 토큰이 받아들여짐(HTTP $renewedNow)" -ForegroundColor Red
            exit 1
        }
        Write-Host "   ✅ aud ∋ $Audience · client_id 최상위 · 리프레시는 새 토큰, 옛 토큰 재사용은 invalid_grant 이고 그 계열이 끝남" -ForegroundColor Green
        Write-Host "   리소스 서버가 받는 액세스 토큰:" -ForegroundColor Gray
        Write-Host "     iss=$($claims.iss)  alg=$tokenAlg  kid=$tokenKid" -ForegroundColor Gray
        Write-Host "     sub=<사용자 id, UUID>  aud=$(@($claims.aud) -join ',')  client_id=$($claims.client_id)" -ForegroundColor Gray
        Write-Host "     수명(exp-iat)=$($claims.exp - $claims.iat)초  scope='$($claims.scope)'  최상위: $(($claims.PSObject.Properties.Name | Sort-Object) -join ',')" -ForegroundColor Gray
        if ($claims.ext) {
            $extReport = @($claims.ext.PSObject.Properties | ForEach-Object {
                $same = ($claims.PSObject.Properties.Name -contains $_.Name) -and (($claims.($_.Name) | ConvertTo-Json -Compress) -eq ($_.Value | ConvertTo-Json -Compress))
                "$($_.Name)$(if ($same) { '=최상위' } else { '≠최상위' })"
            })
            Write-Host "     ext: $($extReport -join ', ')" -ForegroundColor Gray
        }
    }

    # --- 8-2. 로그인이 세션을 열었는가 ---
    # 「로그인 상태 유지」 없이 로그인했어도 세션이 있어야 한다: 같은 브라우저(쿠키)의
    # prompt=none 인가 요청이 화면 없이 code 로 끝난다. 세션이 없으면 login_required,
    # 화면이 폼을 요구하면 그것도 실패다(docs/api/login-flows.md 「Sessions」).
    # 보여 줄 것이 없는 흐름은 로그인 UI 로 보내지지도 않아야 한다(같은 문서 「Flow ids」):
    # 리다이렉트만으로 끝나지 않고 화면 주소(flow=)를 거치면 실패.
    Write-Host "7️⃣-2 세션 재사용 (prompt=none)" -ForegroundColor Yellow
    $silentUrl = "$authUrl&prompt=none"
    $silentOutcome = $null
    $screensOpened = 0
    foreach ($hop in 1..10) {
        $rs = Invoke-CurlCapture -CookieJar $cookieJar -CurlArgs @($silentUrl)
        if (-not $rs.Location) { $silentOutcome = "리다이렉트 없음(status=$($rs.Status))"; break }
        if ($rs.Location -match '[?&]error=([^&]+)') { $silentOutcome = "error=$($matches[1])"; break }
        if ($rs.Location -match '[?&]code=([^&]+)') { $silentOutcome = 'code'; $silentCode = [uri]::UnescapeDataString($matches[1]); break }
        if ($rs.Location -match '[?&]flow=([^&]+)') {
            # 화면이 하는 일: flow 가 무엇을 보여 달라는지 API 에 묻는다. 답은 redirect 여야 한다.
            $silentFlow = $matches[1]
            $screensOpened++
            $kind = if ($rs.Location -match '/consent\?') { 'consent-flows' } else { 'login-flows' }
            $stepResp = & curl.exe -s -c $cookieJar -b $cookieJar "$apiUrl/api/v1/$kind/$silentFlow"
            $stepJson = $stepResp | ConvertFrom-Json
            if ($stepJson.next -ne 'redirect' -or -not $stepJson.redirect_to) { $silentOutcome = "$kind 가 폼을 요구함: $stepResp"; break }
            $silentUrl = $stepJson.redirect_to
        } else {
            $silentUrl = $rs.Location
        }
    }
    if ($silentOutcome -ne 'code') {
        Write-Host "❌ 세션이 재사용되지 않음 — prompt=none 의 답: $silentOutcome" -ForegroundColor Red
        exit 1
    }
    if ($screensOpened -gt 0) {
        Write-Host "❌ 세션은 재사용됐으나 로그인 UI 를 $screensOpened 번 거침 — 화면이 필요 없는 흐름은 API 진입점에서 끝나야 한다" -ForegroundColor Red
        exit 1
    }
    Write-Host "   ✅ 화면 없이 code — 로그인이 세션을 열었고, 로그인 UI 를 거치지 않고 재사용됨" -ForegroundColor Green
    if ($Native) {
        # 위의 재사용 감지가 첫 토큰 계열을 끝냈다. 아래 단계는 세션에서 새로 받은 토큰으로 한다.
        $tokenJson = & curl.exe -s -X POST $tokenEndpoint -d 'grant_type=authorization_code' -d "code=$silentCode" `
            --data-urlencode "redirect_uri=$redirectUri" -d "client_id=$clientId" -d "code_verifier=$codeVerifier" | ConvertFrom-Json
        if (-not $tokenJson.access_token) {
            Write-Host "❌ 세션 재사용의 code 를 토큰으로 바꾸지 못함: $($tokenJson | ConvertTo-Json -Compress)" -ForegroundColor Red
            exit 1
        }
    }

    # --- 8-3. (-WithDelegation) 위임 교환 ---
    $exchangeArgs = $null
    if ($WithDelegation) {
        Write-Host "7️⃣-3 위임 교환 (RFC 8693)" -ForegroundColor Yellow
        $idParts = "$($tokenJson.id_token)".Split('.')
        $ib64 = $idParts[1].Replace('-', '+').Replace('_', '/')
        $ib64 += '=' * ((4 - $ib64.Length % 4) % 4)
        $idClaims = [System.Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($ib64)) | ConvertFrom-Json
        $handle = $idClaims.delegation_grant
        if (-not $handle) {
            Write-Host "❌ ID 토큰에 delegation_grant 가 없음 — delegation 동의가 위임을 만들지 않음" -ForegroundColor Red
            exit 1
        }
        $exchangeArgs = @('-s', '-X', 'POST', $tokenEndpoint,
            '--data-urlencode', 'grant_type=urn:ietf:params:oauth:grant-type:token-exchange',
            '--data-urlencode', "subject_token=$handle",
            '--data-urlencode', 'subject_token_type=urn:authway:params:oauth:token-type:delegation-grant',
            '--data-urlencode', "client_id=$clientId",
            '--data-urlencode', "client_secret=$clientSecret")
        # 수명 5 분 — 시각이 초 단위라 차이가 1 초 올라갈 수 있다(아래 301).
        $exResp = & curl.exe @exchangeArgs
        $ex = $exResp | ConvertFrom-Json
        if (-not $ex.access_token -or $ex.issued_token_type -ne 'urn:ietf:params:oauth:token-type:access_token' -or $ex.refresh_token) {
            Write-Host "❌ 교환 실패 또는 모양이 다름: $exResp" -ForegroundColor Red
            exit 1
        }
        $xParts = $ex.access_token.Split('.')
        $xb64 = $xParts[1].Replace('-', '+').Replace('_', '/')
        $xb64 += '=' * ((4 - $xb64.Length % 4) % 4)
        $x = [System.Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($xb64)) | ConvertFrom-Json
        if ($x.sub -ne $claims.sub -or $x.act.sub -ne $clientId -or $x.tenant_id -ne $Tenant -or ($x.exp - $x.iat) -gt 301) {
            Write-Host "❌ 교환된 토큰의 클레임이 다름 (sub='$($x.sub)' act.sub='$($x.act.sub)' tenant_id='$($x.tenant_id)' 수명=$($x.exp - $x.iat)) — 토큰 훅 판정·top-level act 설정 확인" -ForegroundColor Red
            exit 1
        }
        Write-Host "   ✅ 교환 — sub=사용자, act.sub=client, tenant_id, 수명 $($x.exp - $x.iat)s, 리프레시 없음" -ForegroundColor Green
        # 그 토큰은 client 의 것이지 사용자의 것이 아니다: 사용자 계정 경로는 거절한다.
        $own = & curl.exe -s -o NUL -w "%{http_code}" "$apiUrl/api/v1/users/delegations" -H "Authorization: Bearer $($ex.access_token)"
        if ($own -ne '403') {
            Write-Host "❌ 교환된 토큰이 사용자 계정 경로에서 받아들여짐(HTTP $own) — 403 delegated_token 이어야 한다" -ForegroundColor Red
            exit 1
        }
        Write-Host "   ✅ 교환된 토큰으로 계정 경로 → 403" -ForegroundColor Green

        # --- 8-4. 승인 요청 (배포가 제공할 때) ---
        # client 가 승인을 요청 → 사용자가 계정 앱으로 로그인(같은 브라우저 세션, 화면 없이)
        # 해 승인 → client 의 폴링이 승인에 묶인 토큰을 받는다(docs/api/delegation.md §5).
        $caps = & curl.exe -s "$apiUrl/api/v1/capabilities" | ConvertFrom-Json
        if ($caps.approval_requests -eq $true) {
            Write-Host "7️⃣-4 승인 요청 → 사용자 승인 → 폴링" -ForegroundColor Yellow
            $askResp = & curl.exe -s -X POST "$apiUrl/oauth2/approval-requests" `
                --data-urlencode "subject_token=$handle" `
                --data-urlencode 'subject_token_type=urn:authway:params:oauth:token-type:delegation-grant' `
                --data-urlencode "binding_message=V-$suffix" `
                --data-urlencode 'authorization_details=[{"type":"verify","ref":"smoke"}]' `
                --data-urlencode "client_id=$clientId" --data-urlencode "client_secret=$clientSecret"
            $ask = $askResp | ConvertFrom-Json
            if (-not $ask.auth_req_id -or -not $ask.approve_url) {
                Write-Host "❌ 승인 요청 실패: $askResp" -ForegroundColor Red
                exit 1
            }
            $approvalId = if ($ask.approve_url -match '[?&]request=([^&]+)') { [uri]::UnescapeDataString($matches[1]) } else { $null }
            $approveClient = if ($ask.approve_url -match '[?&]client_id=([^&]+)') { [uri]::UnescapeDataString($matches[1]) } else { $null }
            if (-not $approvalId -or $approveClient -ne $clientId) {
                Write-Host "❌ approve_url 에 client_id 가 없음: $($ask.approve_url)" -ForegroundColor Red
                exit 1
            }

            # 계정 앱 로그인 — 승인 화면이 하는 일(PKCE, 이미 열린 세션).
            $entry = & curl.exe -s "$apiUrl/api/v1/account-entry?client_id=$([uri]::EscapeDataString($clientId))" | ConvertFrom-Json
            # PKCE: 43 자 이상(RFC 7636 §4.1).
            $verifier = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
            $sha = [System.Security.Cryptography.SHA256]::Create()
            $challenge = [Convert]::ToBase64String($sha.ComputeHash([System.Text.Encoding]::ASCII.GetBytes($verifier))).TrimEnd('=').Replace('+', '-').Replace('/', '_')
            $accountUrl = "$issuer/oauth2/auth?client_id=$([uri]::EscapeDataString($entry.client_id))&response_type=code&scope=$([uri]::EscapeDataString($entry.scope))&redirect_uri=$([uri]::EscapeDataString($entry.redirect_uri))&state=a$(New-VerifySuffix)&code_challenge=$challenge&code_challenge_method=S256"
            $accountCode = $null
            foreach ($hop in 1..10) {
                $ra = Invoke-CurlCapture -CookieJar $cookieJar -CurlArgs @($accountUrl)
                if (-not $ra.Location) { break }
                if ($ra.Location -match '[?&]code=([^&]+)') { $accountCode = [uri]::UnescapeDataString($matches[1]); break }
                if ($ra.Location -match '[?&]error=') { break }
                if ($ra.Location -match '[?&]flow=([^&]+)') {
                    $kind = if ($ra.Location -match '/consent\?') { 'consent-flows' } else { 'login-flows' }
                    $step = & curl.exe -s -c $cookieJar -b $cookieJar "$apiUrl/api/v1/$kind/$($matches[1])" | ConvertFrom-Json
                    if ($step.next -ne 'redirect') { break }
                    $accountUrl = $step.redirect_to
                } else {
                    $accountUrl = $ra.Location
                }
            }
            if (-not $accountCode) {
                Write-Host "❌ 계정 앱 로그인이 code 로 끝나지 않음 (마지막 주소: $($ra.Location))" -ForegroundColor Red
                exit 1
            }
            $accountResp = & curl.exe -s -X POST $tokenEndpoint -d 'grant_type=authorization_code' --data-urlencode "code=$accountCode" `
                --data-urlencode "redirect_uri=$($entry.redirect_uri)" --data-urlencode "client_id=$($entry.client_id)" --data-urlencode "code_verifier=$verifier"
            $accountToken = ($accountResp | ConvertFrom-Json).access_token
            if (-not $accountToken) {
                Write-Host "❌ 계정 앱 code 교환 실패: $accountResp" -ForegroundColor Red
                exit 1
            }
            $seen = & curl.exe -s "$apiUrl/api/v1/users/approvals/$approvalId" -H "Authorization: Bearer $accountToken" | ConvertFrom-Json
            if ($seen.binding_message -ne "V-$suffix" -or $seen.status -ne 'pending') {
                Write-Host "❌ 사용자가 보는 승인 요청이 다름: binding='$($seen.binding_message)' status='$($seen.status)'" -ForegroundColor Red
                exit 1
            }
            $approved = & curl.exe -s -o NUL -w "%{http_code}" -X POST "$apiUrl/api/v1/users/approvals/$approvalId/approve" -H "Authorization: Bearer $accountToken"
            if ($approved -ne '204') {
                Write-Host "❌ 승인 실패(HTTP $approved)" -ForegroundColor Red
                exit 1
            }
            Start-Sleep -Seconds ([int]$ask.interval + 1)
            $pollResp = & curl.exe -s -X POST $tokenEndpoint `
                --data-urlencode 'grant_type=urn:authway:params:oauth:grant-type:approval' `
                --data-urlencode "auth_req_id=$($ask.auth_req_id)" `
                --data-urlencode "client_id=$clientId" --data-urlencode "client_secret=$clientSecret"
            $polled = $pollResp | ConvertFrom-Json
            if (-not $polled.access_token) {
                Write-Host "❌ 승인 뒤 폴링이 토큰을 내지 않음: $pollResp" -ForegroundColor Red
                exit 1
            }
            $pParts = $polled.access_token.Split('.')
            $pb64 = $pParts[1].Replace('-', '+').Replace('_', '/')
            $pb64 += '=' * ((4 - $pb64.Length % 4) % 4)
            $p = [System.Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($pb64)) | ConvertFrom-Json
            if ($p.approval.id -ne $approvalId -or $p.act.sub -ne $clientId -or $p.sub -ne $claims.sub -or @($p.authorization_details)[0].type -ne 'verify') {
                Write-Host "❌ 승인 토큰의 클레임이 다름 (approval.id='$($p.approval.id)' act.sub='$($p.act.sub)') — 토큰 훅·top-level approval/authorization_details 설정 확인" -ForegroundColor Red
                exit 1
            }
            Write-Host "   ✅ 승인 → 토큰(approval.id, act.sub, authorization_details)" -ForegroundColor Green
        } else {
            Write-Host "   (승인 요청은 이 배포가 제공하지 않음 — capabilities.approval_requests)" -ForegroundColor Gray
        }
    }

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

    if ($exchangeArgs) {
        $afterResp = & curl.exe @exchangeArgs
        if (($afterResp | ConvertFrom-Json).error -ne 'invalid_grant') {
            Write-Host "❌ 삭제된 계정의 위임이 아직 교환됨: $afterResp" -ForegroundColor Red
            exit 1
        }
        Write-Host "   ✅ 삭제된 계정의 위임은 invalid_grant" -ForegroundColor Green
    }

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
    if ($termsTenantCreated) {
        $delTenant = Invoke-AdminJson -Method DELETE -Path "/api/v1/tenants/$Tenant"
        if ($delTenant.Status -notin @('200', '204')) {
            Write-Host "⚠️  임시 테넌트 삭제 실패(HTTP $($delTenant.Status)) — 수동 확인 필요: $Tenant $($delTenant.Text)" -ForegroundColor Yellow
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
