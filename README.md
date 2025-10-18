# Authway

**버전**: 0.1.0
**상태**: Production Ready

Ory Hydra 기반의 현대적인 OAuth 2.0 / OpenID Connect 인증 서버로, 멀티 테넌시와 소셜 로그인을 지원합니다.

---

## 주요 기능

### 핵심 기능
- **멀티 테넌시** - 완전히 격리된 테넌트 지원 및 중앙 집중식 관리
- **OAuth 2.0 / OpenID Connect** - 표준 준수 인증 프로토콜
- **JWT 토큰** - 안전한 액세스 및 리프레시 토큰
- **이메일 인증** - 회원가입 이메일 인증 및 비밀번호 재설정
- **관리자 콘솔** - OAuth 클라이언트 및 사용자 관리

### 소셜 로그인
- **Google OAuth** - Google 계정으로 로그인
- **추가 제공자** - 확장 가능한 구조

### 보안
- **세션 관리** - Redis 기반 세션 저장
- **CORS 설정** - 안전한 교차 출처 요청 제어
- **SSL/TLS** - 프로덕션 환경 PostgreSQL SSL 지원
- **Azure Key Vault** - 비밀 관리 통합

### 모니터링
- **Application Insights** - APM 및 분산 추적
- **실시간 로그** - 구조화된 로깅 (zap)
- **헬스 체크** - 서비스 상태 모니터링

---

## 빠른 시작

### 사전 요구사항

- **Go 1.21+** - 백엔드 개발
- **Node.js 18+** - 프론트엔드 개발
- **PostgreSQL 15+** - 데이터베이스
- **Redis** - 세션 관리 (선택사항)
- **Ory Hydra** - OAuth 2.0 서버

### 5분 안에 시작하기

```bash
# 저장소 클론
git clone https://github.com/yourusername/authway.git
cd authway

# 환경 변수 설정
cp .env.example .env

# Hydra 실행 (Docker)
docker run -d --name hydra -p 4444:4444 -p 4445:4445 \
  oryd/hydra:v2.2.0 serve all --dev

# 백엔드 실행
cd src/server
go run cmd/main.go

# 프론트엔드 실행 (별도 터미널)
cd packages/web/login-ui
npm install && npm run dev
```

### 서비스 접속

| 서비스 | URL | 설명 |
|---------|-----|-------------|
| **Admin Dashboard** | http://localhost:3000 | 관리자 콘솔 |
| **Login UI** | http://localhost:5000 | 로그인 페이지 |
| **Backend API** | http://localhost:8080 | REST API |

**기본 관리자 비밀번호**: `.env` 파일 참조

---

## 문서

### 시작 가이드

- **[빠른 시작](docs/quick-start.md)** - 로컬 개발 환경 설정
- **[통합 가이드](docs/INTEGRATION_GUIDE.md)** - OAuth 클라이언트 통합
- **[멀티 테넌시](docs/architecture/multi-tenancy.md)** - 테넌트 아키텍처

### 배포 가이드

- **[Azure 아키텍처](docs/deployment/azure-architecture.md)** - 프로덕션 배포 계획
- **[CI/CD 전략](docs/deployment/azure-cicd.md)** - GitHub Actions 워크플로우
- **[Application Insights](docs/monitoring/application-insights.md)** - 모니터링 설정

### API 문서

- **[API 검증 리포트](docs/api/api-verification-report.md)** - API 엔드포인트 검증

---

## 아키텍처

### 기술 스택

**백엔드**:
- Go 1.21+ (Fiber 프레임워크)
- PostgreSQL 15 (주 데이터베이스)
- Redis (세션 및 캐시)
- Ory Hydra (OAuth 2.0 서버)

**프론트엔드**:
- React 18 + TypeScript
- Vite (빌드 도구)
- TailwindCSS (스타일링)
- TanStack Query (데이터 페칭)

**인프라** (Azure):
- Azure Container Apps (백엔드 호스팅)
- Azure Static Web Apps (프론트엔드 호스팅)
- Azure Database for PostgreSQL (관리형 DB)
- Azure Application Insights (모니터링)

