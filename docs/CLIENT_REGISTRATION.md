# Client Registration Guide

**Version**: 0.1.1
**Last Updated**: 2025-11-10

Complete guide for registering OAuth 2.0 clients with Authway, covering both Public Clients (SPA, Mobile) and Confidential Clients (Backend Services).

---

## Overview

Authway supports two types of OAuth 2.0 clients as defined by [RFC 6749 Section 2.1](https://datatracker.ietf.org/doc/html/rfc6749#section-2.1):

| Type | Description | Example Use Cases |
|------|-------------|-------------------|
| **Public Client** | Cannot securely store credentials | SPA, Mobile Apps, Desktop Apps |
| **Confidential Client** | Can securely store credentials | Backend Services, Server-side Apps |

### Key Differences

| Feature | Public Client | Confidential Client |
|---------|--------------|---------------------|
| **client_secret** | ❌ Not used (empty) | ✅ Required |
| **Security** | PKCE (Proof Key for Code Exchange) | Client Secret |
| **Auth Method** | `token_endpoint_auth_method: "none"` | `token_endpoint_auth_method: "client_secret_post"` |
| **Grant Types** | `authorization_code`, `refresh_token` | All grant types including `client_credentials` |

---

## Public Clients (SPA, Mobile Apps)

Public clients run in environments where the source code is accessible to end users (browsers, mobile devices). They **cannot securely store client secrets**.

### Security Model

- ✅ Uses **PKCE** (RFC 7636) to prevent authorization code interception
- ❌ No client_secret stored or transmitted
- ✅ `token_endpoint_auth_method: "none"`

### Custom Client ID

Use a custom `client_id` for branding or organizational purposes:

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
  "id": "...",
  "client_id": "my_spa_app",
  "name": "My SPA Application",
  "public": true,
  "redirect_uris": ["http://localhost:5173"],
  "grant_types": ["authorization_code", "refresh_token"],
  "scopes": ["openid", "profile", "email"],
  "active": true
}
```

**Credentials** (returned separately):
```json
{
  "client_id": "my_spa_app",
  "client_secret": ""
}
```

### Auto-Generated Client ID

Omit `client_id` for automatic generation:

```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "YOUR_TENANT_ID",
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
  "client_id": "authway_AbCd1234567890XyZ",
  ...
}
```

### Usage with Authway SDKs

#### React SDK (@authway/react)

```tsx
import { AuthwayProvider, useAuth } from '@authway/react'

function App() {
  return (
    <AuthwayProvider
      config={{
        domain: 'http://localhost:8081',
        clientId: 'my_spa_app'  // Your public client_id
      }}
    >
      <Dashboard />
    </AuthwayProvider>
  )
}

function Dashboard() {
  const { isAuthenticated, user, loginWithPopup, logout } = useAuth()

  if (!isAuthenticated) {
    return <button onClick={() => loginWithPopup()}>Login</button>
  }

  return (
    <div>
      <h1>Welcome, {user.name}!</h1>
      <button onClick={() => logout()}>Logout</button>
    </div>
  )
}
```

#### JavaScript SDK (@authway/client)

```typescript
import { AuthwayClient } from '@authway/client'

const client = new AuthwayClient({
  domain: 'http://localhost:8081',
  clientId: 'my_spa_app'  // Your public client_id
})

await client.waitForReady()
await client.loginWithPopup()

const token = await client.getAccessToken()
const user = await client.getUser()
```

---

## Confidential Clients (Backend Services)

Confidential clients run in secure environments (servers) where source code and credentials can be protected. They **must use client secrets**.

### Security Model

- ✅ Stores encrypted `client_secret`
- ✅ Uses `client_secret_post` authentication method
- ✅ Supports all OAuth 2.0 grant types

### Custom Credentials

Provide both `client_id` and `client_secret`:

```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "YOUR_TENANT_ID",
    "client_id": "my_backend_service",
    "client_secret": "very_secure_random_secret",
    "name": "My Backend Service",
    "public": false,
    "redirect_uris": ["http://localhost:8080/callback"],
    "grant_types": ["authorization_code", "client_credentials", "refresh_token"],
    "scopes": ["openid", "profile", "email", "api"]
  }'
```

**Response**:
```json
{
  "id": "...",
  "client_id": "my_backend_service",
  "name": "My Backend Service",
  "public": false,
  "redirect_uris": ["http://localhost:8080/callback"],
  "grant_types": ["authorization_code", "client_credentials", "refresh_token"],
  "scopes": ["openid", "profile", "email", "api"],
  "active": true
}
```

**Credentials** (store securely):
```json
{
  "client_id": "my_backend_service",
  "client_secret": "very_secure_random_secret"
}
```

### Auto-Generated Credentials

Omit both `client_id` and `client_secret` for automatic generation:

```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "YOUR_TENANT_ID",
    "name": "My Backend Service",
    "public": false,
    "redirect_uris": ["http://localhost:8080/callback"],
    "grant_types": ["authorization_code", "client_credentials"],
    "scopes": ["openid", "profile", "email"]
  }'
