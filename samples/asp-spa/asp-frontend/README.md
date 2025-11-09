# Authway ASP.NET SPA Sample - React Frontend

React frontend demonstrating **Popup Login** and **Dynamic Claims** using `@authway/react` SDK.

> **💡 Learning OAuth2 Internals?** Check out the [Vanilla JavaScript version](../asp-frontend-vanilla/) that uses `oauth4webapi` directly without framework dependencies. It reveals the OAuth2 PKCE mechanics that this SDK abstracts away.

## 🎯 Features

This sample showcases advanced Authway SDK features:

### 🪟 Popup Login
- **No Page Reload**: Authenticate in a popup window without losing app state
- **postMessage Communication**: Secure callback handling via `callback.html`
- **Same Security**: Uses OAuth 2.0 + PKCE (same as redirect flow)
- **Better UX**: Maintains app context during authentication

### 🎭 Dynamic Claims
- **Runtime Updates**: Modify user claims without re-authentication
- **Automatic Token Refresh**: SDK refreshes ID token with new claims
- **Instant Availability**: Updated claims immediately available via `useClaims()`
- **Flexible Data**: Store any JSON-serializable data (strings, arrays, objects)

### 🎫 Token Management
- **JWT Viewer**: Decode and inspect access tokens
- **Auto Refresh**: SDK automatically refreshes expired tokens
- **Token Info**: View issuer, audience, expiration, and more

### 🔌 API Testing
- **Authenticated Requests**: Test protected API endpoints
- **Bearer Tokens**: Automatic token injection in Authorization header
- **Multiple Endpoints**: Test various API routes with one click

## 🚀 Quick Start

### Prerequisites

- Node.js 18+ and pnpm 9+
- Authway services running (Central API, Auth Backend, Hydra)

### 1. Install Dependencies

```bash
# From the asp-frontend directory
pnpm install
```

### 2. Start Development Server

```bash
pnpm dev
```

The app will be available at `http://localhost:5173`

### 3. Test Features

1. **Popup Login**:
   - Click "Popup Login" button on welcome screen
   - Authenticate in the popup window
   - See the app state preserved after login

2. **Dynamic Claims**:
   - Login and go to "Dynamic Claims" tab
   - Add custom claims (e.g., `role: admin`)
   - See the token automatically refresh with new claims

3. **Token Viewer**:
   - Go to "Token" tab
   - Click "Get Access Token" to view JWT
   - Inspect decoded payload (issuer, expiration, etc.)

4. **API Testing**:
   - Go to "API Test" tab
   - Select an endpoint (e.g., `/api/test`)
   - Click "Send Request" to test authenticated API calls

## 📦 SDK Usage Examples

### Popup Login with postMessage

```tsx
import { useAuth } from '@authway/react'

function LoginButton() {
  const { loginWithPopup } = useAuth()

  const handleLogin = async () => {
    try {
      await loginWithPopup({
        redirectUri: window.location.origin + '/callback.html'
      })
      console.log('✅ Popup login successful!')
    } catch (err) {
      console.error('❌ Login failed:', err)
    }
  }

  return <button onClick={handleLogin}>Popup Login</button>
}
```

### Dynamic Claims Updates

```tsx
import { useAuth, useClaims } from '@authway/react'

function ClaimsManager() {
  const { client } = useAuth()
  const { claims, refreshClaims } = useClaims()

  const updateClaims = async () => {
    // Update claims without re-authentication
    await client.updateUserClaims({
      workspace_id: 'ws_12345',
      role: 'admin'
    })

    // Refresh to get updated claims
    await refreshClaims()

    console.log('New claims:', claims)
  }

  return <button onClick={updateClaims}>Update Claims</button>
}
```

### Access Token for API Calls

```tsx
import { useAuth } from '@authway/react'

function ApiCaller() {
  const { getAccessToken } = useAuth()

  const callApi = async () => {
    // Get token (auto-refreshed if expired)
    const token = await getAccessToken()

    // Make authenticated request
    const response = await fetch('/api/protected', {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    })

    return await response.json()
  }

  return <button onClick={callApi}>Call API</button>
}
```

## 🏗️ Project Structure

```
asp-frontend/
├── public/
│   └── callback.html          # Popup callback handler (postMessage)
├── src/
│   ├── components/
│   │   ├── WelcomeScreen.tsx  # Login page with popup/redirect options
│   │   ├── Dashboard.tsx      # Main dashboard with tabs
│   │   ├── UserProfile.tsx    # User info display
│   │   ├── DynamicClaims.tsx  # Claims management UI
│   │   ├── TokenViewer.tsx    # JWT token viewer
│   │   └── ApiTester.tsx      # API testing interface
│   ├── App.tsx                # Main app with AuthwayProvider
│   ├── App.css                # Comprehensive styling
│   ├── config.ts              # API configuration
│   └── main.tsx               # Entry point
├── package.json               # Dependencies (@authway/client, @authway/react)
└── README.md                  # This file
```

## 🔧 Configuration

### Auth Config (App.tsx)

