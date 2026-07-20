# 배포 후 검증 체크리스트

배포 스크립트가 성공했다는 것은 컨테이너가 교체됐다는 뜻일 뿐, 변경이 **의도대로 동작한다**는 증거가 아니다. 여기 있는 항목은 전부 "로컬에서 검증할 수 없어 배포가 실제 게이트인 것"들이다.

**원칙**: 각 항목은 실행 가능한 명령 또는 쿼리 + 기대 결과를 함께 적는다. "확인함"이 아니라 **출력**이 증거다. 실패하면 그 자리에서 멈추고 롤백 여부를 판단한다.

**사용법**: 배포 후 staging에서 전부 통과한 다음 prod로 넘어간다. 항목이 해소되면(=해당 변경이 여러 배포를 거쳐 안정) 이 문서에서 제거한다 — 영구 목록이 아니라 *미검증 주장의 대기열*이다.

> ## ✅ staging + prod 검증 완료 — 0.4.0 배포됨 (2026-07-20)
>
> **prod**: 5/5 서비스, 헬스 **6/6**, `sync-hydra` **7 클라이언트 동기화·실패 0**(이 단계가 prod에서
> 성공한 첫 사례 — 무인증 호출 결함을 고친 결과). §1·§2·§3·§5 전부 통과, JWT `kid`가 **prod JWKS**에서
> 해석됨. authorization_code → 302 → `auth.authway.in/login` 정상. 검증 클라이언트 삭제 완료,
> **핀 걸린 클라이언트 0건**(=기존 소비자 토큰 형식 무변화).
>
> 남은 항목은 §1-4 콘솔 UI 조작과 §6의 로그인→consent→콜백 전 구간(자격증명 필요)뿐이다.
>
> ## ✅ staging 검증 완료 (2026-07-20)
>
> `7082776` 기준 staging 배포 후 §1(1-1/1-2/1-3), §2, §3(3-1/3-2/3-3), §5를 **전부 실측 통과**했다.
> JWT는 RS256이고 `kid`가 staging JWKS에 존재해 **오프라인 검증이 실제로 가능**함을 확인했다
> (소비자의 원래 요구). authorization_code 플로우는 302로 로그인 UI + `login_challenge`까지 정상.
> 검증용 클라이언트는 삭제했고 핀이 걸린 클라이언트는 0건이다.
>
> **남은 것**: §1-4 콘솔 UI 조작, §6의 로그인→consent→콜백 전 구간(자격증명 필요), prod 배포.
>
> 이 배포에서 **배포 스크립트 결함 2건**이 추가로 드러나 고쳤다 — sync-hydra 무인증 호출(항상 실패),
> 헬스 체크 10초 단일 시도(콜드스타트를 장애로 오판).

> **2026-07-20 갱신 — 다수 항목이 로컬에서 선검증됐다.** Docker로 Hydra v26.2 + Postgres 15를
> 띄워 §2·§3·§4를 실측으로 통과시켰다(각 절 머리말 참조). 남은 배포-전용 항목은 **§5의 az 실호출**과
> **§1-4 콘솔 UI**, **§6 회귀 스모크**다. 로컬 통과가 staging 검증을 대체하지는 않지만,
> "핵심 전제가 참인지"는 이미 확인된 상태로 배포에 들어간다.

---

## 공통 준비

```bash
ISSUER=https://stg-oauth.authway.in          # prod: https://oauth.authway.in
API=https://<central-api-host>               # scripts/deploy/*/.env 의 CONTAINER_APP_API 도메인
ADMIN_KEY=<AUTHWAY_ADMIN_API_KEY>
TENANT=<검증용 테넌트 UUID>
AUTH="Authorization: Bearer $ADMIN_KEY"
```

> ⚠️ 관리 API 인증 헤더는 **`Authorization: Bearer`** 다. `X-API-Key`는 **401**을 받는다(실측 확인).

---

## 1. 클라이언트 생성 검증 규칙 (Run-17 P-B)

> 배경: M2M 클라이언트에 더미 `redirect_uris`를 강요하던 문제와, 클라이언트 입력 오류를 500으로 분류하던 문제. 발견자: VibeBase (2026-07-20).
>
> **1-1~1-3은 로컬 실측으로 통과했다**(실 Postgres + Hydra + 중앙 API 구동, 2026-07-20).
> 이 과정에서 1-1이 **다른 원인으로 여전히 500**임을 발견해 고쳤다 — `clients.redirect_uris`가
> `NOT NULL`인데 GORM이 nil 슬라이스를 명시적 NULL로 써서 컬럼 DEFAULT가 적용되지 않았다.
> 회귀 가드: `pkg/client/service_postgres_test.go`(SQLite 하네스는 이 제약을 재현하지 못한다).
> staging에서는 **확인** 목적으로 재실행한다.

### 1-1. `client_credentials` 전용 클라이언트를 redirect 없이 생성 → **201**

