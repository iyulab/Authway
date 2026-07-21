# Authway ASP.NET Core Sample

ASP.NET Core MVC 애플리케이션으로 Authway OAuth2/OpenID Connect 인증을 구현한 샘플입니다.

## 🎯 지원 환경

- ✅ **로컬 개발**: 로컬 Authway 스택 사용 (docker-compose)
- ✅ **Azure 프로덕션**: Azure 배포된 Authway 사용

## 아키텍처

Authway는 **3-Tier 아키텍처**를 사용합니다:

```
ASP.NET App  →  Auth Backend (8081)  →  Central API (8080)  →  Hydra (4444)
                                      ↓
                                 PostgreSQL + Redis
```

- **Auth Backend** (port 8081): Consumer Apps용 OAuth 콜백 및 프록시
- **Central API** (port 8080): 내부 비즈니스 로직 및 데이터 관리
- **Ory Hydra** (port 4444): OAuth2/OIDC 표준 서버
- **Login UI** (port 3001): 로그인 화면
- **Admin UI** (port 3000): 관리자 대시보드

## 기능

- ✅ **Redirect Login**: 전통적인 전체 페이지 리다이렉트 인증
- ✅ **Popup Login**: 모던 팝업 기반 인증 (postMessage 패턴)
- ✅ **Dynamic Claims**: 재인증 없이 런타임에서 claim 업데이트
- ✅ **Workspace Switching**: 멀티테넌트 워크스페이스 관리
- ✅ **Token Management**: 자동 토큰 갱신 및 관리
- ✅ **PKCE**: Authorization Code 탈취 방지
- ✅ **User Profile**: 사용자 정보 및 Claims 조회

## 사전 요구사항

