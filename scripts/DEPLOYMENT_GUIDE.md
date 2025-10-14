# 🚀 Authway 배포 가이드

Azure 프로덕션 환경에 Authway를 배포하는 방법을 설명합니다.

---

## 📋 배포 전 준비사항

### 1. Azure CLI 설치 및 로그인
```powershell
# Azure CLI 로그인
az login

# 구독 확인
az account show
```

### 2. Static Web Apps 배포 토큰 확인

#### 방법 1: .env 파일 사용 (권장)
```powershell
# scripts/.env 파일 생성 (.env.example 참고)
cd D:\data\Authway\scripts
copy .env.example .env

# .env 파일 편집하여 토큰 입력
# ADMIN_DEPLOYMENT_TOKEN=your-admin-token-here
# LOGIN_DEPLOYMENT_TOKEN=your-login-token-here
```

**장점**:
- Git에서 자동으로 무시됨 (보안)
- 배포 스크립트가 자동으로 로드
- 토큰을 한 곳에서 관리

#### 방법 2: 환경변수 설정 (현재 세션)
```powershell
# Azure Portal에서 토큰 복사
# Static Web Apps → authway-admin → Manage deployment token
$env:ADMIN_DEPLOYMENT_TOKEN = "your-admin-token-here"

# Static Web Apps → authway-login → Manage deployment token
$env:LOGIN_DEPLOYMENT_TOKEN = "your-login-token-here"
```

#### 방법 3: 영구 환경변수 설정
```powershell
# Windows 시스템 환경변수에 추가
[System.Environment]::SetEnvironmentVariable("ADMIN_DEPLOYMENT_TOKEN", "your-token", "User")
[System.Environment]::SetEnvironmentVariable("LOGIN_DEPLOYMENT_TOKEN", "your-token", "User")
```

**토큰 우선순위**: 파라미터 > 환경변수 > .env 파일

---

## 🎨 Admin Dashboard 배포

### 기본 배포
```powershell
cd D:\data\Authway
.\scripts\publish-admin-ui.ps1
```

### 토큰을 파라미터로 전달
```powershell
.\scripts\publish-admin-ui.ps1 -DeploymentToken "your-token"
```

### 배포 과정
1. ✅ 의존성 확인 (node_modules)
2. ✅ .env.production 파일 확인
3. 🔨 프로덕션 빌드 (`npm run build`)
4. ☁️ Azure Static Web Apps 배포
5. 🎉 https://authway-admin.iyulab.com 배포 완료

---

## 🔐 Login UI 배포

### 기본 배포
```powershell
cd D:\data\Authway
.\scripts\publish-login-ui.ps1
```

### 토큰을 파라미터로 전달
```powershell
.\scripts\publish-login-ui.ps1 -DeploymentToken "your-token"
```

### 배포 과정
1. ✅ 의존성 확인 (node_modules)
2. ✅ .env.production 파일 확인
3. 🔨 프로덕션 빌드 (`npm run build`)
4. ☁️ Azure Static Web Apps 배포
5. 🎉 https://auth.iyulab.com 배포 완료

---

## 🔧 Backend API 배포

### 기본 배포 (로컬에서 Docker 빌드)
```powershell
cd D:\data\Authway
.\scripts\publish-api.ps1
```

### Azure에서 빌드 (권장)
```powershell
# ACR에서 직접 빌드 (로컬 Docker 불필요)
.\scripts\publish-api.ps1 -UseAzureBuild
```

### 이미지 빌드 건너뛰기
```powershell
# 이미 빌드된 이미지로 배포만 수행
.\scripts\publish-api.ps1 -SkipBuild
```

### 커스텀 이미지 태그
```powershell
# 특정 버전 태그로 빌드 및 배포
.\scripts\publish-api.ps1 -ImageTag "v1.2.0"
```

### 배포 과정
1. 🔑 Azure 인증 확인
2. 🔨 Docker 이미지 빌드
3. 🏷️ 이미지 태깅
4. 🔐 ACR 로그인
5. ☁️ ACR에 푸시
6. 📦 Container App 업데이트
7. 🎉 https://authway-api.iyulab.com 배포 완료

---

## 📦 전체 배포

모든 컴포넌트를 한 번에 배포하려면:

