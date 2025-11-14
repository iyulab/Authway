# Getting Started with Authway

Complete guide to set up and integrate Authway authentication into your application.

**Latest Version**: 0.1.4 (2025-11-10)

> 🆕 **v0.1.4 Update**: Popup login now supports external OAuth providers (Google, GitHub, etc.) with COOP compatibility. See [Popup Login Guide](./features/POPUP_LOGIN_GUIDE.md#v014-update-google-oauth--external-providers) for details.

## Prerequisites

- Node.js 18+ and pnpm 9+
- Go 1.21+ (for backend development)
- PostgreSQL 15+
- Docker (for running Hydra)

## Installation

### 1. Clone and Install

```bash
git clone https://github.com/iyulab/authway.git
cd authway
pnpm install
```

### 2. Environment Setup

```bash
cp .env.example .env
```

Edit `.env` file:

```bash
# Database
DATABASE_URL=postgresql://user:password@localhost:5432/authway

# Hydra
HYDRA_ADMIN_URL=http://localhost:4445
HYDRA_PUBLIC_URL=http://localhost:4444

# Ports
CENTRAL_API_PORT=8080
AUTH_BACKEND_PORT=8081
```

### 3. Start Services

```bash
# Terminal 1: Start Hydra
docker run -d --name hydra \
  -p 4444:4444 -p 4445:4445 \
  oryd/hydra:v2.2.0 serve all --dev

# Terminal 2: Start Central API
cd apps/central/api
go run cmd/main.go

# Terminal 3: Start Auth Backend
cd apps/branding/auth-api
go run cmd/main.go

# Terminal 4: Start sample app
cd samples/react-sdk-sample
pnpm dev
```

Access the sample app at http://localhost:9004

## SDK Integration

### React Integration

#### 1. Install SDK

```bash
npm install @authway/react
# or
pnpm add @authway/react
```

#### 2. Configure Provider

```tsx
import { AuthwayProvider } from '@authway/react'

function App() {
  return (
    <AuthwayProvider
      config={{
        domain: 'http://localhost:8081',  // Your Auth Backend URL
        clientId: 'your-client-id',
        redirectUri: window.location.origin
      }}
    >
      <YourApp />
    </AuthwayProvider>
  )
}
```

#### 3. Use Authentication

```tsx
import { useAuth } from '@authway/react'

function Dashboard() {
  const {
    isAuthenticated,
    user,
    loginWithPopup,
    loginWithRedirect,
    logout
  } = useAuth()

  if (!isAuthenticated) {
    return (
      <div>
        <button onClick={() => loginWithPopup()}>
          Login with Popup
        </button>
        <button onClick={() => loginWithRedirect()}>
          Login with Redirect
        </button>
      </div>
    )
  }

  return (
    <div>
      <h1>Welcome, {user.name}!</h1>
      <p>Email: {user.email}</p>
      <button onClick={() => logout()}>Logout</button>
    </div>
  )
}
```

### Vanilla JS/TypeScript Integration

#### 1. Install SDK

```bash
npm install @authway/client
```

#### 2. Initialize Client

```typescript
import { AuthwayClient } from '@authway/client'

const client = new AuthwayClient({
  domain: 'http://localhost:8081',
  clientId: 'your-client-id',
  redirectUri: window.location.origin
})

// Wait for config to load
await client.waitForReady()
```

#### 3. Implement Authentication

```typescript
// Login with popup
await client.loginWithPopup()

// Login with redirect
await client.loginWithRedirect()

// Check authentication
if (await client.isAuthenticated()) {
  const user = await client.getUser()
  const token = await client.getAccessToken()
  console.log('User:', user)
}

// Logout
await client.logout()
```

## Register OAuth Client

### Via Setup Script

```bash
cd samples/your-sample
./setup-client.ps1
```

### Manual Registration

Use Central API (recommended):

```bash
# Create OAuth client with logout policy
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "YOUR_TENANT_ID",
    "client_id": "your-client-id",
    "name": "Your App",
    "public": true,
    "redirect_uris": ["http://localhost:3000"],
    "post_logout_redirect_uris": ["http://localhost:3000/logout"],
    "logout_redirect_policy": "lenient",
    "default_logout_uri": "http://localhost:3000/logout",
    "allow_wildcard_logout": false,
    "grant_types": ["authorization_code", "refresh_token"],
    "scopes": ["openid", "profile", "email"]
  }'
```

### Logout Policy Configuration

**Version**: 0.1.5+

Configure logout redirect URI validation per client:

| Policy | Description | Use Case |
|--------|-------------|----------|
| **Strict** (default) | `post_logout_redirect_uri` required | Production |
| **Lenient** | `post_logout_redirect_uri` optional | Development/Staging |
| **Disabled** | No validation (local only) | Local development |

**Example - Development Setup:**
```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "YOUR_TENANT_ID",
    "name": "Dev App",
    "public": true,
    "redirect_uris": ["http://localhost:*"],
    "post_logout_redirect_uris": ["http://localhost:*"],
    "logout_redirect_policy": "lenient",
    "default_logout_uri": "http://localhost:3000/logout",
    "allow_wildcard_logout": true,
    "grant_types": ["authorization_code", "refresh_token"],
    "scopes": ["openid", "profile", "email"]
  }'
```

**Wildcard patterns** (when `allow_wildcard_logout: true`):
- `http://localhost:*` - Matches any port
- `https://*.example.com` - Matches any subdomain

For detailed configuration, see [Logout Redirect Policy Guide](LOGOUT_REDIRECT_POLICY.md).

## Protected API Calls

### Using Access Token

```typescript
const token = await client.getAccessToken()

const response = await fetch('https://api.example.com/protected', {
  headers: {
    'Authorization': `Bearer ${token}`
  }
})
```

### ASP.NET Backend Setup

**중요**: 백엔드는 **Hydra OAuth 엔드포인트**를 사용해야 합니다 (Auth Backend 아님!)

```csharp
// Program.cs
builder.Services.AddAuthentication(JwtBearerDefaults.AuthenticationScheme)
    .AddJwtBearer(options =>
    {
        // ✅ Hydra OAuth 엔드포인트 사용 (JWT 토큰 검증용)
        var authority = builder.Configuration["Authway:Authority"];
        if (!string.IsNullOrEmpty(authority))
        {
            options.Authority = authority;  // http://localhost:4444 (개발) or https://oauth.authway.in (프로덕션)
            options.Audience = builder.Configuration["Authway:Audience"] ?? "api";
            options.RequireHttpsMetadata = !builder.Environment.IsDevelopment();

            options.TokenValidationParameters = new TokenValidationParameters
            {
                ValidateIssuer = true,
                ValidIssuer = authority,
                ValidateAudience = true,  // 프로덕션 필수
                ValidAudience = builder.Configuration["Authway:Audience"] ?? "api",
                ValidateLifetime = true,
                ClockSkew = TimeSpan.FromMinutes(5)
            };
        }
    });

// Protected endpoint
app.MapGet("/api/protected", [Authorize] () =>
{
    return new { message = "This is protected" };
});
```

**appsettings.json 구성**:
```json
{
  "Authway": {
    "Authority": "http://localhost:4444",
    "Audience": "api"
  }
}
```

**프로덕션 (appsettings.Production.json)**:
```json
{
  "Authway": {
    "Authority": "https://oauth.authway.in",
    "Audience": "api"
  }
}
```

## Common Issues

### Config Not Loading

**Problem**: SDK throws "Config not loaded" error

**Solution**: Always call `await client.waitForReady()` before other operations

```typescript
const client = new AuthwayClient(config)
await client.waitForReady()  // ← Important!
await client.loginWithPopup()
```

### CORS Errors

**Problem**: Browser blocks requests from frontend to Auth Backend

**Solution**: Configure CORS in Auth Backend

```go
// auth-api/main.go
r.Use(cors.New(cors.Config{
    AllowOrigins: []string{"http://localhost:3000"},
    AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
    AllowCredentials: true,
}))
```

### Token Validation Fails

**Problem**: Backend returns 401 Unauthorized

**Solution**: Check:
1. `Authority` matches Hydra public URL (4444)
2. `Audience` claim in token matches backend expected value
3. Token not expired
4. JWKS endpoint accessible from backend

## Next Steps

- **[SDK Reference](./SDK_REFERENCE.md)** - Complete API documentation
- **[Samples](../samples/)** - Example applications
  - [React Sample](../samples/react-sdk-sample/)
  - [ASP.NET SPA Sample](../samples/asp-spa/)
- **[OAuth Best Practices](./features/OAUTH_JWT_BEST_PRACTICES.md)** - Security guidelines

## Architecture Overview

```
┌─────────────────┐
│  Frontend App   │  @authway/react or @authway/client
│   (Browser)     │  → Auth Backend (8081) for login/authentication
└────────┬────────┘
         │ Bearer {access_token}
         ▼
┌─────────────────┐
│  Backend API    │  ASP.NET, Node.js, etc.
│  (Your Server)  │  → Hydra (4444) for JWT validation
└────────┬────────┘
         │
         ▼
    ┌─────────┐
    │  Hydra  │  OAuth 2.0 Server (4444: public, 4445: admin)
    └────┬────┘
         │
         ▼
┌─────────────────┐         ┌──────────────┐
│  Central API    │◄────────┤ Auth Backend │
│    (8080)       │         │    (8081)    │
└────────┬────────┘         └──────────────┘
         │
         ▼
   ┌───────────┐
   │PostgreSQL │
   └───────────┘
```

**Key Concepts**:
- **Frontend** → **Auth Backend** (8081): 사용자 인증, 로그인 플로우, SDK 설정
- **Backend API** → **Hydra** (4444): JWT 토큰 검증, OIDC Discovery
- **Central API** (8080): 내부 전용, 직접 노출 금지
- **Auto-Discovery**: 프론트엔드는 Auth Backend URL만 필요, 나머지는 `/.well-known/authway-config`로 자동 발견

**중요**: 프론트엔드와 백엔드는 **다른 엔드포인트를 사용**합니다:
| 구성요소 | 엔드포인트 | 용도 |
|---------|----------|------|
| Frontend SDK | Auth Backend (8081) | 로그인, 토큰 획득 |
| Backend API | Hydra (4444) | JWT 검증 |

## Support

- **Issues**: [GitHub Issues](https://github.com/iyulab/authway/issues)
- **Samples**: Check `samples/` directory for complete examples
