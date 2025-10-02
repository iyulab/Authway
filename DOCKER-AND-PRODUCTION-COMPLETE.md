# ✅ Docker 및 프로덕션 배포 구성 완료!

Authway의 완전한 Docker 개발 환경과 프로덕션 배포 구성이 완료되었습니다.

---

## 📦 생성된 파일들

### Docker 개발 환경

#### 설정 파일
- ✅ `docker-compose.dev.yml` - 개발 환경 Docker Compose
- ✅ `Dockerfile.dev` - 개발용 백엔드 Dockerfile (Hot Reload)
- ✅ `packages/web/login-ui/Dockerfile.dev` - Login UI Dockerfile
- ✅ `packages/web/admin-dashboard/Dockerfile.dev` - Admin Dashboard Dockerfile
- ✅ `.air.toml` - Go Hot Reload 설정
- ✅ `.env.example` - 환경 변수 예시 (MailHog 포함)
- ✅ `.gitignore` - Docker 관련 항목 추가

#### 문서
- ✅ `DOCKER-GUIDE.md` - 상세한 Docker 사용 가이드 (600+ 줄)
- ✅ `START-HERE.md` - 1분 빠른 시작 가이드
- ✅ `QUICK-START.md` - 5분 로컬 설정 가이드
- ✅ `TESTING-GUIDE.md` - 전체 기능 테스트 가이드 (400+ 줄)
- ✅ `DOCKER-SETUP-COMPLETE.md` - Docker 구성 완료 요약

#### 스크립트
- ✅ `scripts/start-dev.sh` - Linux/Mac 대화형 시작 스크립트
- ✅ `scripts/start-dev.ps1` - Windows 대화형 시작 스크립트
- ✅ `scripts/test-email-api.sh` - Linux/Mac API 테스트 스크립트
- ✅ `scripts/test-email-api.ps1` - Windows API 테스트 스크립트

### 프로덕션 배포

#### 설정 파일
- ✅ `docker-compose.prod.yml` - 프로덕션 Docker Compose
- ✅ `.env.production.example` - 프로덕션 환경 변수 예시

#### 문서
- ✅ `PRODUCTION-DEPLOYMENT.md` - 완전한 프로덕션 배포 가이드 (500+ 줄)
  - 사전 준비사항
  - 환경 설정
  - SSL/TLS 인증서 설정
  - 보안 설정
  - Nginx 리버스 프록시 설정
  - 배포 방법
  - 모니터링
  - 백업 및 복구
  - 업데이트 및 유지보수
  - 문제 해결
  - 성능 최적화

---

## 🚀 개발 환경 - 즉시 시작하기

### 방법 1: 한 줄 명령어 (가장 빠름)

```bash
docker-compose -f docker-compose.dev.yml up -d
```

### 방법 2: 대화형 스크립트

**Windows (PowerShell):**
```powershell
.\scripts\start-dev.ps1
```

**Linux/Mac:**
```bash
chmod +x scripts/start-dev.sh
./scripts/start-dev.sh
```

### 서비스 URL

| 서비스 | URL | 설명 |
|--------|-----|------|
| 🎨 Login UI | http://localhost:3001 | 회원가입 & 로그인 |
| 🖥️ Admin Dashboard | http://localhost:3000 | 관리자 대시보드 |
| 🚀 Backend API | http://localhost:8080 | REST API |
| 📧 MailHog | http://localhost:8025 | 이메일 확인 |
| 🗄️ PostgreSQL | localhost:5432 | 데이터베이스 |
| 💾 Redis | localhost:6379 | 캐시 |

---

## 🌐 프로덕션 배포

### 배포 전 준비

1. **환경 변수 설정**
```bash
cp .env.production.example .env.production
nano .env.production
```

2. **필수 비밀키 생성**
```bash
# PostgreSQL 비밀번호
openssl rand -base64 32

# Redis 비밀번호
openssl rand -base64 32

# JWT Access Token Secret
openssl rand -base64 32

# JWT Refresh Token Secret
openssl rand -base64 32
```

3. **SSL 인증서 설정**
```bash
# Let's Encrypt 사용 (무료)
sudo certbot certonly --standalone -d auth.yourdomain.com
sudo certbot certonly --standalone -d login.yourdomain.com
sudo certbot certonly --standalone -d admin.yourdomain.com
```

### 배포 실행

```bash
# 프로덕션 빌드 및 시작
docker-compose -f docker-compose.prod.yml build
docker-compose -f docker-compose.prod.yml up -d

# Nginx와 함께 배포 (SSL/TLS)
docker-compose -f docker-compose.prod.yml --profile with-nginx up -d
```

### 헬스 체크

```bash
# API 서버 확인
curl https://auth.yourdomain.com/health

# 서비스 상태 확인
docker-compose -f docker-compose.prod.yml ps
```

**자세한 내용은 [PRODUCTION-DEPLOYMENT.md](./PRODUCTION-DEPLOYMENT.md) 참조**

---

## 🎯 주요 기능

### 개발 환경

