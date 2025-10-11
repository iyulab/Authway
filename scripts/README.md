# Authway Scripts

유틸리티 스크립트 모음

## 버전 관리

### `update-version.ps1` / `update-version.sh`

프로젝트 전체의 버전을 일괄 업데이트하는 스크립트입니다.

**업데이트 대상 파일:**
- `packages/web/admin-dashboard/package.json` - Admin Dashboard 버전
- `packages/web/login-ui/package.json` - Login UI 버전
- `.env` - 개발 환경 설정
- `.env.example` - 환경 설정 템플릿
- `.env.production.example` - 프로덕션 환경 설정 템플릿
- `docker-compose.dev.yml` - 개발 Docker 설정
- `docker-compose.prod.yml` - 프로덕션 Docker 설정
- `src/server/internal/config/config.go` - Go 백엔드 기본 버전
- `configs/config.production.yaml` - YAML 프로덕션 설정
- `configs/production.yaml` - YAML 설정

**사용법:**

```powershell
# PowerShell (Windows)
.\scripts\update-version.ps1 -Version "0.1.0"
```

```bash
# Bash (Linux/Mac)
./scripts/update-version.sh 0.1.0
```

**버전 형식:**
- Semantic Versioning (semver) 형식을 따릅니다
- 형식: `MAJOR.MINOR.PATCH` (예: `0.1.0`, `1.2.3`)

**업데이트 후 단계:**

1. **변경사항 확인**
   ```bash
   git diff
   ```

2. **package-lock.json 업데이트**
   ```bash
   cd packages/web/admin-dashboard
   npm install

   cd ../login-ui
   npm install
   ```

3. **변경사항 커밋**
   ```bash
   git add .
   git commit -m "chore: bump version to 0.1.0"
   ```

4. **태그 생성 및 푸시**
   ```bash
   git tag v0.1.0
   git push origin main --tags
   ```

## 개발 환경 관리

### `start-dev.ps1` / `start-dev.sh`

Authway 개발 환경을 시작하는 스크립트입니다.

**실행 서비스:**
- PostgreSQL (포트 5432)
- Redis (포트 6379)
- MailHog (포트 1025 SMTP, 8025 UI)
- Backend API (포트 8080)
- Admin Dashboard (포트 3000)

**사용법:**
```powershell
# PowerShell (Windows)
.\start-dev.ps1
```

```bash
# Bash (Linux/Mac)
./start-dev.sh
```

### `stop-dev.ps1` / `stop-dev.sh`

Authway 개발 환경을 중지하는 스크립트입니다.

**사용법:**
```powershell
# PowerShell (Windows)
.\stop-dev.ps1
```

```bash
# Bash (Linux/Mac)
./stop-dev.sh
```

## 데이터베이스 관리

### `migrate.ps1`

데이터베이스 마이그레이션을 실행하는 스크립트입니다.

**사용법:**
```powershell
.\scripts\migrate.ps1
```

## 스크립트 개발 가이드라인

새로운 스크립트를 추가할 때:

1. **크로스 플랫폼 지원**: PowerShell과 Bash 버전 모두 제공
2. **명확한 출력**: 이모지와 색상을 사용하여 진행상황 표시
3. **에러 처리**: 명확한 에러 메시지와 종료 코드
4. **문서화**: 이 README에 사용법 추가
5. **버전 관리**: 스크립트 변경시 변경 로그 작성

## 라이선스

MIT License - Authway 프로젝트의 일부
