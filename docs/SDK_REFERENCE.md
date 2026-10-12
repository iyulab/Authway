# SDK Reference

Complete API reference for `@authway/client` and `@authway/react`.

## @authway/client

Core JavaScript/TypeScript SDK for OAuth 2.0 authentication.

### Installation

```bash
npm install @authway/client
```

### Popup Callback Module

For popup authentication flow, import the auto-executing callback handler in your callback page:

```typescript
// callback.html or callback page
import '@authway/client/popup-callback'
```

This module automatically:
- Detects popup/iframe context
- Extracts OAuth authorization code from URL
- Sends code to parent window via `postMessage`
- Closes popup window after completion

**Usage in HTML**:
```html
<script type="module">
  import '@authway/client/popup-callback'
</script>
```

**Usage in Next.js** (callback route):
```tsx
// app/callback/page.tsx
'use client'
import { useEffect } from 'react'

export default function CallbackPage() {
  useEffect(() => {
    import('@authway/client/popup-callback')
  }, [])

  return <div>Processing authentication...</div>
}
```

### AuthwayClient

Main client class for authentication operations.

#### Constructor

```typescript
constructor(config: AuthwayConfig)
```

**Parameters**:

```typescript
interface AuthwayConfig {
  domain: string              // Authway API URL (e.g., 'http://localhost:8080')
  clientId: string            // OAuth client ID
  issuer?: string             // OIDC issuer (default: discovered from domain)
  redirectUri?: string        // Redirect URI (default: window.location.origin)
  scope?: string              // OAuth scopes (default: 'openid profile email')
  audience?: string           // Token audience
  cacheLocation?: 'memory' | 'sessionstorage' | 'localstorage'  // Where tokens are kept (default: 'localstorage')
}
```

`cacheLocation` decides where the access, ID and refresh tokens are kept:

- `'localstorage'` (the default) — in the browser's `localStorage`: the user stays signed in across page reloads, tabs and browser restarts.
- `'sessionstorage'` — in the tab's `sessionStorage`: they survive a reload and end with the tab. A new tab starts without them.
- `'memory'` — in the client instance only. Nothing is persisted; a reload or a new tab starts without them.

A page that starts without tokens gets them back from the user's session at the provider with [`checkSession()`](#checksession). No location keeps tokens from a script running on the page; see [Where the SDK keeps tokens](FEATURES.md#where-the-sdk-keeps-tokens).

**Example**:

```typescript
import { AuthwayClient } from '@authway/client'

const client = new AuthwayClient({
  domain: 'http://localhost:8080',
  clientId: 'my-client-id'
})
```

#### Methods

##### `waitForReady()`

Wait for provider discovery: the OIDC issuer named by `domain`'s `/.well-known/authway-config` (or `domain` itself), then that issuer's OpenID Connect discovery document. Rejects with `ConfigurationError` when discovery fails.

```typescript
await client.waitForReady(): Promise<void>
```

**Must** be called before any other operations.

##### `loginWithPopup()`

Authenticate using popup window (no page reload).

```typescript
await client.loginWithPopup(options?: PopupOptions): Promise<void>
```

**Parameters**:

```typescript
interface PopupOptions {
  width?: number    // Popup width (default: 500)
  height?: number   // Popup height (default: 700)
}
```

**Example**:

```typescript
await client.loginWithPopup({ width: 600, height: 800 })
```

##### `loginWithRedirect()`

Authenticate using full-page redirect.

```typescript
await client.loginWithRedirect(): Promise<void>
```

Redirects browser to authorization endpoint, then back to `redirectUri`.

##### `handleRedirectCallback()`

Handles the provider's answer at the redirect URI — after `loginWithRedirect()` or `checkSession()`.

```typescript
await client.handleRedirectCallback(url?: string): Promise<RedirectLoginResult>  // tokens, user, appState
```

Call it when the address carries an answer — a `code`, or an `error`:

```typescript
import { LoginRequiredError } from '@authway/client'

const answer = new URLSearchParams(window.location.search)
if (answer.has('code') || answer.has('error')) {
  try {
    const { appState } = await client.handleRedirectCallback()
    // Signed in. appState is what the request carried.
  } catch (error) {
    if (error instanceof LoginRequiredError) {
      // Nobody is signed in: the answer to checkSession(). Not a failure —
      // error.appState says where the user was.
    } else {
      throw error  // the provider refused, e.g. access_denied
    }
  }
}
```

##### `checkSession()`

Gets tokens back from the user's session at the provider, for a page that has none — after a reload with `cacheLocation: 'memory'`, in a new tab with `'sessionstorage'`, or once stored tokens can no longer be refreshed.

```typescript
await client.checkSession(options?: { appState?: any; redirectUri?: string }): Promise<boolean>
```

It sends the browser to the provider and back with `prompt=none`: nothing is shown to the user. It resolves `true` when the browser is on its way — the answer then arrives at the redirect URI, where `handleRedirectCallback()` returns the tokens or throws `LoginRequiredError`. `appState` defaults to `{ returnTo }`, the address the page was at.

