# OAuth 2.0 & JWT Best Practices

ASP.NET SPA 샘플 구현을 통해 학습한 OAuth 2.0 및 JWT 인증의 핵심 개념과 베스트 프랙티스입니다.

## 📚 목차

- [OAuth 2.0 PKCE 구현](#oauth-20-pkce-구현)
- [oauth4webapi 사용 패턴](#oauth4webapi-사용-패턴)
- [JWT Audience 검증](#jwt-audience-검증)
- [동적 클레임 관리](#동적-클레임-관리)
- [보안 고려사항](#보안-고려사항)
- [ASP.NET JWT 통합](#aspnet-jwt-통합)

---

## OAuth 2.0 PKCE 구현

### PKCE란?

**PKCE (Proof Key for Code Exchange, RFC 7636)** 는 Authorization Code Flow의 보안을 강화합니다:

```
1. code_verifier 생성 (43-128자 랜덤 문자열)
2. code_challenge = BASE64URL(SHA256(code_verifier))
3. Authorization 요청에 code_challenge 포함
4. Token 교환 시 원본 code_verifier 제공
5. 서버가 SHA256 해시하여 검증
```

### 구현 예제

```javascript
// 1. Code Verifier 생성
function generateCodeVerifier() {
  const array = new Uint8Array(32);
  crypto.getRandomValues(array);
  return btoa(String.fromCharCode(...array))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '');
}

// 2. Code Challenge 생성
async function generateCodeChallenge(verifier) {
  const encoder = new TextEncoder();
  const data = encoder.encode(verifier);
  const hash = await crypto.subtle.digest('SHA-256', data);
  return btoa(String.fromCharCode(...new Uint8Array(hash)))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '');
}

// 3. Authorization URL 생성
const authUrl = new URL(authorizationServer.authorization_endpoint);
authUrl.searchParams.set('client_id', clientId);
authUrl.searchParams.set('redirect_uri', redirectUri);
authUrl.searchParams.set('response_type', 'code');
authUrl.searchParams.set('scope', 'openid profile email');
authUrl.searchParams.set('code_challenge', codeChallenge);
authUrl.searchParams.set('code_challenge_method', 'S256');
authUrl.searchParams.set('state', state);
authUrl.searchParams.set('audience', 'api');  // ← JWT audience 요청

// 4. 세션 저장
sessionStorage.setItem('code_verifier', codeVerifier);
sessionStorage.setItem('oauth_state', state);

// 5. 리다이렉트
window.location.href = authUrl.toString();
```

### 왜 PKCE가 필요한가?

- **Authorization Code 가로채기 방지**: 공격자가 코드를 가로채도 `code_verifier` 없이는 토큰 교환 불가
- **Public Client 보안**: 클라이언트 시크릿을 안전하게 저장할 수 없는 SPA/모바일 앱 필수
- **RFC 8252 준수**: OAuth 2.0 for Native Apps 권장사항

---

## oauth4webapi 사용 패턴

### 기본 설정

**중요**: oauth4webapi는 기본적으로 HTTPS를 강제합니다. 로컬 개발 시:

```javascript
// ❌ 잘못된 방법: HTTP에서 작동 안 함
const response = await oauth.discoveryRequest(issuer);

// ✅ 올바른 방법: allowInsecureRequests 플래그 사용
const response = await oauth.discoveryRequest(issuer, {
  [oauth.allowInsecureRequests]: true  // 개발 환경에서만!
});

const authorizationServer = await oauth.processDiscoveryResponse(issuer, response, {
  [oauth.allowInsecureRequests]: true
});
```

⚠️ **프로덕션 경고**: `allowInsecureRequests`는 절대 프로덕션에서 사용하지 마세요!

### 토큰 교환 패턴

**필수 단계**:

```javascript
// 1. Authorization Response 검증 (필수!)
const validatedParams = oauth.validateAuthResponse(
  authorizationServer,
  client,
  callbackUrl,      // 전체 URL (code, state 포함)
  storedState       // sessionStorage의 원본 state
);

// 2. 토큰 교환
const tokenResponse = await oauth.authorizationCodeGrantRequest(
  authorizationServer,
  client,
  oauth.None(),     // ← Public client 인증 (3번째 파라미터 필수!)
  validatedParams,  // ← 검증된 파라미터 사용
  redirectUri,
  codeVerifier,
  { [oauth.allowInsecureRequests]: true }
);

// 3. 응답 처리 (함수명 주의!)
const result = await oauth.processAuthorizationCodeResponse(
  authorizationServer,
  client,
  tokenResponse,
  {
    requireIdToken: true,  // ID 토큰 요구
    [oauth.allowInsecureRequests]: true
  }
);

// 4. Claims 추출
const claims = oauth.getValidatedIdTokenClaims(result);
```

### 흔한 실수들

| ❌ 잘못된 방법 | ✅ 올바른 방법 |
|--------------|-------------|
| `validateAuthResponse()` 생략 | 항상 `validateAuthResponse()` 호출 |
| `oauth.None()` 생략 (3번째 파라미터) | Public client는 `oauth.None()` 필수 |
| `processAuthorizationCodeOpenIDResponse()` | `processAuthorizationCodeResponse()` 사용 |
| `if (oauth.isOAuth2Error(result))` | `try-catch` 사용 (oauth4webapi는 에러 throw) |
| 원본 URL을 직접 토큰 교환에 사용 | `validateAuthResponse()`가 반환한 검증된 파라미터 사용 |

### 에러 처리

```javascript
try {
  const result = await oauth.authorizationCodeGrantRequest(/* ... */);
} catch (error) {
  // oauth4webapi는 에러 객체를 throw
  console.error('Token exchange failed:', error.message);

  // isOAuth2Error() 함수는 존재하지 않음
  // try-catch로 에러 처리
}
```

---

## JWT Audience 검증

### Audience란?

**Audience (`aud` claim)** 는 JWT가 의도된 수신자를 식별합니다:

```json
{
  "iss": "http://localhost:4444",
  "sub": "user-123",
  "aud": ["api"],  // ← 이 토큰은 "api" 리소스용
  "exp": 1699999999
}
```

### Hydra Audience 설정의 혼란

**문제**: Hydra 클라이언트 설정에서 `audience`는 **화이트리스트**일 뿐입니다.

```json
// Hydra 클라이언트 설정
{
  "client_id": "my-client",
  "audience": ["api"]  // ← "api"를 요청할 수 있다는 의미일 뿐
}
```

**실제 토큰에 포함시키려면**:

```javascript
// Authorization URL에 명시적으로 추가
authUrl.searchParams.set('audience', 'api');  // ← 필수!
```

### ASP.NET Backend 검증

```csharp
// 올바른 검증 설정
options.TokenValidationParameters = new TokenValidationParameters
{
    ValidateIssuer = true,
    ValidIssuer = authority,
    ValidateAudience = true,      // ← 프로덕션에서 필수
    ValidAudience = "api",         // ← 예상 audience
    ValidateLifetime = true,
    ValidateIssuerSigningKey = true
};
```

### 왜 Audience 검증이 중요한가?

**시나리오**: 공격자가 다른 API용 토큰을 가로챔

```json
// 공격자가 가진 토큰
{
  "aud": ["other-api"],
  "scope": ["admin"]
}
```

- ❌ **Audience 검증 없음**: 내 API가 이 토큰을 받아들임 (보안 취약)
- ✅ **Audience 검증 있음**: `aud`가 일치하지 않아 거부

**결론**: Audience 검증은 토큰이 올바른 API에만 사용되도록 보장합니다.

### 현재 샘플의 임시 해결책

```csharp
// asp-spa 샘플에서 임시로 비활성화
ValidateAudience = false,  // ⚠️ 프로덕션에서는 절대 안 됨!
```

**이유**: Hydra 설정 이슈로 토큰에 `aud` claim이 포함되지 않음
**TODO**: Hydra audience 설정 조사 및 수정 후 검증 활성화

---

## 동적 클레임 관리

### ext 네임스페이스

Authway는 커스텀 클레임을 `ext` 네임스페이스에 저장합니다:

```json
{
  "sub": "user-123",
  "name": "John Doe",
  "email": "john@example.com",
  "ext": {
    "department": "Engineering",
    "role": "admin",
    "preferences": {
      "theme": "dark",
      "language": "ko"
    }
  }
}
```

### 왜 ext를 사용하는가?

| 방법 | 장점 | 단점 |
|------|------|------|
| **루트 레벨** | 직접 접근 간편 | 표준 claim과 충돌 위험 |
| **ext 네임스페이스** | 충돌 방지, 명확한 구분 | 한 단계 더 접근 |

**예시**: `role`이라는 커스텀 claim을 추가하면?

```json
// ❌ 루트 레벨: 미래에 표준 "role" claim이 추가되면 충돌
{
  "role": "admin",
  "name": "John"
}

// ✅ ext 네임스페이스: 안전
{
  "name": "John",
  "ext": {
    "role": "admin"
  }
}
```

### API 사용법

```javascript
// 클레임 추가/업데이트
await fetch('http://localhost:8081/api/v1/claims/user', {
  method: 'PATCH',
  headers: {
    'Authorization': `Bearer ${accessToken}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    claims: {
      department: 'Sales',
      role: 'manager'
    }
  })
});

// 클레임 삭제
await fetch('http://localhost:8081/api/v1/claims/user?key=department', {
  method: 'DELETE',
  headers: { 'Authorization': `Bearer ${accessToken}` }
});
```

### 자동 JSON 파싱

```javascript
// 문자열이 JSON처럼 보이면 자동 파싱
function updateClaim(key, value) {
  let parsedValue = value;

  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
      try {
        parsedValue = JSON.parse(trimmed);
      } catch {}
    }
  }

  // parsedValue는 이제 객체 또는 배열
}
```

---

## 보안 고려사항

### Popup vs Redirect 인증

**세션 키 분리**: 충돌 방지를 위해 다른 키 사용

```javascript
// Redirect 인증
sessionStorage.setItem('code_verifier', codeVerifier);
sessionStorage.setItem('oauth_state', state);

