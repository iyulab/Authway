# Authway 배포 스크립트

2026-04-15 재구성: 타겟별 디렉터리 분리 (`prod/`, `staging/`) + 공통 로직 (`_shared/`).

## 디렉터리 구조

```
scripts/deploy/
├── _shared/                    # 타겟 무관 공통 로직
│   ├── load-env.ps1             # env 로더 + preflight (secrets, az subscription)
│   ├── migration-helpers.ps1    # psql 기반 마이그레이션
│   ├── smoke-audit.ps1          # audit_logs 배포 smoke (fail-closed)
│   ├── check-migration-status*.ps1
│   ├── run-migration*.ps1
│   ├── init-migration-system.ps1
│   ├── migrate-tracking-table.ps1
│   ├── upgrade-tracking-table.ps1
│   ├── force-upgrade-tracking.ps1
│   ├── deploy-with-migration.ps1
│   ├── deploy-cors-update.ps1
│   ├── azure-plan.md
│   └── lib/                     # 타겟 주입형 publish 코어 (직접 호출 지양)
│       ├── publish-api.core.ps1
│       ├── publish-hydra.core.ps1
│       ├── publish-auth-api.core.ps1
│       ├── publish-admin.core.ps1
│       ├── publish-auth-ui.core.ps1
│       └── deploy-all.core.ps1
├── prod/                       # production 배포 (Target=prod 고정 thin wrapper)
│   ├── .env                     # 실제 secret (git-ignored)
│   ├── .env.example
│   ├── publish-api.ps1
│   ├── publish-hydra.ps1
│   ├── publish-auth-api.ps1
│   ├── publish-admin.ps1
│   ├── publish-auth-ui.ps1
│   └── deploy-all.ps1
└── staging/                    # staging 배포 (Target=staging 고정 thin wrapper)
    ├── .env                     # (사용자 생성) staging secret
    ├── .env.example             # 기존 리소스 재사용 기반 (RG=authway, Redis DB=1, authway_staging DB)
    ├── publish-api.ps1
    ├── publish-hydra.ps1
    ├── publish-auth-api.ps1
    ├── publish-admin.ps1
    ├── publish-auth-ui.ps1
    └── deploy-all.ps1
```

## 설계 원칙

- **DRY**: 실제 로직은 `_shared/lib/*.core.ps1`에만 존재. 타겟 디렉터리의 `*.ps1`은 `-Target` 고정 thin wrapper.
- **명시적 분리**: `.env`가 타겟 디렉터리 안에 위치 → prod 스크립트가 staging env를 로드할 수 없음. 실수 방지.
- **확장성**: `canary/`, `dev/` 등 추가 시 디렉터리 복제 + core 호출 시 `-Target` 값 추가 (core는 `ValidateSet` 확장만).
- **fail-closed preflight**: `load-env.ps1`의 `Test-DeploySecrets`와 `Set-AzureSubscription`이 모든 core에서 실행. placeholder/구독 불일치 감지 시 즉시 중단.

## 사전 요구사항 (배포 머신)

다음 CLI/도구가 PATH에 설치되어 있어야 한다. 누락 시 publish 단계에서 명확한 에러로 중단된다.

| 도구 | 용도 | 설치 |
|---|---|---|
| `az` (Azure CLI) | Container Apps / SWA / Postgres flexible-server 등 | https://aka.ms/azcli |
| `docker` | Authway/Auth API 이미지 빌드 + GHCR 푸시 | Docker Desktop 또는 Docker Engine |
| `psql` | 마이그레이션 / smoke / health 쿼리 | PostgreSQL client (Postgres 17 권장) |
| `swa` (`@azure/static-web-apps-cli`) | `publish-admin.core.ps1` 의 SWA 배포 | `npm install -g @azure/static-web-apps-cli` |
| `npx wrangler@latest` | Cloudflare Pages 분기 (auth-ui staging 등) | npm + 인터넷 (npx 자동 다운로드) |
| PowerShell 7+ | 본 스크립트 실행 환경 | https://aka.ms/powershell |

확인:
```powershell
az --version; docker --version; psql --version; swa --version; npx wrangler@latest --version
```

## 사용법

### 전체 배포

```powershell
# prod
.\prod\deploy-all.ps1

# staging
.\staging\deploy-all.ps1

# 옵션
.\prod\deploy-all.ps1 -SkipMigration -SkipBuild
.\prod\deploy-all.ps1 -Services hydra,api   # 일부만
```

### 개별 서비스

```powershell
.\prod\publish-api.ps1                          # 빌드 + 배포 + post-deploy 검증
.\prod\publish-api.ps1 -SkipBuild               # 재배포만
.\prod\publish-api.ps1 -ImageTag v20260415120000  # 특정 태그 롤백
.\staging\publish-hydra.ps1 -UpdateEnvOnly      # env만
```

### audit_logs smoke

```powershell
.\_shared\smoke-audit.ps1 -Target prod -WindowMinutes 10
.\_shared\smoke-audit.ps1 -Target staging -WarnOnly   # 0건이어도 경고만
```

### 마이그레이션 상태 확인

```powershell
# prod/staging DB는 _shared 스크립트가 env를 어떻게 찾는가?
# 현재: migration helpers는 호출 시 hashtable 전달 방식이라 별도 wrapper가 필요.
# TODO: staging/check-migration-status.ps1 wrapper 추가 (현재 prod 전용)
.\_shared\check-migration-status-psql.ps1
```

## 첫 staging 배포 절차 (2026-04-15 이후)

**선결 (인간 작업)**:
1. Postgres: `CREATE DATABASE authway_staging;` + `CREATE ROLE authway_staging LOGIN PASSWORD '…';` + `GRANT ALL ON DATABASE authway_staging TO authway_staging;`
2. Azure: 기존 RG `authway` 내 `-stg` 접미사 Container Apps + Static Web Apps 프로비저닝
3. DNS: `stg-*.authway.in` CNAME 레코드 (7종)
4. OAuth: Google Cloud Console에 `https://stg-auth-api.authway.in/auth/google/callback` 추가
5. `staging/.env.example` → `staging/.env` 복사 후 값 채움 (secrets 전용 신규 생성)

**배포**:
```powershell
.\staging\deploy-all.ps1
```

배포 직후 `_shared/smoke-audit.ps1 -Target staging`이 자동 실행되어 audit_logs 건수 검증.

## 이전 경로에서 마이그레이션

2026-04-15 이전 구조는 `scripts/deploy/` 평면 + `.env` 하나 + `.env.staging`였음. 해당 구조는 **제거됨**. 호환 shim 없음.

- `scripts/deploy/.env` → `scripts/deploy/prod/.env`
- `scripts/deploy/.env.staging` → `scripts/deploy/staging/.env`
- `scripts/deploy/publish-*.ps1` → `scripts/deploy/prod/publish-*.ps1`
- `scripts/deploy/deploy-all.ps1` → `scripts/deploy/prod/deploy-all.ps1`

## 금기사항

- `scripts/deploy/` 는 `.gitignore` 대상. `git add -f` 금지.
- `az account set` 은 preflight가 자동 고정 — 수동 실행 불필요.
- prod secret을 staging `.env`에 복사 금지. rotation 경계 파괴.
- core script (`_shared/lib/*.core.ps1`) 를 `-Target` 없이 직접 호출 금지 (param 필수).
