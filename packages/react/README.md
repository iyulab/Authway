# @authway/react

React hooks and components for Authway OAuth 2.0 / OpenID Connect authentication.

**버전**: 0.1.0

## 설치

```bash
npm install @authway/react @authway/client
# or
pnpm add @authway/react @authway/client
# or
yarn add @authway/react @authway/client
```

**참고**: `@authway/react`는 `@authway/client`를 peerDependency로 사용합니다.

## 주요 기능

- ✅ **OIDC Discovery** - Authway API URL만 지정, 엔드포인트는 OpenID Connect Discovery 로 자동 해석
- ✅ **React Hooks** - useAuth, useUser, useClaims, useAccessToken
- ✅ **팝업 로그인** - 페이지 이동 없이 팝업으로 인증
- ✅ **자동 콜백 처리** - OAuth 콜백 자동 감지 및 처리
- ✅ **클레임 읽기** - ID 토큰의 클레임(`useClaims()`)
- ✅ **TypeScript** - 완전한 타입 정의 제공
- ✅ **SSR 지원** - Next.js, Remix 등과 호환

## 빠른 시작

### 1. AuthwayProvider 설정

```tsx
import { AuthwayProvider } from '@authway/react'

function App() {
  return (
    <AuthwayProvider
      config={{
        domain: 'http://localhost:8080',  // Authway API URL
        clientId: 'your-client-id'
      }}
    >
      <YourApp />
    </AuthwayProvider>
  )
}
```

**중요**: `domain`은 Authway API URL 입니다. OIDC 엔드포인트는 issuer 의 discovery 문서에서 자동으로 해석됩니다.

### 2. 로그인 구현

```tsx
import { useAuth, LoginButton } from '@authway/react'

// 사전 제작된 LoginButton 사용
function SimpleLogin() {
  return <LoginButton>로그인</LoginButton>
}

// 커스텀 로그인 버튼
function CustomLogin() {
  const { loginWithRedirect, loginWithPopup, isLoading } = useAuth()

  return (
    <div>
      <button
        onClick={() => loginWithRedirect()}
        disabled={isLoading}
      >
        리다이렉트 로그인
      </button>

      <button
        onClick={() => loginWithPopup()}
        disabled={isLoading}
      >
        팝업 로그인
      </button>
    </div>
  )
}
```

### 3. 사용자 정보 표시

```tsx
import { useAuth } from '@authway/react'

function Profile() {
  const { isAuthenticated, user, logout, isLoading } = useAuth()

  if (isLoading) {
    return <div>로딩 중...</div>
  }

  if (!isAuthenticated) {
    return <div>로그인이 필요합니다</div>
  }

  return (
    <div>
      <img src={user.picture} alt={user.name} />
      <h2>{user.name}</h2>
      <p>{user.email}</p>
      <button onClick={() => logout()}>로그아웃</button>
    </div>
  )
}
```

## Hooks

### useAuth

인증 상태와 관련 메서드를 제공하는 메인 훅입니다.

```tsx
const {
  // 상태
  isAuthenticated,  // 인증 여부
  isLoading,        // 로딩 상태
  user,             // 사용자 정보
  error,            // 에러 정보

  // 메서드
  loginWithRedirect,  // 리다이렉트 로그인
  loginWithPopup,     // 팝업 로그인
  logout,             // 로그아웃
  getAccessToken,     // Access Token 가져오기
} = useAuth()
```

#### logout 옵션

```tsx
// 기본 로그아웃 (OAuth 서버 세션 종료 + 리다이렉트)
logout({ returnTo: window.location.origin })

// 로컬 전용 로그아웃 (토큰만 삭제, 서버 세션 유지)
logout({ localOnly: true })

// Federated 로그아웃 (소셜 로그인 세션도 종료)
logout({ federated: true, returnTo: window.location.origin })
```

**사용 예시**:

