# Authway Integration Guide

Comprehensive guide for integrating applications with Authway OAuth 2.0 server, based on real-world implementation experience.

## Table of Contents
1. [Prerequisites](#prerequisites)
2. [Quick Start](#quick-start)
3. [OAuth Flow Overview](#oauth-flow-overview)
4. [Best Practices](#best-practices)
5. [Common Pitfalls & Solutions](#common-pitfalls--solutions)
6. [Error Handling](#error-handling)
7. [Testing](#testing)
8. [Production Checklist](#production-checklist)

---

## Prerequisites

### Required Services
- **Ory Hydra**: OAuth 2.0/OIDC provider (default: `http://localhost:4444`)
- **Authway Server**: Authentication server (default: `http://localhost:8080`)
- **PostgreSQL**: Database for users and tenants

### Environment Variables
```bash
# OAuth Client Credentials
CLIENT_ID=your-client-id
CLIENT_SECRET=your-client-secret
REDIRECT_URI=http://localhost:9001/callback

# Authway/Hydra URLs
HYDRA_PUBLIC_URL=http://localhost:4444
HYDRA_ADMIN_URL=http://localhost:4445
AUTHWAY_URL=http://localhost:8080
```

---

## Quick Start

### Step 1: Register Your Application

Register your OAuth client in Authway Admin Console:

```bash
# Navigate to Admin Console
open http://localhost:3000

# Or use the API
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Content-Type: application/json" \
  -d '{
    "client_id": "my-app-client",
    "client_name": "My Application",
    "tenant_id": "your-tenant-id",
    "redirect_uris": ["http://localhost:9001/callback"],
    "grant_types": ["authorization_code", "refresh_token"],
    "response_types": ["code"],
    "scope": "openid profile email"
  }'
```

**IMPORTANT**: Every OAuth client must be registered in Authway with an associated tenant. Hydra-only client registration is not sufficient.

### Step 2: Implement OAuth Flow

#### Server-Side State Management (RECOMMENDED)

**Why**: SameSite cookie policies prevent cookies from surviving multiple cross-site redirects in OAuth flows.

```go
package main

import (
    "authway-samples/shared"
    "time"
)

var stateStore = shared.NewStateStore()

func handleLogin(w http.ResponseWriter, r *http.Request) {
    // Generate secure state
    state, err := shared.GenerateState()
    if err != nil {
        http.Error(w, "Failed to generate state", 500)
        return
    }

    // Store state server-side with metadata
    stateStore.Store(state, map[string]string{
        "client_id": "my-app-client",
        "user_ip":   r.RemoteAddr,
    })

    // Redirect to Hydra authorization endpoint
    authURL := oauthConfig.GetAuthURL(state)
    http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
    code := r.URL.Query().Get("code")
    state := r.URL.Query().Get("state")

    // Retrieve and validate state (one-time use)
    stateData, valid := stateStore.Retrieve(state)
    if !valid {
        http.Error(w, "Invalid or expired state", 400)
        return
    }

    // Exchange code for tokens
    token, err := oauthConfig.ExchangeCode(r.Context(), code)
    if err != nil {
        http.Error(w, "Failed to exchange code", 500)
        return
    }

    // Get user info
    userInfo, err := oauthConfig.GetUserInfo(r.Context(), token.AccessToken)
    // ... create session
}
```

#### Periodic Cleanup

```go
// Clean expired states every 5 minutes
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        stateStore.CleanExpired()
    }
}()
```

---

## OAuth Flow Overview

### Standard Flow (Authorization Code)

```
User → Your App → Hydra → Authway Login → Hydra → Authway Consent → Hydra → Your App
         (1)       (2)         (3)            (4)         (5)           (6)       (7)

1. User clicks "Login" in your app
2. Redirect to Hydra with client_id, redirect_uri, scope
3. Hydra redirects to Authway login page with login_challenge
4. User authenticates, Hydra receives login acceptance
5. Hydra redirects to consent page with consent_challenge
6. User approves, Hydra receives consent acceptance
7. Hydra redirects back to your app with authorization code
```

### Google OAuth Integration Flow

```
Your App → Authway Google Login → Google OAuth → Authway Callback → Hydra Login → Hydra Consent → Your App
  (1)              (2)                 (3)              (4)              (5)            (6)          (7)

1. User clicks "Login with Google"
2. Authway redirects to Google with state parameter
3. User authenticates with Google
4. Google redirects to Authway with code
5. Authway validates user and accepts Hydra login
6. Hydra proceeds to consent flow
7. Final redirect back to your app with code
```

**Key Points**:
- State is stored server-side to survive multiple redirects
- `login_challenge` is embedded in state metadata
- Authway handles tenant matching for SSO

---

## Best Practices

### 1. State Management

✅ **DO**: Use server-side state storage
```go
stateStore := shared.NewStateStore()
stateStore.Store(state, map[string]string{
    "login_challenge": challenge,
    "client_id": clientID,
})
```

❌ **DON'T**: Rely on cookies for state in OAuth flows
```go
// This breaks with SameSite=Lax in cross-site redirects
http.SetCookie(w, &http.Cookie{
    Name:     "oauth_state",
    Value:    state,
    SameSite: http.SameSiteLaxMode,
})
```

### 2. Response Handling

✅ **DO**: Use JSON responses for AJAX/fetch requests
```go
return c.JSON(fiber.Map{
    "redirect_to": hydraURL,
    "sso": true,
})
```

❌ **DON'T**: Use 302 redirects for fetch() calls
```go
// This causes CORS errors with fetch()
return c.Redirect(hydraURL, 302)
```

### 3. Error Messages

✅ **DO**: Provide actionable error messages
```go
return c.Status(500).JSON(fiber.Map{
    "error": "OAuth client not registered",
    "hint": "Register this client in Authway Admin Console",
    "solution": map[string]string{
        "step_1": "Go to http://localhost:3000",
        "step_2": "Navigate to Clients section",
        "step_3": "Register client_id: " + clientID,
    },
})
```

❌ **DON'T**: Return vague errors
```go
return c.Status(500).JSON(fiber.Map{
    "error": "Internal server error",
})
```

### 4. Session Management

✅ **DO**: Handle stale sessions gracefully
```go
if loginReq.Skip && userNotFound {
    // Revoke old sessions
    hydraClient.RevokeUserSessions(subject)

    // Reject with login_required (not error)
    resp, _ := hydraClient.RejectLoginRequest(challenge, "login_required", "Please login again")

    return c.JSON(fiber.Map{
        "redirect_to": resp.RedirectTo,
        "session_cleared": true,
    })
}
```

❌ **DON'T**: Propagate errors to OAuth client
```go
// This breaks the OAuth flow
resp, _ := hydraClient.RejectLoginRequest(challenge, "user_not_found", "...")
```

---

## Common Pitfalls & Solutions

### Issue 1: HTTP 431 (Request Header Too Large)

**Symptom**: URLs become too long with `login_challenge` parameter

**Cause**: Hydra's `login_challenge` can be very long (200+ characters), combined with other query parameters

**Solution**: Store `login_challenge` server-side, only pass short `state` parameter in URLs

```go
// Store challenge in state metadata
stateStore.Store(state, map[string]string{
    "login_challenge": loginChallenge, // Long value stored server-side
})

// Only pass short state in URL
authURL := fmt.Sprintf("%s?state=%s&client_id=%s", googleAuthURL, state, clientID)
```

### Issue 2: "Invalid state parameter" Errors

**Symptom**: State validation fails despite correct implementation

**Cause**: SameSite=Lax cookies don't survive multiple cross-site redirects

**Solution**: Use server-side state storage instead of cookies

```go
// Server-side storage (survives all redirects)
stateStore.Store(state, metadata)

// Later, in callback
stateData, valid := stateStore.Retrieve(state)
```

### Issue 3: Infinite Login Loop

**Symptom**: After login, redirected back to login page repeatedly

**Cause**: Empty or invalid ACR (Authentication Context Class Reference) values

**Solution**: Use `omitempty` JSON tags for optional fields

```go
type AcceptLoginRequest struct {
    Subject     string                 `json:"subject"`
    Remember    bool                   `json:"remember"`
    RememberFor int                    `json:"remember_for"`
    ACR         string                 `json:"acr,omitempty"`        // ← omitempty tag
    Context     map[string]interface{} `json:"context,omitempty"`   // ← omitempty tag
}
```

### Issue 4: CORS Errors During OAuth Flow

**Symptom**: `Access to fetch at 'http://localhost:4444/...' has been blocked by CORS policy`

**Cause**: Frontend using `fetch()` follows 302 redirects, causing cross-origin requests

**Solution**: Return JSON with `redirect_to`, let frontend handle redirect

```go
// Backend: Return JSON instead of 302
return c.JSON(fiber.Map{
    "redirect_to": hydraURL,
})

// Frontend: Handle redirect manually
fetch('/login', {...}).then(res => res.json()).then(data => {
    if (data.redirect_to) {
        window.location.href = data.redirect_to
    }
})
```

### Issue 5: Stale Session Errors

**Symptom**: "User not found" errors during SSO attempts

**Cause**: Hydra has session for deleted/non-existent user

**Solution**: Auto-revoke sessions and reject with `login_required`

```go
if loginReq.Skip && userNotFound {
    // Clean up old sessions
    hydraClient.RevokeUserSessions(subject)

    // Use login_required (not user_not_found) to avoid OAuth error propagation
    hydraClient.RejectLoginRequest(challenge, "login_required", "Please login again")
}
```

---

## Error Handling

### Error Response Format

Authway returns structured error responses:

```json
{
  "error": "OAuth client not registered in Authway",
  "details": "record not found",
  "hint": "Register this OAuth client in Authway Admin Console before using it",
  "solution": {
    "step_1": "Go to Admin Console: http://localhost:3000",
    "step_2": "Navigate to Clients section",
    "step_3": "Register client with client_id: my-app-client"
  },
  "client_id": "my-app-client"
}
```

### Common Errors

| Error Code | Meaning | Solution |
|------------|---------|----------|
| `missing_login_challenge` | Required parameter missing | Include `login_challenge` in URL |
| `invalid_state` | State expired or already used | Restart login flow |
| `oauth_client_not_registered` | Client not in Authway database | Register in Admin Console |
| `oauth_callback_failed` | Google OAuth configuration issue | Check credentials and redirect_uri |
| `hydra_login_failed` | Hydra API error | Verify Hydra is accessible |

---

## Testing

### Manual Testing Checklist

- [ ] Initial login flow works
- [ ] Google OAuth integration works
- [ ] Consent page displays correctly
- [ ] Token exchange successful
- [ ] User profile data retrieved
- [ ] Logout + re-login works (SSO)
- [ ] Cross-tenant isolation verified
- [ ] Session expiration handled
- [ ] Error messages are helpful

### Automated Testing

```go
func TestOAuthFlow(t *testing.T) {
    // 1. Start authorization
    state, _ := shared.GenerateState()
    stateStore.Store(state, map[string]string{"test": "true"})

    authURL := oauthConfig.GetAuthURL(state)
    // Navigate to authURL...

    // 2. Validate callback
    stateData, valid := stateStore.Retrieve(state)
    assert.True(t, valid)
    assert.Equal(t, "true", stateData.Metadata["test"])

    // 3. Exchange code
    token, err := oauthConfig.ExchangeCode(ctx, code)
    assert.NoError(t, err)

    // 4. Get user info
    userInfo, err := oauthConfig.GetUserInfo(ctx, token.AccessToken)
    assert.NoError(t, err)
    assert.NotEmpty(t, userInfo.Email)
}
```

---

## Production Checklist

### Security
- [ ] Use HTTPS for all URLs
- [ ] Set `Secure=true` for cookies
- [ ] Enable CSRF protection
- [ ] Implement rate limiting
- [ ] Rotate client secrets regularly
- [ ] Use strong state generation (32+ bytes)

### State Management
- [ ] Replace in-memory store with Redis/Memcached
- [ ] Configure appropriate TTL (15 minutes recommended)
- [ ] Implement automatic cleanup
- [ ] Monitor state store size
- [ ] Handle distributed deployments

### Monitoring
- [ ] Log all OAuth flow events
- [ ] Track error rates by type
- [ ] Monitor state expiration rates
- [ ] Alert on authentication failures
- [ ] Dashboard for OAuth metrics

### Performance
- [ ] Cache user info appropriately
- [ ] Implement connection pooling
- [ ] Set reasonable timeouts
- [ ] Use CDN for static assets
- [ ] Enable HTTP/2

### Configuration
```bash
# Production Environment Variables
ENVIRONMENT=production

# Security
SESSION_SECRET=<strong-random-secret>
COOKIE_SECURE=true
CORS_ORIGINS=https://myapp.com

# State Storage (Redis)
REDIS_URL=redis://prod-redis:6379
STATE_TTL=900  # 15 minutes

# Monitoring
LOG_LEVEL=info
METRICS_ENABLED=true
SENTRY_DSN=<your-sentry-dsn>

# OAuth
HYDRA_PUBLIC_URL=https://auth.myapp.com
HYDRA_ADMIN_URL=https://auth-admin.myapp.com
AUTHWAY_URL=https://sso.myapp.com
```

---

## Troubleshooting

### Debug Mode

Enable detailed logging in development:

```go
// Server-side
logger.SetLevel(zap.DebugLevel)

// Client-side
console.log('OAuth State:', state)
console.log('Challenge:', challenge)
```

### Useful Commands

```bash
# Check Hydra health
curl http://localhost:4444/health/ready

# Check Authway health
curl http://localhost:8080/health

# View Hydra logs
docker logs hydra -f

# View Authway logs
tail -f logs/authway.log

# Clear all sessions for a user
curl -X DELETE http://localhost:4445/admin/oauth2/auth/sessions/login?subject=user-id
```

### Support

- **Documentation**: https://docs.authway.io
- **GitHub Issues**: https://github.com/yourusername/authway/issues
- **Discord**: https://discord.gg/authway
- **Email**: support@authway.io

---

## Example Applications

Complete working examples available in `/samples`:

- **AppleService**: Full OAuth integration with Google SSO
- **BananaService**: Multi-tenant example
- **shared**: Reusable OAuth helpers

---

## Additional Resources

- [Ory Hydra Documentation](https://www.ory.sh/docs/hydra/)
- [OAuth 2.0 RFC](https://datatracker.ietf.org/doc/html/rfc6749)
- [OpenID Connect Specification](https://openid.net/specs/openid-connect-core-1_0.html)
- [Security Best Current Practice](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-security-topics)

---

**Last Updated**: October 2025
**Version**: 1.0.0
