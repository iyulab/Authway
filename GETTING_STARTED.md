# Authway - OAuth2/OIDC Authentication Platform

Ory Hydra 기반의 3계층 OAuth2/OIDC 인증 플랫폼입니다.

## 시스템 아키텍처

```
Layer 1: OAuth2 Core (Ory Hydra v2.2.0)
         ↓
Layer 2: User Management (Authway API - Go/Fiber)
         ↓
Layer 3: Developer Experience (React frontends)
```

## 기술 스택

### Backend
- **Ory Hydra v2.2.0**: OAuth2/OIDC 서버
- **Go 1.21+**: 백엔드 API 언어
- **Fiber**: 웹 프레임워크
- **PostgreSQL**: 사용자 데이터 저장
- **Redis**: 세션 관리

### Frontend
- **React 18**: UI 라이브러리
- **TypeScript**: 타입 안전성
- **Vite**: 번들러
- **Tailwind CSS**: 스타일링
- **shadcn/ui**: UI 컴포넌트

## 빠른 시작

### 1. 전제 조건
- Docker Desktop 설치
- Go 1.21+ 설치
- Node.js 18+ 설치

### 2. 인프라 서비스 시작
```bash
# 프로젝트 클론 후
cd Authway

# Docker 서비스 시작 (PostgreSQL, Redis, Hydra)
docker-compose up -d

# 서비스 상태 확인
docker-compose ps
```

### 3. 테스트 OAuth 클라이언트 생성
```bash
# OAuth 클라이언트 생성
curl -X POST http://localhost:4445/admin/clients \
  -H "Content-Type: application/json" \
  -d '{
    "client_id": "test-client",
    "client_name": "Test Application",
    "grant_types": ["authorization_code", "refresh_token"],
    "response_types": ["code"],
    "scope": "openid profile email",
    "redirect_uris": ["http://localhost:3000/callback"],
    "token_endpoint_auth_method": "client_secret_basic"
  }'
```

### 4. 테스트 서버 실행
```bash
# 간단한 테스트 서버 실행
cd tools
go run test-server.go
```

### 5. OAuth 플로우 테스트
브라우저에서 다음 URL로 이동:
```
http://localhost:4444/oauth2/auth?client_id=test-client&response_type=code&scope=openid+profile+email&redirect_uri=http://localhost:3000/callback&state=test-state
```

테스트 계정:
- **이메일**: admin@test.com
- **비밀번호**: password

## 프론트엔드 개발 서버

### Login UI 시작
```bash
cd packages/web/login-ui
npm install
npm run dev
# http://localhost:3001에서 실행
```

### Admin Dashboard 시작
```bash
cd packages/web/admin-dashboard
npm install
npm run dev
# http://localhost:3000에서 실행
```

## 주요 엔드포인트

### Hydra (OAuth2 Core)
- **Public API**: http://localhost:4444
- **Admin API**: http://localhost:4445
- **Health**: http://localhost:4444/health/ready

### Authway API (예정)
- **API**: http://localhost:8080
- **Health**: http://localhost:8080/health

### Frontend
- **Admin Dashboard**: http://localhost:3000
- **Login UI**: http://localhost:3001

## 환경 변수

### Hydra
- `DSN`: 데이터베이스 연결 문자열
- `URLS_SELF_ISSUER`: OAuth2 발급자 URL
- `URLS_LOGIN`: 로그인 엔드포인트
- `URLS_CONSENT`: 동의 엔드포인트

### Frontend
- `VITE_API_URL`: 백엔드 API URL
- `VITE_HYDRA_PUBLIC_URL`: Hydra 공개 URL

## 개발 가이드

### OAuth2 플로우
1. **Authorization Request**: 클라이언트가 Hydra로 인증 요청
2. **Login Challenge**: Hydra가 로그인 챌린지 생성
3. **User Authentication**: Login UI에서 사용자 인증
4. **Login Accept**: 인증 성공 시 로그인 수락
5. **Consent Challenge**: Hydra가 동의 챌린지 생성
6. **User Consent**: Consent UI에서 사용자 동의
7. **Consent Accept**: 동의 시 권한 승인
8. **Authorization Code**: 클라이언트로 인증 코드 반환

### 디렉토리 구조
```
Authway/
├── configs/           # 설정 파일
│   └── hydra.yml     # Hydra 설정
├── packages/         # 프론트엔드 패키지
│   └── web/
│       ├── admin-dashboard/  # 관리자 대시보드
│       └── login-ui/         # 로그인/동의 UI
├── src/              # 백엔드 소스 (예정)
├── docker-compose.yml # Docker 서비스 정의
├── tools/            # 개발 도구들
│   ├── test-server.go   # OAuth 플로우 테스트 서버
│   └── README.md        # 도구 사용 가이드
└── .dockerignore     # Docker 빌드 최적화
```

## 트러블슈팅

### Hydra 시작 실패
- `configs/hydra.yml` 파일의 구성 확인
- PostgreSQL/Redis 서비스가 정상 실행 중인지 확인

### OAuth 플로우 오류
- 클라이언트 ID와 redirect_uri 일치 확인
- Hydra 로그 확인: `docker-compose logs hydra`

### 프론트엔드 연결 오류
- API 엔드포인트 URL 확인
- CORS 설정 확인

## 다음 단계

1. **백엔드 API 구현**: Go/Fiber 기반 사용자 관리 API
2. **사용자 등록 기능**: 새 사용자 가입 플로우
3. **클라이언트 관리**: OAuth 클라이언트 CRUD 작업
4. **권한 관리**: 역할 기반 접근 제어
5. **보안 강화**: 프로덕션 환경 설정

## 지원

문제가 있거나 질문이 있으시면 프로젝트 이슈를 생성해주세요.