#### 자동화된 설정
- ✅ **원클릭 시작** - 모든 서비스 한 번에
- ✅ **자동 마이그레이션** - DB 스키마 자동 생성
- ✅ **핫 리로드** - 코드 변경 시 자동 재시작
  - Go: Air 사용
  - React: Vite HMR
- ✅ **이메일 테스트** - MailHog로 실시간 확인
- ✅ **의존성 관리** - Docker가 모두 처리

#### 개발자 경험
- 🔥 **빠른 피드백** - 코드 변경 즉시 반영
- 🐛 **쉬운 디버깅** - 실시간 로그 확인
- 🧪 **격리된 환경** - 로컬 시스템 영향 없음
- 🔄 **간편한 초기화** - 데이터 리셋 원클릭
- 📊 **상태 모니터링** - 모든 서비스 Health Check

### 프로덕션 환경

#### 보안
- 🛡️ **SSL/TLS 지원** - Let's Encrypt 통합
- 🔐 **비밀번호 관리** - 강력한 암호화
- 🚫 **방화벽 설정** - 불필요한 포트 차단
- 📝 **보안 체크리스트** - 배포 전 검증

#### 성능 최적화
- ⚡ **리소스 제한** - 메모리/CPU 제한 설정
- 🔄 **자동 재시작** - 장애 시 자동 복구
- 📈 **헬스 체크** - 서비스 상태 모니터링
- 💾 **데이터 영속성** - 볼륨 마운트

#### 운영 편의성
- 📊 **로그 관리** - 중앙 집중식 로그
- 💾 **자동 백업** - Cron 기반 백업 스크립트
- 🔄 **무중단 업데이트** - 서비스별 순차 배포
- 📧 **이메일 통합** - SendGrid, AWS SES, Gmail 지원

---

## 📋 포함된 서비스

### 개발 환경 (docker-compose.dev.yml)

| 서비스 | 이미지 | 포트 | 설명 |
|--------|--------|------|------|
| PostgreSQL | postgres:15-alpine | 5432 | 데이터베이스 |
| Redis | redis:7-alpine | 6379 | 캐시 및 세션 |
| MailHog | mailhog/mailhog | 1025, 8025 | 이메일 테스트 |
| Authway API | Custom (Dockerfile.dev) | 8080 | 백엔드 API (Hot Reload) |
| Login UI | Custom (Dockerfile.dev) | 3001 | 로그인 UI (HMR) |
| Admin Dashboard | Custom (Dockerfile.dev) | 3000 | 관리자 대시보드 (HMR) |

### 프로덕션 환경 (docker-compose.prod.yml)

| 서비스 | 이미지 | 포트 | 설명 |
|--------|--------|------|------|
| PostgreSQL | postgres:15-alpine | 5432 | 데이터베이스 (영속성) |
| Redis | redis:7-alpine | 6379 | 캐시 (비밀번호 보호) |
| Authway API | Custom (Dockerfile) | 8080 | 프로덕션 빌드 |
| Login UI | Custom (Dockerfile) | 3001 | 정적 빌드 (Nginx) |
| Admin Dashboard | Custom (Dockerfile) | 3000 | 정적 빌드 (Nginx) |
| Nginx | nginx:alpine | 80, 443 | 리버스 프록시 (Optional) |

---

## 🧪 테스트 시나리오

### 1. 회원가입 & 이메일 인증

```bash
# 1. 회원가입 페이지 접속
http://localhost:3001/register

# 2. 정보 입력 후 회원가입

# 3. MailHog에서 인증 이메일 확인
http://localhost:8025

# 4. 인증 링크 클릭

# 5. 로그인 성공 ✅
```

### 2. 비밀번호 재설정

```bash
# 1. 비밀번호 재설정 페이지
http://localhost:3001/forgot-password

# 2. 이메일 입력

# 3. MailHog에서 재설정 이메일 확인
http://localhost:8025

# 4. 링크로 새 비밀번호 설정

# 5. 새 비밀번호로 로그인 ✅
```

### 3. 자동 테스트 스크립트

```bash
# Windows
.\scripts\test-email-api.ps1

# Linux/Mac
./scripts/test-email-api.sh
```

**자세한 테스트 가이드:** [TESTING-GUIDE.md](./TESTING-GUIDE.md)

---

## 🔧 개발 워크플로우

### 백엔드 코드 수정

```bash
# 1. 로컬에서 코드 수정
code src/server/internal/handler/email.go

# 2. 저장하면 자동으로:
#    - Go 서버 재빌드 (Air)
#    - 서비스 재시작
#    - 변경사항 즉시 적용

# 3. 로그로 확인
docker-compose -f docker-compose.dev.yml logs -f authway-api
```

### 프론트엔드 코드 수정

```bash
# 1. React 코드 수정
code packages/web/login-ui/src/pages/LoginPage.tsx

# 2. 저장하면 자동으로:
#    - Vite HMR 작동
#    - 브라우저 자동 갱신
#    - 변경사항 즉시 확인
```

### 데이터베이스 확인