```bash
curl -s -o /tmp/r.json -w '%{http_code}\n' -X POST "$API/api/v1/clients" \
  -H "Content-Type: application/json" -H "$AUTH" \
  -d "{\"tenant_id\":\"$TENANT\",\"name\":\"verify-m2m\",\"public\":false,
       \"grant_types\":[\"client_credentials\"],\"scopes\":[\"api\"]}"
```
- 기대: `201`. 이전 동작은 `RedirectURIs ... failed on the 'min' tag` 400.
- 추가 확인: 응답의 `post_logout_redirect_uris`가 비어 있을 것(더미 전파 없음).

### 1-2. confidential 클라이언트에 `client_id`만 제공 → **400** (500 아님)

```bash
curl -s -o /tmp/r.json -w '%{http_code}\n' -X POST "$API/api/v1/clients" \
  -H "Content-Type: application/json" -H "$AUTH" \
  -d "{\"tenant_id\":\"$TENANT\",\"name\":\"verify-partial\",\"public\":false,
       \"client_id\":\"verify_partial\",
       \"grant_types\":[\"client_credentials\"],\"scopes\":[\"api\"]}"
cat /tmp/r.json
```
- 기대: `400` + 본문 `"code":"confidential_client_partial_credentials"`, `"field":"client_secret"`.

### 1-3. authorization_code 클라이언트를 redirect 없이 생성 → **400**

- 기대: `"code":"redirect_grant_without_redirect_uris"`. (조건부화가 검증을 *약화*시키지 않았다는 반대 방향 확인.)

### 1-4. Admin 콘솔에서 M2M 클라이언트 생성

- Grant Types에서 Authorization Code 해제 → Client Credentials 선택.
- 기대: Redirect URIs 라벨의 `*`가 사라지고, 비운 채 저장된다.

> ✅ 통과 후: `D:\data\VibeBase\claudedocs\upstream-issues\ISSUE-Authway-20260720-client-api-validation-frictions.md`를 `closed/`로 이동(회수 조건 충족).
> 🧹 검증용 클라이언트는 삭제할 것.

---

## 2. 마이그레이션 015 (per-client access token strategy) — ✅ 로컬 선검증됨

> 실 Postgres 15에서 `TestMigrateSmoke` 13/13 통과(2026-07-20). 아래는 배포 환경에서의 확인용이다.
> 단, `count(*)=0` 항목은 **배포 직후에만** 유효하다 — 이후 누군가 opt-in하면 당연히 0이 아니다.

```sql
-- 컬럼 존재 + nullable
SELECT column_name, data_type, is_nullable, column_default
FROM information_schema.columns
WHERE table_name='clients' AND column_name='access_token_strategy';
-- 기대: character varying / YES / NULL

-- 어떤 클라이언트도 opt-in 되지 않았을 것 (마이그레이션은 아무것도 켜지 않는다)
SELECT count(*) FROM clients WHERE access_token_strategy IS NOT NULL;
-- 기대: 0

SELECT version, success FROM schema_migrations WHERE version='015';
-- 기대: 015 / true
```

부팅 로그에 마이그레이션 015 적용 기록이 남는지도 확인한다.

---

## 3. 클라이언트 단위 JWT access token (Run-17 P-C) — ✅ 전제 실증됨

> **P-C의 전제가 참으로 확인됐다**(2026-07-20, 로컬 Hydra v26.2 격리 실험 + Authway 실경로).
> 전역 `STRATEGIES_ACCESS_TOKEN=opaque` 하에서:
> - `access_token_strategy:"jwt"` 클라이언트 → **3 segments JWT**(`eyJhbGci…`)
> - 미핀 클라이언트 → **`ory_at_…` opaque**
> - Authway `PUT {"access_token_strategy":""}` → **200**, DB `NULL`, 이후 토큰 다시 opaque
>
> 즉 §3-1·§3-2·§3-3과 `D80-2`(PUT 전체 치환 = 핀 해제) 모두 실측 통과. staging은 **확인** 목적이다.

### 3-1. jwt opt-in 클라이언트 생성 → 토큰 형식 확인

```bash
curl -s -X POST "$API/api/v1/clients" \
  -H "Content-Type: application/json" -H "$AUTH" \
  -d "{\"tenant_id\":\"$TENANT\",\"name\":\"verify-jwt\",\"public\":false,
       \"grant_types\":[\"client_credentials\"],\"scopes\":[\"api\"],
       \"access_token_strategy\":\"jwt\"}"
# 반환된 client_id / client_secret 사용
TOKEN=$(curl -s -X POST "$ISSUER/oauth2/token" \
  -u "$CID:$CSEC" -d 'grant_type=client_credentials&scope=api' | jq -r .access_token)
echo "$TOKEN" | awk -F. '{print NF" segments"}'
```
- 기대: **3 segments**. `ory_at_…`(2 segments)면 **클라이언트 단위 전략이 동작하지 않는 것** → P-C 재설계 필요(전역 전환 또는 introspection 프록시).
- 헤더 확인: `echo "$TOKEN" | cut -d. -f1 | base64 -d` → `"alg":"RS256"` 류.

