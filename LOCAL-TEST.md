# 🚀 로컬 테스트 3분 가이드

## 1️⃣ 시작 (30초)

```bash
docker-compose -f docker-compose.dev.yml up -d
```

## 2️⃣ 접속 URL

| 서비스 | URL | 용도 |
|--------|-----|------|
| 🎨 **로그인 UI** | http://localhost:3001 | 회원가입/로그인 |
| 📧 **이메일 확인** | http://localhost:8025 | 인증 메일 보기 |
| 🔧 **API** | http://localhost:8080 | 백엔드 |

## 3️⃣ 테스트 시나리오

### ✅ 회원가입 & 인증
1. http://localhost:3001/register 접속
2. 정보 입력 후 가입
3. http://localhost:8025 에서 인증 메일 확인
4. 인증 링크 클릭

### ✅ 로그인
1. http://localhost:3001/login 접속
2. 이메일/비밀번호 입력
3. 로그인 성공 ✓

### ✅ 비밀번호 재설정
1. http://localhost:3001/forgot-password 접속
2. 이메일 입력
3. MailHog에서 재설정 링크 확인
4. 새 비밀번호 설정

## 4️⃣ API 테스트

### 회원가입
```bash
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123",
    "name": "Test User"
  }'
```

### 로그인
```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'
```

### 토큰으로 사용자 정보 조회
```bash
curl http://localhost:8080/api/users/me \
  -H "Authorization: Bearer {access_token}"
```

## 5️⃣ 주요 명령어

```bash
# 로그 확인
docker-compose -f docker-compose.dev.yml logs -f

# 재시작
docker-compose -f docker-compose.dev.yml restart

# 중지
docker-compose -f docker-compose.dev.yml down

# 완전 초기화 (데이터 삭제)
docker-compose -f docker-compose.dev.yml down -v
```

## 🐛 트러블슈팅

### 포트 충돌 시
```bash
# Windows
netstat -ano | findstr :8080
taskkill /PID {PID} /F

# Linux/Mac
lsof -i :8080
kill -9 {PID}
```

### 서비스 상태 확인
```bash
docker-compose -f docker-compose.dev.yml ps
```

## ✅ 체크리스트

- [ ] Docker Desktop 실행 중
- [ ] `docker-compose up -d` 실행 완료
- [ ] http://localhost:3001 접속 확인
- [ ] http://localhost:8025 접속 확인
- [ ] 회원가입 → 이메일 인증 → 로그인 성공

---

**완료!** 🎉 이제 Authway를 로컬에서 사용할 수 있습니다.
