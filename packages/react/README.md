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

- ✅ **Config 자동 검색** - Auth Backend URL만 지정, 나머지 자동 검색
- ✅ **React Hooks** - useAuth, useUser, useClaims, useWorkspace
- ✅ **팝업 로그인** - 페이지 이동 없이 팝업으로 인증
- ✅ **자동 콜백 처리** - OAuth 콜백 자동 감지 및 처리
- ✅ **동적 클레임** - 실시간 사용자 클레임 관리
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
        domain: 'http://localhost:8081',  // Auth Backend URL만 지정!
        clientId: 'your-client-id'
      }}
    >
      <YourApp />
    </AuthwayProvider>
  )
}
```

**중요**: `domain`은 Auth Backend URL(기본 포트 8081)을 지정합니다. OAuth URL, API URL 등은 자동으로 검색됩니다.

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

동적 클레임 관리를 위한 훅입니다.

```tsx
const {
  claims,           // 현재 클레임
  updateClaims,     // 클레임 업데이트
  isLoading,        // 로딩 상태
  error             // 에러 정보
} = useClaims()

// 클레임 업데이트
await updateClaims({
  role: 'admin',
  department: 'engineering'
})
```

### useWorkspace

멀티 테넌시 환경에서 워크스페이스 관리를 위한 훅입니다.

```tsx
const {
  workspace,        // 현재 워크스페이스
  switchWorkspace,  // 워크스페이스 변경
  workspaces,       // 사용 가능한 워크스페이스 목록
} = useWorkspace()
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

### API 호출

Access Token을 사용한 API 호출:

```tsx
function DataFetcher() {
  const { getAccessToken } = useAuth()
  const [data, setData] = useState(null)

  useEffect(() => {
    const fetchData = async () => {
      const token = await getAccessToken()
      const response = await fetch('http://localhost:8081/api/v1/profile/me', {
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

### 동적 클레임 업데이트

```tsx
function ClaimsManager() {
  const { claims, updateClaims, isLoading } = useClaims()

  const handleUpdate = async () => {
    await updateClaims({
      role: 'admin',
      department: 'engineering',
      permissions: ['read', 'write', 'delete']
    })
  }

  if (isLoading) return <div>로딩 중...</div>

  return (
    <div>
      <pre>{JSON.stringify(claims, null, 2)}</pre>
      <button onClick={handleUpdate}>클레임 업데이트</button>
    </div>
  )
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
    domain: 'http://localhost:8081',
    clientId: 'your-client-id',

    // 선택적 설정
    redirectUri: window.location.origin,
    scope: 'openid profile email',
    audience: 'https://api.example.com',
    cacheLocation: 'localstorage',
    useDPoP: false,
  }}

  // OAuth 콜백 자동 처리 비활성화
  skipRedirectCallback={true}

  // 커스텀 로딩 컴포넌트
  loadingComponent={<CustomSpinner />}

  // 커스텀 에러 핸들러
  onError={(error) => {
    console.error('Auth error:', error)
    // Sentry 등으로 리포팅
  }}
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
  domain: 'http://localhost:8081',
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
- [SDK 전체 문서](../../docs/sdk/README.md)
- [빠른 시작 가이드](../../docs/sdk/QUICK_START.md)
- [팝업 로그인 가이드](../../docs/features/POPUP_LOGIN_GUIDE.md)
- [동적 클레임 가이드](../../docs/features/DYNAMIC_CLAIMS.md)

## 라이선스

MIT
