# Azure 커스텀 도메인 설정 후 환경 변수 업데이트 가이드

## 적용된 도메인
- https://authway.iyulab.com → Hydra (Public/Admin API)
- https://authway-api.iyulab.com → Backend API
- https://authway-admin.iyulab.com → Admin UI
- https://auth.iyulab.com → Login UI

## 1. Backend API 환경 변수 업데이트

Azure Portal → Container Apps → `authway-backend` → Environment variables

### 필수 업데이트
```bash
# Hydra Admin URL (내부 통신 사용 - 권장)
HYDRA_ADMIN_URL=http://authway-hydra:8080/admin

# 또는 외부 도메인 사용
HYDRA_ADMIN_URL=https://authway.iyulab.com/admin

# App Base URL (CORS 및 리디렉션용)
APP_BASE_URL=https://authway-api.iyulab.com

# CORS Allowed Origins (콤마로 구분)
CORS_ALLOWED_ORIGINS=https://authway-admin.iyulab.com,https://auth.iyulab.com,http://localhost:5173,http://localhost:3000
```

### 변경 후 Backend API 재시작 필수!

## 2. Admin UI 환경 변수 확인

Azure Portal → Static Web Apps → `authway-admin` → Configuration

```bash
# Backend API URL
VITE_API_URL=https://authway-api.iyulab.com
```

## 3. Login UI 환경 변수 확인

Azure Portal → Static Web Apps → `authway-login` → Configuration

```bash
# Backend API URL (로그인/회원가입용)
VITE_API_URL=https://authway-api.iyulab.com

# Hydra Public URL (OAuth2 리디렉션용)
VITE_HYDRA_PUBLIC_URL=https://authway.iyulab.com
```

## 4. Admin UI에서 클라이언트 생성 시 설정

### Redirect URIs 예시 (ASP.NET Sample용)
```
https://localhost:5001/signin-oidc
http://localhost:5000/signin-oidc
```

### Grant Types
```
authorization_code
refresh_token
```

### Scopes
```
openid
profile
email
```

## 5. ASP.NET Sample 설정

### appsettings.json & appsettings.Development.json
```json
{
  "Authway": {
    "HydraPublicUrl": "https://authway.iyulab.com",
    "ClientId": "클라이언트_ID",
    "ClientSecret": "클라이언트_시크릿"
  }
}
```

## 6. 테스트 순서

### Step 1: Backend API 연결 확인
```bash
curl https://authway-api.iyulab.com/health
# 응답: {"status":"ok","service":"authway","version":"..."}
```

### Step 2: Hydra 연결 확인
```bash
curl https://authway.iyulab.com/health/ready
# 응답: {"status":"ok"}
```

### Step 3: Admin UI에서 새 클라이언트 생성
1. https://authway-admin.iyulab.com 접속
2. 로그인 (admin / 관리자비밀번호)
3. 테넌트 선택
4. Clients 메뉴 → Create Client
5. Redirect URIs에 `https://localhost:5001/signin-oidc` 입력
6. Submit → Client ID와 Secret 복사

### Step 4: Backend API 로그 확인
Azure Portal → Container Apps → authway-backend → Log stream

성공 시 로그:
```
Client created successfully in database and Hydra
```

실패 시 로그:
```
Failed to register client in Hydra
```

### Step 5: Hydra에 클라이언트 등록 확인
```bash
cd scripts
powershell -ExecutionPolicy Bypass -File check-hydra-client.ps1 -ClientId "클라이언트_ID"
```

성공 시 출력:
```
[SUCCESS] Client EXISTS in Hydra!
Client Details:
  Client ID: authway_...
  Client Name: ...
  Redirect URIs:
    - https://localhost:5001/signin-oidc
```

### Step 6: ASP.NET Sample 테스트
1. appsettings.json에 ClientId, ClientSecret 설정
2. `dotnet run` 실행
3. https://localhost:5001 접속
4. "Login" 버튼 클릭
5. https://auth.iyulab.com/login 으로 리디렉션되어야 함
6. 로그인 후 다시 localhost:5001로 돌아와야 함

## 7. 문제 해결

### "invalid_client" 에러가 계속 발생하는 경우

**원인 1**: Backend API가 Hydra Admin URL에 연결 못함
```bash
# Backend API 로그 확인
# "Failed to register client in Hydra" 메시지 찾기
```

**해결**:
- HYDRA_ADMIN_URL 환경 변수 확인
- 내부 통신 권장: `http://authway-hydra:8080/admin`
- Backend API 재시작

**원인 2**: 기존 클라이언트가 Hydra에 등록 안 됨
```bash
# 마이그레이션 스크립트 실행 (로컬에서)
cd scripts
go run migrate-clients-to-hydra.go
```

**원인 3**: Redirect URI 불일치
```bash
# Admin UI에서 클라이언트 설정 확인
# ASP.NET의 redirect_uri와 정확히 일치해야 함
```

## 8. 보안 체크리스트

- [ ] Backend API의 CORS 설정에 필요한 도메인만 포함
- [ ] Admin UI는 관리자만 접근 가능 (API Key 인증)
- [ ] Client Secret은 절대 프론트엔드 코드에 노출하지 않음
- [ ] Production 환경에서는 localhost redirect URI 제거
- [ ] HTTPS만 사용 (HTTP redirect URI는 개발용만)

## 9. 환경별 설정 요약

### Development (로컬)
```bash
HYDRA_PUBLIC_URL=https://authway.iyulab.com
BACKEND_API_URL=https://authway-api.iyulab.com
ADMIN_UI_URL=http://localhost:5173
LOGIN_UI_URL=http://localhost:3000
```

### Production (Azure)
```bash
HYDRA_PUBLIC_URL=https://authway.iyulab.com
BACKEND_API_URL=https://authway-api.iyulab.com
ADMIN_UI_URL=https://authway-admin.iyulab.com
LOGIN_UI_URL=https://auth.iyulab.com
```
