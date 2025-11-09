# Authway 아키텍처 이해

이 문서는 Authway의 실제 서비스 아키텍처와 SDK 설정 방법을 설명합니다.

## 🏗️ 서비스 아키텍처

Authway는 3개의 주요 서비스로 구성되어 있습니다:

### 1. Central API (Port 8080)
```
Location: apps/central/api/
Service: authway
Health: http://localhost:8080/health
```

**역할**:
- 사용자 관리 (User Management)
- 클레임 저장 및 관리 (Claims Storage)
- OAuth 클라이언트 관리
- 비즈니스 로직 처리

**API 엔드포인트**:
- `GET /api/v1/claims` - 사용자 클레임 조회
- `PATCH /api/v1/claims` - 클레임 업데이트
- `GET /api/v1/profile/me` - 사용자 프로필 조회

**중요**: Central API는 CORS 설정이 제한적일 수 있어 브라우저 앱에서 직접 호출하면 CORS 에러가 발생할 수 있습니다.

### 2. Auth Backend (Port 8081) ⭐ **SPA에서 사용**
```
Location: apps/branding/auth-api/
Service: auth-backend
Health: http://localhost:8081/health
```

**역할**:
- OAuth 2.0 로그인/로그아웃 플로우
- Google OAuth 통합
- **API 프록시 역할** (Central API로 요청 전달)
- **CORS 지원** (브라우저 앱을 위한 CORS 헤더 제공)

**API 엔드포인트** (모두 Central API로 프록시):
- `GET /.well-known/authway-config` - SDK 설정 정보
- `GET /api/v1/claims` → `http://localhost:8080/api/v1/claims`
- `PATCH /api/v1/claims` → `http://localhost:8080/api/v1/claims`
- `GET /api/v1/claims/user` → `http://localhost:8080/api/v1/claims/user`
- `PATCH /api/v1/claims/user` → `http://localhost:8080/api/v1/claims/user`
- `GET /api/v1/profile/me` → `http://localhost:8080/api/v1/profile/me`

**OAuth 엔드포인트**:
- `POST /auth/google/login` - Google OAuth 시작
- `GET /auth/google/callback` - OAuth 콜백 처리
- `POST /consent` - 동의 화면 처리

**프록시 동작** (`internal/handler/claims.go`):
```go
// Auth Backend에서 Central API로 프록시
func (h *ClaimsHandler) GetClaims(c *fiber.Ctx) error {
    authHeader := c.Get("Authorization")
    url := fmt.Sprintf("%s/api/v1/claims", h.centralAPIURL) // port 8080
    req.Header.Set("Authorization", authHeader)
    req.Header.Set("X-Internal-API-Key", h.internalAPIKey)
    // Forward request to Central API...
}
```

### 3. Hydra (Ports 4444, 4445)
```
Service: Ory Hydra OAuth 2.0 Server
Public API: http://localhost:4444
Admin API: http://localhost:4445
```

**역할**:
- OAuth 2.0 Authorization Server
- 토큰 발급 및 검증
- Authorization Code 교환
- Refresh Token 관리

**SDK 자동 감지**:
- SDK가 `domain: 'http://localhost:8081'`을 보면 자동으로 Hydra (4444)로 OAuth 요청 라우팅

## 🔄 요청 플로우

### 팝업 로그인 플로우
```
1. 사용자: "Popup Login" 클릭
   └─> SDK: loginWithPopup() 호출

2. SDK: 팝업 창 열기
   └─> http://localhost:4444/oauth2/auth (Hydra)

3. Hydra: 로그인 필요
   └─> http://localhost:8081/auth/google/login (Auth Backend)

4. Auth Backend: Google OAuth 리디렉션
   └─> https://accounts.google.com/o/oauth2/auth

5. Google: 사용자 인증
   └─> http://localhost:8081/auth/google/callback (Auth Backend)

6. Auth Backend: Hydra에 로그인 완료 알림
   └─> http://localhost:4445/admin/oauth2/auth/requests/login/accept

7. Hydra: Authorization Code 발급
   └─> http://localhost:5173/callback.html?code=...&state=...

8. callback.html: postMessage로 부모 창에 code/state 전달
   └─> window.opener.postMessage({ type: 'authway-callback', code, state })

9. SDK: Code를 Token으로 교환
   └─> POST http://localhost:4444/oauth2/token (Hydra)
   └─> Response: { access_token, id_token, refresh_token }

10. SDK: 사용자 클레임 가져오기 ⭐ 여기가 중요!
    └─> GET http://localhost:8081/api/v1/claims (Auth Backend)
    └─> Auth Backend가 Central API (8080)로 프록시
    └─> Response: { claims: { ... } }
```

