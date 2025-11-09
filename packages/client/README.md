# @authway/client

Framework-agnostic authentication client for Authway OAuth 2.0 / OpenID Connect.

**버전**: 0.1.0

## 설치

```bash
npm install @authway/client
# or
pnpm add @authway/client
# or
yarn add @authway/client
```

## 주요 기능

- ✅ **Config 자동 검색** - Auth Backend URL만 지정, 나머지 자동 검색
- ✅ **OAuth 2.0 / OIDC** - Authorization Code Flow with PKCE
- ✅ **자동 토큰 갱신** - Refresh token 기반 자동 갱신
- ✅ **동적 클레임 관리** - 실시간 사용자 클레임 업데이트
- ✅ **팝업 로그인** - 페이지 이동 없이 팝업으로 인증
- ✅ **멀티 테넌시** - 완전히 격리된 테넌트 지원
- ✅ **TypeScript** - 완전한 타입 정의 제공
- ✅ **제로 의존성** - 외부 라이브러리 없음
- ✅ **경량** - ~15KB gzipped

## 빠른 시작

### 기본 설정 (간소화)

```typescript
import { AuthwayClient } from '@authway/client'

// Auth Backend URL만 지정!
// OAuth URL, API URL 등은 자동으로 검색됩니다
const client = new AuthwayClient({
  domain: 'http://localhost:8081',  // Auth Backend URL ()
  clientId: 'your-client-id'
})

// Config 로드 완료 대기 (중요!)
await client.waitForReady()

// 이제 사용 가능
await client.loginWithRedirect()
```

### Config 자동 검색

클라이언트는 초기화 시 Auth Backend의 `/.well-known/authway-config` 엔드포인트에서 자동으로 설정을 가져옵니다:

```typescript
// 내부적으로 다음 요청이 자동 실행됨:
// GET http://localhost:8081/.well-known/authway-config
//
// 응답:
// {
//   "oauth_url": "http://localhost:4444",  // Hydra OAuth 서버
//   "api_url": "http://localhost:8081",    // Auth Backend API
//   "issuer": "http://localhost:4444",
//   "version": "0.1.0"
// }
```

**중요**: Config 로드는 비동기이므로 OAuth 요청 전에 반드시 `waitForReady()`를 호출해야 합니다.

## 로그인

### 리다이렉트 방식

```typescript
// 로그인 페이지로 리다이렉트
await client.loginWithRedirect()

// 콜백 URL에서 처리
const result = await client.handleRedirectCallback()
if (result) {
  console.log('로그인 성공:', result.user)
}
```

### 팝업 방식

```typescript
try {
  const result = await client.loginWithPopup()
  console.log('로그인 성공:', result.user)
} catch (error) {
  console.error('로그인 실패:', error)
}
```

## 사용자 정보

```typescript
// 인증 상태 확인
const isAuthenticated = await client.isAuthenticated()

// 사용자 정보 조회
const user = await client.getUser()
console.log(user)
// {
//   sub: 'user-id',
//   email: 'user@example.com',
//   name: 'User Name',
//   ...
// }
```

## 토큰 관리

```typescript
// Access Token 가져오기 (자동 갱신)
const token = await client.getAccessToken()

// 토큰으로 API 호출
const response = await fetch('http://localhost:8081/api/v1/profile/me', {
  headers: {
    'Authorization': `Bearer ${token}`
  }
})
```

## 동적 클레임 관리

```typescript
// 클레임 조회
const claims = await client.getClaims(token)

// 클레임 업데이트
await client.updateClaims(token, {
  role: 'admin',
  department: 'engineering'
})

// 사용자별 클레임 조회
const userClaims = await client.getUserClaims(token, 'user-id')
```

## 로그아웃

```typescript
// 로그아웃 (로컬 세션만 삭제)
client.logout()

// 로그아웃 (OAuth 서버에서도 세션 삭제)
client.logout({ federated: true })
```

## 고급 설정

```typescript
const client = new AuthwayClient({
  domain: 'http://localhost:8081',
  clientId: 'your-client-id',

  // 선택적 설정
  redirectUri: window.location.origin,
  scope: 'openid profile email',
  audience: 'https://api.example.com',
  cacheLocation: 'localstorage',  // 'localstorage' | 'memory'
  useDPoP: false,  // DPoP (RFC 9449) 사용 여부

  // OAuth URL 직접 지정 (자동 검색 대신)
  // authwayUrl: 'http://localhost:8081',
  // oauthUrl: 'http://localhost:4444'
})
```

## API 레퍼런스

### 인증

- `loginWithRedirect(options?)` - 리다이렉트 방식 로그인
- `loginWithPopup(options?)` - 팝업 방식 로그인
- `handleRedirectCallback(url?)` - OAuth 콜백 처리
- `logout(options?)` - 로그아웃

### 사용자

- `isAuthenticated()` - 인증 상태 확인
- `getUser()` - 사용자 정보 조회
- `getAccessToken()` - Access Token 가져오기 (자동 갱신)

### 클레임

- `getClaims(token)` - 클레임 조회
- `updateClaims(token, claims)` - 클레임 업데이트
- `getUserClaims(token, userId)` - 사용자별 클레임 조회
- `updateUserClaims(token, userId, claims)` - 사용자별 클레임 업데이트

### 유틸리티

- `waitForReady()` - Config 로드 완료 대기 (중요!)

## 관련 문서

- [React SDK (@authway/react)](../react/README.md)
- [SDK 전체 문서](../../docs/sdk/README.md)
- [빠른 시작 가이드](../../docs/sdk/QUICK_START.md)
- [API 소개](../../docs/API_INTRODUCTION.md)

## 라이선스

MIT