```

**Response includes auto-generated credentials**:
```json
{
  "client_id": "authway_XyZ0987654321AbCd",
  "client_secret": "generated_secure_secret_string"
}
```

⚠️ **Important**: Store the `client_secret` securely. It cannot be retrieved later.

---

## Error Cases

### ❌ Confidential Client with Partial Credentials

**Request**:
```json
{
  "client_id": "my_backend",
  "client_secret": "",
  "public": false
}
```

**Error Response** (HTTP 400):
```json
{
  "error": "confidential clients must provide both client_id and client_secret, or neither (got client_id='my_backend', client_secret='(empty)')"
}
```

**Solution**: Provide both credentials or omit both for auto-generation.

### ❌ Invalid Tenant ID

**Request**:
```json
{
  "tenant_id": "invalid-uuid",
  "name": "My App",
  "public": true
}
```

**Error Response** (HTTP 400):
```json
{
  "error": "invalid tenant_id: invalid UUID format"
}
```

---

## Best Practices

### Public Clients

1. ✅ **Use PKCE**: Always enabled by Authway SDKs
2. ✅ **No Secret Storage**: Never try to store or transmit client_secret
3. ✅ **Short-Lived Tokens**: Use short access token lifetimes
4. ✅ **Refresh Token Rotation**: Enable automatic rotation
5. ✅ **HTTPS Only**: Use secure connections in production

### Confidential Clients

1. ✅ **Secure Storage**: Store `client_secret` in environment variables or secret managers
2. ✅ **Rotate Secrets**: Periodically regenerate client secrets
3. ✅ **Network Security**: Restrict access to backend services
4. ✅ **Audit Logging**: Monitor client credential usage
5. ✅ **Least Privilege**: Request only necessary scopes

---

## API Reference

### POST /api/v1/clients

Creates a new OAuth 2.0 client.

**Request Body**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tenant_id` | string (UUID) | ✅ | Tenant identifier |
| `name` | string | ✅ | Client display name |
| `public` | boolean | ✅ | `true` for public, `false` for confidential |
| `client_id` | string | ❌ | Custom client ID (optional) |
| `client_secret` | string | ❌ | Required for confidential if `client_id` provided |
| `redirect_uris` | string[] | ✅ | OAuth redirect URIs |
| `post_logout_redirect_uris` | string[] | ❌ | Whitelisted URIs for post-logout redirection |
| `logout_redirect_policy` | string | ❌ | Validation policy: `strict` (default), `lenient`, or `disabled` |
| `default_logout_uri` | string (URL) | ❌ | Default URI for lenient policy when `post_logout_redirect_uri` is not provided |
| `allow_wildcard_logout` | boolean | ❌ | Allow wildcard patterns in `post_logout_redirect_uris` (e.g., `http://localhost:*`) |
| `grant_types` | string[] | ✅ | Allowed grant types |
| `scopes` | string[] | ✅ | Allowed scopes |
| `description` | string | ❌ | Client description |
| `website` | string (URL) | ❌ | Client website |
| `logo` | string (URL) | ❌ | Client logo URL |

**Response**: HTTP 201 Created
```json
{
  "id": "uuid",
  "client_id": "string",
  "name": "string",
  "public": boolean,
  "redirect_uris": ["string"],
  "grant_types": ["string"],
  "scopes": ["string"],
  "active": true,
  "created_at": "timestamp",
  "updated_at": "timestamp"
}
```

**Credentials** (separate response):
```json
{
  "client_id": "string",
  "client_secret": "string"  // Empty for public clients
}
```

---

## Logout Redirect Policy Configuration

**Version**: 0.1.5+

Authway provides configurable logout redirect URI validation through OpenID Connect RP-Initiated Logout with three policy levels.

### Policy Levels

| Policy | Description | Use Case |
|--------|-------------|----------|
| **Strict** (default) | `post_logout_redirect_uri` required and must be whitelisted | Production environments |
| **Lenient** | `post_logout_redirect_uri` optional, validated if provided, uses default if omitted | Development/Staging |
| **Disabled** | No validation (local development only, blocked in production) | Local development |

### Configuration Example

```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "YOUR_TENANT_ID",
    "name": "My Application",
    "public": true,
    "redirect_uris": ["http://localhost:3000/callback"],
    "post_logout_redirect_uris": [
      "http://localhost:3000/logout",
      "http://localhost:*"
    ],
    "logout_redirect_policy": "lenient",
    "default_logout_uri": "http://localhost:3000/logout",
    "allow_wildcard_logout": true,
    "grant_types": ["authorization_code", "refresh_token"],
    "scopes": ["openid", "profile", "email"]
  }'
```

### Wildcard Pattern Support

Enable `allow_wildcard_logout` to use wildcard patterns:

**Port Wildcards:**
```json
"post_logout_redirect_uris": ["http://localhost:*"]
```
Matches: `http://localhost:3000`, `http://localhost:8080`, etc.