### 프로젝트 구조

```
authway/
├── src/server/              # 백엔드 Go 애플리케이션
│   ├── cmd/main.go         # 메인 엔트리포인트
│   ├── internal/           # 내부 패키지
│   │   ├── config/         # 설정 관리
│   │   ├── handler/        # HTTP 핸들러
│   │   ├── hydra/          # Hydra 클라이언트
│   │   └── telemetry/      # Application Insights
│   └── pkg/                # 공개 패키지
│       ├── admin/          # 관리자 서비스
│       ├── client/         # OAuth 클라이언트 관리
│       ├── tenant/         # 멀티 테넌시
│       └── user/           # 사용자 관리
│
├── packages/web/
│   ├── admin-dashboard/    # React 관리자 UI
│   └── login-ui/           # React 로그인 UI
│
├── scripts/                # 배포 및 유틸리티 스크립트
│   ├── publish-api.ps1
│   ├── publish-login-ui.ps1
│   └── publish-admin-ui.ps1
│
├── docs/                   # 기술 문서
│   ├── quick-start.md
│   ├── deployment/         # 배포 가이드
│   ├── monitoring/         # 모니터링 가이드
│   └── architecture/       # 아키텍처 문서
```

---

## 환경 변수 설정

### 필수 환경 변수

```bash
# 애플리케이션
AUTHWAY_APP_VERSION=0.1.0
AUTHWAY_APP_ENVIRONMENT=development  # development|production
AUTHWAY_APP_PORT=8080

# 데이터베이스
AUTHWAY_DATABASE_HOST=localhost
AUTHWAY_DATABASE_PASSWORD=your-secure-password  # 프로덕션에서 변경 필수!

# 관리자
AUTHWAY_ADMIN_PASSWORD=your-admin-password  # 프로덕션에서 변경 필수!

# JWT (생성: openssl rand -base64 64)
AUTHWAY_JWT_ACCESS_TOKEN_SECRET=your-64-char-secret
AUTHWAY_JWT_REFRESH_TOKEN_SECRET=your-64-char-refresh-secret

# Hydra
AUTHWAY_HYDRA_ADMIN_URL=http://localhost:4445

# Google OAuth (선택사항)
AUTHWAY_GOOGLE_CLIENT_ID=your-google-client-id
AUTHWAY_GOOGLE_CLIENT_SECRET=your-google-client-secret

# Application Insights (선택사항)
AUTHWAY_APPLICATIONINSIGHTS_CONNECTION_STRING=InstrumentationKey=...
AUTHWAY_APPLICATIONINSIGHTS_ENABLED=true
```

**⚠️ 보안 주의**: 프로덕션 환경에서는 모든 기본 비밀번호와 시크릿을 변경해야 합니다.

---

## 테스트

```bash
# 백엔드 테스트
cd src/server
go test ./...

# 프론트엔드 테스트 (Admin Dashboard)
cd packages/web/admin-dashboard
npm test

# 프론트엔드 테스트 (Login UI)
cd packages/web/login-ui
npm test
```

### 테스트 커버리지

```bash
# 백엔드 커버리지
cd src/server
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

---

## Azure 배포

### 배포 스크립트

```bash
# 백엔드 API 배포
cd scripts
.\publish-api.ps1

# Login UI 배포
.\publish-login-ui.ps1

# Admin Dashboard 배포
.\publish-admin-ui.ps1
```

### 프로덕션 체크리스트

- [ ] 모든 기본 비밀번호 및 시크릿 변경
- [ ] JWT 시크릿 생성 (64자 이상)
- [ ] PostgreSQL SSL 활성화 (`ssl_mode=require`)
- [ ] 모든 URL을 HTTPS로 설정
- [ ] 프로덕션 도메인만 CORS 허용
- [ ] SMTP 서비스 설정 (Azure Communication Services 권장)
- [ ] 데이터베이스 자동 백업 활성화
- [ ] Application Insights 연결 문자열 설정

상세한 배포 가이드는 [Azure 배포 문서](docs/deployment/azure-architecture.md)를 참조하세요.

---

## 개발 가이드

### 로컬 개발 환경

```bash
# 1. 의존성 설치
cd src/server && go mod download
cd packages/web/admin-dashboard && npm install
cd packages/web/login-ui && npm install

