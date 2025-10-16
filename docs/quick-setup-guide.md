# Authway 빠른 설정 가이드

## 🚀 커스텀 도메인 적용 후 설정

### 1. Hydra 환경 변수 업데이트 및 재시작

```powershell
.\scripts\publish-hydra.ps1 -UpdateEnvOnly
```

**설정되는 환경 변수**:
- `URLS_SELF_ISSUER=https://authway.iyulab.com`
- `URLS_SELF_PUBLIC=https://authway.iyulab.com`
- `URLS_LOGIN=https://auth.iyulab.com/login`
- `URLS_CONSENT=https://auth.iyulab.com/consent`
- `URLS_ERROR=https://auth.iyulab.com/error`
- `SERVE_COOKIES_SAME_SITE_MODE=Lax`

### 2. Backend API 환경 변수 업데이트 및 재시작

```powershell
.\scripts\publish-api.ps1 -UpdateEnvOnly
```

**설정되는 환경 변수**:
- `AUTHWAY_HYDRA_ADMIN_URL=https://authway-hydra.lemonfield-03f5fb88.koreacentral.azurecontainerapps.io` (Azure Container Apps FQDN 사용, Hydra Client가 /admin/clients 경로 추가)
- `AUTHWAY_CORS_ALLOWED_ORIGINS=https://authway-admin.iyulab.com,https://auth.iyulab.com,http://localhost:5173,http://localhost:3000`

### 3. 환경 변수 검증

```powershell
.\scripts\check-env-vars.ps1
```

모든 서비스의 환경 변수가 올바르게 설정되었는지 확인합니다.

### 4. 클라이언트 생성 및 테스트

#### Step 1: Admin UI에서 클라이언트 생성
1. https://authway-admin.iyulab.com 접속
2. 로그인 (admin / 관리자비밀번호)
3. 테넌트 선택
4. Clients → Create Client
5. 설정:
   - **Name**: ASP.NET Sample
   - **Redirect URIs**:
     ```
     https://localhost:5001/signin-oidc
     http://localhost:5000/signin-oidc
     ```
   - **Grant Types**: `authorization_code`, `refresh_token`
   - **Scopes**: `openid`, `profile`, `email`
6. Submit → Client ID와 Secret 복사

#### Step 2: Hydra 등록 확인

```powershell
.\scripts\check-hydra-client.ps1 -ClientId "클라이언트_ID"
```

**성공 시 출력**:
```
[SUCCESS] Client EXISTS in Hydra!
Client Details:
  Client ID: authway_...
  Redirect URIs:
    - https://localhost:5001/signin-oidc
```

**실패 시**: Backend API 로그 확인
```powershell
az containerapp logs show --name authway-api --resource-group authway --follow
```

성공 메시지: `"Client created successfully in database and Hydra"`
실패 메시지: `"Failed to register client in Hydra"`

#### Step 3: ASP.NET Sample 테스트

1. **appsettings.json 업데이트**:
```json
{
  "Authway": {
    "HydraPublicUrl": "https://authway.iyulab.com",
    "ClientId": "클라이언트_ID",
    "ClientSecret": "클라이언트_시크릿"
  }
}
```

2. **appsettings.Development.json도 동일하게 업데이트**

3. **실행**:
```bash
cd samples/asp-sample
dotnet run
```

4. **테스트**:
   - https://localhost:5001 접속
   - "Login" 버튼 클릭
   - https://auth.iyulab.com/login 으로 리디렉션
   - 로그인 후 localhost:5001로 돌아오는지 확인

## 🔧 문제 해결

### "invalid_client" 에러

**원인 1**: Backend API가 Hydra에 클라이언트를 등록하지 못함

```powershell
# Backend API 로그 확인
az containerapp logs show --name authway-api --resource-group authway --follow
```

**해결**:
```powershell
# Backend API 환경 변수 재설정
.\scripts\publish-api.ps1 -UpdateEnvOnly
```

**원인 2**: Hydra 환경 변수 미설정

```powershell
# Hydra 환경 변수 재설정
.\scripts\publish-hydra.ps1 -UpdateEnvOnly
```

**원인 3**: 기존 클라이언트가 Hydra에 없음

```powershell
# Admin UI에서 클라이언트를 삭제하고 다시 생성
# 또는 마이그레이션 스크립트 실행 (로컬에서)
cd scripts
go run migrate-clients-to-hydra.go
```

### Redirect URI 불일치

Admin UI에서 클라이언트 설정 확인:
- ASP.NET의 `redirect_uri`와 정확히 일치해야 함
- 대소문자, 슬래시(/) 모두 정확해야 함

### CORS 에러

Backend API 환경 변수 확인:
```
CORS_ALLOWED_ORIGINS=https://authway-admin.iyulab.com,https://auth.iyulab.com
```

## 📋 전체 재배포

모든 서비스를 재배포해야 하는 경우:

```powershell
# 1. Hydra 환경 변수만 업데이트 (이미지는 변경 없음)
.\scripts\publish-hydra.ps1 -UpdateEnvOnly

# 2. Backend API 재배포 (이미지 + 환경 변수)
.\scripts\publish-api.ps1

# 3. Admin UI 재배포
.\scripts\publish-admin-ui.ps1

# 4. Login UI 재배포
.\scripts\publish-login-ui.ps1
```

## 📊 모니터링

### Backend API 로그 모니터링
```powershell
az containerapp logs show --name authway-api --resource-group authway --follow
```

### Hydra 로그 모니터링
```powershell
az containerapp logs show --name authway-hydra --resource-group authway --follow
```

### Health Check
```bash
# Hydra
curl https://authway.iyulab.com/health/ready

# Backend API
curl https://authway-api.iyulab.com/health
```

## 🔐 보안 체크리스트

- [ ] Admin UI는 관리자만 접근 가능 (API Key 인증)
- [ ] Client Secret은 절대 프론트엔드 코드에 노출하지 않음
- [ ] Production 환경에서는 localhost redirect URI 제거
- [ ] HTTPS만 사용 (HTTP는 개발용만)
- [ ] CORS는 필요한 도메인만 허용

## 📚 주요 URL

| 서비스 | URL | 용도 |
|--------|-----|------|
| Hydra | https://authway.iyulab.com | OAuth2/OIDC 서버 |
| Backend API | https://authway-api.iyulab.com | 비즈니스 로직 |
| Admin UI | https://authway-admin.iyulab.com | 클라이언트 관리 |
| Login UI | https://auth.iyulab.com | 로그인/회원가입 |
