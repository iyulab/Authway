# Backend Integration Guide

**Version**: 0.1.6
**Last Updated**: 2026-07-20

Complete guide for integrating Authway authentication into backend APIs (ASP.NET, Node.js, Go, etc.)

---

## Overview

Authway uses a **dual-endpoint architecture** where frontend and backend components connect to different endpoints:

| Component | Endpoint | Purpose |
|-----------|----------|---------|
| **Frontend** (SDK) | Auth Backend (`http://localhost:8081`) | User authentication, login flow |
| **Backend** (API) | Ory Hydra (`http://localhost:4444`) | JWT token validation — **requires `access_token_strategy: "jwt"` on the client, see below** |

```
┌─────────────┐
│  Frontend   │  @authway/react or @authway/client
│   (SPA)     │  → Auth Backend (8081) for login
└─────────────┘  → Sends Bearer token to API
      │
      ↓ Authorization: Bearer {token}
┌─────────────┐
│  Backend    │  ASP.NET, Node.js, Go, etc.
│   (API)     │  → Hydra (4444) for JWT validation
└─────────────┘
      │
      ▼ Validates token via OIDC Discovery
 ┌─────────┐
 │  Hydra  │  OAuth 2.0 Server
 └─────────┘  /.well-known/openid-configuration
```

---

## ⚠️ Prerequisite: your client must issue JWT access tokens

**Read this before wiring any resource server.** Authway issues **opaque** access
tokens by default (`ory_at_…`, two segments — not a JWT). An opaque token carries
no claims and no signature a resource server can check, so the JWKS/OIDC-discovery
setup described below **will reject every token** until the calling client is
switched to the JWT format.

Opt the client in per client — there is no need to change the format for everyone:

```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "YOUR_TENANT_ID",
    "name": "Backend Service",
    "public": false,
    "grant_types": ["client_credentials"],
    "scopes": ["api"],
    "access_token_strategy": "jwt"
  }'
```

Existing clients can be switched with `PUT /api/v1/clients/{id}`
(`"access_token_strategy": "jwt"`), or in the Admin Console under
**Access Token Format**. Sending `""` clears the setting and returns the client to
the deployment-wide default.

**The trade-off is real**: a JWT is validated offline, so it stays valid until it
expires — revoking it before then is not possible. Opt in for service-to-service
APIs that need offline validation; leave interactive clients on opaque and check
tokens via the token introspection endpoint instead.

Verify which format you got:

```bash
# 3 segments (xxx.yyy.zzz) = JWT.  Starts with ory_at_ = opaque.
echo "$ACCESS_TOKEN" | awk -F. '{print NF" segments"}'
```

### Where your custom claims land

Registered claims (`sub`, `iss`, `aud`, `exp`, `iat`, `scp`, `client_id`) are always
top-level. **Custom claims are not** — Hydra nests session claims under `ext`, so
the same shape is returned by both the JWT and the introspection endpoint:

```json
{
  "sub": "b1e2…",
  "client_id": "my_app",
  "exp": 1770000000,
  "ext": { "email": "u@example.com", "tenant_id": "662667c1…" }
}
```

This matters because most JWT middlewares surface `ext` as a single claim holding
a JSON object — they do **not** flatten it into individual claims. If your
resource server expects to read a custom claim by its bare name, list those names
in the deployment's `HYDRA_ALLOWED_TOP_LEVEL_CLAIMS` (comma separated); Hydra then
mirrors them to the top level while still keeping them under `ext`. Claim names
are your service's domain vocabulary, so they live in deployment configuration,
never in Authway's code.

Whichever route you take, decode a real token and look before writing the mapping
code — the claim shape is the contract.

---

## ASP.NET Integration

### 1. Install NuGet Packages

```bash
dotnet add package Microsoft.AspNetCore.Authentication.JwtBearer
```

### 2. Configure appsettings.json

