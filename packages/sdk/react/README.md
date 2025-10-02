# @authway/react

React SDK for Authway authentication. Provides easy-to-use components and hooks for integrating OAuth 2.0 / OIDC authentication into React applications.

## Features

- 🔐 **OAuth 2.0 / OIDC** - Full support for OAuth 2.0 and OpenID Connect
- 🔒 **PKCE** - Secure authorization with PKCE (Proof Key for Code Exchange)
- ⚡ **Auto Token Refresh** - Automatic access token refresh
- 🎣 **React Hooks** - Modern hooks-based API (`useAuth`)
- 🛡️ **TypeScript** - Full TypeScript support with type definitions
- 📦 **Lightweight** - Minimal dependencies
- 🎨 **Flexible** - Works with any React application

## Installation

```bash
npm install @authway/react
# or
yarn add @authway/react
# or
pnpm add @authway/react
```

## Quick Start

### 1. Wrap your app with AuthProvider

```tsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import { AuthProvider } from '@authway/react';
import App from './App';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <AuthProvider
      config={{
        authwayUrl: 'http://localhost:4444',
        clientId: 'your-client-id',
        redirectUri: 'http://localhost:3000/callback',
        scope: ['openid', 'profile', 'email'],
      }}
    >
      <App />
    </AuthProvider>
  </React.StrictMode>
);
```

### 2. Use authentication in your components

```tsx
import { useAuth } from '@authway/react';

function App() {
  const { isAuthenticated, user, login, logout, isLoading } = useAuth();

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (!isAuthenticated) {
    return (
      <div>
        <h1>Welcome to Authway</h1>
        <button onClick={login}>Login</button>
      </div>
    );
  }

  return (
    <div>
      <h1>Welcome, {user?.name}!</h1>
      <p>Email: {user?.email}</p>
      <button onClick={logout}>Logout</button>
    </div>
  );
}
```

## API Reference

### AuthProvider

The `AuthProvider` component wraps your application and provides authentication context.

#### Props

| Prop | Type | Required | Description |
|------|------|----------|-------------|
| `config` | `AuthwayConfig` | Yes | Configuration object |
| `children` | `ReactNode` | Yes | Child components |
| `onRedirectCallback` | `() => void` | No | Callback after OAuth redirect |

#### AuthwayConfig

```typescript
interface AuthwayConfig {
  authwayUrl: string;              // Authway server URL
  clientId: string;                // OAuth 2.0 client ID
  redirectUri: string;             // Redirect URI after login
  scope?: string[];                // OAuth scopes (default: ['openid', 'profile', 'email'])
  postLogoutRedirectUri?: string;  // Redirect URI after logout
  autoRefresh?: boolean;           // Auto-refresh tokens (default: true)
  refreshInterval?: number;        // Refresh interval in ms (default: 60000)
}
```

### useAuth

Hook to access authentication state and methods.

```typescript
const {
  isAuthenticated,  // boolean
  isLoading,        // boolean
  user,             // User | null
  accessToken,      // string | null
  idToken,          // string | null
  error,            // string | null
  login,            // () => void
  logout,           // () => void
  getAccessToken,   // () => string | null
  refreshToken,     // () => Promise<void>
} = useAuth();
```

#### Return Values

| Property | Type | Description |
|----------|------|-------------|
| `isAuthenticated` | `boolean` | Whether user is authenticated |
| `isLoading` | `boolean` | Whether authentication is loading |
| `user` | `User \| null` | User information from ID token |
| `accessToken` | `string \| null` | Current access token |
| `idToken` | `string \| null` | Current ID token |
| `error` | `string \| null` | Error message if auth failed |
| `login` | `() => void` | Initiate login flow |
| `logout` | `() => void` | Logout user |
| `getAccessToken` | `() => string \| null` | Get current access token |
| `refreshToken` | `() => Promise<void>` | Manually refresh token |

### withAuth

Higher-order component for protecting routes.

```typescript
const ProtectedComponent = withAuth(YourComponent, {
  LoadingComponent?: React.ComponentType;
  UnauthorizedComponent?: React.ComponentType;
  showLoading?: boolean;
  redirectToLogin?: boolean;
});
```

## Examples

### Protected Route

```tsx
import { useAuth } from '@authway/react';
import { Navigate } from 'react-router-dom';

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth();

  if (isLoading) {
    return <div>Loading...</div>;
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" />;
  }

  return <>{children}</>;
}
```

### Using withAuth HOC

```tsx
import { withAuth } from '@authway/react';

function Dashboard() {
  return <div>Protected Dashboard</div>;
}

export default withAuth(Dashboard, {
  redirectToLogin: true,
});
```

### Making Authenticated API Calls

```tsx
import { useAuth } from '@authway/react';
import axios from 'axios';

function UserProfile() {
  const { getAccessToken } = useAuth();

  const fetchUserData = async () => {
    const token = getAccessToken();

    const response = await axios.get('/api/user/profile', {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    });

    return response.data;
  };

  // Use fetchUserData with React Query, SWR, or useEffect
}
```

### Custom Loading Component

```tsx
import { AuthProvider } from '@authway/react';

function CustomLoader() {
  return (
    <div className="flex items-center justify-center h-screen">
      <div className="animate-spin rounded-full h-32 w-32 border-b-2 border-gray-900" />
    </div>
  );
}

function App() {
  return (
    <AuthProvider
      config={{ /* ... */ }}
      onRedirectCallback={() => {
        // Custom redirect logic after login
        window.location.href = '/dashboard';
      }}
    >
      <YourApp />
    </AuthProvider>
  );
}
```

## User Object

The `user` object contains information from the ID token:

```typescript
interface User {
  sub: string;                // Subject (user ID)
  name?: string;             // Full name
  email?: string;            // Email address
  email_verified?: boolean;  // Email verification status
  picture?: string;          // Profile picture URL
  preferred_username?: string;
  [key: string]: any;        // Additional claims
}
```

## Token Storage

Tokens are stored in `localStorage` with the following keys:

- `authway_access_token` - Access token
- `authway_refresh_token` - Refresh token
- `authway_id_token` - ID token

## Security Best Practices

1. **Always use HTTPS** in production
2. **Validate redirect URIs** on the server side
3. **Keep tokens secure** - never expose them in URLs or logs
4. **Use short-lived access tokens** with refresh tokens
5. **Implement proper CORS** policies
6. **Regular security updates** - keep dependencies updated

## Advanced Usage

### Manual Client Usage

```tsx
import { AuthwayClient } from '@authway/react';

const client = new AuthwayClient({
  authwayUrl: 'http://localhost:4444',
  clientId: 'your-client-id',
  redirectUri: 'http://localhost:3000/callback',
});

// Manual login
await client.login();

// Handle callback
const tokens = await client.handleCallback();

// Get user
const user = client.getUser();

// Refresh token
await client.refreshAccessToken();

// Logout
await client.logout();
```

## Troubleshooting

### "useAuth must be used within an AuthProvider"

Make sure your component is wrapped with `AuthProvider`:

```tsx
<AuthProvider config={/* ... */}>
  <YourComponent />
</AuthProvider>
```

### Tokens not persisting after page reload

Check that `localStorage` is available and not blocked by browser settings.

### CORS errors

Ensure your Authway server has proper CORS configuration for your application domain.

## License

MIT

## Support

For issues and questions, please visit:
- [GitHub Issues](https://github.com/authway/authway/issues)
- [Documentation](https://docs.authway.dev)
