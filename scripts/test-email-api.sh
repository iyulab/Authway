#!/bin/bash

# Authway 이메일 API 테스트 스크립트
# 사용법: ./test-email-api.sh

API_URL="http://localhost:8080"
TEST_EMAIL="test@example.com"
TEST_PASSWORD="testpassword123"

echo "========================================="
echo "Authway 이메일 API 테스트"
echo "========================================="
echo ""

# 색상 정의
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 1. 서버 Health Check
echo "1. 서버 Health Check..."
HEALTH_RESPONSE=$(curl -s "${API_URL}/health")
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ 서버 정상 작동${NC}"
    echo "   Response: ${HEALTH_RESPONSE}"
else
    echo -e "${RED}✗ 서버 연결 실패${NC}"
    exit 1
fi
echo ""

# 2. 인증 이메일 발송 테스트
echo "2. 인증 이메일 발송 테스트..."
SEND_VERIFICATION_RESPONSE=$(curl -s -X POST "${API_URL}/api/email/send-verification" \
  -H "Content-Type: application/json" \
  -d "{\"email\": \"${TEST_EMAIL}\"}")

if echo "${SEND_VERIFICATION_RESPONSE}" | grep -q "successfully"; then
    echo -e "${GREEN}✓ 인증 이메일 발송 성공${NC}"
    echo "   Response: ${SEND_VERIFICATION_RESPONSE}"
    echo -e "${YELLOW}   MailHog에서 이메일을 확인하세요: http://localhost:8025${NC}"
else
    echo -e "${RED}✗ 인증 이메일 발송 실패${NC}"
    echo "   Response: ${SEND_VERIFICATION_RESPONSE}"
fi
echo ""

# 3. 토큰 입력 대기 (사용자가 MailHog에서 토큰 복사)
echo "3. 이메일 인증 테스트..."
echo -e "${YELLOW}   MailHog(http://localhost:8025)에서 인증 토큰을 복사하세요${NC}"
read -p "   인증 토큰을 입력하세요 (Enter로 스킵): " VERIFICATION_TOKEN

if [ -n "${VERIFICATION_TOKEN}" ]; then
    VERIFY_RESPONSE=$(curl -s "${API_URL}/api/email/verify?token=${VERIFICATION_TOKEN}")

    if echo "${VERIFY_RESPONSE}" | grep -q "successfully"; then
        echo -e "${GREEN}✓ 이메일 인증 성공${NC}"
        echo "   Response: ${VERIFY_RESPONSE}"
    else
        echo -e "${RED}✗ 이메일 인증 실패${NC}"
        echo "   Response: ${VERIFY_RESPONSE}"
    fi
else
    echo -e "${YELLOW}   이메일 인증 테스트 스킵${NC}"
fi
echo ""

# 4. 비밀번호 재설정 이메일 발송 테스트
echo "4. 비밀번호 재설정 이메일 발송 테스트..."
FORGOT_PASSWORD_RESPONSE=$(curl -s -X POST "${API_URL}/api/email/forgot-password" \
  -H "Content-Type: application/json" \
  -d "{\"email\": \"${TEST_EMAIL}\"}")

if echo "${FORGOT_PASSWORD_RESPONSE}" | grep -q "successfully"; then
    echo -e "${GREEN}✓ 비밀번호 재설정 이메일 발송 성공${NC}"
    echo "   Response: ${FORGOT_PASSWORD_RESPONSE}"
    echo -e "${YELLOW}   MailHog에서 이메일을 확인하세요: http://localhost:8025${NC}"
else
    echo -e "${RED}✗ 비밀번호 재설정 이메일 발송 실패${NC}"
    echo "   Response: ${FORGOT_PASSWORD_RESPONSE}"
fi
echo ""

# 5. 재설정 토큰 검증 테스트
echo "5. 재설정 토큰 검증 테스트..."
read -p "   재설정 토큰을 입력하세요 (Enter로 스킵): " RESET_TOKEN

if [ -n "${RESET_TOKEN}" ]; then
    VERIFY_RESET_TOKEN_RESPONSE=$(curl -s "${API_URL}/api/email/verify-reset-token?token=${RESET_TOKEN}")

    if echo "${VERIFY_RESET_TOKEN_RESPONSE}" | grep -q "valid"; then
        echo -e "${GREEN}✓ 재설정 토큰 검증 성공${NC}"
        echo "   Response: ${VERIFY_RESET_TOKEN_RESPONSE}"

        # 6. 비밀번호 재설정 테스트
        echo ""
        echo "6. 비밀번호 재설정 테스트..."
        NEW_PASSWORD="newpassword456"
        RESET_PASSWORD_RESPONSE=$(curl -s -X POST "${API_URL}/api/email/reset-password" \
          -H "Content-Type: application/json" \
          -d "{\"token\": \"${RESET_TOKEN}\", \"new_password\": \"${NEW_PASSWORD}\"}")

        if echo "${RESET_PASSWORD_RESPONSE}" | grep -q "successfully"; then
            echo -e "${GREEN}✓ 비밀번호 재설정 성공${NC}"
            echo "   Response: ${RESET_PASSWORD_RESPONSE}"
            echo -e "${YELLOW}   새 비밀번호: ${NEW_PASSWORD}${NC}"
        else
            echo -e "${RED}✗ 비밀번호 재설정 실패${NC}"
            echo "   Response: ${RESET_PASSWORD_RESPONSE}"
        fi
    else
        echo -e "${RED}✗ 재설정 토큰 검증 실패${NC}"
        echo "   Response: ${VERIFY_RESET_TOKEN_RESPONSE}"
    fi
else
    echo -e "${YELLOW}   재설정 토큰 검증 테스트 스킵${NC}"
fi
echo ""

# 7. 잘못된 토큰 테스트
echo "7. 보안 테스트 (잘못된 토큰)..."
INVALID_TOKEN="00000000-0000-0000-0000-000000000000"
INVALID_VERIFY_RESPONSE=$(curl -s "${API_URL}/api/email/verify?token=${INVALID_TOKEN}")

if echo "${INVALID_VERIFY_RESPONSE}" | grep -q "Invalid\|expired\|error"; then
    echo -e "${GREEN}✓ 잘못된 토큰 거부됨 (보안 정상)${NC}"
    echo "   Response: ${INVALID_VERIFY_RESPONSE}"
else
    echo -e "${RED}✗ 보안 취약점: 잘못된 토큰이 허용됨${NC}"
    echo "   Response: ${INVALID_VERIFY_RESPONSE}"
fi
echo ""

echo "========================================="
echo "테스트 완료!"
echo "========================================="
echo ""
echo "다음 단계:"
echo "1. MailHog UI 확인: http://localhost:8025"
echo "2. 데이터베이스 확인: psql -U authway -d authway"
echo "3. 프론트엔드 테스트: http://localhost:3001"
echo ""