```tsx
function AuthStatus() {
  const { isAuthenticated, user, isLoading, error } = useAuth()

  if (isLoading) return <Spinner />
  if (error) return <Error message={error.message} />
  if (!isAuthenticated) return <LoginButton />

  return <div>안녕하세요, {user.name}님!</div>
}
```

### useUser

사용자 정보만 필요할 때 사용하는 경량 훅입니다.

```tsx
const { user, isLoading } = useUser()

// user: {
//   sub: 'user-id',
//   email: 'user@example.com',
//   name: 'User Name',
//   picture: 'https://...',
//   ...
// }
```

### useClaims

로그인한 사용자의 클레임을 ID 토큰에서 읽는 훅입니다. 클레임은 애플리케이션 백엔드가
관리 API(`PATCH /api/v1/users/{id}/claims`)로 저장하며, 브라우저에서는 쓰지 않습니다.
바뀐 클레임은 사용자가 다음에 로그인할 때부터 보입니다.

```tsx
const {
  claims,           // 현재 클레임
  refreshClaims,    // 다시 읽기
  isLoading,        // 로딩 상태
  error             // 에러 정보
} = useClaims()
```

## 컴포넌트

### LoginButton

사전 제작된 로그인 버튼 컴포넌트입니다.

```tsx
import { LoginButton } from '@authway/react'

<LoginButton>로그인</LoginButton>

// 팝업 모드
<LoginButton mode="popup">팝업 로그인</LoginButton>

// 커스텀 스타일
<LoginButton className="custom-btn">로그인</LoginButton>
```

### LogoutButton

사전 제작된 로그아웃 버튼 컴포넌트입니다.

```tsx
import { LogoutButton } from '@authway/react'

<LogoutButton>로그아웃</LogoutButton>

// Federated logout
<LogoutButton federated>로그아웃 (전체)</LogoutButton>
```

### ProtectedRoute

인증이 필요한 라우트를 보호하는 컴포넌트입니다.

```tsx
import { ProtectedRoute } from '@authway/react'

<ProtectedRoute>
  <AdminDashboard />
</ProtectedRoute>

// 리다이렉트 URL 지정
<ProtectedRoute redirectTo="/login">
  <PrivatePage />
</ProtectedRoute>
```

## 고급 사용법

### React Query와 함께 사용하기

React Query (TanStack Query)를 사용하는 앱에서는 logout 전에 쿼리 캐시를 정리해야 합니다.
그렇지 않으면 logout 후에도 보호된 API를 재호출하여 에러가 발생할 수 있습니다.

```tsx
import { useAuth } from '@authway/react'
import { useQueryClient } from '@tanstack/react-query'

function LogoutButton() {
  const { logout } = useAuth()
  const queryClient = useQueryClient()

  const handleLogout = () => {
    // 1. 모든 쿼리 취소 및 캐시 정리 (중요!)
    queryClient.cancelQueries()
    queryClient.clear()

    // 2. Authway 로그아웃
    logout({ returnTo: window.location.origin })
  }

  return <button onClick={handleLogout}>로그아웃</button>
}
```

**참고**: `queryClient.clear()`를 호출하지 않으면, logout 후에도 React Query가 보호된 API를 재호출하여 `SyntaxError: Unexpected token '<'` 에러가 발생할 수 있습니다.

### 백엔드 세션과 함께 로그아웃

백엔드에 별도의 세션이 있는 경우:

```tsx
const handleLogout = async () => {
  // 1. React Query 캐시 정리
  queryClient.cancelQueries()
  queryClient.clear()

  // 2. 백엔드 세션 정리
  await fetch('/api/auth/logout', { method: 'POST' })

  // 3. Authway 로그아웃
  logout({ returnTo: window.location.origin })
}
```

### 팝업 로그인

페이지 이동 없이 팝업으로 로그인:

```tsx
function PopupLogin() {
  const { loginWithPopup } = useAuth()

  const handleLogin = async () => {
    try {
      await loginWithPopup()
      console.log('로그인 성공!')
    } catch (error) {
      console.error('로그인 실패:', error)
    }
  }

  return <button onClick={handleLogin}>팝업 로그인</button>
}
```