**Development (appsettings.Development.json)**:
```json
{
  "Authway": {
    "Authority": "http://localhost:4444",
    "Audience": "api"
  },
  "Cors": {
    "AllowedOrigins": ["http://localhost:5173", "http://localhost:3000"]
  }
}
```

**Production (appsettings.Production.json)**:
```json
{
  "Authway": {
    "Authority": "https://oauth.authway.in",
    "Audience": "api"
  },
  "Cors": {
    "AllowedOrigins": ["https://yourdomain.com"]
  }
}
```

### 3. Configure Authentication (Program.cs)

```csharp
using Microsoft.AspNetCore.Authentication.JwtBearer;
using Microsoft.IdentityModel.Tokens;

var builder = WebApplication.CreateBuilder(args);

// Add Authentication
builder.Services.AddAuthentication(JwtBearerDefaults.AuthenticationScheme)
    .AddJwtBearer(options =>
    {
        // ✅ Use Hydra OAuth endpoint (NOT Auth Backend!)
        var authority = builder.Configuration["Authway:Authority"];
        if (!string.IsNullOrEmpty(authority))
        {
            options.Authority = authority;  // http://localhost:4444 or https://oauth.authway.in
            options.Audience = builder.Configuration["Authway:Audience"] ?? "api";
            options.RequireHttpsMetadata = !builder.Environment.IsDevelopment();

            options.TokenValidationParameters = new TokenValidationParameters
            {
                ValidateIssuer = true,
                ValidIssuer = authority,
                ValidateAudience = true,  // ✅ Required in production
                ValidAudience = builder.Configuration["Authway:Audience"] ?? "api",
                ValidateLifetime = true,
                ClockSkew = TimeSpan.FromMinutes(5)  // Allow 5 min clock skew
            };
        }

        // Optional: Add event handlers for debugging
        options.Events = new JwtBearerEvents
        {
            OnAuthenticationFailed = context =>
            {
                var logger = context.HttpContext.RequestServices
                    .GetRequiredService<ILogger<Program>>();
                logger.LogWarning("JWT authentication failed: {Error}",
                    context.Exception?.Message);
                return Task.CompletedTask;
            }
        };
    });

// Add Authorization
builder.Services.AddAuthorization();

// Add CORS
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
              .AllowCredentials();
    });
});

builder.Services.AddControllers();

var app = builder.Build();

// Configure middleware
app.UseCors();
app.UseAuthentication();
app.UseAuthorization();

app.MapControllers();

app.Run();
```

### 4. Create Protected Endpoints

```csharp
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;

[ApiController]
[Route("api/[controller]")]
public class UsersController : ControllerBase
{
    [HttpGet("me")]
    [Authorize]  // Requires valid JWT token
    public IActionResult GetCurrentUser()
    {
        var userId = User.FindFirst("sub")?.Value;
        var email = User.FindFirst("email")?.Value;
        var name = User.FindFirst("name")?.Value;

        return Ok(new
        {
            id = userId,
            email = email,
            name = name,
            claims = User.Claims.Select(c => new { c.Type, c.Value })
        });
    }

    [HttpGet("public")]
    public IActionResult GetPublicData()
    {
        return Ok(new { message = "This is public data" });
    }
}
```

---

## Node.js Integration

### 1. Install Dependencies

```bash
npm install express express-jwt jwks-rsa cors
```

### 2. Configure Express Server

