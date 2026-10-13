# Authway Features Guide

Comprehensive guide to Authway's key features: i18n, Custom Claims, Popup Login, Logout Policies, and OAuth/JWT best practices.

## Table of Contents

- [Internationalization (i18n)](#internationalization-i18n)
- [Custom Claims](#custom-claims)
- [Popup Login](#popup-login)
- [Who can have an account](#who-can-have-an-account)
- [Signing in with a provider](#signing-in-with-a-provider)
- [Logout & Redirect URIs](#logout--redirect-uris)
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

**From the application** — per sign-in, with the standard `ui_locales`
parameter (the first language the screen has is used; an address with
`?lang=` wins, and the user can still switch on the screen):
```typescript
await client.loginWithRedirect({ ui_locales: 'ko en' })
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

## Custom Claims

An application's backend stores claims for a user — a plan, a role, an
organization — and Authway carries them in the user's tokens (the ID token at
the top level). Only a trusted party writes them: the admin API, or a tenant's
service credential with the `admin.claims:write` scope. The user, and a
browser app holding the user's token, cannot — a relying party may decide
what the user is allowed to do from these claims.

```http
PATCH /api/v1/users/{id}/claims
Authorization: Bearer <admin key, or a service credential's access token>

{ "claims": { "plan": "pro", "trial": null } }
```

A value sets the claim, `null` removes it. Claims Authway asserts
(`tenant_id`, `email`, `name`, `email_verified`) and those OAuth / OpenID
Connect define cannot be set. The SDK reads the claims from the ID token
(`client.getClaims()`, `useClaims()`).

Full reference: [api/claims.md](api/claims.md).

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
4. **Storage Fallback**: If `window.opener` cannot be reached (COOP blocking), the popup leaves its answer in `localStorage` and the opening page, which is polling for it, picks it up
5. **Token Exchange**: Completes OAuth flow and retrieves tokens

**Technical Flow**:
```
App → Popup (Google OAuth) → Consent → Redirect → postMessage → App (authenticated)
                                              ↓ (if COOP blocks)
                                        localStorage → App (polling)
```

### Configuration

**OAuth Client Setup**:
```json
{
  "redirect_uris": [
    "http://localhost:3000",           // Main app
    "http://localhost:3000/callback"   // Popup callback page in your app (imports @authway/client/popup-callback)
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
- Solution: Handled automatically — the popup hands its answer over through `localStorage` instead

**Issue: Popup closes immediately**
- Check redirect URI matches exactly
- Verify CORS configuration

---

## Who can have an account

There is no self-registration with a password. An account comes into being in
one of three ways, and each of them proves the address it is created for — so
an account starts with `email_verified: true`:

| How | What proves the address |
|---|---|
| An administrator's invitation, accepted from the mailed link | the invitation reached it |
| A first sign-in with an emailed link | the link reached it |
| A first sign-in with a provider | the provider says it has verified it (see below) |

The last two create an account only where the tenant lets them. Three tenant
settings (`settings` on `POST` / `PUT /api/v1/tenants`) decide:

| Setting | What it does |
|---|---|
| `signup_mode` | `open` lets a first sign-in create the account. Anything else — the default — is invitation only: the address has to hold a pending invitation. |
| `allowed_domains` | When it names domains, accounts can be created only for addresses in them: an invitation to another address is refused (`email_domain_not_allowed`) and a first sign-in from one creates no account, open signup or not. A domain matches exactly; a subdomain is a different domain. Existing accounts are not affected. |
| `password_min_length` | The least length of a password, when the tenant asks for more than the 8 characters every tenant gets (up to 72). A shorter password is refused with `password_too_short` and the length in force in `min_length`, wherever a password is set. |

A password reset is also done from a mailed link, so it marks the account's
address verified if it was not.

## Signing in with a provider

A user can sign in with an account at Google, GitHub, Microsoft or Apple when
the deployment (or the client) has that provider's credentials and the client
lists the provider in `enabled_auth_providers`. The application does nothing
different: it runs the same authorization code flow and receives the same
tokens, with `amr: ["fed"]` in the ID token.

### Which account a provider sign-in reaches

1. **An account already linked to that provider account** is the one signed in
   to. The link is the provider's own identifier for the account, so a later
   change of address at the provider neither loses the Authway account nor
   reaches a different one.
2. **Otherwise the email address decides, once — and only an address the
   provider has verified.** An existing account in the tenant with that address
   is linked and its address is marked verified (`email_verified: true`).
3. **With no such account**, one is created if the tenant admits the address
   (it was invited, or the tenant has open signup), linked, and marked verified.

A sign-in is refused, and the application receives `error=access_denied` on its
redirect URI, when:

| `error_description` | Why |
|---|---|
| `no account for this address; ask an administrator for an invitation` | The tenant is invitation-only and nobody invited the address. |
| `the sign-in provider has not verified this email address` | The provider did not state that the person controls the address. Without that, whoever controls the provider-side account could sign in as the owner of the address. |
| `this email address already signs in with a different account at this provider` | The account with that address is linked to another account at the same provider. |

### What counts as verified

| Provider | The address is verified when |
|---|---|
| Google | the user info says `verified_email: true` |
| GitHub | it is one of the account's verified addresses (the primary one is used); the address shown on the profile is not used |
| Apple | the ID token says `email_verified` |
| Microsoft | the ID token carries the optional claim `xms_edov: true` — add it to the app registration's token configuration. The mailbox address and sign-in name of a directory account are set by that directory's administrator and are never taken as verified. |

## Logout & Redirect URIs

Control where users are redirected after logout.

> **Where it is enforced**: on RP-initiated logout the authorization server checks the requested
> `post_logout_redirect_uri` against the client's `post_logout_redirect_uris` (exact match; Authway
> syncs the list into the Hydra client on create/update) and alone decides where the browser goes
> once the logout completes.

### Overview

Authway supports:
- ✅ **Single Logout**: Logout from all sessions
- ✅ **Registered Redirects**: post-logout addresses are whitelisted per client
- ✅ **Session Cleanup**: Clear all tokens and sessions
- ✅ **Smart Defaults**: Auto-populate logout URIs from redirect URIs

### Smart Defaults (Zero Configuration)

Authway minimizes configuration by automatically setting logout URIs:

| Field | Smart Default | When |
|-------|---------------|------|
| `post_logout_redirect_uris` | Copies from `redirect_uris` | Not explicitly set |

**Minimal Client Creation** (logout works out of the box):
```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Authorization: Bearer $AUTHWAY_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
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
App → end_session_endpoint?id_token_hint=…&post_logout_redirect_uri=https://app.example.com
  → (URI checked against post_logout_redirect_uris) → logout screen
  → Hydra logout (accept) → Revoke Sessions & Tokens (all clients) → Redirect to URI
```

Accepting the Hydra logout request only ends the browser's login session; by itself it does not
invalidate previously issued access/refresh tokens. The logout handler also revokes every one of
that session's login and consent grants across all clients right after Hydra accepts, which is what
actually invalidates them. This step is best-effort — a failure is logged but does not block the
redirect.

### Redirect URIs

#### Registered addresses

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

There is no per-client policy beyond the list: an address that is not registered
is refused before the logout starts, and a logout started without one ends on the
authorization server's default page.

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

### Telling other applications about a logout

A logout ends the user's session at Authway, for every application. An
application that keeps its **own** server-side session is not in the browser
when that happens, so it registers an address and is told there
(OpenID Connect Back-Channel Logout 1.0):

```json
{ "backchannel_logout_uri": "https://app.example.com/backchannel-logout" }
```

When a session ends, Authway POSTs `logout_token=<JWT>` (form-encoded) to the
address of every application the user was signed in to on that session. It
ends however the session ends: the user signs out of any one application,
signs out everywhere (`POST /api/v1/logout`), resets their password, or
deletes their account.

**What the application does with it**

1. Validate the token like an ID token: signature against the issuer's JWKS,
   `iss` is the issuer, `aud` contains the application's `client_id`, `iat` is
   recent.
2. Check it is a logout token: `events` has the member
   `http://schemas.openid.net/event/backchannel-logout`, and there is no
   `nonce`.
3. End the session it names. The token carries **`sid`**, the same value as
   the `sid` claim of the ID tokens issued on that session — keep `sid` with
   your own session when the user signs in, and end the sessions that have it.
   There is no `sub` in the token.
4. Answer `200`. Anything the application wants to show the user happens the
   next time the browser calls it.

The address is called from the provider, not from a browser: it must be
reachable from where the provider runs, and it receives no cookies.
`GET /api/v1/capabilities` says `backchannel_logout: true` where this is
offered. Front-channel logout (an iframe per application) is not offered —
browsers no longer send the cookies it depends on.

An application without an address is not told: it stays signed in until its
own session expires or it next sends the user through authorization.

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

   // Wildcard patterns are not supported — register each address.
   ```

3. **Let the SDK Identify the Session**:
   ```javascript
   // The SDK adds client_id and id_token_hint (the ID token it holds) to the
   // logout request itself; logout() takes returnTo, or localOnly to skip the redirect
   await client.logout({ returnTo: 'https://app.example.com/logged-out' })
   ```

---

## OAuth & JWT Best Practices

### Token Management

#### Where the SDK keeps tokens

`@authway/client` keeps the access, ID and refresh tokens together, in the
place its `cacheLocation` option names:

| `cacheLocation` | Where | On a page reload | In a new tab |
|-----------------|-------|------------------|--------------|
| `'sessionstorage'` (default) | The tab's `sessionStorage` | The user stays signed in | No tokens — see below |
| `'localstorage'` | The browser's `localStorage`, until logout | The user stays signed in | Signed in |
| `'memory'` | The client instance only — nothing is persisted | No tokens — see below | No tokens — see below |

In either storage the keys start with `authway.cache.<clientId>.` — two
applications on one origin keep their tokens apart.

A page without tokens does not mean a user who has to sign in again. Signing
in opens a session at Authway ([Sessions](api/login-flows.md#sessions)), and
`client.checkSession()` asks for it: the browser goes to the provider and
straight back (`prompt=none` — nothing is shown, no page of the login UI is
loaded), and arrives with tokens if the session is still there. The React
provider does this on load by itself. It costs a round trip on each such page
load, and it holds only as long as the session does — 60 minutes by default,
30 days when the user asked to stay signed in.

```typescript
const client = new AuthwayClient({
  domain: 'https://auth.example.com',
  clientId: 'my-client-id',
  cacheLocation: 'memory'
})
```

Whatever is in `localStorage` or `sessionStorage` can be read by any script
running on the page, so a cross-site scripting flaw exposes the tokens — the
refresh token included. Keeping them in memory narrows that to the running
page, and `sessionStorage` to the tab; neither is a defence against such a
flaw, because a script that cannot read a token can still ask the provider
for a new one ([RFC 10017](https://www.rfc-editor.org/rfc/rfc10017) §5.1.3).
What the choice decides is how long, and how widely, tokens lie around. Keep
untrusted scripts off the page and token lifetimes short whatever you choose;
an application that handles sensitive data keeps tokens on its server instead.

#### Access Token

**Purpose**: API authorization
**Lifetime**: Short (15-60 minutes recommended)
**Storage**: Kept by the SDK (see above)

```typescript
// ✅ Good - Ask the SDK each time; it returns a valid token, refreshing if needed
const accessToken = await client.getAccessToken()

// ❌ Bad - A copy of your own, which the SDK neither refreshes nor clears on logout
localStorage.setItem('access_token', token)
```

#### Refresh Token

**Purpose**: Obtain new access tokens
**Lifetime**: Long (days to months)
**Storage**: Kept by the SDK with the other tokens (see above). A server-side application keeps it on the server; a mobile app in the platform's secure storage

A refresh token is issued when the request asks for the scope
`offline_access` and the client is registered for the `refresh_token` grant.
It is used once: exchanging it returns a new one, and the old one is no longer
accepted. Renewing is not signing in again — the new ID token carries the same
`auth_time` and `amr`. Signing out everywhere ends it.

```typescript
// ✅ Good - Ask for one, and let the SDK renew with it
const client = new AuthwayClient({ ...config, useRefreshTokens: true })
const token = await client.getAccessToken() // Renews first if the token has run out

// ❌ Bad - Manual refresh handling
const refreshToken = localStorage.getItem('refresh_token')
```

#### ID Token

**Purpose**: User authentication proof
**Lifetime**: Short (same as access token)
**Storage**: Kept by the SDK with the other tokens (see above)

The SDK reads the user from the ID token it received from the token endpoint,
over TLS, in exchange for the authorization code — it decodes the token, it
does not verify its signature again. That is what a browser application can
rely on for *display*. A decision that matters is never taken from what the
browser says: the API validates the access token (signature, issuer,
audience) on every call.

```typescript
// ✅ Good - Who is signed in, for the page to show
const user = await client.getUser()

// ❌ Bad - An ID token sent to your API as proof of who is calling
fetch('/api/orders', { headers: { 'X-User': JSON.stringify(user) } })
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

- ✅ Tokens out of storage altogether: `cacheLocation: 'memory'`, with `checkSession()` (the React provider calls it) bringing the user back after a reload
- ✅ Tokens that end with the tab, and no round trip on a reload: the SDK default (`'sessionstorage'`)
- ✅ Signed in across tabs and browser restarts without a round trip: `cacheLocation: 'localstorage'` — the tokens then stay on the device until logout
- ✅ Whichever it is: a strict Content Security Policy and short token lifetimes — no location protects tokens from a script running in the page
- ✅ Server-side app: tokens stay on the server, behind an HttpOnly session cookie (`Secure`, `SameSite`)
- ❌ Don't keep your own copies of tokens outside the SDK

#### Network

- ✅ HTTPS only in production
- ✅ Validate CORS configuration
- ✅ Use secure cookies (`Secure`, `HttpOnly`, `SameSite`)
- ✅ Implement rate limiting

### Common Vulnerabilities

#### XSS (Cross-Site Scripting)

**Risk**: An injected script reads the tokens the page holds, or asks the provider for new ones

**Prevention**:
```typescript
// ✅ Good - Leave no tokens behind in storage; checkSession() restores them after a reload
const client = new AuthwayClient({ ...config, cacheLocation: 'memory' })

// ❌ Bad - An extra copy of the token outside the SDK
localStorage.setItem('token', token)
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
| **Custom Claims** | Claims a backend stores for a user, carried in their tokens | [Claims API](./api/claims.md) |
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
