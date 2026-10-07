# 배포 후 검증

배포 스크립트가 성공했다는 것은 컨테이너가 교체됐다는 뜻일 뿐, 변경이 **의도대로 동작한다**는 증거가 아니다.
여기 있는 항목은 로컬에서 검증할 수 없어 배포된 환경이 실제 게이트인 것들이다.

**원칙**: "확인함"이 아니라 **출력**이 증거다. 실패하면 그 자리에서 멈추고 롤백 여부를 판단한다.
staging 에서 전부 통과한 다음 prod 로 넘어간다.

## 1. 자동 검증 (매 배포)

순서대로 실행한다. 모두 `-Target prod|staging` 의 `.env` 를 읽고, 실패 시 0 이 아닌 종료 코드를 낸다.

### 1-1. Hydra env 전달

```powershell
scripts/deploy/verify/verify-hydra-env.ps1 -Target staging
```

`publish-hydra.core.ps1` 은 env 를 배열로 `az containerapp ... --set-env-vars` 에 넘긴다. 이 스크립트는 public·admin
두 Container App 이 기대한 값(토큰 전략, 로그인·동의·로그아웃·오류 URL)을 실제로 받았는지 확인한다 — 값이 비어 있거나
env 자체가 없으면 실패.

### 1-2. 마이그레이션 적용

```powershell
scripts/deploy/staging/check-migration-status.ps1
```

API 는 기동 시 마이그레이션을 적용한다. 새 API 이미지 배포 뒤에는 보류 0 이어야 한다(종료 코드 0) —
`publish-api.ps1` 이 배포 후 검증 마지막에 이 검사를 자동으로 돌리고, 보류·실패가 있으면 배포를 실패로 끝낸다
(검사 자체를 못 하면 경고만). 비교 기준은 배포한 작업 트리의 마이그레이션 파일이다. 컬럼·테이블을
지우는 마이그레이션이면 배포 **전에도** 한 번 실행해 기준을 남긴다 — [docs/DATABASE.md](../../docs/DATABASE.md#changes-that-cannot-be-undone).

### 1-3. 로그인 → 동의 → 토큰 회귀 스모크

```powershell
scripts/deploy/verify/verify-oauth-smoke.ps1 -Target staging -Tenant <검증용 테넌트 UUID>
```

- 검증용 client·user 를 매 실행 새로 만들고 끝에 항상 삭제한다(성공/실패 무관).
- authorization_code 흐름 전 구간(로그인 흐름 → 비밀번호 로그인 → 동의 흐름 → 콜백 → 토큰 교환)을 실제로 구동한다.
- `-SkipAuditSmoke` 가 없으면 `_shared/smoke-audit.ps1` 을 이어서 실행해 `audit_logs` 에 행이 생겼는지 확인한다.

## 2. 사람이 확인하는 것 (자동화 범위 밖)

- **로그아웃**: RP-initiated 로그아웃(`post_logout_redirect_uri` 를 등록한 클라이언트)으로 등록 주소에 도착하는지.
- **소셜 로그인**: 외부 계정 자격증명이 필요하다. 시작 → 제공자 로그인 → 콜백 → 토큰까지.
- **Admin 콘솔**: 기존 클라이언트 열기 → 저장 → 값 손실 없음(특히 `redirect_uris`, 동의 토글).

## 3. 참고 — 커스텀 클레임 배치

Hydra 는 Authway 가 넣는 커스텀 클레임(`email`, `tenant_id` 등)을 access token 의 `ext` 객체 아래에 둔다.
배포 env `HYDRA_ALLOWED_TOP_LEVEL_CLAIMS=<이름들>` 을 설정하면 그 이름들이 top-level 에도 복사된다(`ext` 사본은 남는다).
미설정이면 Hydra 컨테이너에 `OAUTH2_ALLOWED_TOP_LEVEL_CLAIMS` env 자체가 없어야 한다:

```bash
az containerapp show -n <hydra-app> -g <rg> \
  --query "properties.template.containers[0].env[?name=='OAUTH2_ALLOWED_TOP_LEVEL_CLAIMS']"
```