### 3-2. 미핀 클라이언트는 여전히 opaque

- `access_token_strategy` 없이 만든 클라이언트로 토큰 발급 → `ory_at_` 접두사 확인.
- **회귀 검증**: 이 항목이 깨지면 기존 소비자 전체의 토큰 형식이 바뀐 것이다.

### 3-3. 핀 해제(un-pin)가 동작하는지

- 3-1의 클라이언트를 `PUT /api/v1/clients/{id}` 로 `{"access_token_strategy": ""}` 갱신.
- 기대: 요청이 **200**(400 아님), 이후 발급 토큰이 다시 opaque.
- 남은 미검증 지점은 **Hydra 쪽뿐**이다 — 클라이언트 갱신이 PUT 전체 치환이라 필드 생략이 핀 해제로 이어진다는 가정(`D80-2`). Authway 자체 검증 경로는 `TestUpdateClientRequest_AccessTokenStrategyValidation`으로 로컬 보장됨.
- 실패 양상별 원인: **400**이면 Authway 검증(회귀), **200인데 여전히 JWT**면 Hydra PUT 치환 가정이 틀린 것 → sync 구조체의 `omitempty` 제거 후 명시적 `"opaque"` 전송으로 전환.

---

## 4. 커스텀 클레임 배치 (`ext` 중첩 / top-level 미러링)

> ✅ **env 키명이 실측으로 확인됐다.** 동일 Hydra 이미지를 env 있음/없음으로 각각 띄우고
> authorization_code 플로우를 끝까지 돌려 JWT payload를 디코드한 결과:
> - 미설정: `kind`/`tenant`가 **`ext` 안에만** 존재
> - `OAUTH2_ALLOWED_TOP_LEVEL_CLAIMS=kind,tenant`: 두 이름이 **top-level과 `ext` 양쪽에** 존재
> - 목록에 없는 `unlisted` 클레임은 같은 토큰에서 `ext`에만 남아 음성 대조가 된다
>
> 따라서 아래 4-1·4-2는 배포 환경 **확인**용이다.

### 4-1. 기본 상태 — 커스텀 클레임은 `ext` 아래

- authorization_code 플로우로 JWT access token을 발급받아 payload 디코드.
- 기대: `sub`/`client_id`/`exp`는 top-level, `email`·`tenant_id`는 `ext` 객체 안.

### 4-2. 미러링 활성 시 — top-level에도 나타남

- 배포 env에 `HYDRA_ALLOWED_TOP_LEVEL_CLAIMS=<이름들>` 설정 후 재배포.
- 기대: 해당 이름이 top-level **과** `ext` 양쪽에 존재(미러링은 `ext` 사본을 제거하지 않음).
- 미설정(빈 값)일 때는 `OAUTH2_ALLOWED_TOP_LEVEL_CLAIMS` env 자체가 컨테이너에 없어야 한다:
  ```bash
  az containerapp show -n <hydra-app> -g <rg> \
    --query "properties.template.containers[0].env[?name=='OAUTH2_ALLOWED_TOP_LEVEL_CLAIMS']"
  ```

---

## 5. Hydra 배포 스크립트 env 전달 (PowerShell 배열 splat) — ⚠️ 남은 배포-전용 항목

> `publish-hydra.core.ps1`이 `$TokenEnv` 배열을 `az ... --set-env-vars` 뒤에 넘긴다.
> **PowerShell 측 인자 구성은 실측 확인됐다** — 각 env가 개별 인자로 전개되고,
> `HYDRA_ALLOWED_TOP_LEVEL_CLAIMS` 미설정 시 배열이 1개 원소로 유지돼 빈 인자가 생기지 않는다.
> **남은 미검증은 `az` 자체가 그 인자들을(특히 값에 쉼표가 든 `kind,tenant`) 받아들이는지**뿐이며,
> 이건 로컬로 닫을 수 없다. 배포 직후 아래로 확인한다.

```bash
az containerapp show -n <hydra-app> -g <rg> \
  --query "properties.template.containers[0].env[?name=='STRATEGIES_ACCESS_TOKEN']"
```
- 기대: `[{"name":"STRATEGIES_ACCESS_TOKEN","value":"opaque"}]`.
- 값이 비어 있거나 항목이 없으면 배열 전개가 의도대로 되지 않은 것 → 개별 문자열 인자로 되돌린다.
- public / admin 두 컨테이너 **모두** 확인.

---

## 6. 회귀 스모크 (기존 기능)

- 로그인 → consent → 콜백 정상 (auth-ui).
- 로그아웃 정상.
- `audit_logs` smoke: `scripts/deploy/_shared/smoke-audit.ps1`.
- Admin 콘솔에서 기존 클라이언트 열기 → 저장 → 값 손실 없음(특히 `redirect_uris`, consent 토글).
