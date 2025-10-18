# Authway Azure 배포 계획

**버전**: 1.0
**작성일**: 2025-10-14
**대상**: Azure 프로덕션 환경

---

## 📋 목차

1. [아키텍처 개요](#아키텍처-개요)
2. [Azure 서비스 매핑](#azure-서비스-매핑)
3. [네트워크 아키텍처](#네트워크-아키텍처)
4. [보안 구성](#보안-구성)
5. [배포 환경 구성](#배포-환경-구성)
6. [비용 최적화](#비용-최적화)
7. [모니터링 및 관리](#모니터링-및-관리)
8. [배포 단계](#배포-단계)

---

## 🏗️ 아키텍처 개요

### 전체 아키텍처 다이어그램

```
┌─────────────────────────────────────────────────────────────┐
│                    Internet (Users)                         │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
        ┌────────────────────────────┐
        │   Azure Front Door         │
        │   - Global Load Balancing  │
        │   - WAF & DDoS Protection  │
        │   - SSL Termination        │
        └────────┬───────────┬───────┘
                 │           │
     ┌───────────┘           └──────────┐
     │                                  │
     ▼                                  ▼
┌─────────────────────┐    ┌─────────────────────┐
│ Static Web App 1    │    │ Static Web App 2    │
│ (Admin Dashboard)   │    │ (Login UI)          │
│ - React SPA         │    │ - React SPA         │
│ - CDN Distribution  │    │ - CDN Distribution  │
└──────────┬──────────┘    └──────────┬──────────┘
           │                          │
           └────────┬─────────────────┘
                    │ HTTPS
                    ▼
        ┌────────────────────────────┐
        │ Azure Container Apps       │
        │ (Backend API)              │
        │ - Go Fiber Service         │
        │ - Auto-scaling             │
        │ - Managed Identity         │
        └────┬───────────────┬───────┘
             │               │
   ┌─────────┘               └─────────┐
   │                                   │
   ▼                                   ▼
┌──────────────────────┐   ┌──────────────────────┐
│ Azure Database for   │   │ Azure Cache for      │
│ PostgreSQL           │   │ Redis                │
│ - Flexible Server    │   │ - Premium Tier       │
│ - Private Endpoint   │   │ - Private Endpoint   │
│ - Auto Backup        │   │ - Persistence        │
└──────────────────────┘   └──────────────────────┘
           │
           ▼
┌──────────────────────┐
│ Azure Key Vault      │
│ - Secrets Management │
│ - JWT Keys           │
│ - DB Credentials     │
└──────────────────────┘
```

### 주요 특징

✅ **완전 관리형**: 모든 인프라 구성요소가 Azure PaaS
✅ **확장성**: Auto-scaling 지원 (Container Apps, PostgreSQL)
✅ **보안**: Private Endpoints, VNet 격리, Key Vault
✅ **고가용성**: Multi-zone deployment, 자동 백업
✅ **비용 효율**: Consumption 기반, 최적화된 티어 선택

---

## 🎯 Azure 서비스 매핑

### 현재 → Azure 서비스 매핑

| 현재 구성 | Azure 서비스 | 티어/SKU | 이유 |
|-----------|--------------|----------|------|
| Backend API (Go) | **Azure Container Apps** | Consumption | Serverless, auto-scaling, cost-effective |
| Admin Dashboard | **Static Web Apps** | Standard | CDN, SSL, CI/CD 통합 |
| Login UI | **Static Web Apps** | Standard | SPA에 최적, 글로벌 배포 |
| PostgreSQL | **Azure Database for PostgreSQL** | Flexible Server | 고가용성, 관리 간소화 |
| Redis | **Azure Cache for Redis** | Premium | 영구 저장, 고성능 |
| MailHog (dev) | **Azure Communication Services** | Email | 프로덕션 SMTP 대체 |
| Secrets | **Azure Key Vault** | Standard | 중앙 비밀 관리 |
| Monitoring | **Application Insights** | - | 통합 모니터링 |
| Networking | **Azure Front Door** | Standard | WAF, DDoS, 글로벌 LB |

### 추천 서비스 (선택사항)

| 서비스 | 용도 | 우선순위 |
|--------|------|----------|
| **Azure API Management** | API 게이트웨이, rate limiting | Medium |
| **Azure Blob Storage** | 백업, 로그 아카이빙 | Low |
| **Azure Monitor** | 고급 알림, 대시보드 | Medium |
| **GitHub Actions** | CI/CD 파이프라인 | High |

---

## 🌐 네트워크 아키텍처

### VNet 구성

```
authway-vnet (10.0.0.0/16)
│
├── container-apps-subnet (10.0.1.0/24)
│   └── Azure Container Apps Environment
│
├── postgres-subnet (10.0.2.0/24)
│   └── Private Endpoint → PostgreSQL
│
├── redis-subnet (10.0.3.0/24)
│   └── Private Endpoint → Redis
│
└── integration-subnet (10.0.4.0/24)
    └── VNet Integration for outbound traffic
```

### Private Endpoints

**목적**: 데이터베이스를 공개 인터넷에서 격리

```yaml
Private Endpoints:
  - postgres-pe:
      target: Azure Database for PostgreSQL
      subnet: postgres-subnet
      private_ip: 10.0.2.10

  - redis-pe:
      target: Azure Cache for Redis
      subnet: redis-subnet
      private_ip: 10.0.3.10
```

### DNS 구성

```yaml
Private DNS Zones:
  - postgres.database.azure.com
  - redis.cache.windows.net
  - azurecr.io (Container Registry)

Custom Domains:
  - api.authway.com → Container Apps
  - admin.authway.com → Static Web App (Admin)
  - auth.authway.com → Static Web App (Login)
```

---

## 🔒 보안 구성

### 1. Managed Identity (Passwordless Authentication)

```yaml
Container Apps:
  managed_identity: System-assigned
  permissions:
    - Key Vault: Get Secrets
    - PostgreSQL: Database Access
    - Redis: Cache Access
    - Container Registry: Pull Images
```

**장점**:
- 비밀번호 불필요 (자동 로테이션)
- Azure AD 기반 인증
- 보안 감사 추적

### 2. Azure Key Vault Secrets

```yaml
Key Vault Secrets:
  # JWT Secrets
  - authway-jwt-access-secret
  - authway-jwt-refresh-secret

  # Database
  - authway-postgres-password

  # Admin
  - authway-admin-api-key
  - authway-admin-password

  # OAuth
  - authway-google-client-secret
  - authway-github-client-secret

  # Email
  - authway-smtp-password
```

### 3. Network Security Groups (NSG)

```yaml
container-apps-subnet NSG:
  inbound_rules:
    - allow HTTPS (443) from Front Door
    - deny all other inbound

  outbound_rules:
    - allow PostgreSQL (5432) to postgres-subnet
    - allow Redis (6379) to redis-subnet
    - allow HTTPS (443) to internet (for OAuth, email)

postgres-subnet NSG:
  inbound_rules:
    - allow PostgreSQL (5432) from container-apps-subnet
    - deny all other inbound

redis-subnet NSG:
  inbound_rules:
    - allow Redis (6379) from container-apps-subnet
    - deny all other inbound
```

### 4. Azure Front Door WAF Rules

```yaml
WAF Policy:
  mode: Prevention
  rules:
    - OWASP Core Rule Set 3.2
    - Rate Limiting: 1000 req/min per IP
    - Geo-filtering: Allow specific countries only
    - Bot Protection: Challenge suspected bots
```

### 5. PostgreSQL Security

```yaml
Flexible Server:
  ssl_enforcement: Required (TLS 1.2+)
  public_access: Disabled
  firewall_rules: None (Private Endpoint only)
  azure_ad_authentication: Enabled
  backup:
    retention: 30 days
    geo_redundant: true
```

---

## ⚙️ 배포 환경 구성

### Resource Group 구조

```
authway-prod-rg (Primary Region: Korea Central)
├── Networking
│   ├── authway-vnet
│   ├── authway-nsg-container
│   ├── authway-nsg-postgres
│   └── authway-nsg-redis
│
├── Compute
│   └── authway-container-env (Container Apps Environment)
│       └── authway-api-app (Container App)
│
├── Data
│   ├── authway-postgres (PostgreSQL Flexible Server)
│   └── authway-redis (Redis Premium)
│
├── Web
│   ├── authway-admin-static (Static Web App)
│   └── authway-login-static (Static Web App)
│
├── Security
│   └── authway-keyvault
│
├── Monitoring
│   ├── authway-appinsights
│   └── authway-loganalytics
│
└── Networking (Global)
    └── authway-frontdoor
```

### Environment Variables (Container Apps)

```yaml
Container App Configuration:
  environment_variables:
    # App Config
    - name: AUTHWAY_APP_ENVIRONMENT
      value: production

    - name: AUTHWAY_APP_PORT
      value: "8080"

    - name: AUTHWAY_APP_VERSION
      value: "0.1.0"

    # Database (Connection String from Key Vault)
    - name: AUTHWAY_DATABASE_HOST
      secretRef: postgres-host

    - name: AUTHWAY_DATABASE_PASSWORD
      secretRef: postgres-password

    - name: AUTHWAY_DATABASE_SSL_MODE
      value: "require"

    # Redis
    - name: AUTHWAY_REDIS_HOST
      secretRef: redis-host

    - name: AUTHWAY_REDIS_PASSWORD
      secretRef: redis-password

    - name: AUTHWAY_REDIS_SSL
      value: "true"

    # JWT
    - name: AUTHWAY_JWT_ACCESS_TOKEN_SECRET
      secretRef: jwt-access-secret

    - name: AUTHWAY_JWT_REFRESH_TOKEN_SECRET
      secretRef: jwt-refresh-secret

    # Admin
    - name: AUTHWAY_ADMIN_API_KEY
      secretRef: admin-api-key

    - name: AUTHWAY_ADMIN_PASSWORD
      secretRef: admin-password

    # OAuth
    - name: AUTHWAY_GOOGLE_CLIENT_ID
      value: "[from Google Console]"

    - name: AUTHWAY_GOOGLE_CLIENT_SECRET
      secretRef: google-client-secret

    - name: AUTHWAY_GOOGLE_REDIRECT_URL
      value: "https://api.authway.com/auth/google/callback"

    # Email (Azure Communication Services)
    - name: AUTHWAY_EMAIL_SMTP_HOST
      value: "smtp.azurecomm.net"

    - name: AUTHWAY_EMAIL_SMTP_PORT
      value: "587"

    - name: AUTHWAY_EMAIL_SMTP_USER
      secretRef: smtp-user

    - name: AUTHWAY_EMAIL_SMTP_PASSWORD
      secretRef: smtp-password

    - name: AUTHWAY_EMAIL_FROM_ADDRESS
      value: "noreply@authway.com"

    # CORS
    - name: AUTHWAY_CORS_ALLOWED_ORIGINS
      value: "https://admin.authway.com,https://auth.authway.com"

    # Frontend URLs
    - name: AUTHWAY_FRONTEND_LOGIN_URL
      value: "https://auth.authway.com"

    - name: AUTHWAY_FRONTEND_ADMIN_URL
      value: "https://admin.authway.com"
```

---

## 💰 비용 최적화

### 예상 월간 비용 (USD)

| 서비스 | SKU | 예상 비용 | 비고 |
|--------|-----|-----------|------|
| Container Apps | Consumption (0.5 vCPU, 1GB RAM) | $20-50 | 실제 사용량 기반 |
| Static Web Apps | Standard (x2) | $18 | 각 $9/month |
| PostgreSQL Flexible Server | Burstable B2s (2 vCore, 4GB) | $50-70 | Reserved 시 30% 할인 |
| Azure Cache for Redis | Premium P1 (6GB) | $150 | Geo-replication 포함 |
| Key Vault | Standard | $5 | Secrets 저장 비용 |
| Application Insights | Pay-as-you-go | $10-30 | 데이터 수집량 기반 |
| Front Door | Standard | $35 + traffic | Global routing |
| VNet & Private Endpoints | - | $15 | PE당 $7.30 |
| **총 예상 비용** | | **$303-373/month** | 트래픽 별도 |

### 비용 절감 전략

**1. Reserved Instances (1-3년 약정)**
- PostgreSQL: 30-60% 할인
- Redis: 30-50% 할인
- 예상 절감: $60-100/month

**2. Auto-scaling 최적화**
```yaml
Container Apps:
  min_replicas: 0  # 트래픽 없을 때 0으로 scale down
  max_replicas: 10
  scale_rules:
    - http:
        concurrent_requests: 100
```

**3. PostgreSQL Tier 최적화**
```yaml
# Development/Staging
tier: Burstable B1ms (1 vCore, 2GB) → $25/month

# Production (시작)
tier: Burstable B2s (2 vCore, 4GB) → $50/month

# Production (고트래픽)
tier: General Purpose D2s_v3 (2 vCore, 8GB) → $120/month
```

**4. Redis Tier 선택**
```yaml
# Development: Basic C1 (1GB) → $20/month
# Production: Premium P1 (6GB) → $150/month
# 대안: Standard C2 (2.5GB) → $75/month (Persistence 없음)
```

---

## 📊 모니터링 및 관리

### Application Insights 구성

```yaml
Monitoring:
  application_insights:
    instrumentation_key: from-keyvault

    metrics:
      - Request rate (req/s)
      - Response time (ms)
      - Failure rate (%)
      - CPU & Memory usage
      - Database connection pool
      - Redis cache hit rate

    alerts:
      - API response time > 2s
      - Error rate > 5%
      - CPU usage > 80%
      - Memory usage > 90%
      - Database connections > 80%
```

### Log Analytics 쿼리

```kusto
// 느린 API 요청 탐지
requests
| where timestamp > ago(1h)
| where duration > 2000
| summarize count() by name
| order by count_ desc

// 에러율 모니터링
requests
| where timestamp > ago(1h)
| summarize
    total = count(),
    errors = countif(success == false)
| extend error_rate = errors * 100.0 / total

// 데이터베이스 연결 상태
traces
| where message contains "database"
| where timestamp > ago(5m)
```

### Health Checks

```yaml
Container App Health Probes:
  liveness:
    path: /health
    interval: 30s
    timeout: 5s
    failure_threshold: 3

  readiness:
    path: /ready
    interval: 10s
    timeout: 3s
    failure_threshold: 3

  startup:
    path: /health
    interval: 10s
    timeout: 10s
    failure_threshold: 10
```

---

## 🚀 배포 단계

### Phase 1: 인프라 프로비저닝 (1-2시간)

**1.1 Resource Group & Networking**
```bash
# Azure CLI
az group create --name authway-prod-rg --location koreacentral

# VNet 생성
az network vnet create \
  --resource-group authway-prod-rg \
  --name authway-vnet \
  --address-prefix 10.0.0.0/16 \
  --subnet-name container-apps-subnet \
  --subnet-prefix 10.0.1.0/24
```

**1.2 Key Vault & Secrets**
```bash
# Key Vault 생성
az keyvault create \
  --name authway-keyvault \
  --resource-group authway-prod-rg \
  --location koreacentral

# Secrets 추가
az keyvault secret set --vault-name authway-keyvault \
  --name jwt-access-secret --value "[GENERATE-64-CHAR-SECRET]"
```

**1.3 PostgreSQL Flexible Server**
```bash
az postgres flexible-server create \
  --resource-group authway-prod-rg \
  --name authway-postgres \
  --location koreacentral \
  --admin-user authwayadmin \
  --admin-password "[FROM-KEYVAULT]" \
  --sku-name Standard_B2s \
  --tier Burstable \
  --storage-size 32 \
  --version 15 \
  --public-access None
```

**1.4 Redis Cache**
```bash
az redis create \
  --resource-group authway-prod-rg \
  --name authway-redis \
  --location koreacentral \
  --sku Premium \
  --vm-size P1 \
  --enable-non-ssl-port false
```

### Phase 2: 컨테이너 환경 구성 (30분)

**2.1 Container Registry**
```bash
az acr create \
  --resource-group authway-prod-rg \
  --name authwayacr \
  --sku Basic
```

**2.2 Container Apps Environment**
```bash
az containerapp env create \
  --name authway-container-env \
  --resource-group authway-prod-rg \
  --location koreacentral \
  --logs-workspace-id [LOG-ANALYTICS-ID]
```

**2.3 Deploy Backend Container**
```bash
# Build and push image
docker build -t authwayacr.azurecr.io/authway-api:0.1.0 -f Dockerfile .
docker push authwayacr.azurecr.io/authway-api:0.1.0

# Deploy to Container Apps
az containerapp create \
  --name authway-api-app \
  --resource-group authway-prod-rg \
  --environment authway-container-env \
  --image authwayacr.azurecr.io/authway-api:0.1.0 \
  --target-port 8080 \
  --ingress external \
  --min-replicas 0 \
  --max-replicas 10 \
  --cpu 0.5 --memory 1Gi
```

### Phase 3: 프론트엔드 배포 (30분)

**3.1 Static Web Apps**
```bash
# Admin Dashboard
az staticwebapp create \
  --name authway-admin-static \
  --resource-group authway-prod-rg \
  --location eastasia \
  --sku Standard

# Login UI
az staticwebapp create \
  --name authway-login-static \
  --resource-group authway-prod-rg \
  --location eastasia \
  --sku Standard
```

**3.2 Configure API Backend**
```json
// staticwebapp.config.json
{
  "routes": [
    {
      "route": "/api/*",
      "rewrite": "https://api.authway.com/api/*"
    }
  ],
  "navigationFallback": {
    "rewrite": "/index.html"
  }
}
```

### Phase 4: Front Door 구성 (30분)

**4.1 Create Front Door**
```bash
az afd profile create \
  --profile-name authway-frontdoor \
  --resource-group authway-prod-rg \
  --sku Standard_AzureFrontDoor

# Add endpoints
az afd endpoint create \
  --profile-name authway-frontdoor \
  --endpoint-name authway-api \
  --resource-group authway-prod-rg
```

**4.2 Configure WAF Policy**
```bash
az network front-door waf-policy create \
  --name authwayWafPolicy \
  --resource-group authway-prod-rg \
  --mode Prevention
```

### Phase 5: 모니터링 설정 (20분)

**5.1 Application Insights**
```bash
az monitor app-insights component create \
  --app authway-appinsights \
  --location koreacentral \
  --resource-group authway-prod-rg \
  --workspace [LOG-ANALYTICS-ID]
```

**5.2 Configure Alerts**
```bash
# API response time alert
az monitor metrics alert create \
  --name api-response-time-high \
  --resource-group authway-prod-rg \
  --scopes [CONTAINER-APP-ID] \
  --condition "avg duration > 2000" \
  --window-size 5m \
  --evaluation-frequency 1m
```

---

## 📚 다음 단계

### 준비 완료 후 작업

1. **Infrastructure as Code**
   - [ ] Bicep 템플릿 생성
   - [ ] Terraform 구성 (선택)
   - [ ] 버전 관리

2. **CI/CD 파이프라인**
   - [ ] GitHub Actions workflow
   - [ ] 자동 빌드 & 테스트
   - [ ] 자동 배포

3. **보안 강화**
   - [ ] Azure AD 통합
   - [ ] RBAC 구성
   - [ ] Security Center 검토

4. **성능 테스트**
   - [ ] Load testing (Azure Load Testing)
   - [ ] 병목 지점 식별
   - [ ] 최적화

5. **운영 문서**
   - [ ] Runbook 작성
   - [ ] 장애 복구 절차
   - [ ] 백업/복원 테스트

---

## 📖 참고 문서

- [Azure Container Apps Documentation](https://learn.microsoft.com/azure/container-apps/)
- [Azure Static Web Apps Documentation](https://learn.microsoft.com/azure/static-web-apps/)
- [Azure Database for PostgreSQL](https://learn.microsoft.com/azure/postgresql/)
- [Azure Cache for Redis](https://learn.microsoft.com/azure/azure-cache-for-redis/)
- [Azure Front Door](https://learn.microsoft.com/azure/frontdoor/)

---

**작성자**: Claude Code
**최종 업데이트**: 2025-10-14