```javascript
const express = require('express');
const { expressjwt: jwt } = require('express-jwt');
const jwksRsa = require('jwks-rsa');
const cors = require('cors');

const app = express();

// CORS configuration
app.use(cors({
  origin: process.env.FRONTEND_URL || 'http://localhost:5173',
  credentials: true
}));

// JWT validation middleware
const checkJwt = jwt({
  secret: jwksRsa.expressJwtSecret({
    cache: true,
    rateLimit: true,
    jwksRequestsPerMinute: 5,
    jwksUri: `${process.env.AUTHWAY_AUTHORITY || 'http://localhost:4444'}/.well-known/jwks.json`
  }),

  // Validate audience and issuer
  audience: process.env.AUTHWAY_AUDIENCE || 'api',
  issuer: process.env.AUTHWAY_AUTHORITY || 'http://localhost:4444',
  algorithms: ['RS256']
});

// Public endpoint
app.get('/api/public', (req, res) => {
  res.json({ message: 'This is public data' });
});

// Protected endpoint
app.get('/api/me', checkJwt, (req, res) => {
  res.json({
    id: req.auth.sub,
    email: req.auth.email,
    name: req.auth.name,
    claims: req.auth
  });
});

// Error handling
app.use((err, req, res, next) => {
  if (err.name === 'UnauthorizedError') {
    res.status(401).json({ error: 'Invalid token' });
  } else {
    next(err);
  }
});

const PORT = process.env.PORT || 8080;
app.listen(PORT, () => {
  console.log(`Server running on port ${PORT}`);
});
```

### 3. Environment Variables (.env)

```bash
# Development
AUTHWAY_AUTHORITY=http://localhost:4444
AUTHWAY_AUDIENCE=api
FRONTEND_URL=http://localhost:5173
PORT=8080

# Production
# AUTHWAY_AUTHORITY=https://oauth.authway.in
# AUTHWAY_AUDIENCE=api
# FRONTEND_URL=https://yourdomain.com
```

---

## Go Integration

### 1. Install Dependencies

```bash
go get github.com/golang-jwt/jwt/v5
go get github.com/lestrrat-go/jwx/v2/jwk
```

### 2. Create JWT Middleware

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/golang-jwt/jwt/v5"
    "github.com/lestrrat-go/jwx/v2/jwk"
)

type JWTMiddleware struct {
    jwksURL  string
    audience string
    issuer   string
}

func NewJWTMiddleware(authority, audience string) *JWTMiddleware {
    return &JWTMiddleware{
        jwksURL:  fmt.Sprintf("%s/.well-known/jwks.json", authority),
        audience: audience,
        issuer:   authority,
    }
}

func (m *JWTMiddleware) Validate(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        authHeader := r.Header.Get("Authorization")
        if authHeader == "" {
            http.Error(w, "Missing authorization header", http.StatusUnauthorized)
            return
        }

        tokenString := strings.TrimPrefix(authHeader, "Bearer ")

        // Parse token
        token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
            // Fetch JWKS
            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()

            set, err := jwk.Fetch(ctx, m.jwksURL)
            if err != nil {
                return nil, err
            }

            keyID, ok := token.Header["kid"].(string)
            if !ok {
                return nil, fmt.Errorf("missing kid in token header")
            }

            key, ok := set.LookupKeyID(keyID)
            if !ok {
                return nil, fmt.Errorf("key not found")
            }

            var rawKey interface{}
            if err := key.Raw(&rawKey); err != nil {
                return nil, err
            }

            return rawKey, nil
        })

        if err != nil || !token.Valid {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }

        // Validate claims
        claims, ok := token.Claims.(jwt.MapClaims)
        if !ok {
            http.Error(w, "Invalid claims", http.StatusUnauthorized)
            return
        }

        // Check audience
        aud, ok := claims["aud"].([]interface{})
        if !ok || len(aud) == 0 || aud[0].(string) != m.audience {
            http.Error(w, "Invalid audience", http.StatusUnauthorized)
            return
        }

        // Check issuer
        iss, ok := claims["iss"].(string)
        if !ok || iss != m.issuer {
            http.Error(w, "Invalid issuer", http.StatusUnauthorized)
            return
        }

        // Add claims to context
        ctx := context.WithValue(r.Context(), "claims", claims)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### 3. Use Middleware

