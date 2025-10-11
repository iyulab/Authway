# Authway 멀티테넌시 아키텍처

**목적**: 유연한 사용자 격리 및 SSO 지원

---

## 핵심 개념

### Tenant (테넌트)
- 사용자 풀 격리/공유의 논리적 경계
- 각 Client(앱)는 하나의 Tenant에 소속

### 격리 모드
- **isolated**: 앱별 독립 사용자 (SSO 불가)
- **shared**: 통합 사용자 풀 (SSO 가능)

---

## 4가지 배포 시나리오

### 1. 앱 전용 독립 배포
```bash
AUTHWAY_SINGLE_TENANT_MODE=true
AUTHWAY_TENANT_SLUG="app-a"
```
- 완전 격리, 독립 배포

### 2. 중앙 Auth - 앱별 독립 OAuth
- 중앙 Authway 인스턴스
- Tenant별 isolated mode
- Client별 OAuth 설정

### 3. 중앙 Auth - 즉시 사용
- Default Tenant (shared mode)
- 공통 OAuth 설정
- 자동 SSO

### 4. 중앙 Auth - 그룹별 SSO
- 그룹별 Tenant (shared mode)
- 그룹 내 SSO 공유
- 그룹 간 격리

---

## 데이터 모델

### Tenant
```go
type Tenant struct {
    ID            uuid.UUID
    Name          string
    Slug          string  // URL-friendly
    IsolationMode string  // isolated | shared
    Settings      TenantSettings
    Logo          string
    PrimaryColor  string
    Active        bool
}
```

### User (테넌트 격리)
```go
type User struct {
    ID       uuid.UUID
    TenantID uuid.UUID  // FK to tenants
    Email    string      // Unique per tenant
    // ...
}

// Index: UNIQUE(tenant_id, email)
// → 같은 이메일이 다른 테넌트에 존재 가능
```

### Client (테넌트 소속)
```go
type Client struct {
    ID                 uuid.UUID
    TenantID           uuid.UUID  // FK to tenants
    ClientID           string
    GoogleOAuthEnabled bool       // Client별 OAuth
    GoogleClientID     *string
    // ...
}
```

---

## 인증 흐름

### Isolated Mode
1. Client → Tenant 확인
2. Tenant의 isolation_mode = "isolated"
3. 사용자 조회: `WHERE tenant_id = X AND email = Y`
4. 다른 앱 접속 시 → 재로그인 필요 (SSO 불가)

### Shared Mode
1. Client → Tenant 확인
2. Tenant의 isolation_mode = "shared"
3. 세션 확인 → 같은 Tenant → 자동 승인 (SSO)
4. 다른 앱 접속 시 → 자동 로그인

---

## 환경 변수

### Single Tenant Mode
```bash
AUTHWAY_SINGLE_TENANT_MODE=true
AUTHWAY_TENANT_NAME="My App"
AUTHWAY_TENANT_SLUG="my-app"
```

### Multi-Tenant Mode
```bash
# Default (자동으로 Default Tenant 생성)
# 추가 Tenant는 API로 생성
```

---

## API 구조

```
POST   /api/v1/tenants         # Tenant 생성
GET    /api/v1/tenants         # Tenant 목록
GET    /api/v1/tenants/:id     # Tenant 조회
PUT    /api/v1/tenants/:id     # Tenant 수정
DELETE /api/v1/tenants/:id     # Tenant 삭제
```

모든 API는 Admin API Key 인증 필요

---

**버전**: 1.0
**최종 업데이트**: 2025-10-11