### 소프트웨어
- [.NET 8.0 SDK](https://dotnet.microsoft.com/download/dotnet/8.0) 이상
- [Docker Desktop](https://www.docker.com/products/docker-desktop) (Hydra 실행용)

### Authway 로컬 환경 실행
Authway 전체 스택을 로컬에서 실행해야 합니다:

1. **Auth Backend** - OAuth 콜백 핸들러 (`http://localhost:8081`)
2. **Central API** - 비즈니스 로직 (`http://localhost:8080`)
3. **Ory Hydra** - OAuth2/OIDC 서버 (`http://localhost:4444`)
4. **Login UI** - 로그인 화면 (`http://localhost:3001`)
5. **Admin UI** - 관리자 대시보드 (`http://localhost:3000`)
6. **PostgreSQL** - 데이터베이스 (`localhost:5432`)
7. **Redis** - 세션 저장소 (`localhost:6379`)

```powershell
# Authway 로컬 환경 시작 (권장)
cd D:\data\Authway
.\start-dev.ps1

# 또는 Docker Compose 직접 실행
docker compose up -d
```

## 빠른 시작 (Quick Start)

### 1단계: Authway 서비스 시작

```powershell
# Authway 프로젝트 루트로 이동
cd D:\data\Authway

# 모든 서비스 시작 (Docker Compose)
.\start-dev.ps1
```

### 2단계: ASP.NET 샘플 시작

```powershell
# ASP.NET 샘플 디렉토리로 이동
cd samples\asp-sample

# 자동 설정 + 실행 (권장)
.\start.ps1

# 또는 수동 실행
.\setup-client-local.ps1  # OAuth 클라이언트 등록
dotnet run                 # 애플리케이션 시작
```

### 3단계: 브라우저 테스트

1. http://localhost:5000 접속
2. "Login with Authway" 클릭
3. 로그인 후 프로필 페이지 확인

## OAuth 클라이언트 등록 (수동)

`start.ps1` 스크립트가 자동으로 처리하지만, 수동으로 설정하려면:

### 방법 1: 자동 스크립트 사용 (권장)

```powershell
cd samples\asp-sample
.\setup-client-local.ps1
```

이 스크립트는:
- ✅ Central API에 클라이언트 등록
- ✅ Hydra에 OAuth 클라이언트 등록
- ✅ 서비스 헬스 체크
- ✅ Tenant 자동 선택

### 방법 2: Admin Dashboard 사용

1. Authway Admin Dashboard (`http://localhost:3000`) 접속
2. OAuth 클라이언트 생성:
   ```
   Client ID: asp-sample-client
   Client Secret: asp-sample-secret-change-in-production
   Redirect URIs:
     - http://localhost:5000/signin-oidc
     - https://localhost:5001/signin-oidc
     - http://localhost:5000/callback.html
   Grant Types: authorization_code, refresh_token
   Scopes: openid, profile, email
   ```

## 설정

### 🎯 최고 수준의 간소화 (단일 URL 설정)

Authway는 **API 기반 자동 발견**을 사용하므로 **단 3개의 설정값만 필요합니다**:

```json
{
  "Authway": {
    "Server": "http://localhost:8081",
    "ClientId": "asp-sample-client",
    "ClientSecret": "asp-sample-secret-change-in-production"
  }
}
```

**중요**: `Server`는 **Auth Backend URL** (port 8081)을 사용해야 합니다. Central API (8080)는 내부 전용입니다.

**왜 이렇게 간단한가요?**

- ✅ **API-Driven Discovery**: Server URL에서 Hydra URL(OIDC Authority)를 자동으로 가져옴
- ✅ **OIDC Discovery**: Hydra에서 모든 OAuth/OIDC 엔드포인트 자동 발견
- ✅ **Single Source of Truth**: 하나의 URL만 알면 모든 것이 자동으로 설정됨
- ✅ **No Manual URLs**: Hydra URL, 로그인 URL, 토큰 URL 등 수동 설정 불필요

### 자동 발견 작동 방식

```
1. Server URL (/.well-known/authway-config)
   ↓
2. Hydra URL (issuer) 자동 발견
   ↓
3. OIDC Discovery (/.well-known/openid-configuration)
   ↓
4. 모든 OAuth 엔드포인트 자동 구성 (authorization, token, userinfo, etc.)
```

**Fallback 메커니즘**: Config discovery 실패 시 `Domain` 설정을 자동으로 사용

### 설정 키 설명

| Key | 필수 | 설명 |
|-----|------|------|
| `Server` | ✅ Yes | Authway API Server URL (모든 것이 자동으로 발견됨) |
| `ClientId` | ✅ Yes | OAuth 2.0 Client ID |
| `ClientSecret` | ✅ Yes | OAuth 2.0 Client Secret |

### 로컬 개발 환경

`appsettings.Development.json`:
```json
{
  "Authway": {
    "Server": "http://localhost:8081",
    "Domain": "http://localhost:4444",
    "ClientId": "asp-sample-client",
    "ClientSecret": "asp-sample-secret-change-in-production"
  }
}
```

### Azure 프로덕션 환경

`appsettings.json`:
```json
{
  "Authway": {
    "Server": "https://auth-api.authway.in",
    "Domain": "https://oauth.authway.in",
    "ClientId": "authway_-3k7iQ2u2HblSljVJbaZuA",
    "ClientSecret": "RI8Xjdrrk2nhSYVdF3tet78dgazeId33QyXqUz6Kj1k"
  }
}
```

### 다른 OIDC 제공자와 비교

**Auth0:**
```json
{
  "Domain": "your-tenant.auth0.com",   // Auth0 API에서 모든 것 발견
  "ClientId": "...",
  "ClientSecret": "..."
}
```

**Okta:**
```json
{
  "OktaDomain": "https://your-org.okta.com",   // Okta API에서 모든 것 발견
  "ClientId": "...",
  "ClientSecret": "..."
}
```

**Authway:**
```json
{
  "Server": "https://authway-api.iyulab.com",   // Authway API에서 모든 것 발견
  "ClientId": "...",
  "ClientSecret": "..."
}
```

**결론: Auth0/Okta보다 더 간단! 단 하나의 Server URL만 있으면 됩니다.** ✅

### 환경 변수로 설정 (선택사항)

```powershell
# PowerShell
$env:Authway__Server = "http://localhost:8080"
$env:Authway__ClientId = "asp-sample-client"
$env:Authway__ClientSecret = "dev-secret-change-me"

# Bash
export Authway__Server="http://localhost:8080"
export Authway__ClientId="asp-sample-client"
export Authway__ClientSecret="dev-secret-change-me"
```

### 서비스 실행 확인

```powershell
# Auth Backend (OAuth 콜백)
curl http://localhost:8081/health
curl http://localhost:8081/.well-known/authway-config

# Central API (비즈니스 로직)
curl http://localhost:8080/health

# Hydra Public (OAuth2/OIDC 서버)
curl http://localhost:4444/.well-known/openid-configuration

# Hydra Admin
curl http://localhost:4445/health/ready
```

## 실행 방법

### 옵션 1: 자동 시작 스크립트 (권장)

```powershell
cd samples\asp-sample

# 전체 자동 실행 (클라이언트 설정 + 앱 시작)
.\start.ps1

# 클라이언트 설정 스킵
.\start.ps1 -SkipSetup

# HTTPS 프로파일 사용
.\start.ps1 -Profile https

# Production 모드
.\start.ps1 -Production
```

### 옵션 2: 수동 실행

```powershell
cd samples\asp-sample

# 1. 클라이언트 등록 (처음 한 번만)
.\setup-client-local.ps1

# 2. 앱 실행
dotnet run

# 또는 특정 프로파일 실행
dotnet run --launch-profile http
dotnet run --launch-profile https
dotnet run --launch-profile Production
```

### 옵션 3: Visual Studio / VS Code

1. `asp-sample.csproj` 파일 열기
2. F5 키를 눌러 디버깅 시작
3. 런치 프로파일 선택 가능

## 프로젝트 구조

```
asp-sample/
├── Controllers/
│   └── HomeController.cs          # 메인 컨트롤러 (로그인/로그아웃/프로필)
├── Views/
│   ├── Home/
│   │   ├── Index.cshtml           # 홈 페이지
│   │   ├── Profile.cshtml         # 사용자 프로필 페이지
│   │   └── Error.cshtml           # 에러 페이지
│   ├── Shared/
│   │   └── _Layout.cshtml         # 레이아웃
│   ├── _ViewImports.cshtml
│   └── _ViewStart.cshtml
├── wwwroot/
│   └── css/
│       └── site.css                # 스타일시트
├── Program.cs                      # 애플리케이션 엔트리 포인트
├── appsettings.json                # 기본 설정
├── appsettings.Development.json    # 개발 환경 설정
└── asp-sample.csproj               # 프로젝트 파일
```

## 주요 기능 설명

### 1. OAuth2/OIDC 인증 설정 (Program.cs)

```csharp
// Authway 서버에서 설정 자동 발견
var authwayConfigService = new AuthwayConfigService(httpClient, configuration, logger);
string authorityUrl = await authwayConfigService.GetAuthorityAsync();

builder.Services.AddAuthentication(options =>
{
    options.DefaultScheme = CookieAuthenticationDefaults.AuthenticationScheme;
    options.DefaultChallengeScheme = OpenIdConnectDefaults.AuthenticationScheme;
})
.AddCookie()
.AddOpenIdConnect(options =>
{
    // Authority는 Server URL에서 자동으로 발견됨
    // Server -> /api/v1/config -> issuer (Hydra URL)
    options.Authority = authorityUrl;
    options.ClientId = builder.Configuration["Authway:ClientId"];
    options.ClientSecret = builder.Configuration["Authway:ClientSecret"];

    // 표준 OIDC Authorization Code Flow + PKCE
    options.ResponseType = OpenIdConnectResponseType.Code;
    options.UsePkce = true;

    // 스코프 설정
    options.Scope.Add("openid");
    options.Scope.Add("profile");
    options.Scope.Add("email");

    // 토큰 저장 (Dynamic Claims 사용을 위해 필요)
    options.SaveTokens = true;
    // ...
});
```

**2단계 자동 발견:**
1. **Step 1**: Server URL → `/api/v1/config` → Hydra URL (issuer)
2. **Step 2**: Hydra URL → `/.well-known/openid-configuration` → 모든 OAuth 엔드포인트
3. **결과**: 완전 자동화된 설정!

### 2. 로그인 (HomeController.cs)

```csharp
public IActionResult Login(string returnUrl = "/")
{
    var properties = new AuthenticationProperties
    {
        RedirectUri = returnUrl
    };
    return Challenge(properties, OpenIdConnectDefaults.AuthenticationScheme);
}
```

### 3. 로그아웃 (HomeController.cs)

```csharp
[Authorize]
public async Task<IActionResult> Logout()
{
    await HttpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);

    // Use bare origin URL (without path) for post_logout_redirect_uri
    // This must match exactly what's registered in Hydra client
    var redirectUri = $"{Request.Scheme}://{Request.Host}";

    var properties = new AuthenticationProperties
    {
        RedirectUri = redirectUri
    };

    return SignOut(properties, OpenIdConnectDefaults.AuthenticationScheme);
}
```

> **주의**: `post_logout_redirect_uri`는 Hydra 클라이언트에 등록된 URI와 **정확히** 일치해야 합니다. Trailing slash(`/`)가 있으면 다른 URI로 인식됩니다.

### 4. 사용자 정보 접근

```csharp
// 인증 여부 확인
User.Identity?.IsAuthenticated

// 사용자명
User.Identity?.Name

// Claims 접근
User.Claims

// 토큰 접근
var accessToken = await HttpContext.GetTokenAsync("access_token");
var idToken = await HttpContext.GetTokenAsync("id_token");
```

## 페이지

### 홈 페이지 (`/`)
- 로그인 버튼
- 인증 상태 표시
- 기능 소개

### 프로필 페이지 (`/Home/Profile`)
- 사용자 정보 표시
- Claims 목록
- Access Token, ID Token, Refresh Token 표시
- `[Authorize]` 속성으로 보호됨

## 보안 고려사항

### 프로덕션 배포 시 변경사항

1. **HTTPS 필수**
   ```csharp
   options.RequireHttpsMetadata = true; // Program.cs에서 변경
   ```

2. **Client Secret 보호**
   - Azure Key Vault, AWS Secrets Manager 등 사용
   - 환경 변수로 관리
   - appsettings.json에 하드코딩 금지

3. **Redirect URI 검증**
   - Authway Admin에서 정확한 프로덕션 URL 등록
   - 와일드카드 사용 금지

4. **Cookie 설정 강화**
   ```csharp
   .AddCookie(options =>
   {
       options.Cookie.SecurePolicy = CookieSecurePolicy.Always;
       options.Cookie.SameSite = SameSiteMode.Strict;
       options.Cookie.HttpOnly = true;
   })
   ```

## 문제 해결

### 서비스가 실행되지 않음
**증상**: `start.ps1` 실행 시 서비스 헬스 체크 실패

**해결**:
```powershell
# Authway 루트 디렉토리로 이동
cd D:\data\Authway

# 모든 서비스 시작
.\start-dev.ps1

# 또는 Docker Compose 직접 실행
docker compose up -d

# 서비스 상태 확인
docker ps
```

### Config Discovery 실패
**증상**: `Could not fetch Authway configuration from server`

**해결**:
1. Auth Backend가 실행 중인지 확인:
   ```powershell
   curl http://localhost:8081/health
   curl http://localhost:8081/.well-known/authway-config
   ```

2. Fallback이 작동하는지 확인:
   - `appsettings.Development.json`에 `Domain` 설정 추가
   - `"Domain": "http://localhost:4444"`

### 401 Unauthorized 에러
**원인**: Client ID/Secret 불일치 또는 클라이언트 미등록

**해결**:
```powershell
# 클라이언트 재등록
cd samples\asp-sample
.\setup-client-local.ps1

# Central API에서 클라이언트 확인
curl http://localhost:8080/api/v1/clients

# Hydra에서 클라이언트 확인
curl http://localhost:4445/admin/clients/asp-sample-client
```

### Redirect URI mismatch
**원인**: 등록된 Redirect URI와 실제 콜백 URI 불일치

**해결**:
- 프로토콜(http/https), 포트번호 확인
- `setup-client-local.ps1`의 `$REDIRECT_URIS` 확인
- Hydra 클라이언트 삭제 후 재등록:
  ```powershell
  .\setup-client-local.ps1
  ```

### Logout 시 post_logout_redirect_uri 에러
**증상**: 로그아웃 시 "post_logout_redirect_uri is not a whitelisted" 에러

**원인**: Hydra 클라이언트에 `post_logout_redirect_uris`가 등록되지 않음

**해결**:
1. `setup-client-local.ps1`을 다시 실행 (자동으로 포함됨):
   ```powershell
   .\setup-client-local.ps1
   ```

2. 또는 수동으로 Hydra 클라이언트 업데이트:
   ```powershell
   curl -X PUT http://localhost:4445/admin/clients/asp-sample-client `
     -H "Content-Type: application/json" `
     -d '{"post_logout_redirect_uris": ["http://localhost:5000", "https://localhost:5001"]}'
   ```

**주의**: `post_logout_redirect_uri`는 정확히 일치해야 합니다 (trailing slash 주의)

### HTTPS 인증서 에러 (개발 환경)
```powershell
dotnet dev-certs https --trust
```

### 프로덕션 배포

현재 Azure 프로덕션 환경에는 **Ory Hydra가 배포되지 않았습니다**.

프로덕션 배포를 위한 옵션:

#### 옵션 1: Azure에 Hydra 배포 (권장)
```bash
# Azure Container Apps에 Hydra 배포
# 1. Hydra Admin API
# 2. Hydra Public API
# 3. Hydra DB 마이그레이션
```

#### 옵션 2: 자체 OAuth2 서버 구현
Authway Backend에 Hydra 없이 작동하는 OAuth2/OIDC 엔드포인트 추가

#### 현재 상태
```
https://auth.iyulab.com        → Login UI만 제공 (Hydra 없음)
https://authway-admin.iyulab.com → Admin Dashboard
https://authway-api.iyulab.com  → Backend API (Hydra 의존)
```

**결론**: 이 샘플은 **로컬 개발 전용**입니다. 프로덕션 사용을 위해서는 Azure에 Hydra를 배포해야 합니다.

## Popup Login 구현 옵션

이 샘플은 **자체 PopupCallback 엔드포인트**를 구현하고 있습니다 (`/Home/PopupCallback`). 하지만 Authway는 이제 **3가지 popup callback 옵션**을 제공합니다:

### Option 1: 자체 PopupCallback 사용 (현재 구현)

**장점**: 완전한 제어, 커스터마이징 가능

**구현**: `Controllers/HomeController.cs:72`
```csharp
[Authorize]
public IActionResult PopupCallback([FromQuery] string? origin = null)
{
    // Origin 검증
    var allowedOrigins = configuration.GetSection("Cors:AllowedOrigins").Get<string[]>();
    if (!string.IsNullOrEmpty(origin) && !allowedOrigins.Contains(origin))
    {
        frontendUrl = allowedOrigins[0];
    }

    // postMessage 전송
    ViewBag.FrontendUrl = frontendUrl;
    return View();
}
```

**Redirect URI 등록**:
```
http://localhost:5000/Home/PopupCallback
https://localhost:5001/Home/PopupCallback
```

### Option 2: Authway Login-UI Hosted Callback (권장)

**장점**: 제로 설정, 중앙 관리, 항상 최신

**Redirect URI 등록**:
```
https://login.authway.com/popup-callback
```

**클라이언트 설정**: 변경 없음, 그냥 Redirect URI만 변경

### Option 3: Authway Backend Hosted Callback

**장점**: Backend 제어, 추가 로깅 가능

**Redirect URI 등록**:
```
https://authway-api.iyulab.com/oauth/popup-callback
```

**사용법**: Option 2와 동일, URL만 다름

### 사용 방법

프론트엔드에서 JavaScript로 popup 실행:

```javascript
// Option 1: 자체 callback 사용
const authUrl = 'https://localhost:5001/Home/Login?popup=true&origin=' +
                encodeURIComponent(window.location.origin);

// Option 2: Login-UI callback 사용
const authUrl = 'https://authway-api.iyulab.com/oauth2/auth?' +
                'client_id=asp-sample-dev&' +
                'redirect_uri=https://login.authway.com/popup-callback&' +
                'response_type=code&scope=openid profile email';

// Option 3: Backend callback 사용
const authUrl = 'https://authway-api.iyulab.com/oauth2/auth?' +
                'client_id=asp-sample-dev&' +
                'redirect_uri=https://authway-api.iyulab.com/oauth/popup-callback&' +
                'response_type=code&scope=openid profile email';

// Popup 열기
const popup = window.open(authUrl, 'authway-login',
                         'width=500,height=700');

// postMessage 리스너
window.addEventListener('message', (event) => {
  if (event.data.type === 'authway-callback') {
    const { code, state } = event.data;
    // 서버로 code 전송하여 토큰 교환
  }
});
```

**자세한 가이드**: [Popup Login Integration Guide](../../docs/features/POPUP_LOGIN_INTEGRATION.md)

## 추가 리소스

- [ASP.NET Core Authentication](https://docs.microsoft.com/aspnet/core/security/authentication/)
- [OpenID Connect](https://openid.net/connect/)
- [OAuth 2.0](https://oauth.net/2/)
- [Authway Documentation](https://authway-admin.iyulab.com)

## 라이선스

MIT License