It resolves `false`, and the page stays, when there is nothing to look for: tokens are at hand, or nobody has signed in on this browser. "Not signed in" is asked for once — the SDK does not send the browser round again until the user signs in.

```typescript
await client.waitForReady()
if (!(await client.isAuthenticated()) && (await client.checkSession())) {
  return  // on the way to the provider; this page is about to be replaced
}
```

`@authway/react` does this for you (`checkSessionOnLoad`).

##### `isAuthenticated()`

Check if user is authenticated.

```typescript
await client.isAuthenticated(): Promise<boolean>
```

##### `getUser()`

Get user profile claims.

```typescript
await client.getUser(): Promise<User | null>
```

**Returns**:

```typescript
interface User {
  sub: string
  name?: string
  email?: string
  email_verified?: boolean
  [key: string]: any  // Additional claims
}
```

##### `getAccessToken()`

Get a valid access token.

```typescript
await client.getAccessToken(): Promise<string>
```

Returns the cached token while it is valid; otherwise refreshes it with the refresh token first. It never returns `null`: with no valid token and no refresh token it throws `LoginRequiredError`; if the token endpoint refuses the refresh, the call rejects with that request's error (`ApiError`, or `TimeoutError`).

##### `logout()`

Log out user and clear tokens.

```typescript
await client.logout(options?: LogoutOptions): Promise<void>
```

**Parameters**:

```typescript
interface LogoutOptions {
  returnTo?: string   // URL to redirect after logout
  localOnly?: boolean // Clear local tokens only (no server logout)
  federated?: boolean // Also logout from social providers
}
```

**Examples**:

```typescript
// Standard logout (redirects to client's origin by default)
client.logout()

// Logout with custom return URL
client.logout({ returnTo: 'https://myapp.com/goodbye' })

// Local-only logout (tokens cleared, no server redirect)
client.logout({ localOnly: true })

// Federated logout (also clears social provider sessions)
client.logout({ federated: true })
```

**Note**: If `returnTo` is not specified, the SDK automatically uses the configured `redirectUri` (typically `window.location.origin`), ensuring users return to the app's home page instead of an error page.

**Important**: When using React Query or similar data fetching libraries, clear the cache before calling logout to prevent errors:

```typescript
queryClient.cancelQueries()
queryClient.clear()
client.logout({ returnTo: window.location.origin })
```

---

## @authway/react

React bindings for Authway SDK.

### Installation

```bash
npm install @authway/react
```

### AuthwayProvider

Context provider component.

#### Props

```typescript
interface AuthwayProviderProps {
  config: AuthwayConfig
  children: ReactNode
  onRedirectCallback?: (appState?: any) => void  // default: put the address back to appState.returnTo
  skipRedirectCallback?: boolean                 // default: false
  checkSessionOnLoad?: boolean                   // default: true
}
```

`checkSessionOnLoad` — when a page loads without tokens on a browser a user has signed in on, the provider calls `client.checkSession()`: the browser goes to Authway and back without the user being shown anything, and the page loads signed in if their session is still there. `isLoading` stays `true` in between. If nobody is signed in, the page loads signed out — that is not an `error`.

`onRedirectCallback` is called when the browser has come back, after a login or a session check, with the `appState` of the request; after a session check that is `{ returnTo }`, the address the user was at. An application with a router passes its own and navigates there:

```tsx
<AuthwayProvider config={config} onRedirectCallback={(appState) => navigate(appState?.returnTo ?? '/')}>
```

#### Usage

```tsx
import { AuthwayProvider } from '@authway/react'

function App() {
  return (
    <AuthwayProvider
      config={{
        domain: 'http://localhost:8080',
        clientId: 'my-client-id'
      }}
    >
      <YourApp />
    </AuthwayProvider>
  )
}
```

### useAuth Hook

Access authentication state and methods.

#### Return Value

```typescript
interface AuthState {
  // State
  isAuthenticated: boolean
  isLoading: boolean
  user: User | null
  error: Error | null

  // Methods
  loginWithPopup: (options?: PopupOptions) => Promise<void>
  loginWithRedirect: () => Promise<void>
  logout: (options?: LogoutOptions) => Promise<void>
  getAccessToken: () => Promise<string>  // throws LoginRequiredError when not signed in
}
```

#### Usage

```tsx
import { useAuth } from '@authway/react'

function Dashboard() {
  const {
    isAuthenticated,
    isLoading,
    user,
    error,
    loginWithPopup,
    logout
  } = useAuth()

  if (isLoading) return <div>Loading...</div>
  if (error) return <div>Error: {error.message}</div>

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

### LoginButton Component

Pre-built login button (optional). It starts a redirect login
(`loginWithRedirect`); for a popup login, call `loginWithPopup` from `useAuth`
on a button of your own.

#### Props

```typescript
interface LoginButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  children?: ReactNode                 // default: 'Log In'
  loginOptions?: RedirectLoginOptions  // passed to loginWithRedirect
  className?: string
}
```

#### Usage

```tsx
import { LoginButton } from '@authway/react'

