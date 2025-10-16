# Authway ASP.NET Core Sample

ASP.NET Core MVC 애플리케이션으로 Authway OAuth2/OpenID Connect 인증을 구현한 샘플입니다.

## 🎯 지원 환경

- ✅ **로컬 개발**: 로컬 Hydra 사용 (docker-compose)
- ✅ **Azure 연동**: Azure 배포된 Hydra 사용 (현재 설정)

## 아키텍처

Authway는 **Ory Hydra**를 OAuth2/OIDC 서버로 사용합니다:

```
ASP.NET App  →  Ory Hydra (OAuth2/OIDC)  →  Authway (Login Provider)
```

- **Hydra Public URL**: OAuth2/OIDC 엔드포인트 (`http://localhost:4444`)
- **Authway Backend**: 로그인/동의 처리 (`http://localhost:8080`)
- **Authway Login UI**: 로그인 화면 (`http://localhost:3000`)

## 기능

- ✅ OAuth2/OpenID Connect 인증 (via Ory Hydra)
- ✅ PKCE (Proof Key for Code Exchange) 지원
- ✅ 로그인/로그아웃
- ✅ 사용자 프로필 조회
- ✅ Claims 및 토큰 확인

## 사전 요구사항

### 소프트웨어
- [.NET 8.0 SDK](https://dotnet.microsoft.com/download/dotnet/8.0) 이상
- [Docker Desktop](https://www.docker.com/products/docker-desktop) (Hydra 실행용)

### Authway 로컬 환경 실행
Authway 전체 스택을 로컬에서 실행해야 합니다:

1. **Ory Hydra** - OAuth2/OIDC 서버 (`http://localhost:4444`)
2. **Authway Backend** - Login Provider (`http://localhost:8080`)
3. **Authway Login UI** - 로그인 화면 (`http://localhost:3000`)
4. **PostgreSQL** - 데이터베이스
5. **Redis** - 세션 저장소

```bash
# Authway 로컬 환경 시작
cd D:\data\Authway
docker-compose up -d  # 또는 개발 스크립트 실행
```

## Hydra 클라이언트 등록

### 방법 1: Hydra CLI 사용 (권장)

```bash
# Hydra CLI로 클라이언트 생성
docker exec -it hydra hydra create client \
  --endpoint http://localhost:4445 \
  --id asp-sample-dev \
  --secret dev-secret-change-in-production \
  --grant-types authorization_code,refresh_token \
  --response-types code \
  --scope openid,profile,email \
  --callbacks http://localhost:5000/signin-oidc,https://localhost:5001/signin-oidc \
  --post-logout-callbacks http://localhost:5000/signout-callback-oidc,https://localhost:5001/signout-callback-oidc
```

### 방법 2: Authway Admin Dashboard 사용

1. Authway Admin Dashboard (`http://localhost:3001`)에서 OAuth 클라이언트 생성:
   ```
   Client ID: asp-sample-dev
   Client Type: confidential
   Redirect URIs:
     - https://localhost:5001/signin-oidc
     - http://localhost:5000/signin-oidc
   Post Logout Redirect URIs:
     - https://localhost:5001/signout-callback-oidc
     - http://localhost:5000/signout-callback-oidc
   Grant Types: authorization_code, refresh_token
   Scopes: openid, profile, email
   ```

2. Client Secret을 복사하여 `appsettings.Development.json`에 설정

## 설정

### Azure 배포 환경 (현재 기본 설정)

`appsettings.Development.json`:
```json
{
  "Authway": {
    "HydraPublicUrl": "https://authway-hydra.lemonfield-03f5fb88.koreacentral.azurecontainerapps.io",
    "ClientId": "asp-sample-azure",
    "ClientSecret": "azure-secret-change-in-production"
  }
}
```

**Azure 리소스**:
- Hydra Public API: https://authway-hydra.lemonfield-03f5fb88.koreacentral.azurecontainerapps.io
- Authway Backend: https://authway-api.iyulab.com
- Authway Login UI: https://auth.iyulab.com

### 로컬 개발 환경

로컬 Hydra를 사용하려면 `appsettings.Development.json`을 다음과 같이 변경:
```json
{
  "Authway": {
    "HydraPublicUrl": "http://localhost:4444",
    "ClientId": "asp-sample-dev",
    "ClientSecret": "dev-secret-change-in-production"
  }
}
```

그리고 `Program.cs`에서 `RequireHttpsMetadata = false`로 변경

### 환경 변수로 설정 (선택사항)

```powershell
# PowerShell
$env:Authway__HydraPublicUrl = "http://localhost:4444"
$env:Authway__ClientId = "asp-sample-dev"
$env:Authway__ClientSecret = "your-client-secret"

# Bash
export Authway__HydraPublicUrl="http://localhost:4444"
export Authway__ClientId="asp-sample-dev"
export Authway__ClientSecret="your-client-secret"
```

### Hydra와 Authway 실행 확인

```bash
# Hydra Public (OAuth2/OIDC 서버)
curl http://localhost:4444/.well-known/openid-configuration

# Authway Backend (Login Provider)
curl http://localhost:8080/health
```

## 실행

### 개발 모드

```bash
cd samples/asp-sample
dotnet restore
dotnet run
```

브라우저에서 `https://localhost:5001` 또는 `http://localhost:5000` 접속

### Visual Studio

1. `asp-sample.csproj` 파일을 Visual Studio에서 열기
2. F5 키를 눌러 디버깅 시작

### VS Code

1. VS Code에서 `samples/asp-sample` 폴더 열기
2. F5 키를 눌러 디버깅 시작

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
builder.Services.AddAuthentication(options =>
{
    options.DefaultScheme = CookieAuthenticationDefaults.AuthenticationScheme;
    options.DefaultChallengeScheme = OpenIdConnectDefaults.AuthenticationScheme;
})
.AddCookie()
.AddOpenIdConnect(options =>
{
    // Ory Hydra Public URL (OAuth2/OIDC 서버)
    options.Authority = "http://localhost:4444";
    options.ClientId = "asp-sample-dev";
    options.ClientSecret = "your-client-secret";
    options.ResponseType = OpenIdConnectResponseType.Code;
    options.UsePkce = true;
    // ...
});
```

**중요**: `Authority`는 **Hydra Public URL**입니다. Authway Backend URL이 아닙니다.

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

    var properties = new AuthenticationProperties
    {
        RedirectUri = Url.Action("Index", "Home")
    };

    return SignOut(properties, OpenIdConnectDefaults.AuthenticationScheme);
}
```

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

### Hydra 연결 실패
```
IDX20803: Unable to obtain configuration from: 'http://localhost:4444/.well-known/openid-configuration'
```
**해결**:
- Ory Hydra가 실행 중인지 확인
- `docker ps`로 Hydra 컨테이너 상태 확인
- Hydra Public URL이 올바른지 확인 (`http://localhost:4444`)

### 401 Unauthorized 에러
- Client ID와 Client Secret이 올바른지 확인
- Redirect URI가 Hydra에 등록되어 있는지 확인
- Hydra Admin API에서 클라이언트 확인:
  ```bash
  curl http://localhost:4445/clients/asp-sample-dev
  ```

### Redirect URI mismatch
- Hydra 클라이언트 설정에서 등록한 URI와 실제 콜백 URI가 일치하는지 확인
- 프로토콜(http/https), 포트번호까지 정확히 일치해야 함

### Token 저장 안됨
- `options.SaveTokens = true` 설정 확인

### HTTPS 인증서 에러 (개발 환경)
```bash
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

## 추가 리소스

- [ASP.NET Core Authentication](https://docs.microsoft.com/aspnet/core/security/authentication/)
- [OpenID Connect](https://openid.net/connect/)
- [OAuth 2.0](https://oauth.net/2/)
- [Authway Documentation](https://authway-admin.iyulab.com)

## 라이선스

MIT License