```go
package main

import (
    "encoding/json"
    "net/http"
    "os"
)

func main() {
    authority := os.Getenv("AUTHWAY_AUTHORITY")
    if authority == "" {
        authority = "http://localhost:4444"
    }

    audience := os.Getenv("AUTHWAY_AUDIENCE")
    if audience == "" {
        audience = "api"
    }

    jwtMiddleware := NewJWTMiddleware(authority, audience)

    // Public endpoint
    http.HandleFunc("/api/public", func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]string{
            "message": "This is public data",
        })
    })

    // Protected endpoint
    http.Handle("/api/me", jwtMiddleware.Validate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        claims := r.Context().Value("claims").(jwt.MapClaims)

        json.NewEncoder(w).Encode(map[string]interface{}{
            "id":     claims["sub"],
            "email":  claims["email"],
            "name":   claims["name"],
            "claims": claims,
        })
    })))

    http.ListenAndServe(":8080", nil)
}
```

---

## Logout Flow Integration

### Frontend Logout Implementation

```typescript
// @authway/client
await client.logout({
  returnTo: 'https://yourdomain.com'  // Optional with lenient policy
})

// @authway/react
const { logout } = useAuth()
await logout({ returnTo: 'https://yourdomain.com' })
```

### Backend Logout Policy Configuration

Authway v0.1.5+ supports configurable logout redirect validation:

| Policy | Behavior | Recommended For |
|--------|----------|----------------|
| **Strict** | `post_logout_redirect_uri` required | Production |
| **Lenient** | `post_logout_redirect_uri` optional | Development/Staging |
| **Disabled** | No validation | Local development only |

**Example Client Configuration**:
```json
{
  "client_id": "your-client-id",
  "post_logout_redirect_uris": [
    "https://yourdomain.com",
    "https://yourdomain.com/signout-callback"
  ],
  "logout_redirect_policy": "lenient",
  "default_logout_uri": "https://yourdomain.com",
  "allow_wildcard_logout": false
}
```

**Development with Wildcards**:
```json
{
  "post_logout_redirect_uris": [
    "http://localhost:*",
    "https://*.dev.example.com"
  ],
  "logout_redirect_policy": "lenient",
  "default_logout_uri": "http://localhost:3000",
  "allow_wildcard_logout": true
}
```

### Backend Session Management

While Authway handles OAuth logout, your backend may need additional cleanup:

```csharp
// ASP.NET
app.MapPost("/api/auth/logout", async (HttpContext context) =>
{
    // Clear backend session/cookies if any
    await context.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);

    return Results.Ok(new { message = "Logged out successfully" });
});
```

```javascript
// Node.js Express
app.post('/api/auth/logout', (req, res) => {
  // Clear session if using express-session
  req.session.destroy()

  res.json({ message: 'Logged out successfully' })
})
```

---

## Common Issues & Solutions

### Issue 1: "Unable to obtain configuration from Auth Backend"

**Error**:
```
IDX20803: Unable to obtain configuration from:
'http://localhost:8081/.well-known/openid-configuration'
```

**Cause**: Using Auth Backend URL as Authority

**Solution**: Use Hydra OAuth endpoint instead

```diff
- options.Authority = "http://localhost:8081";  // ❌ Auth Backend
+ options.Authority = "http://localhost:4444";  // ✅ Hydra
```

### Issue 2: "Audience validation failed"

**Error**:
```
IDX10214: Audience validation failed. Audiences: 'xxx'.
Did not match: validationParameters.ValidAudience
```

**Cause**: Frontend not requesting `audience` in authorization flow

**Solution**: Add `audience` parameter to authorization URL

```javascript
// Frontend SDK configuration
const authUrl = new URL(authorizationEndpoint);
authUrl.searchParams.set('audience', 'api');  // ✅ Add this!
```

### Issue 3: CORS errors

**Error**:
```
Access to fetch at 'https://api.example.com' from origin 'https://app.example.com'
has been blocked by CORS policy
```

**Solution**: Configure CORS to allow frontend origin