### API 호출 플로우 (예: 클레임 조회)
```
Browser (SDK)
    ↓ GET /api/v1/claims + Bearer token
Auth Backend (8081)
    ↓ Forward with X-Internal-API-Key
Central API (8080)
    ↓ Validate token & return claims
Auth Backend (8081)
    ↓ Forward response
Browser (SDK)
    ✅ Success with CORS headers
```

## ⚙️ SDK 설정

### ✅ 올바른 설정 (권장)
```typescript
const authConfig = {
  domain: 'http://localhost:8081',  // Auth Backend
  clientId: 'authway_spa_sample_local'
}
```

**장점**:
- ✅ CORS 지원 (Auth Backend가 CORS 헤더 제공)
- ✅ OAuth 플로우 통합 (같은 서비스에서 로그인 & API 처리)
- ✅ 프록시 자동 처리 (Auth Backend가 Central API로 전달)

**SDK 동작**:
- `oauthServerUrl` → `http://localhost:4444` (자동 감지)
- `centralApiUrl` → `http://localhost:8081` (Auth Backend)
- 모든 API 호출 → Auth Backend → Central API (프록시)

### ❌ Central API 직접 사용 (비권장)
```typescript
const authConfig = {
  domain: 'http://localhost:8080',  // Central API 직접
  clientId: 'authway_spa_sample_local'
}
```

**문제점**:
- ❌ CORS 에러 발생 가능 (Central API의 CORS 설정에 따라)
- ❌ Auth Backend 우회 (통합된 인증 플로우 사용 불가)
- ⚠️ SDK가 경고 메시지 표시

## 📊 포트 요약

| 포트 | 서비스 | 용도 | SDK domain 설정 |
|------|--------|------|-----------------|
| 8080 | Central API | 비즈니스 로직 & 저장소 | ❌ 직접 사용 비권장 |
| 8081 | Auth Backend | OAuth + API 프록시 + CORS | ✅ **권장** |
| 4444 | Hydra Public | OAuth 2.0 토큰 발급 | (자동 감지) |
| 4445 | Hydra Admin | Hydra 관리 API | (내부 사용) |
| 5173 | Frontend Dev | Vite 개발 서버 | - |
| 5222 | ASP.NET Backend | ASP.NET 샘플 API | (별도 설정) |

## 🎯 핵심 교훈

1. **SPA는 Auth Backend (8081)를 사용하세요**
   - CORS 지원이 내장되어 있습니다
   - OAuth 플로우와 API 호출이 통합됩니다
   - Central API는 Auth Backend를 통해 접근합니다

2. **Auth Backend는 프록시 역할을 합니다**
   - API 요청을 Central API (8080)로 전달
   - Authorization 헤더와 Internal API Key 추가
   - CORS 헤더를 응답에 추가

3. **SDK는 자동으로 Hydra를 감지합니다**
   - `domain: 'http://localhost:8081'` → OAuth는 4444로 자동 라우팅
   - 개발자는 OAuth 서버 URL을 수동으로 설정할 필요 없음

## 🔍 디버깅 팁

### CORS 에러가 발생하면
```
Access to fetch at 'http://localhost:8080/...' has been blocked by CORS
```
→ **해결**: `domain`을 8081 (Auth Backend)로 변경

### 401 Unauthorized 에러
```
AuthenticationError: Failed to authenticate user
```
→ **확인**:
1. Access Token이 유효한가? (`getAccessToken()`)
2. Auth Backend가 실행 중인가? (http://localhost:8081/health)
3. Central API가 실행 중인가? (http://localhost:8080/health)

### 팝업이 닫히지만 로그인 실패
→ **확인**:
1. `callback.html`이 `/public` 디렉토리에 있는가?
2. Hydra 클라이언트에 `http://localhost:5173/callback.html`이 등록되어 있는가?
3. 콘솔에서 postMessage 에러가 없는가?

## 📚 참고 파일

- SDK 설정: `packages/client/src/types/config.ts`
- SDK 구현: `packages/client/src/AuthwayClient.ts`
- Auth Backend 라우트: `apps/branding/auth-api/cmd/main.go`
- Claims 프록시: `apps/branding/auth-api/internal/handler/claims.go`
- 샘플 앱: `samples/asp-spa/asp-frontend/src/App.tsx`
