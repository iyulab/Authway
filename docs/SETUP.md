# Authway Setup Guide

**Version**: 0.2.0
**Last Updated**: 2025-11-17

Complete guide to install, configure, and integrate Authway authentication.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Client Registration](#client-registration)
- [SDK Integration](#sdk-integration)
- [Configuration](#configuration)
- [Next Steps](#next-steps)

---

## Prerequisites

- **Node.js** 18+ and **pnpm** 9+
- **Go** 1.21+ (for backend development)
- **PostgreSQL** 15+
- **Docker** (for running Hydra)

---

## Quick Start

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
  oryd/hydra:v26.2.0 serve all --dev

# Terminal 2: Start Central API
cd apps/central/api && go run cmd/main.go

# Terminal 3: Start Auth Backend
cd apps/branding/auth-api && go run cmd/main.go

# Terminal 4: Start sample app
cd samples/react-sdk-sample && pnpm dev
```

Access sample app: http://localhost:9004

---

## Client Registration

Authway supports two OAuth 2.0 client types as defined by [RFC 6749](https://datatracker.ietf.org/doc/html/rfc6749#section-2.1):

| Type | Use Case | Security | Example |
|------|----------|----------|---------|
| **Public** | SPA, Mobile, Desktop | PKCE (no secret) | React, Vue, Mobile apps |
| **Confidential** | Backend Services | Client Secret | Node.js API, ASP.NET |

### Public Clients (SPA, Mobile)

Public clients **cannot securely store secrets** - they use PKCE instead.

**Create Public Client**:
```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "YOUR_TENANT_ID",
    "client_id": "my_spa_app",
    "name": "My SPA Application",
    "public": true,
    "redirect_uris": ["http://localhost:5173"],
    "grant_types": ["authorization_code", "refresh_token"],
    "scopes": ["openid", "profile", "email"]
  }'
```

**Response**:
```json
{
  "client_id": "my_spa_app",
  "client_secret": "",  // Empty for public clients
  "public": true,
  "grant_types": ["authorization_code", "refresh_token"]
}
```

**Key Points**:
- ✅ `token_endpoint_auth_method: "none"`
- ✅ PKCE required (automatic in SDKs)
- ❌ No client_secret stored or transmitted

### Confidential Clients (Backend Services)

Confidential clients **can securely store secrets** on the backend.

**Create Confidential Client**:
```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "YOUR_TENANT_ID",
    "name": "Backend Service",
    "public": false,
    "redirect_uris": ["https://api.example.com/callback"],
    "grant_types": ["authorization_code", "refresh_token", "client_credentials"],
    "scopes": ["openid", "profile", "email", "api"]
  }'
```

**Response**:
```json
{
  "client_id": "authway_AbCd1234567890XyZ",
  "client_secret": "secret_1234567890abcdefghijklmnopqrstuvwxyz",
  "public": false,
  "grant_types": ["authorization_code", "refresh_token", "client_credentials"]
}
```

**Key Points**:
- ✅ `token_endpoint_auth_method: "client_secret_post"`
- ✅ Supports `client_credentials` grant
- 🔒 **Store secret securely** (environment variables, vault)

### Redirect URI Configuration

**Best Practices**:

```javascript
// Development
redirect_uris: [
  "http://localhost:3000",
  "http://localhost:5173"
]

// Production
redirect_uris: [
  "https://app.example.com",
  "https://www.example.com/auth/callback"
]

// Logout URIs (optional - auto-populated from redirect_uris if omitted)
post_logout_redirect_uris: [
  "https://app.example.com/logged-out"
]
```

**Rules**:
- ✅ Exact match required (no wildcards by default)
- ✅ HTTPS required in production
- ✅ Multiple URIs supported
- ❌ No path traversal allowed

### Smart Defaults (Zero Configuration)

Authway minimizes boilerplate by auto-populating logout settings:

| Field | Explicit Value | Smart Default |
|-------|----------------|---------------|
| `post_logout_redirect_uris` | Uses provided value | Copies from `redirect_uris` |
| `logout_redirect_policy` | Uses provided value | `"strict"` (production-safe) |
| `default_logout_uri` | Uses provided value | First `redirect_uri` |
| `allow_wildcard_logout` | Uses provided value | `false` (secure default) |

**Example - Minimal Configuration**:
```bash
# Only redirect_uris required - logout URIs auto-configured
curl -X POST http://localhost:8080/api/v1/clients \
  -d '{
    "tenant_id": "...",
    "name": "My App",
    "redirect_uris": ["http://localhost:3000"],
    "public": true,
    "grant_types": ["authorization_code", "refresh_token"],
    "scopes": ["openid", "profile", "email"]
  }'

# Result: post_logout_redirect_uris = ["http://localhost:3000"] (auto)
```

**Override When Needed**:
```bash
# Explicit logout URIs for custom logout flow
curl -X POST http://localhost:8080/api/v1/clients \
  -d '{
    "tenant_id": "...",
    "name": "My App",
    "redirect_uris": ["http://localhost:3000/callback"],
    "post_logout_redirect_uris": ["http://localhost:3000/goodbye"],
    "public": true,
    "grant_types": ["authorization_code", "refresh_token"],
    "scopes": ["openid", "profile", "email"]
  }'
