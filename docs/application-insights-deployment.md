# Application Insights 배포 가이드

Application Insights가 통합된 Authway 애플리케이션을 Azure에 배포하는 방법을 설명합니다.

## 사전 준비사항

✅ `.env.azure` 파일에 Application Insights connection string이 설정되어 있어야 합니다.
```bash
AUTHWAY_APPLICATIONINSIGHTS_CONNECTION_STRING=InstrumentationKey=...;IngestionEndpoint=...;LiveEndpoint=...;ApplicationId=...
AUTHWAY_APPLICATIONINSIGHTS_ENABLED=true
```

## 배포 순서

### 1. Backend API 배포

Backend API는 **두 가지 방법** 중 하나를 선택할 수 있습니다:

#### 방법 A: 환경 변수만 업데이트 (빠름, 재빌드 불필요)

이미 최신 코드가 배포되어 있고 환경 변수만 업데이트하고 싶을 때 사용합니다.

```powershell
cd scripts
.\publish-api.ps1 -UpdateEnvOnly
```

**동작:**
- `.env.azure` 파일을 자동으로 읽습니다
- Azure Container App에 환경 변수를 설정합니다:
  - `AUTHWAY_APPLICATIONINSIGHTS_CONNECTION_STRING`
  - `AUTHWAY_APPLICATIONINSIGHTS_ENABLED=true`
- Container App을 자동으로 재시작합니다

**예상 시간:** 약 1-2분

#### 방법 B: 전체 재배포 (코드 변경사항 포함)

코드가 변경되었거나 새로운 버전을 배포할 때 사용합니다.

```powershell
cd scripts
.\publish-api.ps1
```

**동작:**
1. Docker 이미지 빌드
2. Azure Container Registry에 푸시
3. Container App 업데이트
4. **환경 변수는 자동으로 설정되지 않습니다!**

**배포 후 환경 변수 설정:**
```powershell
.\publish-api.ps1 -UpdateEnvOnly
```

**예상 시간:** 약 5-10분 (빌드 시간 포함)

### 2. Frontend 배포

Login UI와 Admin Dashboard를 배포합니다.

#### 2.1 Login UI 배포

```powershell
cd scripts
.\publish-login-ui.ps1
```

**동작:**
- `.env.production` 파일에서 환경 변수를 읽습니다
- `VITE_APPLICATIONINSIGHTS_CONNECTION_STRING`이 빌드에 자동으로 포함됩니다
- Azure Static Web Apps에 배포됩니다

**예상 시간:** 약 2-3분

#### 2.2 Admin Dashboard 배포

```powershell
cd scripts
.\publish-admin-ui.ps1
```

**동작:**
- `.env.production` 파일에서 환경 변수를 읽습니다
- `VITE_APPLICATIONINSIGHTS_CONNECTION_STRING`이 빌드에 자동으로 포함됩니다
- Azure Static Web Apps에 배포됩니다

**예상 시간:** 약 2-3분

## 전체 배포 스크립트 (한번에 모두 배포)

모든 애플리케이션을 한번에 배포하려면:

```powershell
# Backend API 배포 + 환경 변수 설정
cd scripts
.\publish-api.ps1
.\publish-api.ps1 -UpdateEnvOnly

# Frontend 배포
.\publish-login-ui.ps1
.\publish-admin-ui.ps1
```

## 배포 확인

### Backend API 확인

```powershell
# 로그 확인 (Application Insights 초기화 메시지 확인)
az containerapp logs show --name authway-api --resource-group authway --follow

# 예상 로그:
# "Application Insights initialized successfully" {"enabled": true}
```

### Frontend 확인

1. 브라우저에서 애플리케이션을 엽니다:
   - Login UI: https://auth.iyulab.com
   - Admin Dashboard: https://authway-admin.iyulab.com

2. 브라우저 개발자 도구 (F12) → Console 탭 확인:
   ```
   Application Insights: Initialized successfully
   ```

### Azure Portal에서 텔레메트리 확인

1. Azure Portal → Application Insights 리소스 (`authway-app-insights`)
2. **Live Metrics** 메뉴: 실시간 요청 확인
3. **Failures** 메뉴: 에러 추적 확인
4. **Performance** 메뉴: 성능 메트릭 확인

**참고:** 텔레메트리 데이터가 Azure Portal에 나타나기까지 **2-5분** 정도 소요될 수 있습니다.

## 문제 해결

### Backend: Application Insights가 초기화되지 않음

**증상:** 로그에 "Application Insights is disabled or not configured" 메시지가 나타남

**해결 방법:**
1. `.env.azure` 파일에 connection string이 설정되어 있는지 확인
2. 환경 변수를 다시 설정:
   ```powershell
   .\publish-api.ps1 -UpdateEnvOnly
   ```
3. Azure Portal → Container Apps → authway-api → Environment variables에서 확인:
   - `AUTHWAY_APPLICATIONINSIGHTS_CONNECTION_STRING`
   - `AUTHWAY_APPLICATIONINSIGHTS_ENABLED=true`

### Frontend: Application Insights가 초기화되지 않음

**증상:** 브라우저 콘솔에 "Application Insights: Not configured (optional)" 메시지

**해결 방법:**
1. `.env.production` 파일에 connection string이 있는지 확인:
   - `packages/web/login-ui/.env.production`
   - `packages/web/admin-dashboard/.env.production`
2. 파일을 수정했다면 **반드시 재빌드 및 재배포**:
   ```powershell
   .\publish-login-ui.ps1
   .\publish-admin-ui.ps1
   ```

### Azure Portal에 데이터가 나타나지 않음

**확인 사항:**
1. **대기 시간:** 첫 배포 후 2-5분 대기
2. **Live Metrics 확인:** 실시간 데이터가 먼저 나타납니다
3. **애플리케이션 접속:** 실제로 API 요청이나 페이지 방문이 있어야 데이터가 생성됩니다
4. **Connection String 확인:** Azure Portal → Application Insights → Properties에서 정확한 connection string 확인

## Application Insights 비활성화 방법

운영 중에 Application Insights를 비활성화하려면:

### Backend
```powershell
# .env.azure 파일에서 주석 처리
# AUTHWAY_APPLICATIONINSIGHTS_CONNECTION_STRING=...
# AUTHWAY_APPLICATIONINSIGHTS_ENABLED=false

# 환경 변수 업데이트
.\publish-api.ps1 -UpdateEnvOnly
```

### Frontend
```bash
# .env.production 파일에서 주석 처리
# VITE_APPLICATIONINSIGHTS_CONNECTION_STRING=...

# 재빌드 및 재배포
.\publish-login-ui.ps1
.\publish-admin-ui.ps1
```

## 참고 자료

- [Azure Application Insights 문서](https://docs.microsoft.com/azure/azure-monitor/app/app-insights-overview)
- [Application Insights Go SDK](https://github.com/microsoft/ApplicationInsights-Go)
- [Application Insights JavaScript SDK](https://github.com/microsoft/ApplicationInsights-JS)
