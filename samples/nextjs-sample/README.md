# Authway Next.js Sample

Next.js 15 App Router sample application demonstrating OAuth 2.0 / OIDC authentication with Authway.

## Features

- **OAuth 2.0 + PKCE** - Secure Authorization Code Flow
- **Next.js 15** - App Router with Server Components
- **Auto Token Refresh** - Automatic token renewal
- **Popup Login** - Optional popup-based authentication
- **Minimal Setup** - Just wrap with `AuthwayProvider` and use `useAuth` hook

## Prerequisites

- Node.js 18+
- pnpm (recommended) or npm
- Authway server running locally

## Quick Start

### 1. Start Authway Server

From the project root:

```bash
.\start-dev.ps1
```

### 2. Run Setup Script

```bash
cd samples/nextjs-sample
.\setup-local.ps1
```

This script will:
- Register the OAuth client in Authway and Hydra
- Install dependencies (if needed)
- Start the development server at http://localhost:3100

### Manual Setup

If you prefer manual setup:

```bash
# Install dependencies
pnpm install

# Copy environment file
cp .env.local.example .env.local

# Start development server
pnpm dev
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NEXT_PUBLIC_AUTHWAY_DOMAIN` | `http://localhost:8081` | Auth Backend URL |
| `NEXT_PUBLIC_AUTHWAY_CLIENT_ID` | `nextjs-sample-client` | OAuth Client ID |

### Client Configuration

The sample uses the following OAuth client settings:

- **Client ID**: `nextjs-sample-client`
- **Client Type**: Public (SPA)
- **Redirect URIs**:
  - `http://localhost:3100`
  - `http://localhost:3100/callback`
- **Grant Types**: `authorization_code`, `refresh_token`
- **Scopes**: `openid`, `profile`, `email`

## Project Structure

```
nextjs-sample/
├── src/
│   ├── app/                    # Next.js App Router
│   │   ├── layout.tsx          # Root layout
│   │   ├── providers.tsx       # AuthwayProvider wrapper (only file you need!)
│   │   ├── page.tsx            # Home page
│   │   ├── globals.css         # Global styles
│   │   └── callback/
│   │       └── page.tsx        # OAuth callback (minimal - auto-handled)
│   └── components/
│       ├── Header.tsx          # Navigation header
│       ├── Footer.tsx          # Footer
│       ├── WelcomeScreen.tsx   # Login screen
│       ├── Dashboard.tsx       # User dashboard
│       └── tabs/
│           ├── ProfileTab.tsx  # User profile
│           ├── TokenTab.tsx    # Token information
│           └── ApiTestTab.tsx  # API testing
├── setup-local.ps1             # Setup script
├── package.json
└── README.md
```

## Minimal Integration Guide

### Step 1: Install Package

```bash
pnpm add @authway/react
```

### Step 2: Create Provider Wrapper

Create `src/app/providers.tsx`:

```tsx
'use client'

import { ReactNode } from 'react'
import { AuthwayProvider } from '@authway/react'
import { useRouter } from 'next/navigation'

const authConfig = {
  domain: process.env.NEXT_PUBLIC_AUTHWAY_DOMAIN || 'http://localhost:8081',
  clientId: process.env.NEXT_PUBLIC_AUTHWAY_CLIENT_ID || 'your-client-id',
  redirectUri: typeof window !== 'undefined' ? window.location.origin : '',
}

export function Providers({ children }: { children: ReactNode }) {
  const router = useRouter()

  return (
    <AuthwayProvider
      config={authConfig}
      onRedirectCallback={(appState) => {
        router.replace(appState?.returnTo || '/')
      }}
    >
      {children}
    </AuthwayProvider>
  )
}
```

### Step 3: Wrap Your App

Update `src/app/layout.tsx`:

```tsx
import { Providers } from './providers'

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  )
}
```

### Step 4: Use Auth in Components

```tsx
'use client'

import { useAuth } from '@authway/react'

export default function MyComponent() {
  const {
    isAuthenticated,
    isLoading,
    user,
    loginWithRedirect,
    loginWithPopup,
    logout,
    getAccessToken
  } = useAuth()

  if (isLoading) return <div>Loading...</div>

  if (!isAuthenticated) {
    return <button onClick={() => loginWithRedirect()}>Login</button>
  }

  return <div>Hello, {user?.name}!</div>
}
```

### Step 5: Create Callback Page (Optional)

Create `src/app/callback/page.tsx` for better UX:

```tsx
'use client'

export default function CallbackPage() {
  // AuthwayProvider handles callback automatically!
  // This page just shows a loading state.
  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh' }}>
      <p>Processing authentication...</p>
    </div>
  )
}
```

**That's it!** No custom context, no manual token handling, no callback.html file needed.

## API Reference

### `useAuth()` Hook

```tsx
const {
  // State
  isAuthenticated,    // boolean - Is user logged in?
  isLoading,          // boolean - Is auth state being determined?
  user,               // User | null - User profile data
  error,              // Error | null - Any auth error

  // Actions
  loginWithRedirect,  // (options?) => Promise<void> - Full page redirect login
  loginWithPopup,     // (options?) => Promise<void> - Popup window login
  logout,             // (options?) => void - Log out user
  getAccessToken,     // () => Promise<string> - Get current access token
} = useAuth()
```

### Login Options

```tsx
// Redirect login with return URL
loginWithRedirect({
  appState: { returnTo: '/dashboard' }
})

// Popup login (maintains app state)
await loginWithPopup({
  redirectUri: window.location.origin + '/callback'
})
```

### API Calls with Token

```tsx
const { getAccessToken } = useAuth()

async function fetchProtectedData() {
  const token = await getAccessToken()
  const response = await fetch('/api/protected', {
    headers: { 'Authorization': `Bearer ${token}` }
  })
  return response.json()
}
```

## What AuthwayProvider Handles Automatically

| Feature | Description |
|---------|-------------|
| **Token Storage** | Securely stores tokens in memory with refresh token in secure storage |
| **Token Refresh** | Automatically refreshes expired access tokens |
| **Popup Callback** | Detects popup context and closes window with postMessage |
| **Redirect Callback** | Processes authorization code and exchanges for tokens |
| **PKCE** | Generates and validates code verifier/challenge |
| **State Validation** | Prevents CSRF attacks with state parameter |

## Troubleshooting

### "Popup blocked" error
- Popup login must be triggered by user interaction (click event)
- Check browser popup blocker settings

### "Invalid redirect URI" error
- Ensure redirect URIs are registered in Authway and Hydra
- Check for trailing slashes in configuration

### Token refresh not working
- Verify `refresh_token` grant type is enabled for the client
- Check if refresh token is expired

## Related Links

- [Authway Documentation](../../docs/)
- [React SDK Sample](../react-sdk-sample/)
- [@authway/react Package](../../packages/react/)
- [@authway/client Package](../../packages/client/)

## License

MIT
