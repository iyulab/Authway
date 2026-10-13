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

- ✅ **OIDC Discovery** - Authway API URL만 지정, 엔드포인트는 OpenID Connect Discovery 로 자동 해석
- ✅ **OAuth 2.0 / OIDC** - Authorization Code Flow with PKCE
- ✅ **토큰 갱신** - `useRefreshTokens: true` 면 refresh token 으로 자동 갱신(클라이언트에 `refresh_token` grant·`offline_access` scope 등록 필요)
- ✅ **세션 복원** - 토큰이 없는 페이지에서 `checkSession()` 으로 로그인 상태 복원
- ✅ **클레임 읽기** - ID 토큰의 클레임(`getClaims()`)
- ✅ **팝업 로그인** - 페이지 이동 없이 팝업으로 인증
- ✅ **멀티 테넌시** - 완전히 격리된 테넌트 지원
- ✅ **TypeScript** - 완전한 타입 정의 제공
- ✅ **제로 의존성** - 외부 라이브러리 없음
- ✅ **경량** - ~15KB gzipped

## 빠른 시작

### 기본 설정 (간소화)

```typescript
import { AuthwayClient } from '@authway/client'

// Authway API URL만 지정 — OIDC 엔드포인트는 자동으로 검색됩니다
const client = new AuthwayClient({
  domain: 'http://localhost:8080',
  clientId: 'your-client-id'
})

// Config 로드 완료 대기 (중요!)
await client.waitForReady()

// 이제 사용 가능
await client.loginWithRedirect()
```

### 엔드포인트 자동 검색

초기화 시 클라이언트는 다음 순서로 엔드포인트를 찾습니다:

1. `domain` 의 `/.well-known/authway-config` 에서 OIDC issuer 를 확인합니다. 이 문서가 없는 배포는 `domain` 자신이 issuer 입니다.
   (`issuer` 옵션을 주면 이 단계를 건너뜁니다.)
2. issuer 의 `/.well-known/openid-configuration`(OpenID Connect Discovery)에서 authorize·token·end-session 엔드포인트를 읽습니다.
   게시된 `issuer` 값이 요청한 issuer 와 다르면 설정 오류로 실패합니다.

URL 패턴으로 엔드포인트를 추측하지 않으므로, OIDC Discovery 를 지원하는 어떤 Authway 배포에도 같은 SDK 로 붙습니다.

**중요**: 검색은 비동기입니다. OAuth 요청은 내부적으로 검색 완료를 기다리며, 검색 실패는 `ConfigurationError` 로 드러납니다.
`waitForReady()` 로 미리 확인할 수 있습니다.

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

**참고**: SDK가 팝업 콜백을 자동으로 처리합니다. 별도의 `callback.html` 파일이 필요 없습니다.

#### 팝업 콜백 자동 처리

React를 사용하지 않는 경우, 앱 초기화 시 다음을 호출하세요:

```typescript
// 앱 시작 시 (main.ts 등)
if (AuthwayClient.handlePopupCallback()) {
  // 팝업 컨텍스트에서 실행됨 - 창이 자동으로 닫힘
  return
}

// 일반 앱 초기화 계속
const client = new AuthwayClient(config)
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
const response = await fetch('http://localhost:8080/api/v1/profile/me', {
  headers: {
    'Authorization': `Bearer ${token}`
  }
})
```

## 클레임

```typescript
// ID 토큰의 클레임 (토큰 자체를 설명하는 iss·aud·exp 등은 빠짐)
const claims = await client.getClaims()
```

사용자에게 붙는 커스텀 클레임(플랜·역할 등)은 애플리케이션 **백엔드**가 관리 API
`PATCH /api/v1/users/{id}/claims` 로 저장합니다(관리 키 또는 `admin.claims:write` 범위의
테넌트 서비스 자격증명). 권한 판단에 쓰일 수 있는 값이므로 브라우저나 사용자 토큰으로는 쓸 수
없습니다. 저장한 클레임은 사용자가 다음에 로그인할 때부터 ID 토큰에 실립니다.