// Popup 인증
sessionStorage.setItem('popup_code_verifier', codeVerifier);
sessionStorage.setItem('popup_oauth_state', state);
```

**이유**: 사용자가 여러 탭을 열거나 두 방식을 전환할 때 세션 데이터가 덮어쓰여지는 것을 방지

### postMessage 보안

```javascript
window.addEventListener('message', (event) => {
  // ✅ Origin 검증 필수
  if (event.origin !== window.location.origin) {
    return;  // 다른 origin은 무시
  }

  if (event.data && event.data.type === 'oauth-callback') {
    // 안전하게 처리
  }
});
```

### State 파라미터

**CSRF 공격 방지**:

```javascript
// 1. 랜덤 state 생성
const state = generateCodeVerifier();
sessionStorage.setItem('oauth_state', state);

// 2. Authorization URL에 포함
authUrl.searchParams.set('state', state);

// 3. Callback에서 검증
const returnedState = new URLSearchParams(window.location.search).get('state');
const storedState = sessionStorage.getItem('oauth_state');

if (returnedState !== storedState) {
  throw new Error('Invalid state - possible CSRF attack');
}

// 4. 사용 후 삭제
sessionStorage.removeItem('oauth_state');
sessionStorage.removeItem('code_verifier');
```

### 토큰 저장

| 저장소 | 장점 | 단점 | 권장 |
|--------|------|------|------|
| **sessionStorage** | XSS로부터 상대적으로 안전, 탭 닫으면 삭제 | 탭마다 별도 세션 | ✅ SPA 권장 |
| **localStorage** | 지속성, 탭 간 공유 | XSS 취약, 영구 저장 | ⚠️ 주의 필요 |
| **메모리** | 가장 안전 | 새로고침 시 손실 | ✅ SDK 내부 권장 |
| **Cookie (httpOnly)** | XSS 방어 | CSRF 취약, 서버 필요 | ✅ 서버 세션용 |

**샘플 앱 선택**: `sessionStorage` (단순성과 보안 균형)

---

## ASP.NET JWT 통합

### JWT Bearer 인증 설정

```csharp
// Program.cs
builder.Services.AddAuthentication(JwtBearerDefaults.AuthenticationScheme)
    .AddJwtBearer(options =>
    {
        options.Authority = "http://localhost:4444";  // Hydra
        options.Audience = "api";
        options.RequireHttpsMetadata = false;  // 개발 환경에서만

        options.TokenValidationParameters = new TokenValidationParameters
        {
            ValidateIssuer = true,
            ValidIssuer = "http://localhost:4444",
            ValidateAudience = true,     // 프로덕션 필수
            ValidAudience = "api",
            ValidateLifetime = true,      // 만료 검증
            ValidateIssuerSigningKey = true  // 서명 검증
        };
    });
