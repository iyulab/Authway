# SDK Reference

Complete API reference for `@authway/client` and `@authway/react`.

## @authway/client

Core JavaScript/TypeScript SDK for OAuth 2.0 authentication.

### Installation

```bash
npm install @authway/client
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
  domain: string              // Auth Backend URL (e.g., 'http://localhost:8081')
  clientId: string            // OAuth client ID
  redirectUri?: string        // Redirect URI (default: window.location.origin)
  scope?: string              // OAuth scopes (default: 'openid profile email')
  audience?: string           // Token audience
  popupRedirectUri?: string   // Popup callback URI
}
```

**Example**:

```typescript
import { AuthwayClient } from '@authway/client'

const client = new AuthwayClient({
  domain: 'http://localhost:8081',
  clientId: 'my-client-id'
})
```

#### Methods

##### `waitForReady()`

Wait for config to load from Auth Backend.

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

Process OAuth callback after redirect login.

```typescript
await client.handleRedirectCallback(): Promise<void>
```

Call this on your callback page:

```typescript
// callback.html or App.tsx
if (window.location.search.includes('code=')) {
  await client.handleRedirectCallback()
  // User is now authenticated
}
```

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

Get current access token.

```typescript
await client.getAccessToken(): Promise<string | null>
```

Returns `null` if not authenticated or token expired.

##### `logout()`

Log out user and clear tokens.

```typescript
await client.logout(options?: LogoutOptions): Promise<void>
```

**Parameters**:

```typescript
interface LogoutOptions {
  returnTo?: string  // URL to redirect after logout
}
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
}
```

#### Usage

```tsx
import { AuthwayProvider } from '@authway/react'

function App() {
  return (
    <AuthwayProvider
      config={{
        domain: 'http://localhost:8081',
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
  getAccessToken: () => Promise<string | null>
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

Pre-built login button (optional).

#### Props

```typescript
interface LoginButtonProps {
  children?: ReactNode
  mode?: 'popup' | 'redirect'  // default: 'popup'
  className?: string
  onClick?: () => void
}
```

#### Usage

```tsx
import { LoginButton } from '@authway/react'

function App() {
  return (
    <div>
      <LoginButton mode="popup">Sign In</LoginButton>
      <LoginButton mode="redirect">Sign In with Redirect</LoginButton>
    </div>
  )
}
```

---

## Advanced Usage

### Custom Token Refresh

```typescript
// Check token expiry and refresh if needed
const token = await client.getAccessToken()
if (!token) {
  // Token expired, need re-authentication
  await client.loginWithPopup()
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
  popupRedirectUri?: string
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

#### `ConfigNotLoadedError`

SDK config not loaded before operation.

**Solution**: Call `await client.waitForReady()` first

#### `AuthenticationError`

Authentication failed or was cancelled.

**Solution**: Check user cancelled popup, or OAuth configuration issues

#### `TokenExpiredError`

Access token expired.

**Solution**: Re-authenticate user

### Error Types

```typescript
class AuthwayError extends Error {
  code: string
  details?: any
}

// Specific error types
class ConfigNotLoadedError extends AuthwayError
class AuthenticationError extends AuthwayError
class TokenExpiredError extends AuthwayError
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
- Config discovery: Authway uses `domain` (Auth Backend URL), Auth0 uses `domain` + `audience`
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
