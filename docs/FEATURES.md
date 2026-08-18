# Authway Features Guide

**Version**: 0.2.0
**Last Updated**: 2025-12-03

Comprehensive guide to Authway's key features: i18n, Dynamic Claims, Popup Login, Logout Policies, and OAuth/JWT best practices.

## Table of Contents

- [Internationalization (i18n)](#internationalization-i18n)
- [Dynamic Claims](#dynamic-claims)
- [Popup Login](#popup-login)
- [Logout & Redirect Policies](#logout--redirect-policies)
- [OAuth & JWT Best Practices](#oauth--jwt-best-practices)

---

## Internationalization (i18n)

Multi-language support for Auth UI with automatic browser detection and user-selectable language switching.

### Overview

Authway Auth UI supports:
- ✅ **Korean (ko)** - Primary language
- ✅ **English (en)** - Fallback language
- ✅ **Automatic Detection** - Browser language preference detection
- ✅ **Language Switcher** - User can change language anytime
- ✅ **Persistent Selection** - Language preference saved in localStorage

### How It Works

**Language Detection Priority**:
1. URL query parameter (`?lang=ko`)
2. localStorage (`authway-language`)
3. Browser settings (`navigator.language`)
4. Fallback: English (en)

### Customizing Language

**Via URL**:
```
https://auth.example.com/login?lang=ko
https://auth.example.com/login?lang=en
```

**Via SDK (future)**:
```typescript
const client = new AuthwayClient({
  domain: 'http://localhost:8081',
  clientId: 'my-client-id',
  locale: 'ko' // Force specific language
})
```

### Translation Files

Located in `apps/branding/auth-ui/src/i18n/locales/`:

```
locales/
├── ko/                     # Korean
│   ├── common.json         # Common UI strings
│   ├── auth.json           # Login, register, validation
│   ├── consent.json        # OAuth consent page
│   ├── password.json       # Password reset flow
│   └── errors.json         # OAuth error codes
└── en/                     # English (same structure)
```

### Adding New Languages

1. Create language directory (e.g., `locales/ja/`)
2. Copy English files as templates
3. Translate all strings
4. Update `src/i18n/index.ts` to include new language

---

## Dynamic Claims

Manage user claims at runtime without modifying tokens.

### Overview

Dynamic claims allow you to:
- ✅ Add/update/remove user claims in real-time
- ✅ Override token claims without re-authentication
- ✅ Implement role-based access control (RBAC)
- ✅ Customize user attributes per application

### How It Works

```
Token Claims (static) + Dynamic Claims (runtime) = Final User Object
```

**Example**:
```json
{
  "sub": "user123",           // From token
  "email": "user@example.com", // From token
  "role": "admin",            // Dynamic claim (overrides token)
  "department": "engineering"  // Dynamic claim (added)
}
```

### API Usage

**Set Dynamic Claims**:
```bash
POST /api/v1/users/{user_id}/claims
```
```json
{
  "role": "admin",
  "department": "engineering",
  "permissions": ["read", "write", "delete"]
}
```

**Get User with Dynamic Claims**:
```bash
GET /userinfo
Authorization: Bearer {access_token}
```
```json
{
  "sub": "user123",
  "email": "user@example.com",
  "role": "admin",           // Dynamic claim
  "department": "engineering" // Dynamic claim
}
```

### SDK Integration

**React**:
```tsx
const { user } = useAuth()

// user object includes both token and dynamic claims
console.log(user.role)       // 'admin' (from dynamic claims)
console.log(user.department) // 'engineering'
```

**Vanilla JS**:
```typescript
const user = await client.getUser()
console.log(user.role) // Dynamic claims merged automatically
```

### Use Cases

1. **Role-Based Access Control**:
   ```typescript
   if (user.role === 'admin') {
     // Show admin panel
   }
   ```

2. **Feature Flags**:
   ```json
   {
     "features": {
       "beta_feature": true,
       "new_ui": false
     }
   }
   ```

3. **Organizational Data**:
   ```json
   {
     "organization": "acme-corp",
     "department": "sales",
     "team": "enterprise"
   }
   ```

---

## Popup Login

No-redirect authentication flow supporting both local and external OAuth providers (Google, GitHub, etc.).

### Benefits

- ✅ **No Page Reload**: User stays on current page
- ✅ **Better UX**: Faster, smoother authentication flow
- ✅ **State Preservation**: Application state maintained
- ✅ **External Providers**: Google, GitHub, Microsoft, etc. (v0.1.4+)

### Quick Start

**React**:
```tsx
import { useAuth } from '@authway/react'

function LoginButton() {
  const { loginWithPopup } = useAuth()

  return (
    <button onClick={() => loginWithPopup()}>
      Login
    </button>
  )
}
```

**Vanilla JS**:
```typescript
import { AuthwayClient } from '@authway/client'

const client = new AuthwayClient({ /* config */ })
await client.loginWithPopup()
```

### How It Works (v0.1.4)

**For External OAuth Providers** (Google, GitHub, etc.):

1. **Popup Window**: Opens OAuth provider (e.g., Google)
2. **SessionStorage Persistence**: Saves OAuth state before navigation
3. **COOP Handling**: Uses `window.opener.postMessage()` for cross-origin communication
4. **Hidden Iframe Fallback**: If `window.opener` is null (COOP blocking), uses iframe pattern
5. **Token Exchange**: Completes OAuth flow and retrieves tokens

**Technical Flow**:
```
App → Popup (Google OAuth) → Consent → Redirect → postMessage → App (authenticated)
                                              ↓ (if COOP blocks)
                                        Hidden Iframe → postMessage
```

### Configuration

**OAuth Client Setup**:
```json
{
  "redirect_uris": [
    "http://localhost:3000",           // Main app
    "http://localhost:8081/callback"   // OAuth callback (for popup)
  ]
}
```

**CORS Headers** (automatically configured):
```
Access-Control-Allow-Origin: http://localhost:3000
Access-Control-Allow-Credentials: true
```

### Troubleshooting

**Issue: Popup blocked by browser**
- Solution: User must initiate action (e.g., button click)
- Don't call `loginWithPopup()` on page load

**Issue: COOP blocking `window.opener`**
- Solution: Automatically handled by hidden iframe pattern (v0.1.4+)

**Issue: Popup closes immediately**
- Check redirect URI matches exactly
- Verify CORS configuration

---

## Logout & Redirect Policies

Control where users are redirected after logout with flexible validation policies.

### Overview

Authway supports:
- ✅ **Single Logout**: Logout from all sessions
- ✅ **Redirect Policies**: Whitelist, custom validation
- ✅ **Front-Channel Logout**: Notify all apps
- ✅ **Session Cleanup**: Clear all tokens and sessions
- ✅ **Smart Defaults**: Auto-populate logout URIs from redirect URIs

### Smart Defaults (Zero Configuration)

Authway minimizes configuration by automatically setting logout URIs:

| Field | Smart Default | When |
|-------|---------------|------|
| `post_logout_redirect_uris` | Copies from `redirect_uris` | Not explicitly set |
| `logout_redirect_policy` | `"strict"` | Not explicitly set |
| `default_logout_uri` | First `redirect_uri` | Not explicitly set |
| `allow_wildcard_logout` | `false` | Not explicitly set |

**Minimal Client Creation** (logout works out of the box):
```bash
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

### Logout Flow

```
App → /logout?post_logout_redirect_uri=https://app.example.com →
  Hydra Logout → Clear Session → Redirect to URI
```

### Redirect URI Policies

#### 1. Whitelist Policy (Recommended)

**Database Configuration**:
```sql
INSERT INTO clients (client_id, post_logout_redirect_uris)
VALUES ('my_app', ARRAY[
  'https://app.example.com',
  'https://app.example.com/logged-out',
  'https://www.example.com'
]);
```

**API Configuration**:
```bash
POST /api/v1/clients
```
```json
{
  "client_id": "my_app",
  "post_logout_redirect_uris": [
    "https://app.example.com",
    "https://app.example.com/logged-out"
  ]
}
```

#### 2. Dynamic Policy (Advanced)

**Custom Validation Function**:
```go
func ValidateLogoutRedirect(uri string, clientID string) bool {
  // Custom logic
  if strings.HasPrefix(uri, "https://") && strings.HasSuffix(uri, ".example.com") {
    return true
  }
  return false
}
```

### SDK Usage

**React**:
```tsx
const { logout } = useAuth()

// Logout with redirect
logout({ returnTo: 'https://app.example.com/logged-out' })

// Logout without redirect
logout()
```

**Vanilla JS**:
```typescript
// Logout with redirect
await client.logout({ returnTo: 'https://app.example.com/logged-out' })

// Logout without redirect
await client.logout()
```

### Front-Channel Logout

Notify all applications when user logs out:

**Configuration**:
```json
{
  "frontchannel_logout_uri": "https://app.example.com/logout",
  "frontchannel_logout_session_required": true
}
```

**Implementation**:
```html
<!-- Logout endpoint that clears local state -->
<script>
  // Clear local storage, cookies, etc.
  localStorage.clear()
  sessionStorage.clear()
</script>
```

### Security Best Practices

1. **Always Use HTTPS in Production**:
   ```json
   {
     "post_logout_redirect_uris": [
       "https://app.example.com"  // ✅
     ]
   }
   ```

2. **Prefer Exact URI Matching**:
   ```javascript
   // ✅ Good - exact match, no configuration needed
   "https://app.example.com/logged-out"

   // ⚠️ Wildcard patterns are supported (opt-in via allow_wildcard_logout)
   // for cases like preview/staging subdomains, but widen what an attacker
   // can redirect to — prefer an explicit whitelist where possible.
   "https://*.example.com/logged-out"
   ```

3. **Validate Client ID**:
   ```javascript
   // Always pass id_token_hint for validation
   await client.logout({ idTokenHint: user.idToken })
   ```

---

## OAuth & JWT Best Practices

### Token Management

#### Access Token

**Purpose**: API authorization
**Lifetime**: Short (15-60 minutes recommended)
**Storage**: Memory only (never localStorage)

```typescript
// ✅ Good - Store in memory
let accessToken: string | null = null
accessToken = await client.getAccessToken()

// ❌ Bad - Don't persist
localStorage.setItem('access_token', token) // XSS vulnerability
```

#### Refresh Token

**Purpose**: Obtain new access tokens
**Lifetime**: Long (days to months)
**Storage**: HttpOnly cookie (backend) or secure storage (mobile)

```typescript
// ✅ Good - SDK handles refresh automatically
const token = await client.getAccessToken() // Auto-refreshes if needed

// ❌ Bad - Manual refresh handling
const refreshToken = localStorage.getItem('refresh_token')
```

#### ID Token

**Purpose**: User authentication proof
**Lifetime**: Short (same as access token)
**Storage**: Memory or HttpOnly cookie

```typescript
// ✅ Good - Validate on every use
const user = await client.getUser() // Validates ID token

// ❌ Bad - Trust without validation
const claims = JSON.parse(atob(idToken.split('.')[1])) // No signature verification
```

### PKCE (Proof Key for Code Exchange)

**Required for Public Clients** (SPA, Mobile)

```typescript
// ✅ Automatic in SDKs
const client = new AuthwayClient({ /* config */ })
await client.loginWithPopup() // PKCE enabled automatically

// Manual PKCE (low-level)
const codeVerifier = generateRandomString(43)
const codeChallenge = await sha256(codeVerifier)

// Authorization request
window.location.href = `${authEndpoint}?
  code_challenge=${codeChallenge}&
  code_challenge_method=S256&
  ...`

// Token exchange
fetch(tokenEndpoint, {
  method: 'POST',
  body: JSON.stringify({
    code_verifier: codeVerifier,
    ...
  })
})
```

### Security Checklist

#### Authentication

- ✅ Use PKCE for public clients
- ✅ Validate `state` parameter (CSRF protection)
- ✅ Use `nonce` for ID token validation
- ✅ Validate token signatures (JWKs)
- ✅ Check token expiration (`exp` claim)

#### Authorization

- ✅ Validate access token on every API request
- ✅ Check `aud` (audience) claim
- ✅ Verify scopes match required permissions
- ✅ Use principle of least privilege

#### Storage

- ✅ Access Token: Memory only
- ✅ Refresh Token: HttpOnly cookie (secure, sameSite)
- ✅ ID Token: Memory or HttpOnly cookie
- ❌ Never use localStorage for tokens

#### Network

- ✅ HTTPS only in production
- ✅ Validate CORS configuration
- ✅ Use secure cookies (`Secure`, `HttpOnly`, `SameSite`)
- ✅ Implement rate limiting

### Common Vulnerabilities

#### XSS (Cross-Site Scripting)

**Risk**: Attacker steals tokens from localStorage

**Prevention**:
```typescript
// ✅ Good
const token = await client.getAccessToken() // Memory only

// ❌ Bad
localStorage.setItem('token', token) // Vulnerable to XSS
```

#### CSRF (Cross-Site Request Forgery)

**Risk**: Attacker tricks user into malicious request

**Prevention**:
```typescript
// ✅ Good - SDK validates state automatically
await client.loginWithRedirect() // Generates random state

// ❌ Bad - No CSRF protection
window.location.href = authUrl // No state parameter
```

#### Token Replay

**Risk**: Attacker reuses intercepted token

**Prevention**:
- ✅ Short token lifetime (15-60 min)
- ✅ Token binding (optional)
- ✅ Rotate refresh tokens on use

### Token Validation (Backend)

**Validate Every Request**:

```javascript
// Node.js example
const jwt = require('jsonwebtoken')
const jwksClient = require('jwks-rsa')

const client = jwksClient({
  jwksUri: 'https://your-hydra/.well-known/jwks.json'
})

function getKey(header, callback) {
  client.getSigningKey(header.kid, (err, key) => {
    callback(null, key.publicKey || key.rsaPublicKey)
  })
}

// Verify token
jwt.verify(token, getKey, {
  audience: 'your-api',
  issuer: 'https://your-hydra',
  algorithms: ['RS256']
}, (err, decoded) => {
  if (err) return res.status(401).send('Invalid token')
  req.user = decoded
  next()
})
```

---

## Summary

### Quick Reference

| Feature | Use Case | Documentation |
|---------|----------|---------------|
| **Dynamic Claims** | Runtime user attributes, RBAC | [Database Schema](./DATABASE.md) |
| **Popup Login** | No-redirect auth, external OAuth | [Setup Guide](./SETUP.md) |
| **Logout Policies** | Secure redirect control | [Setup Guide](./SETUP.md) |
| **OAuth/JWT** | Security best practices | [Backend Integration](./BACKEND_INTEGRATION.md) |

### Next Steps

- **[Setup Guide](./SETUP.md)** - Get started with Authway
- **[SDK Reference](./SDK_REFERENCE.md)** - Complete API documentation
- **[Backend Integration](./BACKEND_INTEGRATION.md)** - Protect your APIs
- **[Deployment Guide](./DEPLOYMENT.md)** - Production deployment

---

**Need Help?**
- GitHub Issues: [github.com/iyulab/authway/issues](https://github.com/iyulab/authway/issues)
- Documentation: [./README.md](./README.md)