```powershell
# 1. Backend API
.\scripts\publish-api.ps1 -UseAzureBuild

# 2. Admin Dashboard
.\scripts\publish-admin-ui.ps1

# 3. Login UI
.\scripts\publish-login-ui.ps1
```

---

## 🔍 배포 확인

### Health Check
```powershell
# Backend API
curl https://authway-api.iyulab.com/health

# 응답 예시
# {"service":"authway","status":"ok","timestamp":"2025-10-14T12:00:00Z","version":"0.1.0"}
```

### 브라우저 테스트
```
Admin Dashboard: https://authway-admin.iyulab.com
Login UI:        https://auth.iyulab.com
```

### Container App 로그 확인
```powershell
az containerapp logs show \
  --name authway-api \
  --resource-group authway \
  --follow
```

---

## ⚠️ 문제 해결

### 배포 토큰 오류
```
❌ 배포 토큰이 필요합니다.
```
**해결**:
1. Azure Portal에서 Static Web Apps의 deployment token 확인
2. 환경변수 설정 또는 파라미터로 전달

### Docker 빌드 실패
```
❌ Docker 빌드 실패
```
**해결**:
1. Docker Desktop이 실행 중인지 확인
2. `-UseAzureBuild` 옵션 사용 (Azure에서 빌드)

### ACR 로그인 실패
```
❌ ACR 로그인 실패
```
**해결**:
```powershell
az login
az acr login --name authwayacr
```

### Container App 업데이트 실패
```
❌ Container App 업데이트 실패
```
**해결**:
```powershell
# Container App 상태 확인
az containerapp show \
  --name authway-api \
  --resource-group authway \
  --query "properties.runningStatus"

# 리비전 확인
az containerapp revision list \
  --name authway-api \
  --resource-group authway
```

---

## 📊 환경변수 관리

### .env.production 파일 업데이트

**Admin Dashboard** (`packages/web/admin-dashboard/.env.production`):
```env
VITE_API_URL=https://authway-api.iyulab.com
VITE_HYDRA_PUBLIC_URL=http://localhost:4444
```

**Login UI** (`packages/web/login-ui/.env.production`):
```env
VITE_API_URL=https://authway-api.iyulab.com
VITE_HYDRA_PUBLIC_URL=http://localhost:4444
```

### Backend 환경변수 업데이트

```powershell
# .env.azure 파일 수정 후
az containerapp update \
  --name authway-api \
  --resource-group authway \
  --set-env-vars @.env.azure
```

---

## 🔄 롤백 절차

### Frontend 롤백
Static Web Apps는 자동으로 이전 배포를 유지합니다.
Azure Portal에서 이전 버전으로 전환 가능합니다.

### Backend 롤백
```powershell
# 이전 이미지 태그로 롤백
.\scripts\publish-api.ps1 -ImageTag "v1.1.0" -SkipBuild

# 또는 특정 리비전으로 롤백
az containerapp revision set-mode \
  --name authway-api \
  --resource-group authway \
  --mode single \
  --revision authway-api--0000003
```

---

## 📝 배포 체크리스트

### 배포 전
- [ ] .env.production 파일 확인 및 업데이트
- [ ] Azure CLI 로그인 확인
- [ ] 배포 토큰 준비 (Static Web Apps)
- [ ] 코드 변경사항 커밋 및 푸시
- [ ] 로컬 테스트 완료

### 배포 후
- [ ] Health Check 확인
- [ ] Admin Dashboard 접속 테스트
- [ ] Login UI 접속 테스트
- [ ] 주요 기능 동작 확인
- [ ] Container App 로그 확인
- [ ] 브라우저 캐시 클리어 공지

---

## 🎯 자동화 (선택사항)

### GitHub Actions로 자동 배포

`.github/workflows/deploy.yml`:
```yaml
name: Deploy to Azure

on:
  push:
    branches: [ main ]

jobs:
  deploy-backend:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v3
      - name: Deploy Backend
        run: .\scripts\publish-api.ps1 -UseAzureBuild

  deploy-frontend:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v3
      - name: Deploy Admin Dashboard
        run: .\scripts\publish-admin-ui.ps1 -DeploymentToken ${{ secrets.ADMIN_TOKEN }}
      - name: Deploy Login UI
        run: .\scripts\publish-login-ui.ps1 -DeploymentToken ${{ secrets.LOGIN_TOKEN }}
```

---

**작성일**: 2025-10-14
**버전**: Authway v0.1.0
