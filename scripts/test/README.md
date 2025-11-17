# 테스트 스크립트

마이그레이션 시스템 및 프로젝트 구조를 검증하기 위한 테스트 스크립트 모음입니다.

## 마이그레이션 테스트

### `test-auto-migration.ps1`
자동 마이그레이션 시스템을 독립적으로 테스트합니다.

**용도**:
- `Get-PendingMigrations` 함수 검증
- 반환값 타입 및 구조 분석
- 마이그레이션 디렉토리 경로 확인

**실행**:
```powershell
.\scripts\test\test-auto-migration.ps1
```

### `test-tracking-table.ps1`
추적 테이블(`schema_migrations`)의 내용을 직접 확인합니다.

**용도**:
- 적용된 마이그레이션 버전 확인
- 실행 시간 및 성공 여부 검증

**실행**:
```powershell
.\scripts\test\test-tracking-table.ps1
```

### `test-psql-connection.ps1`
PostgreSQL 데이터베이스 연결을 테스트합니다.

**용도**:
- psql 연결 정상 작동 확인
- 환경 변수 설정 검증
- 간단한 쿼리 실행 테스트

**실행**:
```powershell
.\scripts\test\test-psql-connection.ps1
```

### `test-db-query.ps1`
데이터베이스 쿼리 실행을 테스트합니다.

**실행**:
```powershell
.\scripts\test\test-db-query.ps1
```

### `test-env-loading.ps1`
환경 변수 로딩을 테스트합니다.

**용도**:
- `.env` 파일 로딩 검증
- 필수 환경 변수 존재 확인
- 비밀번호 마스킹 확인

**실행**:
```powershell
.\scripts\test\test-env-loading.ps1
```

## 프로젝트 구조 검증

### `test-restructure.ps1`
프로젝트 재구조화 후 디렉토리 및 파일 구조를 검증합니다.

**용도**:
- 필수 디렉토리 존재 확인
- 핵심 파일 위치 검증
- 구조적 무결성 체크

**실행**:
```powershell
.\scripts\test\test-restructure.ps1
```

## 일반 사용법

모든 테스트는 프로젝트 루트에서 실행해야 합니다:

```powershell
# 프로젝트 루트로 이동
cd D:\data\Authway

# 개별 테스트 실행
.\scripts\test\test-auto-migration.ps1
.\scripts\test\test-psql-connection.ps1
```

## 필수 조건

- PowerShell 5.1 이상
- PostgreSQL psql 클라이언트 설치
- `scripts/deploy/.env` 파일 설정 완료
- 데이터베이스 연결 정보 유효

## 문제 해결

### psql을 찾을 수 없는 경우
```powershell
# PostgreSQL 설치 확인
choco install postgresql

# VSCode 재시작 또는 새 터미널 열기
```

### 환경 변수 오류
```powershell
# .env 파일 존재 확인
Test-Path scripts\deploy\.env

# 환경 변수 테스트
.\scripts\test\test-env-loading.ps1
```

### 데이터베이스 연결 실패
```powershell
# 연결 테스트
.\scripts\test\test-psql-connection.ps1

# 추적 테이블 확인
.\scripts\test\test-tracking-table.ps1
```