```csharp
// ASP.NET
builder.Services.AddCors(options =>
{
    options.AddDefaultPolicy(policy =>
    {
        policy.WithOrigins("https://app.example.com")
              .AllowAnyMethod()
              .AllowAnyHeader()
              .AllowCredentials();
    });
});
```

---

## Testing

### 1. Get Access Token

Use frontend to login and get access token, or use curl:

```bash
# Login via Authway (returns authorization code)
curl "http://localhost:8081/oauth2/auth?client_id=your-client-id&response_type=code&redirect_uri=http://localhost:3000&scope=openid%20profile%20email&audience=api"

# Exchange code for token
curl -X POST "http://localhost:4444/oauth2/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&code=YOUR_CODE&redirect_uri=http://localhost:3000&client_id=your-client-id&code_verifier=YOUR_VERIFIER"
```

### 2. Test Protected Endpoint

```bash
curl -H "Authorization: Bearer YOUR_ACCESS_TOKEN" \
  http://localhost:8080/api/me
```

### 3. Verify Token Claims

```bash
# Decode JWT (use jwt.io or jq)
echo "YOUR_ACCESS_TOKEN" | cut -d '.' -f 2 | base64 -d | jq
```

Expected claims:
```json
{
  "iss": "http://localhost:4444",
  "sub": "user-id",
  "aud": ["api"],
  "exp": 1234567890,
  "iat": 1234567890,
  "email": "user@example.com",
  "name": "User Name"
}
```

---

## Security Best Practices

### 1. Always Validate Audience

```csharp
// ✅ Required in production
ValidateAudience = true,
ValidAudience = "api"
```

### 2. Use HTTPS in Production

```csharp
// ✅ Enforce HTTPS
options.RequireHttpsMetadata = !builder.Environment.IsDevelopment();
```

### 3. Configure Clock Skew

```csharp
// Allow 5 minutes clock skew between servers
ClockSkew = TimeSpan.FromMinutes(5)
```

### 4. Implement Rate Limiting

```csharp
// ASP.NET
builder.Services.AddRateLimiter(options =>
{
    options.GlobalLimiter = PartitionedRateLimiter.Create<HttpContext, string>(context =>
        RateLimitPartition.GetFixedWindowLimiter(
            partitionKey: context.User.Identity?.Name ?? context.Request.Headers.Host.ToString(),
            factory: _ => new FixedWindowRateLimiterOptions
            {
                PermitLimit = 100,
                Window = TimeSpan.FromMinutes(1)
            }));
});
```

### 5. Log Authentication Failures

```csharp
options.Events = new JwtBearerEvents
{
    OnAuthenticationFailed = context =>
    {
        var logger = context.HttpContext.RequestServices
            .GetRequiredService<ILogger<Program>>();
        logger.LogWarning("Authentication failed: {Error}", context.Exception?.Message);
        return Task.CompletedTask;
    }
};
```

---

## Production Deployment Checklist

- [ ] Use HTTPS (`RequireHttpsMetadata = true`)
- [ ] Set correct Authority URL (`https://oauth.authway.in`)
- [ ] Enable audience validation (`ValidateAudience = true`)
- [ ] Configure CORS with specific origins (not `*`)
- [ ] Implement rate limiting
- [ ] Add authentication logging
- [ ] Set appropriate clock skew (5-10 minutes max)
- [ ] Use environment variables for secrets
- [ ] Configure token expiration policies
- [ ] Test with production Authway endpoints

---

## Next Steps

- **[OAuth Best Practices](./FEATURES.md#oauth--jwt-best-practices)** - Security guidelines
- **[Client Registration](./SETUP.md#client-registration)** - Register OAuth clients
- **[SDK Reference](./SDK_REFERENCE.md)** - Frontend integration
- **[Samples](../samples/)** - Working examples

---

## Support

- **Issues**: [GitHub Issues](https://github.com/iyulab/authway/issues)
- **Documentation**: [https://github.com/iyulab/authway/docs](README.md)
