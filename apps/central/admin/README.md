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

## 앱 플로우

```
1. 로그인 (LoginPage)
   ↓
2. 테넌트 선택 (TenantSelectionPage) - Global Level
   - 테넌트 목록 조회
   - 새 테넌트 생성
   ↓
3. 테넌트 앱 (Layout) - Tenant Level
   - 대시보드: 현재 테넌트 통계 (앱, 사용자)
   - 앱(클라이언트) 관리: OAuth2 클라이언트 CRUD
   - 사용자 관리: 사용자 목록 및 편집
   - 설정: 테넌트 정보, 전환, 삭제
```

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

## 주요 기능

### 1. 테넌트 선택 (Global Level)

- 테넌트 목록 표시 및 선택
- 새 테넌트 생성 (이름, slug 자동 생성)
- 선택한 테넌트로 앱 진입

### 2. 대시보드 (Tenant Level)

- 현재 테넌트의 앱(클라이언트) 통계
- 현재 테넌트의 사용자 통계
- 최근 앱/사용자 목록
- 시스템 상태 정보

### 3. OAuth 클라이언트 관리

- 새 OAuth 클라이언트 등록 및 설정
- 클라이언트 정보 업데이트 (이름, redirect URIs, scopes 등)
- 클라이언트 삭제 및 시크릿 재생성
- Public/Confidential 클라이언트 지원

### 4. 사용자 관리

- 등록된 사용자 목록 조회 및 검색
- 사용자 정보 편집 (이름, 아바타)
- 사용자 삭제

### 5. 설정

- 현재 테넌트 정보 확인
- 테넌트 전환 기능
- 테넌트 삭제 (Danger Zone)
- 시스템 설정 확인

## 프로젝트 구조

```
admin/
├── public/                    # 정적 파일
├── src/
│   ├── components/            # 재사용 가능한 컴포넌트
│   │   ├── ui/               # 기본 UI 컴포넌트
│   │   ├── clients/          # 클라이언트 관련 컴포넌트
│   │   ├── users/            # 사용자 관련 컴포넌트
│   │   ├── dashboard/        # 대시보드 컴포넌트
│   │   └── Layout.tsx        # 메인 레이아웃
│   ├── pages/                 # 페이지 컴포넌트
│   │   ├── LoginPage.tsx
│   │   ├── TenantSelectionPage.tsx
│   │   ├── DashboardPage.tsx
│   │   ├── ClientsPage.tsx
│   │   ├── UsersPage.tsx
│   │   └── SettingsPage.tsx
│   ├── stores/                # Zustand 상태 관리
│   │   ├── auth.ts           # 인증 상태
│   │   └── tenant.ts         # 테넌트 선택 상태
│   ├── lib/                   # 유틸리티 및 설정
│   │   └── api.ts            # API 클라이언트
│   ├── App.tsx               # 메인 앱 (라우팅)
│   └── main.tsx              # 엔트리포인트
├── .env.example              # 환경 변수 예제
├── package.json
├── tsconfig.json             # TypeScript 설정
├── vite.config.ts            # Vite 설정
└── tailwind.config.js        # TailwindCSS 설정
```

## API 엔드포인트

| 기능 | 메서드 | 엔드포인트 |
|------|--------|------------|
| 테넌트 목록 | GET | `/api/v1/tenants` |
| 테넌트 생성 | POST | `/api/v1/tenants` |
| 테넌트 삭제 | DELETE | `/api/v1/tenants/:id` |
| 클라이언트 목록 | GET | `/api/v1/clients?tenant_id=` |
| 클라이언트 생성 | POST | `/api/v1/clients` |
| 클라이언트 수정 | PUT | `/api/v1/clients/:id` |
| 클라이언트 삭제 | DELETE | `/api/v1/clients/:id` |
| 사용자 목록 | GET | `/api/v1/users?tenant_id=` |
| 사용자 수정 | PUT | `/api/v1/users/:id` |
| 사용자 삭제 | DELETE | `/api/v1/users/:id` |

## 문제 해결

### CORS 오류

백엔드 API의 CORS 설정을 확인하세요:
```bash
# .env 파일에서
AUTHWAY_CORS_ALLOWED_ORIGINS=http://localhost:3000
```

### 빌드 오류

의존성을 재설치하세요:
```bash
rm -rf node_modules package-lock.json
npm install
```

---

**GitHub**: https://github.com/iyulab/Authway
