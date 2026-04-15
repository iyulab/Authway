# Auth Backend - OAuth Callback 전담 서비스

## 개요

Auth Backend는 환경별로 배포되어 Google OAuth callback을 전담 처리하는 경량 서비스입니다.

## 책임

✅ **담당**:
- Auth UI 정적 파일 serving
- Google OAuth 시작 (`POST /auth/google/login`)
- Google OAuth callback 처리 (`GET /auth/google/callback`)
- Token exchange (Google code → access token)
- Central API 호출 (user authentication)
- Hydra login accept/reject

❌ **담당하지 않음**:
- User CRUD (Central Backend)
- Tenant management (Central Backend)
- Business logic (Central Backend)
- **Audit logging** (`audit_logs`) — Auth Backend는 UI 프록시 계층이므로 audit 기록을 수행하지 않는다. 모든 상태 변경(로그인/회원가입/consent 수락·거부/세션 폐기 등)의 audit 기록은 Central API에서만 수행된다. Branding 계층에서 중복 기록을 추가하면 동일 이벤트가 이중 기록되어 forensics가 어려워진다. 근거: `claudedocs/HANDOFF.md` P4c 항목.

## 프로젝트 구조

```
auth-backend/
├── cmd/
│   └── main.go                 # Entry point
├── internal/
│   ├── config/
│   │   └── config.go           # Configuration
│   ├── service/
│   │   ├── google.go           # Google OAuth service
│   │   ├── central_api.go      # Central API client
│   │   └── hydra.go            # Hydra client
│   └── handler/
│       ├── oauth.go            # OAuth handlers
│       └── health.go           # Health check
├── static/                     # Auth UI build output
│   ├── index.html
│   └── assets/
├── go.mod
├── go.sum
├── Dockerfile
└── .env.example
```

## 환경 변수

```bash
# Server
PORT=8080
ENVIRONMENT=development
STATIC_PATH=./static

# Google OAuth (environment-specific)
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret
GOOGLE_REDIRECT_URI=https://auth.alldot.ai/auth/google/callback

# Central API (shared)
CENTRAL_API_URL=https://authway-api.iyulab.com
INTERNAL_API_KEY=your-secure-internal-api-key

# Hydra (shared - central)
HYDRA_ADMIN_URL=https://oauth.iyulab.com:4445
HYDRA_PUBLIC_URL=https://oauth.iyulab.com:4444
```

## API 엔드포인트

### Health Check
```
GET /health
Response: "OK"
```

### Static Files
```
GET /
GET /assets/*
Response: Auth UI static files
```

### Google OAuth Login
```
POST /auth/google/login
Content-Type: application/json

Request:
{
  "login_challenge": "xxx",
  "client_id": "apple-service-client"
}

Response:
{
  "redirect_url": "https://accounts.google.com/o/oauth2/v2/auth?..."
}
```

### Google OAuth Callback
```
GET /auth/google/callback?code=xxx&state=yyy

Process:
1. Exchange code for token
2. Get user info from Google
3. Call Central API to authenticate user
4. Accept Hydra login
5. Redirect to Hydra OAuth endpoint
```

## OAuth 플로우

```
1. User clicks "Sign in with Google" in Auth UI
   ↓
2. Auth UI → POST /auth/google/login
   ↓
3. Auth Backend → Generate Google OAuth URL
   ↓
4. User → Google consent screen (shows auth.alldot.ai)
   ↓
5. Google → GET /auth/google/callback?code=xxx
   ↓
6. Auth Backend:
   - Exchange code for token
   - Get user info
   - Call Central API (POST /internal/auth/google)
   - Accept Hydra login
   ↓
7. Redirect to Hydra → Service
```

## 로컬 개발

### 1. 의존성 설치
```bash
cd apps/auth-backend
go mod download
```

### 2. 환경 변수 설정
```bash
cp .env.example .env
# Edit .env with your values
```

### 3. Auth UI 빌드 및 복사
```bash
cd ../auth-ui
npm run build
cp -r dist/* ../auth-backend/static/
```

### 4. 실행
```bash
cd ../auth-backend
go run cmd/main.go
```

서버가 http://localhost:8080 에서 실행됩니다.

## Docker 빌드

```bash
# From project root
docker build -t authway-auth-backend:latest -f apps/auth-backend/Dockerfile .
docker run -p 8080:8080 --env-file apps/auth-backend/.env authway-auth-backend:latest
```

## 배포

### Azure Container App 배포

```powershell
# From project root
.\scripts\deploy\publish-auth-backend.ps1
```

환경별로 자동 배포됩니다:
- iyulab: auth-backend.iyulab.com
- alldot: auth-backend.alldot.ai
- ironhive: auth-backend.ironhive.com

## 구현 상태

### ✅ 완료된 작업
- [x] 프로젝트 구조 생성
- [x] go.mod 초기화
- [x] Config 구조
- [x] Google OAuth service
- [x] Central API client
- [x] Hydra client
- [x] OAuth handlers (GoogleLogin, GoogleCallback)
- [x] Health check handler
- [x] main.go (Fiber app setup)
- [x] Dockerfile
- [x] .env.example

### 📋 다음 단계
- [ ] Auth UI 개발 (React/Vue)
- [ ] Central Backend 수정 (POST /internal/auth/google endpoint)
- [ ] 로컬 테스트 및 통합 테스트
- [ ] 배포 스크립트 작성 (publish-auth-backend.ps1)
- [ ] Azure Container Apps 배포 (3개 환경)

## 참고 문서

- [FINAL_ARCHITECTURE.md](../../docs/FINAL_ARCHITECTURE.md) - 전체 아키텍처
- [OAUTH_FLOW_ANALYSIS.md](../../docs/OAUTH_FLOW_ANALYSIS.md) - OAuth 플로우 분석
- [LOGIN_BACKEND_ARCHITECTURE.md](../../docs/LOGIN_BACKEND_ARCHITECTURE.md) - Login Backend 설계

## 구현 완료 파일

Auth Backend 핵심 구현이 완료되었습니다:

1. ✅ **internal/config/config.go** - 환경 변수 기반 설정 관리
2. ✅ **internal/service/google.go** - Google OAuth 2.0 클라이언트
3. ✅ **internal/service/central_api.go** - Central Backend API 클라이언트
4. ✅ **internal/service/hydra.go** - Hydra Admin API 클라이언트
5. ✅ **internal/handler/oauth.go** - OAuth 로그인 및 콜백 핸들러
6. ✅ **internal/handler/health.go** - Health check 엔드포인트
7. ✅ **cmd/main.go** - Fiber 웹 서버 진입점
8. ✅ **Dockerfile** - 멀티스테이지 Docker 빌드
9. ✅ **.env.example** - 환경 변수 템플릿

다음 단계는 Auth UI 개발 및 Central Backend에 내부 인증 엔드포인트를 추가하는 것입니다.