```bash
# PostgreSQL 접속
docker exec -it authway-postgres psql -U authway -d authway

# 사용자 확인
SELECT email, name, email_verified FROM users;

# 이메일 인증 데이터
SELECT * FROM email_verifications;
```

---

## 📊 서비스 관리

### 시작/중지

```bash
# 모든 서비스 시작
docker-compose -f docker-compose.dev.yml up -d

# 모든 서비스 중지
docker-compose -f docker-compose.dev.yml down

# 특정 서비스만 재시작
docker-compose -f docker-compose.dev.yml restart authway-api
```

### 로그 확인

```bash
# 모든 로그
docker-compose -f docker-compose.dev.yml logs -f

# 특정 서비스 로그
docker-compose -f docker-compose.dev.yml logs -f authway-api

# 최근 100줄
docker-compose -f docker-compose.dev.yml logs --tail=100
```

### 상태 확인

```bash
# 컨테이너 상태
docker-compose -f docker-compose.dev.yml ps

# 리소스 사용량
docker stats

# Health check
curl http://localhost:8080/health
```

---

## 🐛 문제 해결

### 컨테이너가 시작 안 됨

```bash
# 로그 확인
docker-compose -f docker-compose.dev.yml logs

# 완전 재시작
docker-compose -f docker-compose.dev.yml down
docker-compose -f docker-compose.dev.yml up -d --build
```

### 포트 충돌

```bash
# 사용 중인 포트 확인
netstat -ano | findstr :8080    # Windows
lsof -i :8080                   # Linux/Mac

# 프로세스 종료 후 재시작
```

### 데이터 초기화

```bash
# 모든 데이터 삭제
docker-compose -f docker-compose.dev.yml down -v

# 재시작
docker-compose -f docker-compose.dev.yml up -d
```

---

## 📚 다음 단계

### 상세 문서
- 📘 [START-HERE.md](./START-HERE.md) - 1분 빠른 시작
- 🐳 [DOCKER-GUIDE.md](./DOCKER-GUIDE.md) - Docker 전체 가이드
- 🧪 [TESTING-GUIDE.md](./TESTING-GUIDE.md) - 테스트 가이드
- ⚡ [QUICK-START.md](./QUICK-START.md) - 로컬 설정
- 🚀 [PRODUCTION-DEPLOYMENT.md](./PRODUCTION-DEPLOYMENT.md) - 프로덕션 배포

### SDK 및 통합
- 📦 [React SDK](./packages/sdk/react/README.md)
- 📖 [API 문서](./docs/API.md)

### 프로덕션 배포
- 🌐 [프로덕션 배포 가이드](./PRODUCTION-DEPLOYMENT.md)
- ☁️ [Kubernetes 설정](./k8s/)
- 🔐 [보안 가이드](./docs/SECURITY.md)

---

## 🎉 완료!

이제 다음을 할 수 있습니다:

### 개발
1. ✅ **개발 환경 실행** - 단 한 줄로
2. ✅ **코드 수정** - 자동 리로드
3. ✅ **기능 테스트** - 이메일 인증, 비밀번호 재설정
4. ✅ **데이터베이스 관리** - 쉬운 접근
5. ✅ **로그 모니터링** - 실시간 확인

### 프로덕션
1. ✅ **보안 설정** - SSL/TLS, 강력한 비밀번호
2. ✅ **배포** - Docker Compose 또는 Kubernetes
3. ✅ **모니터링** - 로그 및 헬스 체크
4. ✅ **백업** - 자동화된 백업 스크립트
5. ✅ **유지보수** - 무중단 업데이트

**Happy coding! 🚀**

---

## 💡 팁

### 성능 최적화

```bash
# 사용하지 않는 컨테이너 정리
docker system prune -a

# 이미지 캐시 정리
docker builder prune
```

### 개발 팁
- 환경 변수는 `.env` 파일에 추가
- 볼륨 마운트로 실시간 코드 동기화
- Health check로 서비스 준비 상태 확인
- MailHog로 이메일 플로우 테스트

### 보안
- 프로덕션에서는 `docker-compose.prod.yml` 사용
- 환경 변수에 민감한 정보 저장 금지
- HTTPS 사용
- 정기적인 이미지 업데이트
- 백업 자동화

### 프로덕션 체크리스트
- [ ] 모든 비밀번호와 시크릿 키를 강력하게 설정
- [ ] SSL/TLS 인증서 설정 완료
- [ ] 방화벽 규칙 적용
- [ ] `.env.production` 파일 권한 설정 (600)
- [ ] CORS 설정 확인
- [ ] 데이터베이스 백업 자동화
- [ ] 로그 모니터링 설정
- [ ] 정기적인 보안 업데이트

---

## 🤝 지원

문제가 있으신가요?

- 🐛 [GitHub Issues](https://github.com/authway/authway/issues)
- 💬 [Discord](https://discord.gg/authway)
- 📧 [Email](mailto:hello@authway.dev)

---

<p align="center">
  <strong>Authway로 인증을 더 쉽게! 🚀</strong>
</p>
