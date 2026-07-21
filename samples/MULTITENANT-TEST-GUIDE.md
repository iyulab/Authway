# Multi-Tenant OAuth 로컬 테스트 가이드

기존 샘플 서비스(AppleService, BananaService, ChocolateService)를 사용하여 multi-tenant OAuth를 테스트합니다.

## 아키텍처

```
중앙 API (localhost:8080)
│
├─ Fruits Tenant (SSO 공유)
│  ├─ AppleService (localhost:9001)
│  └─ BananaService (localhost:9002)
│
└─ Sweets Tenant (독립 인증)
   └─ ChocolateService (localhost:9003)
```

## 테스트 시나리오

### 시나리오 1: Tenant 내 SSO 공유 (Fruits)
```
1. AppleService (localhost:9001) 로그인
2. BananaService (localhost:9002) 접속
3. 자동 로그인 확인 ✓ (같은 Fruits tenant)
```

### 시나리오 2: Tenant 간 격리 (Fruits vs Sweets)
```
1. AppleService (localhost:9001) 로그인 (Fruits tenant)
2. ChocolateService (localhost:9003) 접속 (Sweets tenant)
3. 재로그인 필요 확인 ✓ (다른 tenant)
```

## 빠른 시작

### 1. 인프라 및 백엔드 시작
```powershell
.\start-dev.ps1
```

이 명령은 자동으로:
- PostgreSQL, Redis, MailHog, Hydra 시작
- Backend API 시작 (localhost:8080)
- Admin Dashboard 시작 (localhost:3000)
- Login UI 시작 (localhost:5000)
- Tenant 및 Client 자동 등록 (setup-clients.ps1)

### 2. 샘플 서비스 시작
```powershell
.\samples\start-all-services.ps1
```

3개의 터미널이 열립니다:
- 🍎 AppleService (localhost:9001)
- 🍌 BananaService (localhost:9002)
- 🍫 ChocolateService (localhost:9003)

## 수동 실행 (옵션)

### Backend만 시작
```powershell
cd src\server
go run cmd/main.go
```

### 각 서비스 개별 시작
```powershell
# AppleService
cd samples\AppleService
go run main.go

# BananaService (새 터미널)
cd samples\BananaService
go run main.go

# ChocolateService (새 터미널)
cd samples\ChocolateService
go run main.go
```

## 테스트 방법

### 1. Tenant 내 SSO 테스트 (Fruits)

1. **AppleService에 로그인**
   ```
   http://localhost:9001 접속
   → "Login with Authway" 클릭
   → Login UI (localhost:5000)로 리디렉션
   → Google 로그인
   → AppleService로 돌아와서 프로필 확인
   ```

2. **BananaService 자동 로그인 확인**
   ```
   http://localhost:9002 접속
   → "Login with Authway" 클릭
   → 자동 로그인 확인 ✅ (같은 Fruits tenant)
   ```

### 2. Tenant 격리 테스트 (Fruits vs Sweets)

1. **위에서 Fruits tenant 로그인 상태 유지**

2. **ChocolateService 접속**
   ```
   http://localhost:9003 접속
   → "Login with Authway" 클릭
   → 다시 로그인 필요 ✅ (다른 Sweets tenant)
   ```

### 3. Logout 테스트

```
AppleService에서 Logout
→ BananaService에서도 로그아웃 확인 ✅ (같은 tenant)
→ ChocolateService는 영향 없음 ✅ (다른 tenant)
```

## 확인 사항

### Admin Dashboard (localhost:3000)
```
Tenants 페이지:
- fruits tenant 확인
- sweets tenant 확인

Clients 페이지:
- apple-service-client (fruits tenant)
- banana-service-client (fruits tenant)
- chocolate-service-client (sweets tenant)

Users 페이지:
- 로그인한 사용자 정보 확인
- Tenant 정보 함께 표시
```

### MailHog (localhost:8025)
```
이메일 인증 테스트:
- 회원가입 시 인증 이메일 확인
- 비밀번호 재설정 이메일 확인
```

## 중지

```powershell
# 샘플 서비스만 중지
.\samples\start-all-services.ps1 -StopOnly

# 전체 개발 환경 중지
.\stop-dev.ps1
```

## 예상 결과

✅ **SSO 공유 (Fruits tenant)**
- AppleService 로그인 → BananaService 자동 로그인
- AppleService logout → BananaService도 로그아웃

✅ **Tenant 격리 (Fruits vs Sweets)**
- Fruits tenant 로그인 ≠ Sweets tenant 로그인
- 각 tenant별 독립적인 사용자 세션
- Tenant 간 데이터 격리

✅ **중앙 API 공유**
- 모든 tenant가 같은 API (localhost:8080) 사용
- Admin Dashboard에서 모든 tenant 관리 가능
- 통합 데이터베이스에 tenant 정보 함께 저장

## Google OAuth 추가 (선택사항)

현재는 Hydra를 통한 OAuth를 사용하지만, Login UI에서 Google OAuth를 추가하려면:

### 1. Google Cloud Console 설정
```
OAuth 2.0 클라이언트 ID 생성:

승인된 JavaScript 원본:
  http://localhost:5000

승인된 리디렉션 URI:
  http://localhost:5000/callback
  http://localhost:5000/auth/google/callback
```

### 2. Login UI 환경 변수
```env
VITE_GOOGLE_CLIENT_ID=<your-client-id>
VITE_GOOGLE_REDIRECT_URI=http://localhost:5000/auth/google/callback
```

### 3. 테스트
```
Login UI에서 "Sign in with Google" 버튼 클릭
→ Google 동의 화면에 localhost:5000 표시
→ 인증 후 Login UI로 리디렉션
→ API로 토큰 전송 및 사용자 생성
```

## 트러블슈팅

### 샘플 서비스가 시작되지 않음
```powershell
# 포트 확인
Get-NetTCPConnection -LocalPort 9001,9002,9003 -State Listen

# 수동으로 포트 해제
.\samples\start-all-services.ps1 -StopOnly
```

### Client 등록 실패
```powershell
# 수동으로 client 재등록
.\samples\setup-clients.ps1
```

### Hydra 연결 실패
```powershell
# Docker 컨테이너 확인
docker ps | findstr hydra

# Hydra 재시작
docker compose restart hydra
```

### SSO가 작동하지 않음
```
원인: 같은 tenant의 서비스인데 SSO가 안됨
해결: 
1. Hydra에서 client가 올바른 tenant에 등록되었는지 확인
2. 브라우저 쿠키 삭제 후 재시도
3. Admin Dashboard에서 client의 tenant_id 확인
```

## 다음 단계

이 로컬 테스트가 성공하면:

1. **프로덕션 아키텍처 적용**
   ```
   authway-shared: PostgreSQL + Redis
   
   iyulab-authway:
     - API (authway-api.iyulab.com)
     - Admin (authway-admin.iyulab.com) 
     - Login (auth.iyulab.com)
   
   alldot-authway:
     - Login (auth.alldot.ai)
   
   ironhive-authway:
     - Login (auth.ironhive.com)
   ```

2. **각 Login UI별 Google OAuth Client**
   - auth.iyulab.com → Google Client ID #1
   - auth.alldot.ai → Google Client ID #2
   - auth.ironhive.com → Google Client ID #3

3. **Google 동의 화면 브랜딩**
   - iyulab 사용자 → "iyulab.com" 표시
   - alldot 사용자 → "alldot.ai" 표시
   - ironhive 사용자 → "ironhive.com" 표시
