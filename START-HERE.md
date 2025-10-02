# 🎉 Authway 시작하기

환영합니다! 이 문서는 Authway를 **가장 빠르게** 시작하는 방법을 안내합니다.

---

## ⚡ 초고속 시작 (1분)

### 단 한 줄로 실행

```bash
docker-compose -f docker-compose.dev.yml up -d
```

그게 전부입니다! 🎉

---

## 🌐 서비스 접속

| 서비스 | URL | 용도 |
|--------|-----|------|
| 🎨 **Login UI** | **http://localhost:3001** | 회원가입 & 로그인 |
| 📧 **MailHog** | **http://localhost:8025** | 이메일 확인 |
| 🚀 Backend API | http://localhost:8080 | API 서버 |

---

## 📝 첫 번째 테스트

### 1. 회원가입
1. 브라우저에서 http://localhost:3001/register 접속
2. 정보 입력:
   - 이메일: test@example.com
   - 비밀번호: testpassword123
   - 이름: Test User
3. "회원가입" 버튼 클릭

### 2. 이메일 인증
1. MailHog 열기: http://localhost:8025
2. "Authway - 이메일 인증" 이메일 찾기
3. 인증 링크 클릭

### 3. 로그인
1. 인증 완료 후 로그인 페이지로 이동
2. 이메일과 비밀번호로 로그인
3. 성공! ✅

---

## 🛠️ 추가 기능 테스트

### 비밀번호 재설정
1. http://localhost:3001/forgot-password
2. 이메일 입력
3. MailHog에서 재설정 이메일 확인
4. 링크로 새 비밀번호 설정

### 인증 이메일 재발송
1. http://localhost:3001/resend-verification
2. 이메일 입력
3. MailHog에서 새 인증 이메일 확인

---

## 📚 더 알아보기

완료! 기본 기능을 모두 테스트했습니다.

**다음 단계:**

### 상세 가이드
- 📘 [Docker 가이드](./DOCKER-GUIDE.md) - Docker 사용법 전체
- 🧪 [테스트 가이드](./TESTING-GUIDE.md) - 모든 기능 테스트
- ⚡ [빠른 시작](./QUICK-START.md) - 로컬 설정 가이드

### SDK 사용
- 📦 [React SDK](./packages/sdk/react/README.md) - React 앱 통합
- 📖 [API 문서](./docs/API.md) - REST API 레퍼런스

### 개발
- 🔧 [개발 가이드](./CONTRIBUTING.md) - 기여 방법
- 🚀 [배포 가이드](./DEPLOYMENT-GUIDE.md) - 프로덕션 배포

---

## 🐛 문제 해결

### 서비스가 시작되지 않음

```bash
# 로그 확인
docker-compose -f docker-compose.dev.yml logs -f

# 재시작
docker-compose -f docker-compose.dev.yml restart
```

### 포트가 이미 사용 중

```bash
# Windows
netstat -ano | findstr :8080
taskkill /PID <PID> /F

# Linux/Mac
lsof -i :8080
kill -9 <PID>
```

### 완전 초기화

```bash
# 모든 데이터 삭제 및 재시작
docker-compose -f docker-compose.dev.yml down -v
docker-compose -f docker-compose.dev.yml up -d
```

---

## 🎯 주요 명령어

```bash
# 시작
docker-compose -f docker-compose.dev.yml up -d

# 중지
docker-compose -f docker-compose.dev.yml down

# 로그 확인
docker-compose -f docker-compose.dev.yml logs -f

# 상태 확인
docker-compose -f docker-compose.dev.yml ps

# 재시작
docker-compose -f docker-compose.dev.yml restart

# 특정 서비스 재시작
docker-compose -f docker-compose.dev.yml restart authway-api
```

---

## 🤝 도움이 필요하신가요?

- 🐛 [이슈 리포트](https://github.com/authway/authway/issues)
- 💬 [Discord](https://discord.gg/authway)
- 📧 [Email](mailto:hello@authway.dev)

---

## 🎉 축하합니다!

Authway를 성공적으로 실행했습니다!

이제 다음을 할 수 있습니다:
- ✅ 회원가입 및 이메일 인증
- ✅ 로그인 및 로그아웃
- ✅ 비밀번호 재설정
- ✅ React SDK로 앱 통합
- ✅ REST API 사용

**Happy coding! 🚀**
