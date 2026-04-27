# Authway Azure 단일 배포 계획

## 개요
모든 서비스를 **authway** 단일 리소스그룹에 배포합니다.

## 구성 요소

### 이미 배포된 리소스
1. **PostgreSQL**: `authway.postgres.database.azure.com`
   - Azure Database for PostgreSQL 유연한 서버
   - 멀티 테넌트 데이터베이스

2. **Redis**: `authway.redis.cache.windows.net`
   - Azure Cache for Redis
   - 세션 및 캐시 저장소

3. **Static Web App - Admin**: `admin.authway.in`
   - Azure Static Web App (배포됨)
   - URL: https://polite-pond-0fdbbbd00.3.azurestaticapps.net
   - Custom Domain: https://admin.authway.in

4. **Static Web App - Auth UI**: `auth.authway.in`
   - Azure Static Web App (배포됨)
   - URL: https://happy-plant-063723400.3.azurestaticapps.net
   - Custom Domain: https://auth.authway.in

### 배포 필요한 Container Apps

1. **authway-hydra** (`oauth.authway.in`)
   - Ory Hydra OAuth 2.0 / OIDC 서버
   - Container: oryd/hydra:v2.2.0 + nginx (사이드카)
   - Ports: 4444 (public), 4445 (admin)
   - 리소스: 0.25 CPU, 0.5Gi Memory

2. **authway-api** (`api.authway.in`)
   - Central Backend API (Go)
   - 테넌트/사용자 관리, 내부 API
   - Port: 8080
   - 리소스: 0.5 CPU, 1Gi Memory

3. **auth-api** (`auth-api.authway.in`)
   - Auth Backend (Go)
   - 소비자 앱 인터페이스, OAuth 플로우
   - Port: 8081
   - 리소스: 0.5 CPU, 1Gi Memory

## 배포 스크립트

### 1. 인프라 구성
```bash
scripts/deploy/setup-infrastructure.ps1
```
- Container App Environment 생성
- Container Apps 리소스 생성 (비용 최적화 설정)
- 환경 변수 및 시크릿 구성

### 2. 개별 서비스 배포
```bash
scripts/deploy/publish-admin.ps1      # Admin Dashboard
scripts/deploy/publish-auth-ui.ps1    # Auth UI
scripts/deploy/publish-hydra.ps1      # Hydra Container
scripts/deploy/publish-api.ps1        # Central API
scripts/deploy/publish-auth-api.ps1   # Auth Backend
```

### 3. 전체 배포
```bash
scripts/deploy/deploy-all.ps1
```
- 모든 서비스 순차적 배포
- 헬스 체크 및 검증

## 환경 변수 구성

환경 변수는 `scripts/deploy/.env` 파일에서 관리합니다.

### 필수 환경 변수
```bash
# Resource Group
RESOURCE_GROUP=authway
LOCATION=koreacentral
CONTAINER_ENVIRONMENT=authway-env

# Database (이미 배포됨)
AUTHWAY_DATABASE_HOST=authway.postgres.database.azure.com
AUTHWAY_DATABASE_PORT=5432
AUTHWAY_DATABASE_NAME=authway
AUTHWAY_DATABASE_USER=authway
AUTHWAY_DATABASE_PASSWORD=<secret>

# Redis (이미 배포됨)
AUTHWAY_REDIS_HOST=authway.redis.cache.windows.net
AUTHWAY_REDIS_PORT=6380
AUTHWAY_REDIS_PASSWORD=<secret>
AUTHWAY_REDIS_TLS_ENABLED=true

# Container Apps
CONTAINER_APP_HYDRA=authway-hydra
CONTAINER_APP_API=authway-api
CONTAINER_APP_AUTH_API=auth-api

# Static Web Apps (이미 배포됨)
STATIC_WEB_APP_ADMIN=authway-admin
STATIC_WEB_APP_AUTH_UI=auth-ui
ADMIN_DEPLOYMENT_TOKEN=<token>
AUTH_UI_DEPLOYMENT_TOKEN=<token>

# Container Registry
GITHUB_TOKEN=<ghcr-token>
GITHUB_USER=iyulab

# URLs
HYDRA_ISSUER=https://oauth.authway.in
LOGIN_URL=https://auth.authway.in/login
CONSENT_URL=https://auth.authway.in/consent
ERROR_URL=https://auth.authway.in/error
API_URL=https://api.authway.in
AUTH_API_URL=https://auth-api.authway.in
ADMIN_URL=https://admin.authway.in
```

## Container App 리소스 최적화

### 비용 경제적 구성 (Consumption Plan)

#### authway-hydra
- CPU: 0.25 cores
- Memory: 0.5Gi
- Min Replicas: 0 (scale to zero)
- Max Replicas: 3
- Scale Rule: HTTP requests (100 concurrent)

#### authway-api
- CPU: 0.5 cores
- Memory: 1Gi
- Min Replicas: 0 (scale to zero)
- Max Replicas: 5
- Scale Rule: HTTP requests (100 concurrent)

#### auth-api
- CPU: 0.5 cores
- Memory: 1Gi
- Min Replicas: 0 (scale to zero)
- Max Replicas: 5
- Scale Rule: HTTP requests (100 concurrent)

### 예상 비용 (월간)
- Container Apps: ~$15-30 (저트래픽 시)
- PostgreSQL: ~$50 (B1ms)
- Redis: ~$20 (Basic C0)
- Static Web Apps: Free (Standard plan)
- **Total: ~$85-100/month**

## 배포 순서

1. **인프라 구성** (1회만 실행)
   ```bash
   ./scripts/deploy/setup-infrastructure.ps1
   ```

2. **Container Apps 배포**
   ```bash
   # Hydra 먼저 배포 (OAuth 서버)
   ./scripts/deploy/publish-hydra.ps1

   # Central API 배포
   ./scripts/deploy/publish-api.ps1

   # Auth Backend 배포
   ./scripts/deploy/publish-auth-api.ps1
   ```

3. **Static Web Apps 배포**
   ```bash
   # Admin Dashboard
   ./scripts/deploy/publish-admin.ps1

   # Auth UI
   ./scripts/deploy/publish-auth-ui.ps1
   ```

4. **또는 전체 배포**
   ```bash
   ./scripts/deploy/deploy-all.ps1
   ```

## 헬스 체크

배포 후 다음 엔드포인트들이 정상 작동하는지 확인:

- Hydra Public: https://oauth.authway.in/.well-known/openid-configuration
- Hydra Admin: https://oauth.authway.in/admin/health/ready
- Central API: https://api.authway.in/health
- Auth Backend: https://auth-api.authway.in/health
- Admin Dashboard: https://admin.authway.in
- Auth UI: https://auth.authway.in

## 롤백 전략

Container App 배포 실패 시:
```bash
# 이전 리비전으로 롤백
az containerapp revision list --name <app-name> --resource-group authway
az containerapp revision activate --name <app-name> --resource-group authway --revision <revision-name>
```

## 모니터링

- Azure Portal → Container Apps → Metrics
- Application Insights (추후 구성 가능)
- Log Analytics Workspace (Container App Environment)

## 보안 고려사항

1. **시크릿 관리**: Azure Key Vault 사용 권장 (추후)
2. **HTTPS**: 모든 엔드포인트 HTTPS 강제
3. **CORS**: 프로덕션 환경에 맞게 제한적 구성
4. **Database**: SSL 연결 필수
5. **Container Registry**: Private registry (GHCR) 사용