## 로그아웃

```typescript
// 로그아웃 — 로컬 토큰 삭제 후 provider 의 end-session 엔드포인트로 이동
await client.logout({ returnTo: window.location.origin })

// 로컬 세션만 삭제
await client.logout({ localOnly: true })
```

## 고급 설정

```typescript
const client = new AuthwayClient({
  domain: 'http://localhost:8080',
  clientId: 'your-client-id',

  // 선택적 설정
  redirectUri: window.location.origin,
  scope: 'openid profile email',
  audience: 'https://api.example.com',
  cacheLocation: 'sessionstorage',  // 'sessionstorage' (기본값) | 'localstorage' | 'memory'
  useDPoP: false,  // DPoP (RFC 9449) 사용 여부

  // OIDC issuer 직접 지정 (authway-config 조회 생략, 엔드포인트는 여전히 discovery 로)
  // issuer: 'http://localhost:4444'
})
```

### 토큰 보관 위치와 새로고침

| `cacheLocation` | 새로고침 | 새 탭 |
|---|---|---|
| `'sessionstorage'` (기본값) | 로그인 유지 | 토큰 없음 |
| `'localstorage'` | 로그인 유지 | 로그인 유지 |
| `'memory'` | 토큰 없음 | 토큰 없음 |

토큰이 없는 페이지는 `checkSession()` 으로 되살린다 — 사용자가 Authway 에 로그인해 둔 세션이 살아 있으면
브라우저가 Authway 를 다녀오며 토큰을 다시 받는다(화면은 뜨지 않는다):

```typescript
import { AuthwayClient, LoginRequiredError } from '@authway/client'

await client.waitForReady()
const answer = new URLSearchParams(window.location.search)
if (answer.has('code') || answer.has('error')) {
  try {
    const { appState } = await client.handleRedirectCallback()
    // 로그인됨 — appState.returnTo 는 사용자가 있던 주소
  } catch (error) {
    if (!(error instanceof LoginRequiredError)) throw error
    // 로그인되어 있지 않음(오류가 아니다) — 로그인 버튼을 보여 준다
  }
} else if (!(await client.isAuthenticated()) && (await client.checkSession())) {
  return // Authway 로 가는 중 — 이 페이지는 곧 바뀐다
}
```

어느 위치든 페이지에서 실행되는 스크립트로부터 토큰을 지켜 주지는 않는다. 고르는 것은 토큰이 얼마나 오래,
얼마나 넓게 남아 있는가다.

## API 레퍼런스

### 인증

- `loginWithRedirect(options?)` - 리다이렉트 방식 로그인
- `loginWithPopup(options?)` - 팝업 방식 로그인
- `handleRedirectCallback(url?)` - OAuth 콜백 처리 (로그인되어 있지 않다는 답이면 `LoginRequiredError`)
- `checkSession(options?)` - 토큰이 없는 페이지에서 Authway 의 세션으로 로그인 상태 복원 (`prompt=none` 왕복, 화면 없음). 브라우저를 보내면 `true`
- `logout(options?)` - 로그아웃

### 사용자

- `isAuthenticated()` - 인증 상태 확인
- `getUser()` - 사용자 정보 조회
- `getAccessToken()` - Access Token 가져오기 (자동 갱신)

### 클레임

- `getClaims()` - ID 토큰의 클레임
- `getIdTokenClaims()` - ID 토큰 전체

### 유틸리티

- `waitForReady()` - Config 로드 완료 대기 (중요!)

## 관련 문서

- [React SDK (@authway/react)](../react/README.md)
- [SDK 레퍼런스](../../docs/SDK_REFERENCE.md)
- [기능 가이드 — 팝업 로그인·로그아웃·토큰 보관](../../docs/FEATURES.md)
- [샘플 — 브라우저 앱](../../samples/react-sdk-sample)

## 라이선스

MIT
