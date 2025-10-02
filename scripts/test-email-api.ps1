# Authway 이메일 API 테스트 스크립트 (PowerShell)
# 사용법: .\test-email-api.ps1

$API_URL = "http://localhost:8080"
$TEST_EMAIL = "test@example.com"
$TEST_PASSWORD = "testpassword123"

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host "Authway 이메일 API 테스트" -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan
Write-Host ""

# 1. 서버 Health Check
Write-Host "1. 서버 Health Check..." -ForegroundColor Yellow
try {
    $healthResponse = Invoke-RestMethod -Uri "$API_URL/health" -Method Get
    Write-Host "✓ 서버 정상 작동" -ForegroundColor Green
    Write-Host "   Response: $($healthResponse | ConvertTo-Json -Compress)"
}
catch {
    Write-Host "✗ 서버 연결 실패" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)"
    exit 1
}
Write-Host ""

# 2. 인증 이메일 발송 테스트
Write-Host "2. 인증 이메일 발송 테스트..." -ForegroundColor Yellow
try {
    $body = @{
        email = $TEST_EMAIL
    } | ConvertTo-Json

    $sendVerificationResponse = Invoke-RestMethod -Uri "$API_URL/api/email/send-verification" `
        -Method Post `
        -ContentType "application/json" `
        -Body $body

    Write-Host "✓ 인증 이메일 발송 성공" -ForegroundColor Green
    Write-Host "   Response: $($sendVerificationResponse | ConvertTo-Json -Compress)"
    Write-Host "   MailHog에서 이메일을 확인하세요: http://localhost:8025" -ForegroundColor Cyan
}
catch {
    Write-Host "✗ 인증 이메일 발송 실패" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)"
}
Write-Host ""

# 3. 이메일 인증 테스트
Write-Host "3. 이메일 인증 테스트..." -ForegroundColor Yellow
Write-Host "   MailHog(http://localhost:8025)에서 인증 토큰을 복사하세요" -ForegroundColor Cyan
$verificationToken = Read-Host "   인증 토큰을 입력하세요 (Enter로 스킵)"

if ($verificationToken) {
    try {
        $verifyResponse = Invoke-RestMethod -Uri "$API_URL/api/email/verify?token=$verificationToken" `
            -Method Get

        Write-Host "✓ 이메일 인증 성공" -ForegroundColor Green
        Write-Host "   Response: $($verifyResponse | ConvertTo-Json -Compress)"
    }
    catch {
        Write-Host "✗ 이메일 인증 실패" -ForegroundColor Red
        Write-Host "   Error: $($_.Exception.Message)"
    }
}
else {
    Write-Host "   이메일 인증 테스트 스킵" -ForegroundColor Yellow
}
Write-Host ""

# 4. 비밀번호 재설정 이메일 발송 테스트
Write-Host "4. 비밀번호 재설정 이메일 발송 테스트..." -ForegroundColor Yellow
try {
    $body = @{
        email = $TEST_EMAIL
    } | ConvertTo-Json

    $forgotPasswordResponse = Invoke-RestMethod -Uri "$API_URL/api/email/forgot-password" `
        -Method Post `
        -ContentType "application/json" `
        -Body $body

    Write-Host "✓ 비밀번호 재설정 이메일 발송 성공" -ForegroundColor Green
    Write-Host "   Response: $($forgotPasswordResponse | ConvertTo-Json -Compress)"
    Write-Host "   MailHog에서 이메일을 확인하세요: http://localhost:8025" -ForegroundColor Cyan
}
catch {
    Write-Host "✗ 비밀번호 재설정 이메일 발송 실패" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)"
}
Write-Host ""

# 5. 재설정 토큰 검증 테스트
Write-Host "5. 재설정 토큰 검증 테스트..." -ForegroundColor Yellow
$resetToken = Read-Host "   재설정 토큰을 입력하세요 (Enter로 스킵)"

if ($resetToken) {
    try {
        $verifyResetTokenResponse = Invoke-RestMethod -Uri "$API_URL/api/email/verify-reset-token?token=$resetToken" `
            -Method Get

        Write-Host "✓ 재설정 토큰 검증 성공" -ForegroundColor Green
        Write-Host "   Response: $($verifyResetTokenResponse | ConvertTo-Json -Compress)"

        # 6. 비밀번호 재설정 테스트
        Write-Host ""
        Write-Host "6. 비밀번호 재설정 테스트..." -ForegroundColor Yellow
        $newPassword = "newpassword456"

        try {
            $body = @{
                token = $resetToken
                new_password = $newPassword
            } | ConvertTo-Json

            $resetPasswordResponse = Invoke-RestMethod -Uri "$API_URL/api/email/reset-password" `
                -Method Post `
                -ContentType "application/json" `
                -Body $body

            Write-Host "✓ 비밀번호 재설정 성공" -ForegroundColor Green
            Write-Host "   Response: $($resetPasswordResponse | ConvertTo-Json -Compress)"
            Write-Host "   새 비밀번호: $newPassword" -ForegroundColor Cyan
        }
        catch {
            Write-Host "✗ 비밀번호 재설정 실패" -ForegroundColor Red
            Write-Host "   Error: $($_.Exception.Message)"
        }
    }
    catch {
        Write-Host "✗ 재설정 토큰 검증 실패" -ForegroundColor Red
        Write-Host "   Error: $($_.Exception.Message)"
    }
}
else {
    Write-Host "   재설정 토큰 검증 테스트 스킵" -ForegroundColor Yellow
}
Write-Host ""

# 7. 잘못된 토큰 테스트
Write-Host "7. 보안 테스트 (잘못된 토큰)..." -ForegroundColor Yellow
$invalidToken = "00000000-0000-0000-0000-000000000000"

try {
    $invalidVerifyResponse = Invoke-RestMethod -Uri "$API_URL/api/email/verify?token=$invalidToken" `
        -Method Get -ErrorAction Stop

    Write-Host "✗ 보안 취약점: 잘못된 토큰이 허용됨" -ForegroundColor Red
    Write-Host "   Response: $($invalidVerifyResponse | ConvertTo-Json -Compress)"
}
catch {
    Write-Host "✓ 잘못된 토큰 거부됨 (보안 정상)" -ForegroundColor Green
    Write-Host "   Error Message: $($_.Exception.Message)"
}
Write-Host ""

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host "테스트 완료!" -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "다음 단계:" -ForegroundColor Yellow
Write-Host "1. MailHog UI 확인: http://localhost:8025"
Write-Host "2. 데이터베이스 확인: psql -U authway -d authway"
Write-Host "3. 프론트엔드 테스트: http://localhost:3001"
Write-Host ""
