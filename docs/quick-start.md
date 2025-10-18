# Authway 빠른 시작 가이드

Authway를 5분 안에 로컬에서 실행하고 테스트하는 방법입니다.

## 사전 요구사항

- **Go** 1.21 이상
- **Node.js** 18 이상
- **PostgreSQL** 15 이상
- **Redis** (선택사항, 세션 관리용)
- **Ory Hydra** (OAuth2 서버)

## 1단계: 저장소 클론

```bash
git clone https://github.com/yourusername/authway.git
cd authway
```

## 2단계: 환경 변수 설정

```bash
# 백엔드 환경 변수
cp .env.example .env

# 필수 환경 변수 설정
# - PostgreSQL 연결 정보
# - JWT 시크릿
# - Google OAuth 클라이언트 ID/Secret (선택사항)
```

## 3단계: 데이터베이스 설정

```bash
# PostgreSQL 데이터베이스 생성
createdb authway

# 마이그레이션 실행 (자동으로 테이블 생성됨)
```

## 4단계: Ory Hydra 실행

```bash
# Docker를 사용한 Hydra 실행
docker run -d \
  --name hydra \
  -p 4444:4444 \
  -p 4445:4445 \
  oryd/hydra:v2.2.0 \
  serve all --dev
```

## 5단계: 백엔드 실행

```bash
# 백엔드 API 서버 실행
cd src/server
go run cmd/main.go

# 서버가 http://localhost:8080 에서 실행됩니다
```

## 6단계: 프론트엔드 실행

### Login UI
```bash
cd packages/web/login-ui
npm install
npm run dev

# http://localhost:5000 에서 실행됩니다
```

### Admin Dashboard
```bash
cd packages/web/admin-dashboard
npm install
npm run dev

# http://localhost:3000 에서 실행됩니다
```

## 7단계: 테스트

1. **Admin Console 접속**: http://localhost:3000
   - 기본 관리자 계정으로 로그인
   - 새 OAuth 클라이언트 등록

2. **Login UI 테스트**: http://localhost:5000
   - 회원가입 테스트
   - 로그인 테스트
   - Google OAuth 테스트 (설정한 경우)

3. **API Health Check**:
```bash
curl http://localhost:8080/health
```

## 다음 단계

- [통합 가이드](INTEGRATION_GUIDE.md) - OAuth 클라이언트 통합 방법
- [Azure 배포](deployment/azure-architecture.md) - 프로덕션 배포 가이드
- [Application Insights](monitoring/application-insights.md) - 모니터링 설정

## 문제 해결

### PostgreSQL 연결 오류
- PostgreSQL 서비스가 실행 중인지 확인
- `.env` 파일의 연결 정보가 올바른지 확인

### Hydra 연결 오류
- Hydra 컨테이너가 실행 중인지 확인: `docker ps`
- Hydra health check: `curl http://localhost:4444/health/ready`

### CORS 오류
- `.env` 파일의 `AUTHWAY_CORS_ALLOWED_ORIGINS` 확인
- 프론트엔드 URL이 허용된 origin에 포함되어 있는지 확인

## 개발 팁

- **Hot Reload**: 프론트엔드는 자동으로 리로드됩니다
- **백엔드 로그**: 자세한 로그는 `zap.DebugLevel` 설정
- **데이터베이스 리셋**: `scripts/reset-db.sh` 사용 (주의: 모든 데이터 삭제)

---

더 자세한 정보는 [README](../README.md)를 참조하세요.
