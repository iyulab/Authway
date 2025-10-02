# 🐳 Authway Docker 빠른 시작 가이드

Docker Compose를 사용하여 **단 한 줄의 명령어**로 Authway의 모든 서비스를 실행할 수 있습니다.

## ⚡ 초고속 시작 (1분)

```bash
# 1. 저장소 클론 또는 이동
cd D:\data\Authway

# 2. 모든 서비스 실행 (한 줄!)
docker-compose -f docker-compose.dev.yml up -d

# 3. 로그 확인 (선택사항)
docker-compose -f docker-compose.dev.yml logs -f
```

**그게 전부입니다!** 🎉

서비스가 준비되면 다음 URL에서 접속할 수 있습니다:

| 서비스 | URL | 설명 |
|--------|-----|------|
| 🎨 Login UI | http://localhost:3001 | 사용자 인증 UI |
| 🖥️ Admin Dashboard | http://localhost:3000 | 관리자 대시보드 (프로필: full) |
| 🚀 Backend API | http://localhost:8080 | REST API |
| 📧 MailHog | http://localhost:8025 | 이메일 테스트 UI |
| 🗄️ PostgreSQL | localhost:5432 | 데이터베이스 |
| 💾 Redis | localhost:6379 | 캐시 |

---

## 📋 목차

