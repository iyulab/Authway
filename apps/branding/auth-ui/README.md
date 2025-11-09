# Authway Login UI

Authway OAuth 2.0 인증 흐름을 위한 사용자 로그인 인터페이스입니다.

## 기술 스택

- **React 18** + TypeScript
- **Vite** - 빌드 도구 및 개발 서버
- **TailwindCSS** - 스타일링 + Animations
- **TanStack Query** - 서버 상태 관리
- **React Router** - 라우팅
- **Zustand** - 클라이언트 상태 관리
- **React Hook Form** + Zod - 폼 검증
- **Application Insights** - 프론트엔드 모니터링

## 시작하기

### 사전 요구사항

- Node.js 18 이상
- npm 또는 yarn
- Authway 백엔드 API 실행 중 (http://localhost:8080)
- Ory Hydra 실행 중 (http://localhost:4444)

### 설치

```bash
# 의존성 설치
npm install
```

### 환경 변수 설정

`.env.example` 파일을 복사하여 `.env` 파일 생성:

```bash
cp .env.example .env
```

필수 환경 변수:

```bash
# 백엔드 API URL
VITE_API_URL=http://localhost:8080

# Application Insights (선택사항)
VITE_APPLICATIONINSIGHTS_CONNECTION_STRING=InstrumentationKey=...
```

### 개발 서버 실행

```bash
# 개발 모드 (http://localhost:3001)
npm run dev
```

### 빌드

```bash
# 프로덕션 빌드
npm run build

# 빌드 미리보기
npm run preview
```

## 스크립트

| 명령어 | 설명 |
|--------|------|
| `npm run dev` | 개발 서버 실행 (포트 3001) |
| `npm run build` | 프로덕션 빌드 |
| `npm run preview` | 빌드된 앱 미리보기 |
| `npm run lint` | ESLint 실행 |
| `npm test` | Vitest 실행 |
| `npm run test:ui` | Vitest UI 모드 |
| `npm run test:coverage` | 테스트 커버리지 확인 |

## 주요 기능

### 1. 로그인 페이지

- **사용자 인증**: 이메일/비밀번호 로그인
- **Google OAuth**: Google 계정으로 로그인
- **Remember Me**: 로그인 상태 유지
- **에러 처리**: 사용자 친화적인 에러 메시지

### 2. 회원가입

- **계정 생성**: 새 사용자 등록
- **이메일 인증**: 이메일 확인 링크 발송
- **폼 검증**: 실시간 입력 검증

### 3. OAuth 동의 페이지

- **권한 승인**: OAuth 클라이언트 권한 표시
- **Scope 선택**: 개별 권한 선택 가능
- **Remember Consent**: 동의 기억 (1시간)

### 4. 비밀번호 재설정

- **재설정 요청**: 이메일로 재설정 링크 발송
- **새 비밀번호 설정**: 안전한 비밀번호 생성

## OAuth 2.0 흐름

Login UI는 Ory Hydra의 OAuth 2.0 흐름을 구현합니다:

```
1. 사용자가 OAuth 클라이언트에서 로그인 시도
   ↓
2. Hydra가 login_challenge와 함께 Login UI로 리다이렉트
   ↓
3. 사용자가 로그인 (이메일/비밀번호 또는 Google)
   ↓
4. Login UI가 Hydra에 로그인 승인 전송
   ↓
5. Hydra가 consent_challenge와 함께 Consent 페이지로 리다이렉트
   ↓
6. 사용자가 권한 승인
   ↓
7. Hydra가 authorization code와 함께 OAuth 클라이언트로 리다이렉트
```

## 프로젝트 구조

```
login-ui/
├── public/                    # 정적 파일
│   └── staticwebapp.config.json  # Azure Static Web Apps 설정
├── src/
│   ├── components/            # 재사용 가능한 컴포넌트
│   │   ├── GoogleLoginButton.tsx
│   │   └── Layout.tsx
│   ├── pages/                 # 페이지 컴포넌트
│   │   ├── HomePage.tsx       # 메인 페이지
│   │   ├── LoginPage.tsx      # 로그인
│   │   ├── ConsentPage.tsx    # OAuth 동의
│   │   ├── RegisterPage.tsx   # 회원가입
│   │   └── ErrorPage.tsx      # 에러 페이지
│   ├── lib/                   # 유틸리티 및 설정
│   │   ├── api.ts            # Axios 설정
│   │   └── telemetry.ts      # Application Insights
│   ├── utils/                 # 헬퍼 함수
│   │   └── validation.ts     # Zod 스키마
│   ├── App.tsx               # 메인 앱 컴포넌트
│   └── main.tsx              # 엔트리포인트
├── .env.example              # 환경 변수 예제
├── package.json
├── tsconfig.json             # TypeScript 설정
├── vite.config.ts            # Vite 설정
└── tailwind.config.js        # TailwindCSS 설정
```

## 개발 가이드

### 코딩 스타일

- **TypeScript**: 타입 안전성 엄격하게 유지
- **컴포넌트**: 함수형 컴포넌트 + Hooks 사용
- **스타일링**: TailwindCSS 유틸리티 클래스
- **애니메이션**: tailwindcss-animate 활용
- **상태 관리**:
  - 서버 상태: TanStack Query
  - 클라이언트 상태: Zustand
- **폼**: React Hook Form + Zod 검증

### URL 파라미터 처리

Login UI는 Hydra에서 전달되는 `login_challenge`와 `consent_challenge`를 처리합니다:

```tsx
import { useSearchParams } from 'react-router-dom';

const LoginPage = () => {
  const [searchParams] = useSearchParams();
  const loginChallenge = searchParams.get('login_challenge');

  // login_challenge를 백엔드 API로 전송
  // ...
};
```

### Google OAuth 통합

```tsx
import GoogleLoginButton from '@/components/GoogleLoginButton';

const LoginPage = () => {
  return (
    <div>
      {/* 일반 로그인 폼 */}
      <form>...</form>

      {/* Google 로그인 버튼 */}
      <GoogleLoginButton loginChallenge={loginChallenge} />
    </div>
  );
};
```

### API 호출

```tsx
import { useMutation } from '@tanstack/react-query';
import api from '@/lib/api';

// 로그인 요청
const loginMutation = useMutation({
  mutationFn: async (credentials) => {
    const response = await api.post('/authenticate', {
      login_challenge: loginChallenge,
      email: credentials.email,
      password: credentials.password,
      remember: credentials.remember
    });
    return response.data;
  },
  onSuccess: (data) => {
    // Hydra로 리다이렉트
    if (data.redirect_to) {
      window.location.href = data.redirect_to;
    }
  }
});
```

## 테스트

```bash
# 테스트 실행
npm test

# 커버리지 확인
npm run test:coverage

# UI 모드로 테스트
npm run test:ui
```

### 테스트 예제

```tsx
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect } from 'vitest';
import LoginPage from './pages/LoginPage';

describe('LoginPage', () => {
  it('submits login form', async () => {
    const user = userEvent.setup();
    render(<LoginPage />);

    await user.type(screen.getByLabelText(/email/i), 'test@example.com');
    await user.type(screen.getByLabelText(/password/i), 'password123');
    await user.click(screen.getByRole('button', { name: /login/i }));

    await waitFor(() => {
      expect(screen.queryByText(/error/i)).not.toBeInTheDocument();
    });
  });
});
```

## Azure 배포

```bash
# Azure Static Web Apps에 배포
cd ../../../scripts
.\publish-login-ui.ps1
```

배포 스크립트는 다음을 수행합니다:
1. `.env.production` 환경 변수 로드
2. 프로덕션 빌드 실행
3. Azure Static Web Apps에 배포

자세한 내용은 [Azure 배포 가이드](../../../docs/deployment/azure-architecture.md)를 참조하세요.

## 문제 해결

### CORS 오류

백엔드 API의 CORS 설정을 확인하세요:
```bash
# .env 파일에서
AUTHWAY_CORS_ALLOWED_ORIGINS=http://localhost:3001
```

### login_challenge 누락 오류

Hydra를 통해 정상적으로 리다이렉트되었는지 확인:
```
정상 URL: http://localhost:3001/login?login_challenge=abc123...
잘못된 URL: http://localhost:3001/login
```

### Google OAuth 오류

백엔드에 Google OAuth 설정이 되어 있는지 확인:
```bash
# 백엔드 .env 파일
AUTHWAY_GOOGLE_CLIENT_ID=your-google-client-id
AUTHWAY_GOOGLE_CLIENT_SECRET=your-google-client-secret
AUTHWAY_GOOGLE_REDIRECT_URL=http://localhost:8080/auth/google/callback
```

### Application Insights 오류

Connection String이 올바른지 확인:
```bash
# .env 파일에서
VITE_APPLICATIONINSIGHTS_CONNECTION_STRING=InstrumentationKey=...
```

### 빌드 오류

의존성을 재설치하세요:
```bash
rm -rf node_modules package-lock.json
npm install
```

## 보안 고려사항

1. **CSRF 보호**: OAuth state 파라미터 사용
2. **HTTPS**: 프로덕션 환경에서 필수
3. **Secure Cookies**: 프로덕션에서 Secure, SameSite 설정
4. **입력 검증**: Zod 스키마로 모든 입력 검증
5. **XSS 방지**: React의 자동 이스케이핑 활용

## 기여

기여는 언제나 환영합니다!

1. 코드 포맷팅: `npm run lint`
2. 테스트 실행: `npm test`
3. 변경사항 커밋

---

**버전**: 0.1.0
**최종 업데이트**: 2025-10-18