```tsx
const authConfig = {
  domain: 'http://localhost:8081',  // Auth Backend URL (recommended for SPAs)
  clientId: 'authway_spa_sample_local',
  useDPoP: false  // Optional: Enable DPoP for token binding
}

// SDK automatically:
// ✅ Detects OAuth server (Hydra on port 4444) from domain
// ✅ Routes API calls through Auth Backend (8081) which proxies to Central API (8080)
// ✅ Benefits from CORS support provided by Auth Backend
//
// Note: domain should be Auth Backend URL (port 8081 for local dev)
// Auth Backend handles CORS and proxies API calls to Central API
```

### API Base URL (config.ts)

```tsx
export const API_BASE_URL = 'http://localhost:5222'  // ASP.NET Backend
```

## 🎨 Components Overview

### WelcomeScreen
- Displays login options (redirect vs popup)
- Shows features and tech stack
- Handles login errors

### Dashboard
- Tab navigation (Profile, Claims, Token, API)
- Manages active tab state
- Routes to feature components

### DynamicClaims
- Claim update form with validation
- Example claims (role, department, workspace_id)
- Real-time claim refresh
- Usage examples and documentation

### TokenViewer
- JWT token display with copy button
- Decoded payload viewer
- Expiration info and time remaining
- Token usage examples

### ApiTester
- Endpoint selection
- Authenticated API calls
- Response display with syntax highlighting
- Error handling

## 🪟 How Popup Login Works

1. **User Clicks Popup Login**: App calls `loginWithPopup()`
2. **Popup Opens**: New window opens to OAuth authorization URL
3. **User Authenticates**: Logs in via Google (or other provider)
4. **Redirect to callback.html**: Popup redirects to `/callback.html`
5. **postMessage Communication**: `callback.html` sends auth code to parent
6. **SDK Completes Flow**: Parent window exchanges code for tokens
7. **Popup Closes**: Authentication complete, popup auto-closes

### callback.html

```html
<script>
  // Parse OAuth callback parameters
  const urlParams = new URLSearchParams(window.location.search)
  const code = urlParams.get('code')
  const state = urlParams.get('state')
  const error = urlParams.get('error')
  const error_description = urlParams.get('error_description')

  // Send OAuth parameters to parent via postMessage
  window.opener.postMessage({
    type: 'authway-callback',
    code: code,
    state: state,
    error: error,
    error_description: error_description
  }, window.location.origin)

  // Close popup after sending message
  setTimeout(() => window.close(), 1000)
</script>
```

## 🎭 How Dynamic Claims Work

1. **User Updates Claim**: Calls `client.updateUserClaims()`
2. **Server Updates**: Backend stores claim in user metadata
3. **Token Refresh**: SDK automatically refreshes ID token
4. **New Claims Available**: Updated claims in `useClaims()` hook
5. **No Re-auth Needed**: User stays logged in throughout

### Use Cases

- **Workspace Switching**: Update `workspace_id` when user changes workspace
- **Role Changes**: Update `role` or `permissions` when admin changes user role
- **Feature Flags**: Add `features: ["beta"]` for gradual feature rollouts
- **Custom Metadata**: Store any app-specific data (preferences, settings, etc.)

## 🔐 Security Features

- **OAuth 2.0 + PKCE**: Authorization Code Flow with Proof Key for Code Exchange
- **Popup postMessage**: Secure cross-window communication
- **Auto Token Refresh**: Silent refresh with refresh tokens
- **Bearer Tokens**: Standard JWT authentication for API calls

## 📚 SDK Documentation

- **[@authway/client](../../../packages/client/README.md)**: Framework-agnostic client
- **[@authway/react](../../../packages/react/README.md)**: React integration
- **[Dynamic Claims Guide](../../../docs/features/DYNAMIC_CLAIMS.md)**: Detailed claims documentation
- **[Popup Login Guide](../../../docs/features/POPUP_LOGIN_GUIDE.md)**: Popup flow documentation

## 🐛 Troubleshooting

### Popup Blocked
- **Issue**: Browser blocks popup window
- **Solution**: Allow popups for `localhost:5173` in browser settings

### CORS Errors
- **Issue**: CORS errors when calling API
- **Solution**: Ensure ASP.NET backend has CORS configured for `http://localhost:5173`

### Token Not Refreshing
- **Issue**: Dynamic claims not appearing after update
- **Solution**: Call `refreshClaims()` after `updateUserClaims()`

### callback.html Not Found
- **Issue**: 404 error on popup callback
- **Solution**: Ensure `callback.html` is in `public/` directory and accessible at `/callback.html`

## 🚀 Production Build

```bash
# Build for production
pnpm build

# Preview production build
pnpm preview
```

Build output will be in `dist/` directory.

## 📝 License

MIT - Part of the Authway project

## 🙏 Credits

Built with:
- **@authway/client** - Framework-agnostic OAuth client
- **@authway/react** - React hooks and components
- **React 18** - UI library
- **TypeScript** - Type safety
- **Vite** - Fast build tool