```

### CORS 설정

```csharp
var corsOrigins = builder.Configuration
    .GetSection("Cors:AllowedOrigins")
    .Get<string[]>() ?? new[] { "http://localhost:5173" };

builder.Services.AddCors(options =>
{
    options.AddDefaultPolicy(policy =>
    {
        policy.WithOrigins(corsOrigins)
              .AllowAnyMethod()
              .AllowAnyHeader()
              .AllowCredentials();  // 인증 쿠키 허용
    });
});
```

### 중복 Claims 처리

**문제**: JWT의 `scp` 배열이 여러 개의 `scope` claim으로 변환됨

```json
// JWT
{
  "scp": ["openid", "profile", "email"]
}

// ASP.NET Claims
[
  { Type: "scope", Value: "openid" },
  { Type: "scope", Value: "profile" },
  { Type: "scope", Value: "email" }
]
```

**해결책**: `GroupBy`로 중복 처리

```csharp
app.MapGet("/api/me", [Authorize] (HttpContext context) =>
{
    var user = context.User;

    // ❌ 잘못된 방법: ToDictionary는 중복 키에서 실패
    // var claims = user.Claims.ToDictionary(c => c.Type, c => c.Value);

    // ✅ 올바른 방법: GroupBy로 중복 처리
    var claims = user.Claims
        .GroupBy(c => c.Type)
        .ToDictionary(
            g => g.Key,
            g => g.Count() > 1
                ? (object)g.Select(c => c.Value).ToArray()  // 배열로
                : g.First().Value  // 단일 값
        );

    return new { claims };
});
```

### 디버깅 이벤트

```csharp
options.Events = new JwtBearerEvents
{
    OnAuthenticationFailed = context =>
    {
        var logger = context.HttpContext.RequestServices
            .GetRequiredService<ILogger<Program>>();
        logger.LogError(context.Exception, "Authentication failed");
        return Task.CompletedTask;
    },

    OnTokenValidated = context =>
    {
        var logger = context.HttpContext.RequestServices
            .GetRequiredService<ILogger<Program>>();
        logger.LogInformation("Token validated for: {User}",
            context.Principal?.Identity?.Name);
        return Task.CompletedTask;
    }
};
```

---

## 참고 자료

### RFC 문서
- **[RFC 6749](https://tools.ietf.org/html/rfc6749)** - OAuth 2.0 Authorization Framework
- **[RFC 7636](https://tools.ietf.org/html/rfc7636)** - PKCE (Proof Key for Code Exchange)
- **[RFC 8252](https://tools.ietf.org/html/rfc8252)** - OAuth 2.0 for Native Apps
- **[RFC 7519](https://tools.ietf.org/html/rfc7519)** - JSON Web Token (JWT)

### 라이브러리
- **[oauth4webapi](https://github.com/panva/oauth4webapi)** - Official OAuth 2.0 client library
- **[Ory Hydra](https://www.ory.sh/hydra/docs/)** - OAuth 2.0 and OpenID Connect server

### Authway 문서
- **[ASP.NET SPA Sample](../../samples/asp-spa/)** - 전체 구현 예제
- **[Popup Login Guide](./POPUP_LOGIN_GUIDE.md)** - 팝업 인증 상세 가이드
- **[Dynamic Claims](./DYNAMIC_CLAIMS.md)** - 동적 클레임 관리

---

## 요약

### ✅ 반드시 해야 할 것

1. **PKCE 사용**: Public client는 필수
2. **oauth4webapi 검증 패턴**: `validateAuthResponse()` → `authorizationCodeGrantRequest()` → `processAuthorizationCodeResponse()`
3. **Audience 검증**: 프로덕션에서 `ValidateAudience = true`
4. **State 검증**: CSRF 공격 방지
5. **Origin 검증**: postMessage 사용 시
6. **HTTPS 사용**: 프로덕션 환경

### ⚠️ 하지 말아야 할 것

1. **allowInsecureRequests 프로덕션 사용**: HTTP는 개발에서만
2. **Audience 검증 비활성화**: 토큰 대체 공격 취약
3. **State 파라미터 생략**: CSRF 공격 가능
4. **토큰 localStorage 무분별 저장**: XSS 취약
5. **oauth.None() 생략**: Public client 인증 실패
6. **검증 없이 토큰 교환**: oauth4webapi 요구사항

### 🎓 학습 포인트

- **oauth4webapi는 명시적**: 각 단계 검증 필수
- **Hydra audience는 화이트리스트**: 명시적 요청 필요
- **ext 네임스페이스**: 클레임 충돌 방지
- **GroupBy 패턴**: ASP.NET 중복 claims 처리
- **세션 키 분리**: Popup/Redirect 충돌 방지