**Subdomain Wildcards:**
```json
"post_logout_redirect_uris": ["https://*.example.com"]
```
Matches: `https://app.example.com`, `https://staging.example.com`, etc.

### Policy Behavior

**Strict Policy (Production):**
```http
# ✅ Success - URI provided and whitelisted
GET /oauth2/sessions/logout?post_logout_redirect_uri=https://example.com/logout

# ❌ Error - URI missing (required in strict mode)
GET /oauth2/sessions/logout
```

**Lenient Policy (Development):**
```http
# ✅ Success - URI provided and whitelisted
GET /oauth2/sessions/logout?post_logout_redirect_uri=http://localhost:3000/logout

# ✅ Success - URI missing, uses default_logout_uri
GET /oauth2/sessions/logout

# ❌ Error - URI not whitelisted
GET /oauth2/sessions/logout?post_logout_redirect_uri=http://malicious.com
```

**Disabled Policy (Local Only):**
```http
# ✅ Success - No validation (any URI accepted)
GET /oauth2/sessions/logout?post_logout_redirect_uri=http://any-uri.com

# Note: Automatically blocked when AUTHWAY_ENV=production
```

### Field Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `post_logout_redirect_uris` | `string[]` | `[]` | Whitelisted URIs for post-logout redirection (OIDC RP-Initiated Logout) |
| `logout_redirect_policy` | `string` | `"strict"` | Validation strictness: `strict`, `lenient`, or `disabled` |
| `default_logout_uri` | `string` | `null` | Default URI for lenient policy when `post_logout_redirect_uri` is not provided |
| `allow_wildcard_logout` | `boolean` | `false` | Allow wildcard patterns (e.g., `http://localhost:*`, `https://*.example.com`) |

### Security Recommendations

| Environment | Recommended Policy | Wildcard | Reasoning |
|-------------|-------------------|----------|-----------|
| **Production** | `strict` | No | Maximum security, explicit whitelist |
| **Staging** | `lenient` | Optional | Flexibility with validation |
| **Development** | `lenient` | Yes | Convenience with security |
| **Local** | `disabled` | N/A | Maximum convenience (blocked in prod) |

For complete documentation, see [LOGOUT_REDIRECT_POLICY.md](LOGOUT_REDIRECT_POLICY.md).

---

## Migration from Previous Versions

If you previously used dummy `client_secret` for public clients, they will continue to work. However, we recommend:

1. **New clients**: Use the updated API (no `client_secret` for public)
2. **Existing clients**: No action required (backward compatible)
3. **Optional cleanup**: You can update existing public clients to remove stored secrets

---

## Standards Compliance

This implementation follows:

- [RFC 6749 - OAuth 2.0 Authorization Framework](https://datatracker.ietf.org/doc/html/rfc6749)
  - Section 2.1: Client Types (Public vs Confidential)
- [RFC 7636 - PKCE](https://datatracker.ietf.org/doc/html/rfc7636)
  - Proof Key for Code Exchange by OAuth Public Clients
- [OAuth 2.0 for Browser-Based Apps (Draft)](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-browser-based-apps)
- [OAuth 2.0 Security Best Current Practice](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-security-topics)

---

## Examples

### Complete Examples

See working examples in:
- [React SDK Sample](../samples/react-sdk-sample/) - Full-featured React demo
- [ASP.NET SPA Sample](../samples/asp-spa/) - Backend + Frontend integration

### Quick Start Scripts

```bash
# Create public client for SPA
TENANT_ID="your-tenant-id"

curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\": \"$TENANT_ID\",
    \"client_id\": \"my_spa\",
    \"name\": \"My SPA\",
    \"public\": true,
    \"redirect_uris\": [\"http://localhost:5173\"],
    \"grant_types\": [\"authorization_code\", \"refresh_token\"],
    \"scopes\": [\"openid\", \"profile\", \"email\"]
  }"

# Create confidential client for backend
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\": \"$TENANT_ID\",
    \"client_id\": \"my_backend\",
    \"client_secret\": \"$(openssl rand -base64 32)\",
    \"name\": \"My Backend Service\",
    \"public\": false,
    \"redirect_uris\": [\"http://localhost:8080/callback\"],
    \"grant_types\": [\"authorization_code\", \"client_credentials\"],
    \"scopes\": [\"openid\", \"profile\", \"email\", \"api\"]
  }"
```

---

## Troubleshooting

### Issue: "must provide both client_id and client_secret"

**Cause**: Confidential client with only `client_id` or only `client_secret`
**Solution**: Provide both or omit both for auto-generation

### Issue: Client secret not working

**Cause**: Using `client_secret` with public client
**Solution**: Set `"public": true` and omit `client_secret`

### Issue: Custom client_id ignored

**Cause**: Old API behavior (pre-fix)
**Solution**: Update to latest version (0.1.1+)

---

## Support

- **Documentation**: [https://github.com/iyulab/authway/docs](../README.md)
- **Issues**: [GitHub Issues](https://github.com/iyulab/authway/issues)
- **Changelog**: [CHANGELOG.md](../CHANGELOG.md)
