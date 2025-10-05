# 데이터베이스 마이그레이션 가이드

## 📋 사전 준비

### 1. 백업 필수

```bash
# PostgreSQL 백업
pg_dump -U authway -d authway_dev > backup_before_migration_$(date +%Y%m%d_%H%M%S).sql

# 또는 Docker 환경
docker exec authway-postgres pg_dump -U authway authway > backup_before_migration_$(date +%Y%m%d_%H%M%S).sql
```

### 2. 마이그레이션 전 확인

```sql
-- 현재 데이터 확인
SELECT COUNT(*) FROM users;
SELECT COUNT(*) FROM oauth_clients;

-- 테이블 구조 확인
\d users
\d oauth_clients
```

---

## 🚀 마이그레이션 실행

### 로컬 개발 환경

```bash
# PostgreSQL에 직접 연결
psql -U authway -d authway_dev -f scripts/migrations/001_add_multi_tenancy.sql

# 또는 Docker 환경
docker exec -i authway-postgres psql -U authway -d authway < scripts/migrations/001_add_multi_tenancy.sql
```

### 프로덕션 환경

```bash
# 1. 점검 모드 활성화
# 2. 백업 확인
# 3. 마이그레이션 실행
psql -U authway -d authway_production -f scripts/migrations/001_add_multi_tenancy.sql

# 4. 검증 (아래 섹션 참고)
# 5. 점검 모드 해제
```

---

## ✅ 마이그레이션 검증

### 1. 테이블 생성 확인

```sql
-- Tenants 테이블 확인
SELECT COUNT(*) FROM tenants;
-- 결과: 최소 1 (default tenant)

-- Default tenant 확인
SELECT * FROM tenants WHERE slug = 'default';
-- 결과: id = '00000000-0000-0000-0000-000000000001'

-- 테이블 구조 확인
\d tenants
\d users
\d clients
\d sessions
```

### 2. 인덱스 확인

```sql
-- Users 복합 유니크 인덱스 확인
SELECT indexname, indexdef
FROM pg_indexes
WHERE tablename = 'users' AND indexname = 'unique_tenant_email';
-- 결과: UNIQUE (tenant_id, email)

-- Clients tenant_id 인덱스 확인
SELECT indexname FROM pg_indexes WHERE tablename = 'clients';
```

### 3. 데이터 마이그레이션 확인

```sql
-- 모든 사용자가 tenant_id를 가지는지 확인
SELECT COUNT(*) FROM users WHERE tenant_id IS NULL;
-- 결과: 0

-- 모든 사용자가 default tenant에 속하는지 확인
SELECT COUNT(*) FROM users WHERE tenant_id = '00000000-0000-0000-0000-000000000001';
-- 결과: 전체 사용자 수와 동일

-- Demo client 확인
SELECT * FROM clients WHERE client_id = 'demo-client-default';
```

### 4. 트리거 확인

```sql
-- Update timestamp 트리거 확인
SELECT tgname FROM pg_trigger WHERE tgrelid = 'tenants'::regclass;
SELECT tgname FROM pg_trigger WHERE tgrelid = 'users'::regclass;
SELECT tgname FROM pg_trigger WHERE tgrelid = 'clients'::regclass;
```

---

## 🔄 롤백 (문제 발생 시)

### 즉시 롤백

```bash
# 백업 복원
psql -U authway -d authway_dev < backup_before_migration_YYYYMMDD_HHMMSS.sql

# 또는 롤백 스크립트 실행
psql -U authway -d authway_dev -f scripts/migrations/ROLLBACK_001.sql
```

### 롤백 검증

```sql
-- oauth_clients 테이블 복원 확인
\d oauth_clients

-- users 테이블 email unique 제약 확인
\d users

-- tenants 테이블 삭제 확인
SELECT COUNT(*) FROM tenants;
-- 결과: ERROR - relation "tenants" does not exist (정상)
```

---

## 🧪 테스트 시나리오

### Scenario 1: 같은 이메일 다른 테넌트

```sql
-- Tenant A 생성
INSERT INTO tenants (name, slug, active)
VALUES ('Tenant A', 'tenant-a', true);

-- Tenant B 생성
INSERT INTO tenants (name, slug, active)
VALUES ('Tenant B', 'tenant-b', true);

-- 같은 이메일로 두 테넌트에 사용자 생성
INSERT INTO users (tenant_id, email, password_hash, email_verified)
VALUES (
    (SELECT id FROM tenants WHERE slug = 'tenant-a'),
    'user@example.com',
    '$2a$10$abcdefg',
    true
);

INSERT INTO users (tenant_id, email, password_hash, email_verified)
VALUES (
    (SELECT id FROM tenants WHERE slug = 'tenant-b'),
    'user@example.com',
    '$2a$10$abcdefg',
    true
);

-- 확인: 두 개의 별도 계정
SELECT email, tenant_id FROM users WHERE email = 'user@example.com';
-- 결과: 2 rows (다른 tenant_id)
```