function App() {
  return (
    <div>
      <LoginButton>Sign In</LoginButton>
      <LoginButton loginOptions={{ appState: { returnTo: '/dashboard' } }}>
        Sign In and Return to Dashboard
      </LoginButton>
    </div>
  )
}
```

---

## Advanced Usage

### Custom Token Refresh

```typescript
// getAccessToken() refreshes an expired token itself and throws when it cannot
import { ApiError, LoginRequiredError } from '@authway/client'

let token: string
try {
  token = await client.getAccessToken()
} catch (err) {
  // LoginRequiredError: no valid token and no refresh token
  // ApiError: the token endpoint refused the refresh (e.g. the refresh token expired)
  if (!(err instanceof LoginRequiredError || err instanceof ApiError)) throw err
  token = await client.getAccessTokenWithPopup()  // sign in again
}
```

### Protected API Calls

```typescript
async function callProtectedAPI() {
  const token = await client.getAccessToken()

  const response = await fetch('https://api.example.com/protected', {
    headers: {
      'Authorization': `Bearer ${token}`
    }
  })

  return await response.json()
}
```

### Handle Authentication Errors

```tsx
function App() {
  const { error } = useAuth()

  useEffect(() => {
    if (error) {
      console.error('Authentication error:', error)
      // Show error toast, redirect to error page, etc.
    }
  }, [error])

  // ...
}
```

### SSR Considerations

For Next.js or other SSR frameworks:

```tsx
// pages/_app.tsx
import { AuthwayProvider } from '@authway/react'

function MyApp({ Component, pageProps }) {
  return (
    <AuthwayProvider
      config={{
        domain: process.env.NEXT_PUBLIC_AUTH_DOMAIN,
        clientId: process.env.NEXT_PUBLIC_CLIENT_ID
      }}
    >
      <Component {...pageProps} />
    </AuthwayProvider>
  )
}
```

---

## Type Definitions

### AuthwayConfig

```typescript
interface AuthwayConfig {
  domain: string
  clientId: string
  redirectUri?: string
  scope?: string
  audience?: string
  cacheLocation?: 'memory' | 'sessionstorage' | 'localstorage'  // default: 'localstorage'
}
```

### User

```typescript
interface User {
  sub: string
  name?: string
  email?: string
  email_verified?: boolean
  picture?: string
  [key: string]: any
}
```

### PopupOptions

```typescript
interface PopupOptions {
  width?: number
  height?: number
}
```

### LogoutOptions

```typescript
interface LogoutOptions {
  returnTo?: string
}
```

---

## Error Handling

### Common Errors

#### `ConfigurationError`

`domain` or `clientId` is missing (thrown by the constructor), or provider discovery failed (rejects `waitForReady()` and the methods that need the provider's endpoints).

**Solution**: Check `domain` and `clientId`, and that the deployment named by `domain` is reachable from the browser

#### `AuthenticationError`

Authentication failed or was cancelled.

**Solution**: Check user cancelled popup, or OAuth configuration issues

#### `LoginRequiredError`

The user is not signed in. Thrown by `getAccessToken()` and `getIdToken()` when there is no valid token and no refresh token to renew it with, and by `handleRedirectCallback()` when the provider answers `checkSession()` with `login_required`, `consent_required` or `interaction_required` (`error.appState` then carries the request's `appState`).

**Solution**: Send the user to an ordinary login when they ask for one

### Error Types

All are exported from `@authway/client`.

```typescript
class AuthwayError extends Error {
  code: string
  statusCode?: number
}

// Specific error types
class ConfigurationError extends AuthwayError        // code: 'configuration_error'
class ApiError extends AuthwayError                  // code: 'api_error'; statusCode and response of the failed request
class AuthenticationError extends AuthwayError       // code: the OAuth error, or 'authentication_error'
class LoginRequiredError extends AuthenticationError // code: 'login_required'
class TokenError extends AuthwayError                // code: 'token_error'
class MissingRefreshTokenError extends TokenError    // code: 'missing_refresh_token'
class TimeoutError extends AuthwayError              // code: 'timeout_error'
class PopupTimeoutError extends AuthwayError         // code: 'timeout'
class PopupCancelledError extends AuthwayError       // code: 'cancelled'
```

---

## Migration Guide

### From Auth0

```typescript
// Auth0
import { Auth0Provider, useAuth0 } from '@auth0/auth0-react'

// Authway
import { AuthwayProvider, useAuth } from '@authway/react'
```

**Key Differences**:
- Config discovery: Authway uses `domain` (Authway API URL) and OpenID Connect discovery, Auth0 uses `domain` + `audience`
- Methods: Similar API surface, minor naming differences

### From Firebase Auth

```typescript
// Firebase
import { signInWithPopup, signOut } from 'firebase/auth'

// Authway
const { loginWithPopup, logout } = useAuth()
```

---

## Examples

See complete examples in:
- **[React Sample](../samples/react-sdk-sample/)** - Full React integration
- **[ASP.NET SPA](../samples/asp-spa/)** - React + Vanilla JS versions

## Support

- **Issues**: [GitHub Issues](https://github.com/iyulab/authway/issues)
- **Package Source**:
  - [@authway/client](../packages/client/)
  - [@authway/react](../packages/react/)
