# Authway 멀티테넌시 구현 로드맵

**작성일**: 2025-10-11
**상태**: 구현 완료 (100%)

---

## 구현 완료된 Phase

### Phase 1: DB 마이그레이션 스키마
- Tenants 테이블 설계
- Users/Clients 테이블에 tenant_id 추가
- 복합 인덱스 `(tenant_id, email)` 설정
- Default Tenant 자동 생성

**파일**: `scripts/migrations/001_add_multi_tenancy.sql`

---

### Phase 2: 모델 및 서비스
- `pkg/tenant/models.go` - Tenant 모델
- `pkg/tenant/service.go` - CRUD 로직
- User/Client 모델 tenant_id 통합

**주요 메서드**:
- CreateTenant, GetTenantBySlug, UpdateTenant, DeleteTenant
- EnsureDefaultTenant, CreateSingleTenant

---

### Phase 3: API 엔드포인트
- `pkg/tenant/handler.go` - HTTP 핸들러
- Tenant CRUD API (5개 엔드포인트)
- Validator 통합

**라우트**:
```
POST   /api/v1/tenants
GET    /api/v1/tenants
GET    /api/v1/tenants/:id
PUT    /api/v1/tenants/:id
DELETE /api/v1/tenants/:id
```

---

### Phase 4: 인증 흐름
- User Service tenant 필터링
- Client Service tenant 검증
- `GetByEmailAndTenant` 메서드
- `GetByTenant` 메서드

---

### Phase 5: 환경 변수
- TenantConfig 구조체
- Single Tenant Mode 설정
- main.go 초기화 로직

**환경 변수**:
- `AUTHWAY_TENANT_SINGLE_TENANT_MODE`
- `AUTHWAY_TENANT_TENANT_NAME`
- `AUTHWAY_TENANT_TENANT_SLUG`

---

### Phase 6: 미들웨어
- Admin Auth 미들웨어
- Bearer Token 검증
- `RequireAdmin()` 헬퍼

**파일**: `pkg/middleware/admin.go`

---

## 4가지 시나리오 지원 현황

### ✅ 시나리오 1: 독립 배포
Single Tenant Mode로 지원

### ✅ 시나리오 2: 앱별 독립 OAuth
Client별 OAuth 설정 필드 구현

### ✅ 시나리오 3: 즉시 사용
Default Tenant 자동 생성

### ✅ 시나리오 4: 그룹별 SSO
Tenant별 세션 관리 준비 완료

---

## 다음 단계 (권장)

### 배포
```bash
# 1. DB 마이그레이션 실행
psql -U authway -d authway < scripts/migrations/001_add_multi_tenancy.sql

# 2. 빌드
cd src/server
go build -o ../../bin/authway ./cmd/

# 3. 실행
../../bin/authway
```

### 테스트
- [ ] Tenant Service 단위 테스트
- [ ] API 통합 테스트
- [ ] E2E 시나리오 테스트

---

**버전**: 1.0
**최종 업데이트**: 2025-10-11
