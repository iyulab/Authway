# 🚀 Authway React SDK Sample

이 샘플은 **@authway/client**와 **@authway/react** 패키지를 사용하여 OAuth 2.0 인증을 구현하는 방법을 보여줍니다.

## 📋 사전 요구사항

로컬 Authway 개발 환경이 실행 중이어야 합니다:

```powershell
# 프로젝트 루트에서
.\start-dev.ps1
```

실행 후 다음 서비스들이 사용 가능해야 합니다:
- ✅ Authway API: http://localhost:8080
- ✅ Admin Dashboard: http://localhost:3000
- ✅ Login UI: http://localhost:3001
- ✅ Ory Hydra: http://localhost:4444 (Public), http://localhost:4445 (Admin)

## 🚀 빠른 시작

### 방법 1: 자동 스크립트 (권장)

```powershell
# samples/react-sdk-sample 디렉토리에서
.\start-sample.ps1
```

이 스크립트는 다음을 자동으로 수행합니다:
1. ✅ OAuth 클라이언트 등록
2. ✅ @authway/client 빌드
3. ✅ @authway/react 빌드
4. ✅ 의존성 설치
5. ✅ 개발 서버 시작

### 방법 2: 수동 실행

#### 1. OAuth 클라이언트 등록

```powershell
.\setup-client.ps1
```

#### 2. SDK 패키지 빌드

```powershell
# @authway/client 빌드
cd ..\..\packages\client
pnpm build

# @authway/react 빌드
cd ..\react
pnpm build

# 샘플 디렉토리로 돌아가기
cd ..\..\samples\react-sdk-sample
```

#### 3. 의존성 설치 및 실행

```powershell
pnpm install
pnpm dev
```

#### 4. 브라우저에서 확인

http://localhost:9004 접속

## 🎯 기능

이 샘플 앱은 다음 기능을 시연합니다:

### ✅ 인증 플로우
- OAuth 2.0 Authorization Code Flow with PKCE
- 로그인 / 로그아웃
- 자동 리다이렉트 콜백 처리
- 세션 관리

### ✅ React Hooks
- `useAuth()` - 인증 상태 및 메서드
- `useUser()` - 사용자 프로필
- `useAccessToken()` - 액세스 토큰 관리

### ✅ UI 기능
- 사용자 프로필 표시
- 토큰 정보 보기 및 디코딩
- API 테스트 (인증된 요청)
- 에러 처리
- 로딩 상태

## 📁 프로젝트 구조

```
react-sdk-sample/
├── src/
│   ├── App.tsx          # 메인 애플리케이션
│   ├── main.tsx         # 엔트리 포인트
│   └── index.css        # 스타일
├── index.html           # HTML 템플릿
├── package.json         # 의존성
├── vite.config.ts       # Vite 설정
├── tsconfig.json        # TypeScript 설정
├── setup-client.ps1     # OAuth 클라이언트 등록
├── start-sample.ps1     # 빠른 시작 스크립트
└── README.md            # 이 파일
```

## 🔧 설정

### OAuth 클라이언트 설정

**Client ID**: `react-sdk-sample-client`
**Redirect URI**: `http://localhost:9004/callback`
**Port**: `9004`

설정은 `src/App.tsx`에서 확인/변경 가능:

```typescript
const config = {
  domain: 'localhost:8080',
  clientId: 'react-sdk-sample-client',
  redirectUri: 'http://localhost:9004/callback',
  scope: 'openid profile email'
}
```

## 📚 코드 예제

### 기본 사용법

```tsx
import { AuthwayProvider, useAuth } from '@authway/react'

function App() {
  return (
    <AuthwayProvider config={config}>
      <MyApp />
    </AuthwayProvider>
  )
}

function MyApp() {
  const { isAuthenticated, user, loginWithRedirect, logout } = useAuth()

  if (!isAuthenticated) {
    return <button onClick={() => loginWithRedirect()}>로그인</button>
  }

  return (
    <div>
      <p>환영합니다, {user.name}!</p>
      <button onClick={() => logout()}>로그아웃</button>
    </div>
  )
}
```

### 액세스 토큰 사용

```tsx
import { useAccessToken } from '@authway/react'

function ApiCall() {
  const { getToken } = useAccessToken()

  const callApi = async () => {
    const token = await getToken()
    const response = await fetch('/api/data', {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    })
    return response.json()
  }

  return <button onClick={callApi}>API 호출</button>
}
```

## 🐛 문제 해결

### "Authway is not running" 에러

```powershell
# 프로젝트 루트에서 Authway 시작
.\start-dev.ps1
```

### "Client not registered" 에러

```powershell
# OAuth 클라이언트 재등록
.\setup-client.ps1
```

### SDK 빌드 에러

```powershell
# SDK 패키지 재빌드
cd ..\..\packages\client
pnpm build

cd ..\react
pnpm build
```

### 의존성 에러

```powershell
# 의존성 재설치
rm -rf node_modules
pnpm install
```

## 🔗 관련 링크

- **SDK 설계 문서**: [../../docs/sdk/PACKAGE_DESIGN.md](../../docs/sdk/PACKAGE_DESIGN.md)
- **빠른 시작 가이드**: [../../docs/sdk/QUICK_START.md](../../docs/sdk/QUICK_START.md)
- **@authway/client**: [../../packages/client/README.md](../../packages/client/README.md)
- **@authway/react**: [../../packages/react/README.md](../../packages/react/README.md)

## 📊 서비스 URL

| 서비스 | URL | 설명 |
|--------|-----|------|
| **React SDK Sample** | http://localhost:9004 | 이 샘플 앱 |
| **Admin Dashboard** | http://localhost:3000 | Authway 관리 콘솔 |
| **Login UI** | http://localhost:3001 | 호스팅 로그인 UI |
| **Backend API** | http://localhost:8080 | Authway API |
| **MailHog** | http://localhost:8025 | 이메일 테스트 |

## 💡 다음 단계

1. **코드 탐색**: `src/App.tsx`에서 구현 확인
2. **커스터마이징**: UI와 로직을 자유롭게 수정
3. **문서 읽기**: SDK 전체 기능 학습
4. **프로덕션 배포**: 실제 앱에 SDK 통합

## 🤝 기여

버그 발견이나 개선 제안은 GitHub Issues로 제보해주세요!

---

**Version**: 1.0.0-alpha.1
**Last Updated**: 2025-10-24