# 2. Hydra 실행 (Docker)
docker run -d --name hydra -p 4444:4444 -p 4445:4445 \
  oryd/hydra:v2.2.0 serve all --dev

# 3. 백엔드 실행
cd src/server && go run cmd/main.go

# 4. 프론트엔드 실행
cd packages/web/login-ui && npm run dev
cd packages/web/admin-dashboard && npm run dev
```

### 코드 규칙

- **Go**: `gofmt`, `go vet`, `golint`
- **TypeScript**: ESLint, Prettier
- **커밋**: Conventional Commits (`feat:`, `fix:`, `docs:` 등)

### 코드 포맷팅

```bash
# Go 코드 포맷팅
cd src/server
go fmt ./...

# TypeScript 코드 포맷팅
cd packages/web/login-ui
npm run format
```

---

## 멀티 테넌시

Authway는 두 가지 운영 모드를 지원합니다:

### 멀티 테넌트 모드 (기본값)

하나의 인스턴스에서 여러 테넌트를 격리하여 운영:

```bash
AUTHWAY_TENANT_SINGLE_TENANT_MODE=false
```

- 각 테넌트는 독립적인 사용자 및 OAuth 클라이언트 보유
- 데이터베이스 레벨에서 완전한 격리
- 관리 API를 통한 테넌트 관리

### 단일 테넌트 모드

하나의 전용 테넌트로 운영:

```bash
AUTHWAY_TENANT_SINGLE_TENANT_MODE=true
AUTHWAY_TENANT_TENANT_NAME="My Company"
AUTHWAY_TENANT_TENANT_SLUG="my-company"
```

- 간단한 설정
- 멀티 테넌트 오버헤드 없음
- 전용 배포에 적합

자세한 내용은 [멀티 테넌시 문서](docs/architecture/multi-tenancy.md)를 참조하세요.

---

## 주요 엔드포인트

### 인증

- `GET/POST /login` - 로그인 페이지
- `POST /authenticate` - 로그인 제출
- `GET/POST /consent` - 동의 페이지
- `POST /consent/accept` - 동의 승인
- `POST /register` - 회원가입

### 소셜 로그인

- `GET/POST /auth/google/login` - Google 로그인 시작
- `GET /auth/google/callback` - Google OAuth 콜백

### API

- `GET /health` - Health check
- `GET /api/v1/profile/:id` - 사용자 프로필
- `POST /api/v1/clients` - OAuth 클라이언트 생성
- `GET /api/v1/clients` - 클라이언트 목록

---

## 기여

기여는 언제나 환영합니다!

1. 프로젝트 Fork
2. Feature 브랜치 생성 (`git checkout -b feature/amazing-feature`)
3. 변경사항 커밋 (`git commit -m 'feat: add amazing feature'`)
4. 브랜치 Push (`git push origin feature/amazing-feature`)
5. Pull Request 생성

---

## 라이선스

MIT License - 자세한 내용은 [LICENSE](LICENSE) 파일을 참조하세요.

---

## 감사의 말

- [Ory Hydra](https://www.ory.sh/hydra/) - OAuth 2.0 서버
- [Fiber](https://gofiber.io/) - Go 웹 프레임워크
- [React](https://react.dev/) - UI 라이브러리
- [PostgreSQL](https://www.postgresql.org/) - 데이터베이스
- [Redis](https://redis.io/) - 캐싱 및 세션

---

## 지원

- **문서**: [docs/](docs/) 디렉토리 참조
- **버그 리포트**: [GitHub Issues](https://github.com/yourusername/authway/issues)
- **질문 및 토론**: [GitHub Discussions](https://github.com/yourusername/authway/discussions)

---

**버전**: 0.1.0
**최종 업데이트**: 2025-10-18
