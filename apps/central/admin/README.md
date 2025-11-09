# Authway Admin Dashboard

Authway OAuth 클라이언트 및 사용자를 관리하는 관리자 대시보드입니다.

## 기술 스택

- **React 18** + TypeScript
- **Vite** - 빌드 도구 및 개발 서버
- **TailwindCSS** - 스타일링
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
# 개발 모드 (http://localhost:3000)
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
| `npm run dev` | 개발 서버 실행 (포트 3000) |
| `npm run build` | 프로덕션 빌드 |
| `npm run preview` | 빌드된 앱 미리보기 |
| `npm run lint` | ESLint 실행 |
| `npm test` | Vitest 실행 |
| `npm run test:ui` | Vitest UI 모드 |
| `npm run test:coverage` | 테스트 커버리지 확인 |

## 주요 기능

### 1. OAuth 클라이언트 관리

- 새 OAuth 클라이언트 등록 및 설정
- 클라이언트 정보 업데이트 (이름, redirect URIs, scopes 등)
- 클라이언트 삭제 및 시크릿 재생성
- Public/Confidential 클라이언트 지원
- 클라이언트별 설정 관리 (skip_consent, token_strategy 등)

### 2. 사용자 관리

- 등록된 사용자 목록 조회 및 검색
- 사용자 상세 정보 확인
- 이름, 이메일로 사용자 검색
- 사용자 클레임 조회

### 3. 테넌트 관리 (멀티 테넌시 모드)

- 새 테넌트 생성 및 관리
- 테넌트 정보 수정 (이름, slug, 설정)
- 테넌트별 데이터 격리 확인
- 단일/멀티 테넌트 모드 지원

### 4. 설정 관리

- OAuth 클라이언트별 설정 (Google OAuth, 동의 화면 등)
- 시스템 전역 설정
- 환경변수 기반 설정 우선순위

## 프로젝트 구조

```
admin-dashboard/
├── public/                    # 정적 파일
│   └── staticwebapp.config.json  # Azure Static Web Apps 설정
├── src/
│   ├── components/            # 재사용 가능한 컴포넌트
│   │   ├── Layout.tsx
│   │   └── Navbar.tsx
│   ├── pages/                 # 페이지 컴포넌트
│   │   ├── ClientsPage.tsx
│   │   ├── TenantsPage.tsx
│   │   └── UsersPage.tsx
│   ├── lib/                   # 유틸리티 및 설정
│   │   ├── api.ts            # Axios 설정
│   │   └── telemetry.ts      # Application Insights
│   ├── utils/                 # 헬퍼 함수
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
- **상태 관리**:
  - 서버 상태: TanStack Query
  - 클라이언트 상태: Zustand
- **폼**: React Hook Form + Zod 검증

### 새 페이지 추가

```tsx
// src/pages/NewPage.tsx
import { FC } from 'react';

const NewPage: FC = () => {
  return (
    <div className="container mx-auto px-4 py-8">
      <h1 className="text-2xl font-bold">New Page</h1>
    </div>
  );
};

export default NewPage;
```

```tsx
// src/App.tsx에 라우트 추가
<Route path="/new" element={<NewPage />} />
```

### API 호출

```tsx
import { useQuery, useMutation } from '@tanstack/react-query';
import api from '@/lib/api';

// GET 요청
const { data, isLoading, error } = useQuery({
  queryKey: ['clients'],
  queryFn: async () => {
    const response = await api.get('/api/v1/clients');
    return response.data;
  }
});

// POST 요청
const mutation = useMutation({
  mutationFn: async (newClient) => {
    const response = await api.post('/api/v1/clients', newClient);
    return response.data;
  },
  onSuccess: () => {
    // 성공 시 처리
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
import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import App from './App';

describe('App', () => {
  it('renders without crashing', () => {
    render(<App />);
    expect(screen.getByText(/Authway/i)).toBeInTheDocument();
  });
});
```

## Azure 배포

```bash
# Azure Static Web Apps에 배포
cd ../../../scripts
.\publish-admin-ui.ps1
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
AUTHWAY_CORS_ALLOWED_ORIGINS=http://localhost:3000
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

## 기여

기여는 언제나 환영합니다!

1. 코드 포맷팅: `npm run lint`
2. 테스트 실행: `npm test`
3. 변경사항 커밋

---

**버전**: 0.1.0
**최종 업데이트**: 2025-10-26