### Scenario 2: Tenant 삭제 방지

```sql
-- Default tenant 삭제 시도 (실패해야 함)
DELETE FROM tenants WHERE slug = 'default';
-- 결과: 애플리케이션 레벨에서 방지 (현재는 가능하지만 Service에서 방지)

-- 사용자 있는 Tenant 삭제 시도
-- 애플리케이션 레벨에서 방지 (TenantService.DeleteTenant)
```

---

## 📊 마이그레이션 영향 분석

### 변경된 테이블

| 테이블 | 변경 내용 | Breaking Change |
|--------|-----------|-----------------|
| `users` | `tenant_id` 추가, email UNIQUE 제거 | ✅ Yes |
| `oauth_clients` | `clients`로 이름 변경, 구조 변경 | ✅ Yes |
| `user_sessions` | `sessions`로 이름 변경, `tenant_id` 추가 | ✅ Yes |
| `consent_grants` | `tenant_id` 추가 | ⚠️ Partial |

### 새로 생성된 테이블

- `tenants`
- `email_verifications`
- `password_resets`

### 애플리케이션 코드 영향

**수정 필요한 부분**:
1. ✅ User 모델 (완료)
2. ✅ Client 모델 (완료)
3. ⏳ User Service (진행 중)
4. ⏳ Client Service (진행 중)
5. ⏳ Auth Service (진행 중)

---

## 🔧 문제 해결

### 문제: 마이그레이션 중 에러

```
ERROR:  column "tenant_id" cannot be null
```

**해결**:
1. 마이그레이션 스크립트가 순서대로 실행되었는지 확인
2. Default tenant가 먼저 생성되었는지 확인
3. `UPDATE users SET tenant_id = ...` 실행 확인

### 문제: 복합 유니크 인덱스 충돌

```
ERROR:  duplicate key value violates unique constraint "unique_tenant_email"
```

**원인**: 같은 tenant_id에 이미 같은 이메일 존재

**해결**:
```sql
-- 중복 확인
SELECT tenant_id, email, COUNT(*)
FROM users
GROUP BY tenant_id, email
HAVING COUNT(*) > 1;

-- 중복 제거 (백업 후!)
DELETE FROM users
WHERE id NOT IN (
    SELECT MIN(id) FROM users GROUP BY tenant_id, email
);
```

### 문제: 기존 세션 무효화

**현상**: 마이그레이션 후 모든 사용자 로그아웃

**원인**: `sessions` 테이블에 `tenant_id` 추가됨

**해결**: 정상 동작 (의도된 동작)
- 사용자는 재로그인 필요
- 새 세션에 `tenant_id` 포함됨

---

## 📝 체크리스트

마이그레이션 전:
- [ ] 백업 완료
- [ ] 점검 모드 활성화 (프로덕션)
- [ ] 마이그레이션 스크립트 검토
- [ ] 롤백 스크립트 준비

마이그레이션 후:
- [ ] Tenants 테이블 생성 확인
- [ ] Default tenant 생성 확인
- [ ] Users tenant_id NOT NULL 확인
- [ ] 복합 유니크 인덱스 확인
- [ ] Clients 테이블 생성 확인
- [ ] 트리거 생성 확인
- [ ] 애플리케이션 재시작
- [ ] 로그인 테스트
- [ ] API 동작 확인

롤백 필요 시:
- [ ] 백업 복원 또는 롤백 스크립트 실행
- [ ] 롤백 검증
- [ ] 애플리케이션 재시작

---

## 🚦 다음 단계

마이그레이션 완료 후:

1. **애플리케이션 코드 배포**
   - Tenant 모델 포함
   - User/Client 모델 수정 포함
   - 초기화 로직 포함

2. **환경 변수 설정**
   ```bash
   # Single Tenant Mode (선택)
   AUTHWAY_SINGLE_TENANT_MODE=false

   # Admin API Key
   AUTHWAY_ADMIN_API_KEY=your-secure-admin-key-here

   # Authway 공통 OAuth (선택)
   AUTHWAY_GOOGLE_CLIENT_ID=...
   AUTHWAY_GOOGLE_CLIENT_SECRET=...
   ```

3. **첫 Tenant 생성**
   ```bash
   curl -X POST http://localhost:8080/api/v1/tenants \
     -H "Authorization: Bearer ${AUTHWAY_ADMIN_API_KEY}" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "My First Tenant",
       "slug": "my-tenant"
     }'
   ```

---

**작성자**: Claude Code
**버전**: 1.0