1. [사전 준비](#사전-준비)
2. [기본 사용법](#기본-사용법)
3. [서비스 구성](#서비스-구성)
4. [기능 테스트](#기능-테스트)
5. [개발 워크플로우](#개발-워크플로우)
6. [문제 해결](#문제-해결)
7. [프로덕션 배포](#프로덕션-배포)

---

## 사전 준비

### 필수 소프트웨어

✅ **Docker Desktop** (Windows/Mac) 또는 **Docker Engine** (Linux)

**설치 확인:**
```bash
docker --version
docker-compose --version
```

**최소 요구사항:**
- Docker: 20.10+
- Docker Compose: 2.0+
- 시스템 메모리: 4GB 이상
- 디스크 공간: 10GB 이상

### Docker 설치

**Windows/Mac:**
- [Docker Desktop 다운로드](https://www.docker.com/products/docker-desktop)

**Linux (Ubuntu):**
```bash
# Docker 설치
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Docker Compose 설치
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose
```

---

## 기본 사용법

### 전체 서비스 시작

```bash
# 백그라운드 실행 (권장)
docker-compose -f docker-compose.dev.yml up -d

# 포그라운드 실행 (로그 실시간 확인)
docker-compose -f docker-compose.dev.yml up
```

### Admin Dashboard 포함 시작

```bash
# Admin Dashboard를 포함한 전체 서비스
docker-compose -f docker-compose.dev.yml --profile full up -d
```

### 서비스 중지

```bash
# 모든 서비스 중지
docker-compose -f docker-compose.dev.yml down

# 데이터 볼륨까지 삭제 (완전 초기화)
docker-compose -f docker-compose.dev.yml down -v
```

### 서비스 재시작

```bash
# 특정 서비스 재시작
docker-compose -f docker-compose.dev.yml restart authway-api

# 모든 서비스 재시작
docker-compose -f docker-compose.dev.yml restart
```

### 로그 확인

```bash
# 모든 서비스 로그
docker-compose -f docker-compose.dev.yml logs -f

# 특정 서비스 로그
docker-compose -f docker-compose.dev.yml logs -f authway-api
docker-compose -f docker-compose.dev.yml logs -f login-ui

# 최근 100줄만 확인
docker-compose -f docker-compose.dev.yml logs --tail=100 authway-api
```

### 서비스 상태 확인

```bash
# 실행 중인 컨테이너 확인
docker-compose -f docker-compose.dev.yml ps

# 상세 정보
docker-compose -f docker-compose.dev.yml ps -a
```

---

## 서비스 구성

### 🗄️ PostgreSQL
- **이미지:** postgres:15-alpine
- **포트:** 5432
- **자격증명:**
  - 사용자: `authway`
  - 비밀번호: `authway`
  - 데이터베이스: `authway`

**직접 접속:**
```bash
docker exec -it authway-postgres psql -U authway -d authway
```

### 💾 Redis
- **이미지:** redis:7-alpine
- **포트:** 6379

**직접 접속:**
```bash
docker exec -it authway-redis redis-cli
```

### 📧 MailHog (이메일 테스트)
- **이미지:** mailhog/mailhog
- **SMTP 포트:** 1025
- **웹 UI:** http://localhost:8025

MailHog는 실제로 이메일을 발송하지 않고 캡처하여 웹 UI로 보여줍니다.

### 🚀 Authway Backend API
- **빌드:** Dockerfile.dev
- **포트:** 8080
- **핫 리로드:** Air (Go 파일 변경 시 자동 재시작)

**API Health Check:**
```bash
curl http://localhost:8080/health
```

### 🎨 Login UI (React + Vite)
- **빌드:** packages/web/login-ui/Dockerfile.dev
- **포트:** 3001
- **핫 리로드:** Vite HMR (파일 변경 시 자동 갱신)

### 🖥️ Admin Dashboard (React + Vite) [Optional]
- **빌드:** packages/web/admin-dashboard/Dockerfile.dev
- **포트:** 3000
- **프로필:** `full`

---

## 기능 테스트

### 1. Health Check

```bash
# Backend API 상태 확인
curl http://localhost:8080/health

# 예상 응답:
# {"status":"ok","service":"authway","version":"1.0.0","timestamp":"..."}
```

### 2. 회원가입 및 이메일 인증

#### Step 1: 회원가입
1. 브라우저에서 http://localhost:3001/register 접속
2. 정보 입력:
   - 이메일: test@example.com
   - 비밀번호: testpassword123
   - 이름: Test User
3. "회원가입" 버튼 클릭

#### Step 2: 인증 이메일 확인
1. MailHog UI 접속: http://localhost:8025
2. "Authway - 이메일 인증" 제목의 이메일 확인
3. 이메일 내용에서 인증 링크 클릭

#### Step 3: 로그인
1. 인증 완료 후 자동으로 로그인 페이지로 이동
2. 가입한 이메일과 비밀번호로 로그인

### 3. 비밀번호 재설정

#### Step 1: 비밀번호 찾기
1. http://localhost:3001/forgot-password 접속
2. 이메일 주소 입력
3. "재설정 링크 보내기" 버튼 클릭

#### Step 2: 재설정 이메일 확인
1. MailHog UI에서 "Authway - 비밀번호 재설정" 이메일 확인
2. 재설정 링크 클릭

#### Step 3: 새 비밀번호 설정
1. 새 비밀번호 입력 (최소 8자)
2. 비밀번호 확인 입력
3. "비밀번호 변경" 버튼 클릭

### 4. API 직접 테스트

#### 인증 이메일 발송
```bash
curl -X POST http://localhost:8080/api/email/send-verification \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com"}'
```

#### 비밀번호 재설정 요청
```bash
curl -X POST http://localhost:8080/api/email/forgot-password \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com"}'
```

---

## 개발 워크플로우

### 핫 리로드 (자동 갱신)

Docker Compose 개발 환경은 **핫 리로드**를 지원합니다:

**백엔드 (Go):**
- `src/` 디렉토리의 Go 파일 변경 시 Air가 자동으로 재빌드 및 재시작
- 변경사항이 즉시 반영됨

**프론트엔드 (React):**
- `packages/web/login-ui/src/` 파일 변경 시 Vite HMR이 자동으로 갱신
- 브라우저 새로고침 없이 변경사항 확인

### 로컬 파일 편집

```bash
# 로컬에서 파일 수정
code src/server/internal/handler/email.go

# 컨테이너가 자동으로 변경사항 감지 및 재시작
# 로그에서 확인 가능:
docker-compose -f docker-compose.dev.yml logs -f authway-api
```

### 데이터베이스 마이그레이션

```bash
# 백엔드가 시작될 때 자동으로 마이그레이션 실행
# 수동 마이그레이션이 필요한 경우:
docker exec -it authway-api go run src/server/cmd/main.go migrate
```

### 데이터베이스 초기화

```bash
# 모든 데이터 삭제 및 재시작
docker-compose -f docker-compose.dev.yml down -v
docker-compose -f docker-compose.dev.yml up -d

# PostgreSQL만 초기화
docker-compose -f docker-compose.dev.yml stop postgres
docker volume rm authway_postgres_data
docker-compose -f docker-compose.dev.yml up -d postgres
```

### 컨테이너 내부 접속

```bash
# Backend 컨테이너 접속
docker exec -it authway-api sh

# Login UI 컨테이너 접속
docker exec -it authway-login-ui sh

# PostgreSQL 컨테이너 접속
docker exec -it authway-postgres psql -U authway -d authway
```

---

## 문제 해결

### 컨테이너가 시작되지 않음

**증상:** 특정 컨테이너가 계속 재시작됨

**해결:**
```bash
# 로그 확인
docker-compose -f docker-compose.dev.yml logs authway-api

# 컨테이너 상태 확인
docker-compose -f docker-compose.dev.yml ps

# 완전 재시작
docker-compose -f docker-compose.dev.yml down
docker-compose -f docker-compose.dev.yml up -d
```

### 포트 충돌

**증상:** `port is already allocated` 오류

**해결:**
```bash
# Windows
netstat -ano | findstr :8080
taskkill /PID <PID> /F

# Linux/Mac
lsof -i :8080
kill -9 <PID>

# 또는 docker-compose.dev.yml에서 포트 변경:
# ports:
#   - "8081:8080"  # 호스트 포트를 8081로 변경
```

### 데이터베이스 연결 실패

**증상:** `Failed to connect to database`

**해결:**
```bash
# PostgreSQL 상태 확인
docker-compose -f docker-compose.dev.yml logs postgres

# PostgreSQL 헬스체크
docker exec authway-postgres pg_isready -U authway

# PostgreSQL 재시작
docker-compose -f docker-compose.dev.yml restart postgres

# 연결 테스트
docker exec -it authway-postgres psql -U authway -d authway -c "SELECT 1;"
```

### 이미지 빌드 실패

**증상:** 빌드 중 오류 발생

**해결:**
```bash
# 캐시 없이 재빌드
docker-compose -f docker-compose.dev.yml build --no-cache

# 특정 서비스만 재빌드
docker-compose -f docker-compose.dev.yml build --no-cache authway-api

# 전체 재빌드 및 시작
docker-compose -f docker-compose.dev.yml up -d --build
```

### 볼륨 권한 문제

**증상:** `permission denied` 오류

**해결:**
```bash
# Windows (Docker Desktop 설정 확인)
# Settings > Resources > File Sharing에서 프로젝트 디렉토리 추가

# Linux (볼륨 소유자 변경)
sudo chown -R $USER:$USER .
```

### 로그가 보이지 않음

**해결:**
```bash
# 실시간 로그 확인
docker-compose -f docker-compose.dev.yml logs -f --tail=100

# 특정 서비스 로그
docker-compose -f docker-compose.dev.yml logs -f authway-api
```

### 메모리 부족

**증상:** 컨테이너가 자주 종료됨

**해결:**
```bash
# Docker 메모리 제한 확인 (Docker Desktop)
# Settings > Resources > Memory 를 최소 4GB로 설정

# 사용하지 않는 컨테이너 정리
docker system prune -a --volumes
```

---

## 유용한 명령어 모음

### 개발 환경 관리

```bash
# 전체 시작
docker-compose -f docker-compose.dev.yml up -d

# 전체 중지
docker-compose -f docker-compose.dev.yml down

# 재시작 (코드 변경 후)
docker-compose -f docker-compose.dev.yml restart authway-api

# 로그 확인
docker-compose -f docker-compose.dev.yml logs -f

# 상태 확인
docker-compose -f docker-compose.dev.yml ps
```

### 데이터베이스 작업

```bash
# PostgreSQL 접속
docker exec -it authway-postgres psql -U authway -d authway

# SQL 쿼리 실행
docker exec -it authway-postgres psql -U authway -d authway -c "SELECT * FROM users;"

# 데이터베이스 백업
docker exec authway-postgres pg_dump -U authway authway > backup.sql

# 데이터베이스 복원
cat backup.sql | docker exec -i authway-postgres psql -U authway -d authway
```

### Redis 작업

```bash
# Redis 접속
docker exec -it authway-redis redis-cli

# 모든 키 확인
docker exec -it authway-redis redis-cli KEYS '*'

# 특정 키 값 확인
docker exec -it authway-redis redis-cli GET key_name

# 캐시 초기화
docker exec -it authway-redis redis-cli FLUSHALL
```

### 컨테이너 관리

```bash
# 컨테이너 리소스 사용량
docker stats

# 특정 컨테이너 리소스 확인
docker stats authway-api

# 컨테이너 내부 프로세스 확인
docker top authway-api

# 컨테이너 상세 정보
docker inspect authway-api
```

---

## 프로덕션 배포

개발 환경 테스트가 완료되면 프로덕션 배포를 진행할 수 있습니다.

### 프로덕션용 docker-compose 사용

```bash
# 프로덕션 빌드 및 실행
docker-compose -f docker-compose.yml up -d

# 또는 프로덕션 전용 설정
docker-compose -f docker-compose.prod.yml up -d
```

### 주요 차이점

| 항목 | 개발 환경 | 프로덕션 환경 |
|------|----------|-------------|
| 빌드 | Dockerfile.dev | Dockerfile |
| 핫 리로드 | ✅ 활성화 | ❌ 비활성화 |
| 디버그 로그 | ✅ 상세 | ⚠️ 최소화 |
| 볼륨 마운트 | ✅ 소스 코드 | ❌ 바이너리만 |
| 이메일 | MailHog | SMTP (SendGrid 등) |
| 데이터베이스 | 로컬 | 관리형 DB |
| HTTPS | ❌ HTTP | ✅ HTTPS |

### 환경 변수 설정

프로덕션 환경에서는 `.env` 파일이나 환경 변수로 설정:

```bash
# .env.production 파일 생성
AUTHWAY_EMAIL_SMTP_HOST=smtp.sendgrid.net
AUTHWAY_EMAIL_SMTP_PORT=587
AUTHWAY_EMAIL_SMTP_USER=apikey
AUTHWAY_EMAIL_SMTP_PASSWORD=your-sendgrid-api-key
AUTHWAY_JWT_ACCESS_TOKEN_SECRET=production-secret-key
```

자세한 내용은 [DEPLOYMENT-GUIDE.md](./DEPLOYMENT-GUIDE.md)를 참고하세요.

---

## 추가 리소스

- 📘 [빠른 시작 가이드](./QUICK-START.md)
- 🧪 [상세 테스트 가이드](./TESTING-GUIDE.md)
- 🚀 [배포 가이드](./DEPLOYMENT-GUIDE.md)
- 📦 [React SDK 문서](./packages/sdk/react/README.md)
- 📄 [API 문서](./docs/API.md)

---

## 🎉 완료!

Docker Compose로 Authway 개발 환경을 성공적으로 구축했습니다!

이제 다음을 진행할 수 있습니다:
1. ✅ 로컬 개발 및 테스트
2. ✅ 새로운 기능 개발
3. ✅ API 통합 테스트
4. ✅ 프로덕션 배포 준비

**문제가 발생하면:**
- GitHub Issues: https://github.com/authway/authway/issues
- 문서: [TESTING-GUIDE.md](./TESTING-GUIDE.md)