```

---

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
        clientId: 'my_spa_app',
        redirectUri: window.location.origin
      }}
    >
      <YourApp />
    </AuthwayProvider>
  )
}
```

#### 3. Use Auth Hooks

```tsx
import { useAuth } from '@authway/react'

function Dashboard() {
  const { isAuthenticated, user, loginWithPopup, logout } = useAuth()

  if (!isAuthenticated) {
    return <button onClick={() => loginWithPopup()}>Login</button>
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
  clientId: 'my_spa_app',
  redirectUri: window.location.origin
})

// Wait for auto-discovery
await client.waitForReady()
```

#### 3. Authentication

```typescript
// Login with popup (recommended)
await client.loginWithPopup()

// Login with redirect
await client.loginWithRedirect()

// Get user info
const user = await client.getUser()
console.log(user.name, user.email)

// Get access token
const token = await client.getAccessToken()

// Logout
await client.logout()
```

---

## Configuration

### Auth Backend Configuration

The Auth Backend (port 8081) is your app's entry point.

**Required Environment Variables**:

```bash
# Server
PORT=8081
HOST=localhost

# Central API (internal)
CENTRAL_API_URL=http://localhost:8080

# OAuth
OAUTH_CLIENT_ID=auth-backend-client
OAUTH_CLIENT_SECRET=your-secret

# Session
SESSION_SECRET=random-session-secret
COOKIE_DOMAIN=localhost
```

### Auto-Discovery Endpoint

Apps automatically discover OAuth configuration:

```bash
GET http://localhost:8081/.well-known/authway-config
```

**Response**:
```json
{
  "issuer": "http://localhost:4444",
  "authorization_endpoint": "http://localhost:4444/oauth2/auth",
  "token_endpoint": "http://localhost:4444/oauth2/token",
  "userinfo_endpoint": "http://localhost:8081/userinfo",
  "end_session_endpoint": "http://localhost:8081/logout",
  "jwks_uri": "http://localhost:4444/.well-known/jwks.json"
}
```

### CORS Configuration

For cross-origin requests, configure CORS in Central API:

```bash
# In Central API .env
ALLOWED_ORIGINS=http://localhost:3000,http://localhost:5173,https://app.example.com
```

Or use database configuration (recommended):
```sql
INSERT INTO clients (tenant_id, client_id, allowed_origins)
VALUES ('tenant-id', 'my_spa_app', ARRAY['http://localhost:3000', 'https://app.example.com']);
```

---

## Next Steps

### Development
- **[SDK Reference](./SDK_REFERENCE.md)** - Complete API documentation
- **[Backend Integration](./BACKEND_INTEGRATION.md)** - Integrate with your backend
- **[Features Guide](./FEATURES.md)** - Dynamic claims, popup login, logout policies

### Deployment
- **[Deployment Guide](./DEPLOYMENT.md)** - Azure, Docker, production setup
- **[Database Migrations](./DATABASE.md)** - Schema changes and versioning

### Samples
- **[React SDK Sample](../samples/react-sdk-sample/)** - Full-featured React demo
- **[ASP.NET Sample](../samples/asp-spa/)** - Backend + Frontend integration

---

## Troubleshooting

### Common Issues

**Issue: "Client not found"**
- Verify `client_id` is registered
- Check tenant_id matches

**Issue: "Redirect URI mismatch"**
- Ensure exact URI match in client registration
- Check protocol (http vs https)

**Issue: "CORS error"**
- Add origin to `allowed_origins` in client config
- Verify CORS headers in Central API

**Issue: "Invalid client_secret"** (Confidential clients)
- Verify secret is correct
- Check `token_endpoint_auth_method` is `"client_secret_post"`

### Getting Help

- **GitHub Issues**: [github.com/iyulab/authway/issues](https://github.com/iyulab/authway/issues)
- **Documentation**: [./README.md](./README.md)
- **Samples**: [../samples/](../samples/)