**참고**: SDK가 팝업 콜백을 자동으로 처리합니다. 별도의 `callback.html` 파일이 필요 없습니다. Auth0, MSAL과 동일한 방식으로 동작합니다.

### API 호출

Access Token을 사용한 API 호출:

```tsx
function DataFetcher() {
  const { getAccessToken } = useAuth()
  const [data, setData] = useState(null)

  useEffect(() => {
    const fetchData = async () => {
      const token = await getAccessToken()
      const response = await fetch('http://localhost:8080/api/v1/profile/me', {
        headers: {
          'Authorization': `Bearer ${token}`
        }
      })
      const data = await response.json()
      setData(data)
    }

    fetchData()
  }, [getAccessToken])

  return <div>{JSON.stringify(data)}</div>
}
```

### 클레임 표시

```tsx
function ClaimsView() {
  const { claims, isLoading } = useClaims()

  if (isLoading) return <div>로딩 중...</div>

  // 클레임은 백엔드가 관리 API 로 저장한다 — 브라우저에서 쓰지 않는다
  return <pre>{JSON.stringify(claims, null, 2)}</pre>
}
```

### 커스텀 로딩/에러 처리

```tsx
function CustomAuth() {
  const { isLoading, error, isAuthenticated, user } = useAuth()

  if (isLoading) {
    return (
      <div className="flex justify-center items-center h-screen">
        <Spinner />
      </div>
    )
  }

  if (error) {
    return (
      <div className="error-container">
        <h2>인증 오류</h2>
        <p>{error.message}</p>
        <button onClick={() => window.location.reload()}>
          다시 시도
        </button>
      </div>
    )
  }

  if (!isAuthenticated) {
    return <LoginPage />
  }

  return <Dashboard user={user} />
}
```

## 고급 설정

```tsx
<AuthwayProvider
  config={{
    domain: 'http://localhost:8080',
    clientId: 'your-client-id',

    // 선택적 설정
    redirectUri: window.location.origin,
    scope: 'openid profile email',
    audience: 'https://api.example.com',
    cacheLocation: 'localstorage',  // 기본값 | 'sessionstorage' | 'memory'
    useDPoP: false,
  }}

  // OAuth 콜백 자동 처리 비활성화 (기본값 false)
  skipRedirectCallback={false}

  // 토큰이 없는 페이지에서 Authway 의 세션으로 로그인 상태를 되살린다 (기본값 true).
  // 사용자가 이 브라우저에서 로그인한 적이 있을 때만 Authway 를 다녀온다 — 화면은 뜨지 않고,
  // 그동안 isLoading 이 true 다. 세션이 없으면 로그아웃 상태로 로드된다(error 가 아니다).
  checkSessionOnLoad={true}

  // 로그인·세션 확인에서 돌아왔을 때 호출된다. 세션 확인 뒤의 appState 는 { returnTo } —
  // 사용자가 있던 주소. 라우터를 쓰는 앱은 여기서 그 주소로 이동한다.
  onRedirectCallback={(appState) => navigate(appState?.returnTo ?? '/')}
>
  <App />
</AuthwayProvider>
```

## TypeScript

완전한 타입 정의가 제공됩니다:

```tsx
import {
  AuthwayConfig,
  User,
  Claims,
  Workspace
} from '@authway/react'

const config: AuthwayConfig = {
  domain: 'http://localhost:8080',
  clientId: 'your-client-id'
}

const user: User = {
  sub: 'user-id',
  email: 'user@example.com',
  name: 'User Name',
  // ...
}
```

## 관련 문서

- [Client SDK (@authway/client)](../client/README.md)
- [SDK 레퍼런스](../../docs/SDK_REFERENCE.md)
- [기능 가이드 — 팝업 로그인·로그아웃·토큰 보관](../../docs/FEATURES.md)
- [샘플 — 브라우저 앱](../../samples/react-sdk-sample)

## 라이선스

MIT
