# Authway 빠른 시작 가이드

5분 안에 Authway를 로컬에서 실행하고 테스트하는 방법입니다.

## 1단계: 필수 소프트웨어 설치 (5분)

```bash
# PostgreSQL 설치 확인
psql --version

# Redis 설치 확인 (Docker 사용 권장)
docker run -d -p 6379:6379 redis:alpine

# MailHog 실행 (이메일 테스트용)
docker run -d -p 1025:1025 -p 8025:8025 mailhog/mailhog
```

## 2단계: 데이터베이스 설정 (1분)

```bash
# PostgreSQL에 접속
psql -U postgres

# 데이터베이스 및 사용자 생성
CREATE DATABASE authway;
CREATE USER authway WITH PASSWORD 'authway';
GRANT ALL PRIVILEGES ON DATABASE authway TO authway;
\q
```

## 3단계: 환경 설정 (30초)

```bash
# .env 파일 복사
cp .env.example .env

# .env 파일 수정 (이미 로컬 개발용 설정이 되어 있음)
# 필요시 에디터로 열어서 확인
```

## 4단계: 백엔드 실행 (30초)

```bash
# 의존성 설치 및 실행
cd src/server
go mod download
go run cmd/main.go

# 새 터미널에서 Health Check
curl http://localhost:8080/health
```

## 5단계: 프론트엔드 실행 (1분)

```bash
# 새 터미널에서
cd packages/web/login-ui
npm install
npm run dev

# 브라우저에서 확인
# http://localhost:3001
```

## 6단계: 기능 테스트 (2분)

### 방법 1: 웹 UI 사용

1. 브라우저에서 http://localhost:3001/register 접속
2. 이메일과 비밀번호 입력하여 회원가입
3. MailHog UI(http://localhost:8025)에서 인증 이메일 확인
4. 인증 링크 클릭하여 이메일 인증
5. 로그인 페이지에서 로그인

### 방법 2: 테스트 스크립트 사용

```bash
# Windows (PowerShell)
cd scripts
.\test-email-api.ps1

# Linux/Mac (Bash)
cd scripts
chmod +x test-email-api.sh
./test-email-api.sh
```

## 확인 사항

### MailHog (이메일 확인)
- URL: http://localhost:8025
- 모든 발송된 이메일을 여기서 확인할 수 있습니다

### Backend API
- URL: http://localhost:8080
- Health: http://localhost:8080/health
- API 문서: http://localhost:8080/swagger (설정 후)

### Frontend
- Login UI: http://localhost:3001
- Admin Dashboard: http://localhost:3000

### Database
```sql
-- PostgreSQL에 접속
psql -U authway -d authway

-- 사용자 확인
SELECT email, name, email_verified FROM users;

-- 이메일 인증 데이터 확인
SELECT * FROM email_verifications ORDER BY created_at DESC LIMIT 5;

-- 비밀번호 재설정 데이터 확인
SELECT * FROM password_resets ORDER BY created_at DESC LIMIT 5;
```

## 문제 해결

### "서버 연결 실패"
```bash
# 백엔드가 실행 중인지 확인
curl http://localhost:8080/health

# 포트가 사용 중인지 확인
# Windows
netstat -ano | findstr :8080

# Linux/Mac
lsof -i :8080
```

### "데이터베이스 연결 실패"
```bash
# PostgreSQL이 실행 중인지 확인
# Windows
sc query postgresql-x64-14

# Linux
sudo service postgresql status

# 데이터베이스 존재 확인
psql -U postgres -l | grep authway
```

### "이메일 발송 실패"
```bash
# MailHog가 실행 중인지 확인
curl http://localhost:8025

# Docker로 MailHog 재시작
docker ps | grep mailhog
docker restart <container_id>
```

## 다음 단계

✅ 로컬 환경 설정 완료!

이제 다음을 진행할 수 있습니다:

1. **상세 테스트**: [TESTING-GUIDE.md](./TESTING-GUIDE.md) 참고
2. **React SDK 사용**: [packages/sdk/react/README.md](./packages/sdk/react/README.md) 참고
3. **배포 준비**: [DEPLOYMENT-GUIDE.md](./DEPLOYMENT-GUIDE.md) 참고

## 주요 URL 정리

| 서비스 | URL | 설명 |
|--------|-----|------|
| Backend API | http://localhost:8080 | Authway 백엔드 서버 |
| Login UI | http://localhost:3001 | 로그인 UI |
| Admin Dashboard | http://localhost:3000 | 관리자 대시보드 |
| MailHog | http://localhost:8025 | 이메일 확인 UI |
| PostgreSQL | localhost:5432 | 데이터베이스 |
| Redis | localhost:6379 | 캐시 서버 |

## 테스트 계정

회원가입 시 아무 이메일이나 사용 가능합니다. 이메일은 MailHog에 수신됩니다.

예시:
- 이메일: test@example.com
- 비밀번호: testpassword123 (최소 8자)

## 지원

- 문제 발생 시: [GitHub Issues](https://github.com/authway/authway/issues)
- 상세 문서: [TESTING-GUIDE.md](./TESTING-GUIDE.md)
- 배포 가이드: [DEPLOYMENT-GUIDE.md](./DEPLOYMENT-GUIDE.md)
