# Authway 로컬 테스트 가이드

이 가이드는 Authway의 모든 기능을 로컬 환경에서 테스트하는 방법을 설명합니다.

## 목차
1. [사전 준비](#사전-준비)
2. [환경 설정](#환경-설정)
3. [데이터베이스 설정](#데이터베이스-설정)
4. [이메일 서버 설정](#이메일-서버-설정)
5. [백엔드 실행](#백엔드-실행)
6. [프론트엔드 실행](#프론트엔드-실행)
7. [기능 테스트](#기능-테스트)

---

## 사전 준비

### 필수 소프트웨어

1. **Go 1.21+**
   ```bash
   go version
   ```

2. **Node.js 18+**
   ```bash
   node --version
   npm --version
   ```

3. **PostgreSQL 14+**
   ```bash
   psql --version
   ```

4. **Redis**
   ```bash
   redis-cli --version
   ```

5. **Ory Hydra** (OAuth 2.0 서버)
   ```bash
   hydra version
   ```

### 설치 방법 (Windows)

**PostgreSQL:**
```bash
# Chocolatey 사용
choco install postgresql

# 또는 공식 사이트에서 다운로드
# https://www.postgresql.org/download/windows/
```

**Redis:**
```bash
# Windows용 Redis는 WSL2 사용 권장
wsl --install
wsl
sudo apt update
sudo apt install redis-server
redis-server
```

**Ory Hydra:**
```bash
# Docker 사용 (권장)
docker pull oryd/hydra:v2.2.0
```

---

## 환경 설정

### 1. .env 파일 생성

프로젝트 루트에 `.env` 파일을 생성하고 다음 내용을 입력합니다:

```bash
# .env 파일 복사
cp .env.example .env
```

### 2. 환경 변수 설정

`.env` 파일을 열고 다음 항목들을 설정합니다:

```env
# Application
AUTHWAY_APP_NAME=Authway
AUTHWAY_APP_VERSION=1.0.0
AUTHWAY_APP_ENVIRONMENT=development
AUTHWAY_APP_PORT=8080
AUTHWAY_APP_BASE_URL=http://localhost:8080

# Database (PostgreSQL)
AUTHWAY_DATABASE_HOST=localhost
AUTHWAY_DATABASE_PORT=5432
AUTHWAY_DATABASE_USER=authway
AUTHWAY_DATABASE_PASSWORD=authway
AUTHWAY_DATABASE_NAME=authway
AUTHWAY_DATABASE_SSL_MODE=disable

# Redis
AUTHWAY_REDIS_HOST=localhost
AUTHWAY_REDIS_PORT=6379
AUTHWAY_REDIS_PASSWORD=
AUTHWAY_REDIS_DB=0

# JWT Configuration
AUTHWAY_JWT_ACCESS_TOKEN_SECRET=your-secret-key-change-in-production
AUTHWAY_JWT_REFRESH_TOKEN_SECRET=your-refresh-secret-key-change-in-production
AUTHWAY_JWT_ACCESS_TOKEN_EXPIRY=15m
AUTHWAY_JWT_REFRESH_TOKEN_EXPIRY=7d
AUTHWAY_JWT_ISSUER=authway

# OAuth Configuration
AUTHWAY_OAUTH_AUTHORIZE_CODE_EXPIRY=10m
AUTHWAY_OAUTH_ALLOWED_GRANT_TYPES=authorization_code,refresh_token
AUTHWAY_OAUTH_ALLOWED_SCOPES=openid,profile,email
AUTHWAY_OAUTH_REQUIRE_PKCE=true

# CORS
AUTHWAY_CORS_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:3001

# Email (SMTP) - MailHog 사용 (로컬 테스트용)
AUTHWAY_EMAIL_SMTP_HOST=localhost
AUTHWAY_EMAIL_SMTP_PORT=1025
AUTHWAY_EMAIL_SMTP_USER=
AUTHWAY_EMAIL_SMTP_PASSWORD=
AUTHWAY_EMAIL_FROM_EMAIL=noreply@authway.dev
AUTHWAY_EMAIL_FROM_NAME=Authway
```

---

## 데이터베이스 설정

### 1. PostgreSQL 시작

```bash
# Windows (서비스로 실행 중이면 생략)
# 시작: net start postgresql-x64-14
# 중지: net stop postgresql-x64-14

# WSL2/Linux
sudo service postgresql start
```

### 2. 데이터베이스 및 사용자 생성

```bash
# PostgreSQL에 접속
psql -U postgres

# 데이터베이스 생성
CREATE DATABASE authway;

# 사용자 생성
CREATE USER authway WITH PASSWORD 'authway';

# 권한 부여
GRANT ALL PRIVILEGES ON DATABASE authway TO authway;

# 종료
\q
```

### 3. 데이터베이스 확인

```bash
psql -U authway -d authway -c "SELECT version();"
```

---

## 이메일 서버 설정

로컬 개발을 위해 **MailHog**를 사용합니다. MailHog는 이메일을 실제로 발송하지 않고 캡처하여 웹 UI로 보여주는 도구입니다.

### MailHog 설치 및 실행

**방법 1: Docker 사용 (권장)**

```bash
docker run -d -p 1025:1025 -p 8025:8025 mailhog/mailhog
```

**방법 2: 바이너리 다운로드**

```bash
# Windows
# https://github.com/mailhog/MailHog/releases 에서 다운로드

# 실행
./MailHog_windows_amd64.exe
```

**방법 3: Go 설치**

```bash
go install github.com/mailhog/MailHog@latest
MailHog
```

### MailHog 접속

브라우저에서 다음 주소로 접속:
```
http://localhost:8025
```

이 UI에서 발송된 모든 이메일을 확인할 수 있습니다.

---

## 백엔드 실행

### 1. 의존성 설치

```bash
cd src/server
go mod download
```

### 2. 서버 실행

```bash
# 개발 모드
go run cmd/main.go

# 또는 빌드 후 실행
go build -o authway.exe cmd/main.go
./authway.exe
```

### 3. 서버 확인

```bash
# Health check
curl http://localhost:8080/health
```

예상 응답:
```json
{
  "status": "ok",
  "service": "authway",
  "version": "1.0.0",
  "timestamp": "2024-10-02T..."
}
```

### 4. API 엔드포인트 확인

백엔드가 다음 엔드포인트를 제공하는지 확인:

- `POST /register` - 회원가입
- `POST /api/email/send-verification` - 인증 이메일 발송
- `GET /api/email/verify` - 이메일 인증
- `POST /api/email/forgot-password` - 비밀번호 재설정 요청
- `GET /api/email/verify-reset-token` - 재설정 토큰 검증
- `POST /api/email/reset-password` - 비밀번호 재설정

---

## 프론트엔드 실행

### 1. Login UI 실행

```bash
cd packages/web/login-ui
npm install
npm run dev
```

Login UI가 `http://localhost:3001`에서 실행됩니다.

### 2. Admin Dashboard 실행 (선택사항)

```bash
cd packages/web/admin-dashboard
npm install
npm run dev
```

Admin Dashboard가 `http://localhost:3000`에서 실행됩니다.

### 3. 프론트엔드 확인

브라우저에서 다음 주소로 접속:
- Login UI: http://localhost:3001
- Admin Dashboard: http://localhost:3000

---

## 기능 테스트

### 1. 회원가입 및 이메일 인증

#### Step 1: 회원가입
1. http://localhost:3001/register 접속
2. 다음 정보 입력:
   - 이메일: test@example.com
   - 비밀번호: testpassword123
   - 이름: Test User
3. "회원가입" 버튼 클릭

#### Step 2: 인증 이메일 확인
1. MailHog UI 접속: http://localhost:8025
2. "Authway - 이메일 인증" 제목의 이메일 확인
3. 이메일 내의 인증 링크 복사

#### Step 3: 이메일 인증
1. 복사한 링크를 브라우저에 붙여넣기
2. "인증 완료!" 메시지 확인
3. "로그인하기" 버튼 클릭

### 2. 인증 이메일 재발송

#### Step 1: 재발송 페이지 접속
1. http://localhost:3001/resend-verification 접속

#### Step 2: 이메일 입력
1. 가입한 이메일 주소 입력 (test@example.com)
2. "인증 이메일 보내기" 버튼 클릭

#### Step 3: 이메일 확인
1. MailHog UI에서 새 인증 이메일 확인
2. 링크를 통해 인증 완료

### 3. 비밀번호 재설정

#### Step 1: 비밀번호 찾기
1. http://localhost:3001/forgot-password 접속
2. 이메일 주소 입력 (test@example.com)
3. "재설정 링크 보내기" 버튼 클릭

#### Step 2: 재설정 이메일 확인
1. MailHog UI 접속
2. "Authway - 비밀번호 재설정" 이메일 확인
3. 재설정 링크 복사

#### Step 3: 새 비밀번호 설정
1. 복사한 링크를 브라우저에 붙여넣기
2. 새 비밀번호 입력 (최소 8자)
3. 비밀번호 확인 입력
4. "비밀번호 변경" 버튼 클릭
5. "재설정 완료!" 메시지 확인

#### Step 4: 새 비밀번호로 로그인
1. 자동으로 로그인 페이지로 이동
2. 이메일과 새 비밀번호로 로그인 시도
3. 로그인 성공 확인

### 4. 토큰 만료 테스트

#### 인증 토큰 만료 (6시간 후)
1. 인증 이메일 발송
2. 6시간 이상 대기 (또는 DB에서 직접 만료 시간 수정)
3. 만료된 토큰으로 인증 시도
4. "토큰이 만료되었습니다" 오류 메시지 확인

#### 재설정 토큰 만료 (1시간 후)
1. 비밀번호 재설정 이메일 발송
2. 1시간 이상 대기
3. 만료된 토큰으로 재설정 시도
4. "토큰이 만료되었거나 유효하지 않습니다" 오류 확인

### 5. React SDK 테스트

#### Step 1: SDK 빌드
```bash
cd packages/sdk/react
npm install
npm run build
```

#### Step 2: 예제 앱 실행
```bash
# 기본 예제
cd examples/basic
npm install
npm run dev
```

#### Step 3: SDK 기능 테스트
1. 예제 앱 접속 (http://localhost:5173)
2. "Login with Authway" 버튼 클릭
3. OAuth 플로우 진행
4. 로그인 후 사용자 정보 표시 확인
5. "Logout" 버튼 클릭
6. 로그아웃 확인

---

## 데이터베이스 직접 확인

### 이메일 인증 데이터 확인

```sql
-- PostgreSQL에 접속
psql -U authway -d authway

-- 이메일 인증 데이터 확인
SELECT * FROM email_verifications ORDER BY created_at DESC LIMIT 5;

-- 컬럼 설명:
-- id: UUID
-- user_id: 사용자 ID
-- token: 인증 토큰 (UUID)
-- expires_at: 만료 시간 (6시간)
-- verified: 인증 완료 여부
-- created_at: 생성 시간
```

### 비밀번호 재설정 데이터 확인

```sql
-- 비밀번호 재설정 데이터 확인
SELECT * FROM password_resets ORDER BY created_at DESC LIMIT 5;

-- 컬럼 설명:
-- id: UUID
-- user_id: 사용자 ID
-- token: 재설정 토큰 (UUID)
-- expires_at: 만료 시간 (1시간)
-- used: 사용 여부
-- created_at: 생성 시간
```

### 사용자 데이터 확인

```sql
-- 사용자 확인
SELECT id, email, name, email_verified, created_at
FROM users
ORDER BY created_at DESC LIMIT 5;
```

---

## API 직접 테스트 (curl)

### 1. 인증 이메일 발송

```bash
curl -X POST http://localhost:8080/api/email/send-verification \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com"
  }'
```

예상 응답:
```json
{
  "message": "Verification email sent successfully"
}
```

### 2. 이메일 인증

```bash
# 토큰은 MailHog에서 확인한 링크에서 추출
curl -X GET "http://localhost:8080/api/email/verify?token=YOUR_TOKEN_HERE"
```

예상 응답:
```json
{
  "message": "Email verified successfully"
}
```

### 3. 비밀번호 재설정 요청

```bash
curl -X POST http://localhost:8080/api/email/forgot-password \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com"
  }'
```

예상 응답:
```json
{
  "message": "Password reset link sent successfully"
}
```

### 4. 재설정 토큰 검증

```bash
curl -X GET "http://localhost:8080/api/email/verify-reset-token?token=YOUR_TOKEN_HERE"
```

예상 응답:
```json
{
  "valid": true,
  "message": "Reset token is valid"
}
```

### 5. 비밀번호 재설정

```bash
curl -X POST http://localhost:8080/api/email/reset-password \
  -H "Content-Type: application/json" \
  -d '{
    "token": "YOUR_TOKEN_HERE",
    "new_password": "newpassword123"
  }'
```

예상 응답:
```json
{
  "message": "Password reset successfully"
}
```

---

## 문제 해결

### 1. 데이터베이스 연결 실패

**증상:** `Failed to connect to database`

**해결:**
```bash
# PostgreSQL 실행 확인
sudo service postgresql status

# 데이터베이스 존재 확인
psql -U postgres -l | grep authway

# 권한 확인
psql -U postgres -c "SELECT rolname FROM pg_roles WHERE rolname='authway';"
```

### 2. 이메일 발송 실패

**증상:** `Failed to send email`

**해결:**
```bash
# MailHog 실행 확인
curl http://localhost:8025

# SMTP 설정 확인
echo $AUTHWAY_EMAIL_SMTP_HOST
echo $AUTHWAY_EMAIL_SMTP_PORT

# MailHog 재시작
docker restart mailhog
```

### 3. 프론트엔드 API 연결 실패

**증상:** CORS 오류 또는 Network Error

**해결:**
1. .env 파일에서 CORS 설정 확인
2. 백엔드 로그에서 CORS 오류 확인
3. 프론트엔드 VITE_API_URL 환경 변수 확인

```bash
# packages/web/login-ui/.env
VITE_API_URL=http://localhost:8080
```

### 4. 마이그레이션 오류

**증상:** Database migration failed

**해결:**
```sql
-- 테이블 수동 생성
psql -U authway -d authway

-- 이메일 인증 테이블
CREATE TABLE IF NOT EXISTS email_verifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    token VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    verified BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 비밀번호 재설정 테이블
CREATE TABLE IF NOT EXISTS password_resets (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    token VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 인덱스 생성
CREATE INDEX idx_email_verifications_user_id ON email_verifications(user_id);
CREATE INDEX idx_email_verifications_token ON email_verifications(token);
CREATE INDEX idx_password_resets_user_id ON password_resets(user_id);
CREATE INDEX idx_password_resets_token ON password_resets(token);
```

---

## 추가 테스트 시나리오

### 보안 테스트

1. **중복 토큰 사용 방지**
   - 비밀번호 재설정 토큰을 한 번 사용
   - 같은 토큰으로 다시 재설정 시도
   - "토큰이 이미 사용되었습니다" 오류 확인

2. **유효하지 않은 토큰**
   - 임의의 UUID로 인증 시도
   - "Invalid or expired token" 오류 확인

3. **이메일 열거 공격 방지**
   - 존재하지 않는 이메일로 비밀번호 재설정 요청
   - 동일한 성공 메시지 반환 확인 (이메일 존재 여부 노출 안 됨)

### 성능 테스트

1. **다중 요청**
   ```bash
   # 100개의 동시 요청
   for i in {1..100}; do
     curl -X POST http://localhost:8080/api/email/send-verification \
       -H "Content-Type: application/json" \
       -d "{\"email\": \"test$i@example.com\"}" &
   done
   wait
   ```

2. **토큰 만료 정리**
   - 만료된 토큰이 자동으로 정리되는지 확인
   - 데이터베이스 크기 모니터링

---

## 모니터링

### 로그 확인

```bash
# 백엔드 로그 (실행 중)
# 콘솔에서 실시간 확인

# 특정 오류 검색
grep -i error logs/authway.log

# 이메일 발송 로그 확인
grep -i "email sent" logs/authway.log
```

### 성능 모니터링

```bash
# PostgreSQL 연결 수
psql -U authway -d authway -c "SELECT count(*) FROM pg_stat_activity;"

# Redis 상태
redis-cli info stats

# 이메일 큐 크기 (MailHog)
curl http://localhost:8025/api/v2/messages | jq '.total'
```

---

## 테스트 체크리스트

### 이메일 인증
- [ ] 회원가입 후 인증 이메일 수신
- [ ] 인증 링크 클릭 시 인증 완료
- [ ] 인증 완료 후 로그인 가능
- [ ] 인증 이메일 재발송 가능
- [ ] 만료된 토큰 처리

### 비밀번호 재설정
- [ ] 비밀번호 찾기 이메일 수신
- [ ] 재설정 링크로 새 비밀번호 설정
- [ ] 새 비밀번호로 로그인 가능
- [ ] 토큰 만료 처리
- [ ] 사용된 토큰 재사용 방지

### React SDK
- [ ] SDK 빌드 성공
- [ ] 예제 앱 실행
- [ ] OAuth 로그인 플로우
- [ ] 사용자 정보 표시
- [ ] 로그아웃 기능

### 보안
- [ ] 이메일 열거 공격 방지
- [ ] 토큰 재사용 방지
- [ ] 토큰 만료 처리
- [ ] HTTPS 준비 (프로덕션)

---

## 다음 단계

모든 테스트가 완료되면:

1. **프로덕션 준비**
   - SMTP 서버 설정 (SendGrid, AWS SES 등)
   - 환경 변수 보안 강화
   - HTTPS 설정
   - 도메인 설정

2. **배포**
   - Docker 컨테이너 빌드
   - Kubernetes 설정
   - CI/CD 파이프라인

3. **모니터링**
   - 로그 수집 (ELK, Datadog)
   - 메트릭 모니터링 (Prometheus, Grafana)
   - 알림 설정

---

## 참고 자료

- [Authway 문서](./README.md)
- [React SDK 문서](./packages/sdk/react/README.md)
- [API 문서](./docs/API.md)
- [배포 가이드](./DEPLOYMENT-GUIDE.md)
