# Authway

오픈소스 OAuth 2.0 인증 서버

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![React Version](https://img.shields.io/badge/React-18+-61DAFB?style=flat&logo=react)](https://reactjs.org)

---

## 개요

Authway는 Go로 작성된 OAuth 2.0 인증 서버입니다. 자체 인증 시스템을 운영하고자 하는 프로젝트를 위해 만들어졌습니다.

**주요 목표:**
- 표준 OAuth 2.0 / OpenID Connect 구현
- 간단한 설정과 배포
- 오픈소스 라이선스 (MIT)

---

## 기능

### 인증
- OAuth 2.0 Authorization Code Flow (PKCE 지원)
- JWT 기반 토큰 발급 및 검증
- 이메일/비밀번호 로그인
- 이메일 인증
- 비밀번호 재설정

### 관리
- 사용자 관리 API
- OAuth 클라이언트 관리
- React 기반 관리자 대시보드
- React SDK (@authway/react)

### 인프라
- Docker 기반 배포
- PostgreSQL 데이터베이스
- Redis 캐싱
- MailHog 이메일 테스트 (개발 환경)

---

## 빠른 시작

### Docker로 실행

```bash
git clone https://github.com/authway/authway.git
cd authway
docker-compose -f docker-compose.dev.yml up -d
```

**서비스 URL:**
- Login UI: http://localhost:3001
- Backend API: http://localhost:8080
- MailHog: http://localhost:8025

**다음 단계:**
1. http://localhost:3001 접속
2. 회원가입 진행
3. MailHog에서 인증 이메일 확인
4. 로그인

---

## 아키텍처

```
┌─────────────────────────┐
│    Your Applications    │
│  (Service A, B, C...)   │
└───────────┬─────────────┘
            │ OAuth 2.0
┌───────────▼─────────────┐
│    Authway API (Go)     │
│  - OAuth 2.0/OIDC       │
│  - User Management      │
│  - Email Services       │
└───────────┬─────────────┘
            │
    ┌───────┼───────┐
    │       │       │
┌───▼──┐ ┌──▼──┐ ┌─▼────┐
│Users │ │Redis│ │ SMTP │
│  DB  │ │     │ │      │
└──────┘ └─────┘ └──────┘
```

---

## React SDK 사용

### 설치

```bash
npm install @authway/react
```

### 사용 예시

```tsx
import { AuthProvider, useAuth } from '@authway/react'

function App() {
  return (
    <AuthProvider
      authwayUrl="http://localhost:8080"
      clientId="your-client-id"
      redirectUri="http://localhost:3000/callback"
    >
      <YourApp />
    </AuthProvider>
  )
}

function LoginButton() {
  const { login, isAuthenticated, user } = useAuth()

  if (isAuthenticated) {
    return <div>Welcome, {user?.name}</div>
  }

  return <button onClick={login}>로그인</button>
}
```

---

## 기술 스택

**Backend:**
- Go 1.21+
- Fiber (Web Framework)
- GORM (ORM)
- PostgreSQL 15
- Redis 7

**Frontend:**
- React 18
- TypeScript
- Vite
- Tailwind CSS
- shadcn/ui

**Infrastructure:**
- Docker
- Docker Compose

---

## 문서

- [시작하기](./START-HERE.md) - 1분 빠른 시작
- [Docker 가이드](./DOCKER-GUIDE.md) - 개발 환경 설정
- [테스트 가이드](./TESTING-GUIDE.md) - 기능 테스트
- [React SDK](./packages/sdk/react/README.md) - SDK 사용법
- [프로덕션 배포](./PRODUCTION-DEPLOYMENT.md) - 배포 가이드