# Authway CI/CD 전략 (Azure)

**버전**: 1.0
**작성일**: 2025-10-14
**대상**: Azure 배포 환경

---

## 📋 목차

1. [개요](#개요)
2. [CI/CD 파이프라인 구조](#cicd-파이프라인-구조)
3. [GitHub Actions 전략](#github-actions-전략)
4. [배포 워크플로우](#배포-워크플로우)
5. [환경별 배포 전략](#환경별-배포-전략)
6. [보안 및 Secrets 관리](#보안-및-secrets-관리)
7. [모니터링 및 롤백](#모니터링-및-롤백)

---

## 🎯 개요

Authway Azure 배포를 위한 CI/CD 전략은 다음 원칙을 따릅니다:

**핵심 원칙**:
- ✅ **Infrastructure as Code**: 모든 인프라는 Bicep으로 정의
- ✅ **자동화 우선**: 수동 작업 최소화
- ✅ **환경 분리**: dev/staging/prod 완전 격리
- ✅ **안전한 배포**: 단계별 검증 및 롤백 가능
- ✅ **보안 중심**: Secrets 관리 및 권한 최소화

---

## 🏗️ CI/CD 파이프라인 구조

### 전체 파이프라인 플로우

```
┌─────────────────────────────────────────────────────────────┐
│                     1. Code Push                            │
│              (main, staging, dev branch)                    │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                  2. Build & Test                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Backend    │  │    Admin     │  │   Login UI   │     │
│  │   (Go Test)  │  │  (npm test)  │  │  (npm test)  │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│              3. Infrastructure Deployment                   │
│                   (Bicep Template)                          │
│  • VNet, NSG, Private Endpoints                            │
│  • PostgreSQL, Redis                                        │
│  • Container Apps Environment                               │
│  • Static Web Apps                                          │
│  • Key Vault, Monitoring                                    │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│              4. Application Deployment                      │
│  ┌──────────────────────────────────────────────┐          │
│  │  Backend: Build Docker → Push ACR → Update  │          │
│  │  Admin: Build React → Deploy Static Web App │          │
│  │  Login: Build React → Deploy Static Web App │          │
│  └──────────────────────────────────────────────┘          │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│               5. Database Migrations                        │
│              (Run SQL scripts via psql)                     │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                  6. Health Checks                           │
│  • Backend API (/health)                                    │
│  • Admin Dashboard (HTTP 200)                               │
│  • Login UI (HTTP 200)                                      │
│  • Database connectivity                                    │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│              7. Post-Deployment Tests                       │
│  • Integration tests                                        │
│  • Smoke tests                                              │
│  • Performance baseline                                     │
└─────────────────────────────────────────────────────────────┘
```

---

## 🔧 GitHub Actions 전략

### Repository Secrets 구성

```yaml
# Azure Credentials
AZURE_CREDENTIALS:
  description: Azure Service Principal JSON
  example: |
    {
      "clientId": "...",
      "clientSecret": "...",
      "subscriptionId": "...",
      "tenantId": "..."
    }

# PostgreSQL
POSTGRES_ADMIN_USERNAME: authwayadmin
POSTGRES_ADMIN_PASSWORD: [secure-password]

# JWT Secrets
JWT_ACCESS_SECRET: [64+ char random string]
JWT_REFRESH_SECRET: [64+ char random string]

# Admin
ADMIN_API_KEY: [secure-api-key]
ADMIN_PASSWORD: [secure-password]

# OAuth
GOOGLE_CLIENT_ID: [from Google Console]
GOOGLE_CLIENT_SECRET: [from Google Console]

# Static Web Apps Deployment Tokens
AZURE_STATIC_WEB_APPS_API_TOKEN_ADMIN: [from Azure]
AZURE_STATIC_WEB_APPS_API_TOKEN_LOGIN: [from Azure]
```

### Environment 구성

```yaml
# GitHub Environments
environments:
  dev:
    protection_rules: none
    secrets: dev-specific secrets

  staging:
    protection_rules:
      - required_reviewers: 1
    secrets: staging-specific secrets

  prod:
    protection_rules:
      - required_reviewers: 2
      - deployment_branches: main only
    secrets: production secrets
```

---

## 📦 배포 워크플로우

### 워크플로우 1: Infrastructure Deployment

**트리거**: 수동 (workflow_dispatch) 또는 인프라 변경 시

```yaml
name: Deploy Infrastructure

on:
  workflow_dispatch:
    inputs:
      environment:
        type: choice
        options: [dev, staging, prod]

  push:
    paths:
      - 'deployments/azure/bicep/**'

jobs:
  deploy-infra:
    runs-on: ubuntu-latest
    environment: ${{ github.event.inputs.environment }}

    steps:
      # 1. Azure 로그인
      - uses: azure/login@v1
        with:
          creds: ${{ secrets.AZURE_CREDENTIALS }}

      # 2. Resource Group 생성
      - name: Create Resource Group
        run: |
          az group create \
            --name authway-$ENV-rg \
            --location koreacentral

      # 3. Bicep 배포
      - uses: azure/arm-deploy@v1
        with:
          resourceGroupName: authway-$ENV-rg
          template: deployments/azure/bicep/main.bicep
          parameters: deployments/azure/bicep/parameters.$ENV.json
```

**예상 소요 시간**: 15-30분

---

### 워크플로우 2: Backend Deployment

**트리거**: main/staging/dev 브랜치 푸시

```yaml
name: Deploy Backend

on:
  push:
    branches: [main, staging, dev]
    paths:
      - 'src/server/**'
      - 'Dockerfile'

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest

    steps:
      # 1. Go 테스트
      - name: Run Go Tests
        run: |
          cd src/server
          go test ./...

      # 2. Docker 빌드
      - name: Build Docker Image
        run: |
          docker build -t authway-api:${{ github.sha }} .

      # 3. ACR 푸시
      - name: Push to ACR
        run: |
          az acr login --name authwayacr
          docker tag authway-api:${{ github.sha }} \
            authwayacr.azurecr.io/authway-api:${{ github.sha }}
          docker push authwayacr.azurecr.io/authway-api:${{ github.sha }}

      # 4. Container App 업데이트
      - name: Update Container App
        run: |
          az containerapp update \
            --name authway-api-$ENV \
            --resource-group authway-$ENV-rg \
            --image authwayacr.azurecr.io/authway-api:${{ github.sha }}

      # 5. Health Check
      - name: Health Check
        run: |
          for i in {1..30}; do
            if curl -f https://api-$ENV.authway.com/health; then
              exit 0
            fi
            sleep 10
          done
          exit 1
```

**예상 소요 시간**: 5-10분

---

### 워크플로우 3: Frontend Deployment

**트리거**: main/staging/dev 브랜치 푸시

```yaml
name: Deploy Frontend

on:
  push:
    branches: [main, staging, dev]
    paths:
      - 'packages/web/**'

jobs:
  deploy-admin:
    runs-on: ubuntu-latest
    steps:
      # 1. 테스트
      - name: Run Tests
        working-directory: packages/web/admin-dashboard
        run: npm test

      # 2. 빌드
      - name: Build
        working-directory: packages/web/admin-dashboard
        env:
          VITE_API_URL: https://api-$ENV.authway.com
        run: npm run build

      # 3. 배포
      - uses: Azure/static-web-apps-deploy@v1
        with:
          azure_static_web_apps_api_token: ${{ secrets.AZURE_STATIC_WEB_APPS_API_TOKEN_ADMIN }}
          app_location: 'packages/web/admin-dashboard'
          output_location: 'dist'

  deploy-login:
    runs-on: ubuntu-latest
    steps:
      # Admin과 동일한 프로세스
      [...]
```

**예상 소요 시간**: 3-5분

---

### 워크플로우 4: Database Migrations

**트리거**: 수동 또는 migration 파일 변경 시

```yaml
name: Run Migrations

on:
  workflow_dispatch:
    inputs:
      environment:
        type: choice
        options: [dev, staging, prod]

jobs:
  migrate:
    runs-on: ubuntu-latest
    environment: ${{ github.event.inputs.environment }}

    steps:
      # 1. PostgreSQL 연결 정보 가져오기
      - name: Get PostgreSQL Host
        run: |
          POSTGRES_HOST=$(az postgres flexible-server show \
            --resource-group authway-$ENV-rg \
            --name authway-postgres-$ENV \
            --query fullyQualifiedDomainName -o tsv)
          echo "POSTGRES_HOST=$POSTGRES_HOST" >> $GITHUB_ENV

      # 2. 마이그레이션 실행
      - name: Run Migrations
        run: |
          PGPASSWORD=${{ secrets.POSTGRES_ADMIN_PASSWORD }} \
          psql -h $POSTGRES_HOST \
            -U authwayadmin \
            -d authway \
            -f scripts/migrations/001_add_multi_tenancy.sql

      # 3. 검증
      - name: Verify Migration
        run: |
          # 테이블 존재 확인
          PGPASSWORD=${{ secrets.POSTGRES_ADMIN_PASSWORD }} \
          psql -h $POSTGRES_HOST \
            -U authwayadmin \
            -d authway \
            -c "\dt"
```

**예상 소요 시간**: 1-2분

---

## 🌍 환경별 배포 전략

### Development (dev)

```yaml
배포 전략:
  trigger: 모든 dev 브랜치 푸시
  approval: 불필요
  rollback: 자동 (health check 실패 시)

리소스:
  PostgreSQL: Burstable B1ms (1 vCore)
  Redis: Basic C1 (1GB)
  Container Apps: min=0, max=3

특징:
  - 빠른 배포 (5분 이내)
  - 비용 최소화
  - Private Endpoints 비활성화
```

### Staging

```yaml
배포 전략:
  trigger: staging 브랜치 푸시
  approval: 1명 필요
  rollback: 수동

리소스:
  PostgreSQL: General Purpose D2s_v3 (2 vCore)
  Redis: Standard C2 (2.5GB)
  Container Apps: min=1, max=5

특징:
  - 프로덕션 동일 구성
  - 성능 테스트 환경
  - Private Endpoints 활성화
```

### Production (prod)

```yaml
배포 전략:
  trigger: main 브랜치 푸시
  approval: 2명 필요 + 테스트 통과
  rollback: Blue-Green 또는 수동

리소스:
  PostgreSQL: General Purpose D2s_v3 (2 vCore) + Zone Redundant
  Redis: Premium P1 (6GB) + Geo-replication
  Container Apps: min=2, max=10

특징:
  - 고가용성 (Multi-zone)
  - 자동 백업 활성화
  - Private Endpoints 필수
  - WAF + DDoS Protection
```

---

## 🔐 보안 및 Secrets 관리

### Azure Key Vault 통합

**원칙**: 모든 민감한 정보는 Key Vault에 저장

```yaml
배포 시 프로세스:
  1. GitHub Secrets → Azure Key Vault에 저장
  2. Container Apps → Managed Identity로 Key Vault 접근
  3. 애플리케이션 → Environment Variables로 주입

예시:
  # GitHub Actions에서 Key Vault 업데이트
  - name: Update Key Vault Secrets
    run: |
      az keyvault secret set \
        --vault-name authway-kv-$ENV \
        --name jwt-access-secret \
        --value "${{ secrets.JWT_ACCESS_SECRET }}"
```

### Managed Identity 권한

```yaml
Container App Managed Identity:
  permissions:
    - Key Vault: Get Secrets, List Secrets
    - ACR: AcrPull
    - PostgreSQL: Contributor (database level)
    - Redis: Contributor

Static Web Apps:
  permissions:
    - Container Apps: Read (API 호출)
```

### Secret Rotation 전략

```yaml
JWT Secrets:
  rotation: 매 90일
  process: 수동 (Key Vault 업데이트 → Container App 재시작)

PostgreSQL Password:
  rotation: 매 90일
  process: Key Vault 업데이트 → Connection String 갱신

OAuth Secrets:
  rotation: 필요 시 (Google Console에서 변경 시)
```

---

## 📊 모니터링 및 롤백

### 배포 모니터링

```yaml
실시간 모니터링 항목:
  - Container App Logs (Application Insights)
  - Error Rate (목표: < 1%)
  - Response Time (목표: < 500ms)
  - CPU/Memory Usage (목표: < 80%)
  - Database Connection Pool

알림 채널:
  - Email
  - Slack/Teams (webhook)
  - Azure Monitor Alert
```

### 롤백 전략

#### 1. 자동 롤백 (Health Check 실패)

```bash
# Health check가 3회 연속 실패 시 자동 롤백
if [ "$HEALTH_CHECK_FAILURES" -ge 3 ]; then
  echo "Rolling back to previous version"

  az containerapp revision set-mode \
    --name authway-api-$ENV \
    --resource-group authway-$ENV-rg \
    --mode single \
    --revision authway-api-$ENV--previous
fi
```

#### 2. 수동 롤백

```bash
# 이전 버전 확인
az containerapp revision list \
  --name authway-api-$ENV \
  --resource-group authway-$ENV-rg

# 특정 버전으로 롤백
az containerapp revision activate \
  --name authway-api-$ENV \
  --resource-group authway-$ENV-rg \
  --revision authway-api-$ENV--abc123
```

#### 3. Blue-Green 배포 (권장 - Prod)

```yaml
배포 프로세스:
  1. 새 버전을 Green으로 배포
  2. Green 환경 health check
  3. 트래픽 일부를 Green으로 전환 (10%)
  4. 모니터링 (10분)
  5. 문제 없으면 트래픽 100% Green으로
  6. Blue 환경 제거

롤백:
  - 트래픽을 Blue로 즉시 전환 (1분 이내)
```

---

## 📝 배포 체크리스트

### 사전 준비

- [ ] Azure 구독 및 권한 확인
- [ ] GitHub Secrets 등록 완료
- [ ] Service Principal 생성 및 권한 부여
- [ ] 도메인 및 SSL 인증서 준비
- [ ] Google OAuth 설정 완료

### 인프라 배포

- [ ] Resource Group 생성
- [ ] VNet 및 Subnet 구성
- [ ] PostgreSQL Flexible Server 배포
- [ ] Redis Cache 배포
- [ ] Key Vault 생성 및 Secrets 등록
- [ ] Container Apps Environment 생성
- [ ] Static Web Apps 생성

### 애플리케이션 배포

- [ ] Backend Docker 이미지 빌드 및 푸시
- [ ] Container App 배포 및 구성
- [ ] Admin Dashboard 빌드 및 배포
- [ ] Login UI 빌드 및 배포
- [ ] 데이터베이스 마이그레이션 실행

### 검증

- [ ] Backend API Health Check
- [ ] Admin Dashboard 접속 확인
- [ ] Login UI 접속 확인
- [ ] 회원가입/로그인 테스트
- [ ] OAuth 로그인 테스트
- [ ] 이메일 전송 테스트

### 모니터링 설정

- [ ] Application Insights 구성
- [ ] Alert Rules 설정
- [ ] Dashboard 구성
- [ ] 로그 수집 확인

---

## 🚀 구현 우선순위

### Phase 1: 수동 배포 (현재)

```
우선순위: 높음
목표: Bicep 템플릿으로 인프라 수동 배포

작업:
✅ Bicep 템플릿 작성
✅ 배포 스크립트 작성
⏳ 수동 배포 가이드 문서화
⏳ 검증 체크리스트 작성
```

### Phase 2: 기본 CI/CD (다음 단계)

```
우선순위: 중간
목표: GitHub Actions 기본 워크플로우

작업:
- Backend 자동 빌드 및 배포
- Frontend 자동 빌드 및 배포
- 자동 테스트 실행
- Health Check 자동화
```

### Phase 3: 고급 CI/CD (향후)

```
우선순위: 낮음
목표: 완전 자동화 및 최적화

작업:
- Blue-Green 배포
- Canary 배포
- 자동 롤백
- 성능 테스트 자동화
- 보안 스캔 통합
```

---

## 📚 참고 자료

### Azure DevOps 문서
- [Azure Container Apps CI/CD](https://learn.microsoft.com/azure/container-apps/github-actions)
- [Static Web Apps Deployment](https://learn.microsoft.com/azure/static-web-apps/deploy-azure-pipelines)
- [Bicep Deployment Pipeline](https://learn.microsoft.com/azure/azure-resource-manager/bicep/deploy-github-actions)

### Best Practices
- [GitHub Actions Best Practices](https://docs.github.com/actions/learn-github-actions/security-hardening-for-github-actions)
- [Azure Security Best Practices](https://learn.microsoft.com/azure/security/fundamentals/best-practices-and-patterns)

---

**작성자**: Claude Code
**최종 업데이트**: 2025-10-14
**문서 상태**: 초안 (Phase 1)
