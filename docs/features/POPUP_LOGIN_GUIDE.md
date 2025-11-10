# Popup Login Guide

Complete guide for implementing popup-based authentication with Authway OAuth 2.0 / OpenID Connect.

**Version**: 1.1.0 (Updated for v0.1.4)
**Last Updated**: 2025-11-10
**Status**: Production Ready

---

## 📋 Table of Contents

1. [Overview](#overview)
2. [v0.1.4 Update: Google OAuth & External Providers](#v014-update-google-oauth--external-providers)
3. [Architecture](#architecture)
4. [Frontend Implementation](#frontend-implementation)
5. [Backend Implementation](#backend-implementation)
6. [CORS Configuration](#cors-configuration)
7. [Troubleshooting](#troubleshooting)
8. [Security Considerations](#security-considerations)
9. [Testing Checklist](#testing-checklist)

---

## Overview

### Why Popup Login?

- ✅ **Better User Experience**: Authentication without leaving the current page
- ✅ **Context Preservation**: SPA state maintained (form inputs, scroll position)
- ✅ **Fast Response**: Immediate login state update when popup closes
- ✅ **Modern UX**: Mobile app-like experience

### Technology Stack

- **Frontend**: React 18 + TypeScript (or vanilla JavaScript)
- **Backend**: ASP.NET Core (any version) or any OAuth-compatible server
- **Authentication**: Authway OpenID Connect with PKCE
- **Communication**: postMessage API for cross-window messaging

---

## v0.1.4 Update: Google OAuth & External Providers

### The Problem: Cross-Origin-Opener-Policy (COOP) Blocking

When using popup login with external OAuth providers (Google, GitHub, Facebook, etc.), browsers enforce COOP which **blocks `window.opener` access** after cross-origin redirects.

**Flow**:
```
Main App (localhost:5173)
  → Opens popup → Auth UI (localhost:3001)
  → Google OAuth (accounts.google.com) [COOP blocks window.opener]
  → Back to Auth UI (localhost:3001)
  → ❌ window.opener === null (blocked by COOP)
```

### The Solution: SessionStorage + Hidden Iframe + postMessage

**v0.1.4** implements a hybrid approach to solve COOP blocking:

1. **SessionStorage Persistence**: Survives cross-origin redirects (unlike `window.opener`)
2. **Hidden Iframe**: Loads Hydra OAuth flow without losing popup context
3. **postMessage Communication**: Sends authorization code from iframe to popup to main window

#### Complete Flow (v0.1.4)

```
┌─────────────────┐
│ Main Window     │ (localhost:5173 - SDK)
│ (SDK)           │
└────────┬────────┘
         │ 1. loginWithPopup() → Opens popup
         │
         ↓
┌─────────────────┐
│ Popup Window    │ (localhost:3001 - Auth UI)
│ GoogleLoginBtn  │
└────────┬────────┘
         │ 2. Detects popup mode: window.opener !== null
         │ 3. Sets sessionStorage: authway_popup_mode = 'true'
         │ 4. Redirects to Google OAuth
         │
         ↓
┌─────────────────┐
│ Google OAuth    │ (accounts.google.com)
│ Login Page      │
└────────┬────────┘
         │ 5. User authenticates
         │ 6. Redirects back → ⚠️ COOP blocks window.opener
         │
         ↓
┌─────────────────┐
│ ConsentPage     │ (localhost:3001 - Auth UI, still in popup)
│ (Popup)         │
└────────┬────────┘
         │ 7. window.opener === null ❌ (COOP blocked)
         │ 8. sessionStorage.getItem('authway_popup_mode') === 'true' ✅
         │ 9. Creates hidden iframe
         │ 10. Loads Hydra redirect URL in iframe
         │
         ↓
┌─────────────────┐
│ Hidden Iframe   │ (localhost:4444 → localhost:5173)
│ Hydra → callback│
└────────┬────────┘
         │ 11. Hydra generates authorization code
         │ 12. Redirects iframe to callback.html (localhost:5173)
         │
         ↓
┌─────────────────┐
│ callback.html   │ (in iframe)
│ (Iframe)        │
└────────┬────────┘
         │ 13. Detects: window.self !== window.top (in iframe)
         │ 14. Parses code/state from URL
         │ 15. window.parent.postMessage({ type: 'authway-callback', code, state }, '*')
         │
         ↓
┌─────────────────┐
│ ConsentPage     │ (receives postMessage from iframe)
│ (Popup)         │
└────────┬────────┘
         │ 16. Receives code via postMessage
         │ 17. (window.opener || window.parent).postMessage({ code, state }, parentOrigin)
         │ 18. sessionStorage.removeItem('authway_popup_mode')
         │ 19. Closes popup
         │
         ↓
┌─────────────────┐
│ Main Window     │ (receives postMessage from popup)
│ (SDK)           │
└────────┬────────┘
         │ 20. SDK exchanges code for tokens
         │ 21. ✅ Login complete
```

#### Key Implementation Details

**1. GoogleLoginButton.tsx** - SessionStorage Flag
```typescript
if (isPopupMode) {
  console.log('[GoogleLogin] Popup mode detected - setting sessionStorage flag')
  sessionStorage.setItem('authway_popup_mode', 'true')
} else {
  sessionStorage.removeItem('authway_popup_mode')
}
window.location.href = data.redirect_url
```

**2. ConsentPage.tsx** - Dual Detection + postMessage Listener
```typescript
// Check both window.opener AND sessionStorage (COOP survival)
const hasWindowOpener = window.opener !== null && window.opener !== window
const isSessionStoragePopup = sessionStorage.getItem('authway_popup_mode') === 'true'
const isPopupMode = hasWindowOpener || isSessionStoragePopup

if (isPopupMode) {
  // Create hidden iframe
  const iframe = document.createElement('iframe')
  iframe.style.display = 'none'
  document.body.appendChild(iframe)

  // Listen for postMessage from iframe
  const messageHandler = (event: MessageEvent) => {
    if (event.data?.type === 'authway-callback') {
      const { code, state } = event.data

      // Forward to main window
      const targetWindow = window.opener || window.parent
      targetWindow.postMessage({ type: 'authway-callback', code, state }, parentOrigin)

      // Cleanup
      sessionStorage.removeItem('authway_popup_mode')
      window.close()
    }
  }

  window.addEventListener('message', messageHandler)
  iframe.src = redirectUrl // Load Hydra URL
}
```

**3. callback.html** - Iframe Detection + postMessage Sender
```html
<script>
(function() {
  const isInIframe = window.self !== window.top;

  if (isInIframe) {
    // Parse code from URL
    const urlParams = new URLSearchParams(window.location.search);
    const code = urlParams.get('code');
    const state = urlParams.get('state');

    // Send to parent (ConsentPage/LoginPage popup)
    window.parent.postMessage({
      type: 'authway-callback',
      code, state
    }, '*');
  } else {
    // Popup mode - send to opener
    if (window.opener) {
      window.opener.postMessage({
        type: 'authway-callback',
        code, state
      }, window.location.origin);
      setTimeout(() => window.close(), 500);
    }
  }
})();
</script>
```

### Why This Works

1. **SessionStorage Persistence**: Unlike `window.opener`, sessionStorage survives cross-origin redirects
2. **Hidden Iframe Isolation**: OAuth flow happens in iframe, popup window stays intact with opener reference
3. **postMessage Chain**: callback.html → ConsentPage → Main Window (via opener/parent references)
4. **Fallback URL Polling**: If postMessage fails, ConsentPage polls iframe URL (100ms interval, 5s max)

### Browser Compatibility

✅ **Tested & Working**:
- Chrome 120+ (COOP enforced)
- Firefox 121+
- Edge 120+
- Safari 17+ (with popup permission)

### Backward Compatibility

✅ **100% Backward Compatible**:
- Redirect mode unchanged (no impact)
- Popup mode without external OAuth unchanged
- Only external OAuth popup flows use new mechanism

---

## Architecture

### Complete Flow

```
┌─────────────┐
│  Frontend   │
│ (port 5174) │
└──────┬──────┘
       │ 1. Open popup with origin parameter
       │    /api/auth/login?popup=true&origin=http://localhost:5174
       ↓
┌─────────────┐
│   Backend   │
│ (port 5000) │
└──────┬──────┘
       │ 2. Store origin in AuthenticationProperties
       │ 3. Redirect to Authway OIDC
       ↓
┌─────────────┐
│   Authway   │
│    OIDC     │
└──────┬──────┘
       │ 4. User authenticates
       │ 5. Callback to /signin-oidc
       ↓
┌─────────────┐
│   Backend   │
│ OnTicketReceived event
└──────┬──────┘
       │ 6. Detect popup mode
       │ 7. Redirect to /api/auth/popup-callback?origin=...
       ↓
┌─────────────┐
│   Backend   │
│ PopupCallback action
└──────┬──────┘
       │ 8. Validate origin against allowed origins
       │ 9. Return HTML with postMessage script
       ↓
┌─────────────┐
│  Popup      │
│  Window     │
└──────┬──────┘
       │ 10. postMessage to parent window (targetOrigin: origin)
       │     { type: 'authway-login-success', success: true }
       ↓
┌─────────────┐
│  Frontend   │
│  (receives  │
│  message)   │
└──────┬──────┘
       │ 11. Close popup
       │ 12. Refresh auth state
       └─→ ✅ Login complete
```

---

## Frontend Implementation

### Option 1: Using @authway/react Package

**Recommended for React applications**

```tsx
import { useAuth } from '@authway/react';

function LoginButton() {
  const { loginWithPopup } = useAuth();

  const handleLogin = async () => {
    try {
      await loginWithPopup({
        width: 500,
        height: 700
      });
      console.log('Login successful!');
    } catch (error) {
      console.error('Login failed:', error);
      alert(`Login failed: ${error.message}`);
    }
  };

  return (
    <button onClick={handleLogin}>
      Login with Popup
    </button>
  );
}
```

### Option 2: Custom usePopupLogin Hook

**For custom implementations or non-React frameworks**

**File**: `src/hooks/usePopupLogin.ts`

```typescript
import { useState, useCallback, useEffect, useRef } from 'react';

export interface PopupLoginOptions {
  width?: number;
  height?: number;
  onSuccess?: () => void;
  onError?: (error: Error) => void;
  timeout?: number;
}

export interface PopupLoginState {
  isLoading: boolean;
  error: string | null;
}

const DEFAULT_OPTIONS: Required<PopupLoginOptions> = {
  width: 500,
  height: 700,
  onSuccess: () => {},
  onError: () => {},
  timeout: 300000, // 5 minutes
};

export function usePopupLogin(options: PopupLoginOptions = {}) {
  const [state, setState] = useState<PopupLoginState>({
    isLoading: false,
    error: null,
  });

  const opts = { ...DEFAULT_OPTIONS, ...options };
  const cleanupRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    return () => {
      if (cleanupRef.current) {
        cleanupRef.current();
      }
    };
  }, []);

  const login = useCallback(async () => {
    setState({ isLoading: true, error: null });

    try {
      // ⚠️ CRITICAL: Pass current origin for postMessage targetOrigin
      const currentOrigin = window.location.origin;
      const authUrl = `${import.meta.env.VITE_API_URL || 'http://localhost:5000'}/api/auth/login?popup=true&origin=${encodeURIComponent(currentOrigin)}`;

      // Calculate centered popup position
      const left = window.screen.width / 2 - opts.width / 2;
      const top = window.screen.height / 2 - opts.height / 2;

      // Open popup window
      const popup = window.open(
        authUrl,
        'authway-login-popup',
        `width=${opts.width},height=${opts.height},left=${left},top=${top},scrollbars=yes,resizable=yes,location=no,toolbar=no,menubar=no,status=no`
      );

      if (!popup) {
        throw new Error('Popup was blocked. Please allow popups for this site.');
      }

      popup.focus();

      // Wait for popup to complete authentication
      await new Promise<void>((resolve, reject) => {
        let timeoutId: NodeJS.Timeout;
        let intervalId: NodeJS.Timeout;

        const cleanup = () => {
          if (timeoutId) clearTimeout(timeoutId);
          if (intervalId) clearInterval(intervalId);
          window.removeEventListener('message', messageHandler);
          cleanupRef.current = null;
        };

        cleanupRef.current = cleanup;

        const messageHandler = (event: MessageEvent) => {
          // ⚠️ SECURITY: Verify origin to prevent XSS attacks
          const allowedOrigins = [
            window.location.origin,
            'http://localhost:5000',
            'http://localhost:3000',
            'http://localhost:5173',
            'http://localhost:5174',
            import.meta.env.VITE_API_URL,
          ].filter(Boolean);

          if (!allowedOrigins.some(origin => event.origin === origin)) {
            console.warn('Received message from untrusted origin:', event.origin);
            return;
          }

          // Check for authentication success message
          if (event.data && event.data.type === 'authway-login-success') {
            cleanup();

            try {
              popup.close();
            } catch (err) {
              // Ignore close errors
            }

            if (event.data.success) {
              resolve();
            } else {
              reject(new Error(event.data.error || 'Authentication failed'));
            }
          }
        };

        // Listen for postMessage from popup
        window.addEventListener('message', messageHandler);

        // Timeout after specified duration
        timeoutId = setTimeout(() => {
          cleanup();
          try {
            popup.close();
          } catch (err) {
            // Ignore close errors
          }
          reject(new Error('Login timeout - authentication took too long'));
        }, opts.timeout);

        // Check if popup was closed by user
        intervalId = setInterval(() => {
          try {
            if (popup.closed) {
              cleanup();
              reject(new Error('Popup was closed by user'));
            }
          } catch (err) {
            // Cross-origin policy may block popup.closed access
          }
        }, 1000);
      });

      // Success!
      setState({ isLoading: false, error: null });
      opts.onSuccess();

    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error occurred';
      console.error('Popup login failed:', error);
      setState({ isLoading: false, error: errorMessage });
      opts.onError(error instanceof Error ? error : new Error(errorMessage));
    }
  }, [opts]);

  return {
    login,
    state,
  };
}
```

### Usage in Components

```typescript
import { usePopupLogin } from '@/hooks/usePopupLogin';

function LoginButton() {
  const { login, state } = usePopupLogin({
    onSuccess: () => {
      console.log('Login successful!');
      window.location.reload(); // or update auth state
    },
    onError: (error) => {
      console.error('Login failed:', error);
      alert(`Login failed: ${error.message}`);
    }
  });

  return (
    <button onClick={login} disabled={state.isLoading}>
      {state.isLoading ? 'Authenticating...' : 'Login with Popup'}
    </button>
  );
}
```

---

## Backend Implementation

### 1. AuthController - Login Action

**File**: `Controllers/AuthController.cs`

```csharp
/// <summary>
/// Initiate Authway OIDC login flow (supports both redirect and popup modes)
/// </summary>
[HttpGet("login")]
[AllowAnonymous]
public IActionResult Login(
    [FromQuery] string? returnUrl = null,
    [FromQuery] bool popup = false,
    [FromQuery] string? origin = null)
{
    var properties = new AuthenticationProperties();

    // Store popup mode in authentication properties for callback handling
    if (popup)
    {
        properties.Items["popup_mode"] = "true";

        // ⚠️ CRITICAL: Store origin for postMessage targetOrigin validation
        if (!string.IsNullOrEmpty(origin))
        {
            properties.Items["popup_origin"] = origin;
            _logger.LogInformation("Storing popup origin: {Origin}", origin);
        }

        // Don't set RedirectUri for popup, OnTicketReceived will handle it
    }
    else
    {
        properties.RedirectUri = returnUrl ?? "/";
    }

    return Challenge(properties, OpenIdConnectDefaults.AuthenticationScheme);
}
```

### 2. Program.cs - OIDC Events Configuration

```csharp
.AddOpenIdConnect(OpenIdConnectDefaults.AuthenticationScheme, options =>
{
    // Authway OIDC Configuration
    options.Authority = builder.Configuration["Authway:Authority"];
    options.ClientId = builder.Configuration["Authway:ClientId"];
    options.ClientSecret = builder.Configuration["Authway:ClientSecret"];
    options.ResponseType = OpenIdConnectResponseType.Code;
    options.UsePkce = true;

    // Scopes
    options.Scope.Clear();
    options.Scope.Add("openid");
    options.Scope.Add("profile");
    options.Scope.Add("email");

    // Endpoints
    options.CallbackPath = "/signin-oidc";
    options.SignedOutCallbackPath = "/signout-callback-oidc";

    // Token validation
    options.SaveTokens = true;
    options.GetClaimsFromUserInfoEndpoint = true;

    // Events for popup mode handling
    options.Events = new OpenIdConnectEvents
    {
        OnTicketReceived = context =>
        {
            // ⚠️ CRITICAL: Check if this is a popup callback
            var isPopup = context.Properties?.Items.ContainsKey("popup_mode") == true;

            if (isPopup)
            {
                // Get stored origin from authentication properties
                var popupOrigin = context.Properties?.Items.TryGetValue("popup_origin", out var origin) == true
                    ? origin
                    : null;

                // Redirect to PopupCallback action with origin parameter
                var redirectUrl = !string.IsNullOrEmpty(popupOrigin)
                    ? $"/api/auth/popup-callback?origin={Uri.EscapeDataString(popupOrigin)}"
                    : "/api/auth/popup-callback";

                context.Response.Redirect(redirectUrl);
                context.HandleResponse();
            }
            return Task.CompletedTask;
        },

        OnAuthenticationFailed = context =>
        {
            var logger = context.HttpContext.RequestServices.GetRequiredService<ILogger<Program>>();
            logger.LogError(context.Exception, "OIDC Authentication failed");
            context.HandleResponse();
            context.Response.Redirect($"/api/auth/error?message={Uri.EscapeDataString(context.Exception.Message)}");
            return Task.CompletedTask;
        }
    };
});
```

### 3. AuthController - PopupCallback Action

```csharp
/// <summary>
/// Popup callback endpoint - sends postMessage to parent window and closes popup
/// </summary>
[HttpGet("popup-callback")]
[Authorize]
public IActionResult PopupCallback([FromQuery] string? origin = null)
{
    _logger.LogInformation("Popup authentication completed successfully");

    var frontendUrl = origin ?? "http://localhost:3000"; // Default fallback

    // ⚠️ SECURITY: Validate origin is in allowed origins list
    var allowedOrigins = _configuration.GetSection("Cors:AllowedOrigins").Get<string[]>() ??
        new[] { "http://localhost:5173", "http://localhost:5174", "http://localhost:3000" };

    if (!string.IsNullOrEmpty(origin) && !allowedOrigins.Contains(origin))
    {
        _logger.LogWarning("Popup origin {Origin} not in allowed origins list, using fallback", origin);
        frontendUrl = allowedOrigins[0];
    }
    else if (!string.IsNullOrEmpty(origin))
    {
        _logger.LogInformation("Using popup origin from parameter: {FrontendUrl}", frontendUrl);
    }
    else
    {
        // Fallback to first allowed origin from configuration
        frontendUrl = allowedOrigins[0];
        _logger.LogWarning("No origin parameter provided, using fallback frontend URL: {FrontendUrl}", frontendUrl);
    }

    // Return HTML that sends postMessage to opener and closes the popup
    var html = $@"
<!DOCTYPE html>
<html>
<head>
    <title>Login Successful</title>
    <meta charset='utf-8'>
    <style>
        body {{
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', sans-serif;
            display: flex;
            align-items: center;
            justify-content: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
        }}
        .container {{
            text-align: center;
        }}
        .spinner {{
            width: 50px;
            height: 50px;
            border: 4px solid rgba(255, 255, 255, 0.3);
            border-top-color: white;
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin: 0 auto 1rem;
        }}
        @keyframes spin {{
            to {{ transform: rotate(360deg); }}
        }}
        h2 {{ margin: 0.5rem 0; }}
        p {{ margin: 0.5rem 0; opacity: 0.9; }}
    </style>
</head>
<body>
    <div class='container'>
        <div class='spinner'></div>
        <h2>✅ Login Successful!</h2>
        <p>Closing popup...</p>
    </div>
    <script>
        (function() {{
            try {{
                if (window.opener && !window.opener.closed) {{
                    // ⚠️ CRITICAL: Send success message to parent window with validated origin
                    window.opener.postMessage({{
                        type: 'authway-login-success',
                        success: true,
                        timestamp: new Date().toISOString()
                    }}, '{frontendUrl}');

                    // Close popup after a short delay
                    setTimeout(() => {{
                        window.close();
                    }}, 1000);
                }} else {{
                    // If no opener, redirect to frontend app
                    console.log('No opener window found, redirecting to frontend');
                    window.location.href = '{frontendUrl}';
                }}
            }} catch (error) {{
                console.error('Popup callback error:', error);
                // Fallback: redirect to frontend
                window.location.href = '{frontendUrl}';
            }}
        }})();
    </script>
</body>
</html>";

    return Content(html, "text/html");
}
```

---

## CORS Configuration

### ⚠️ CRITICAL: Allow All Frontend Ports

**Problem**: `PopupCallback` origin validation fails if frontend port is not in allowed origins.

**Solution**: Add all development ports to `appsettings.Development.json`

**File**: `appsettings.Development.json`

```json
{
  "Cors": {
    "AllowedOrigins": [
      "http://localhost:5173",
      "http://localhost:5174",
      "http://localhost:3000",
      "http://127.0.0.1:5173",
      "http://127.0.0.1:5174",
      "http://127.0.0.1:3000"
    ],
    "AllowedMethods": ["GET", "POST", "PUT", "DELETE", "OPTIONS"],
    "AllowedHeaders": ["Content-Type", "Authorization", "X-Requested-With"],
    "AllowCredentials": true
  }
}
```

**Program.cs Configuration**:

```csharp
builder.Services.AddCors(options =>
{
    var allowedOrigins = builder.Configuration.GetSection("Cors:AllowedOrigins").Get<string[]>() ??
        new[] { "http://localhost:5173", "http://localhost:5174", "http://localhost:3000" };

    options.AddPolicy("AllowedOriginsPolicy", policy =>
    {
        policy.WithOrigins(allowedOrigins)
              .AllowAnyMethod()
              .AllowAnyHeader()
              .AllowCredentials();
    });
});

// Enable CORS
app.UseCors("AllowedOriginsPolicy");
```

---

## Troubleshooting

### Problem 1: postMessage targetOrigin Mismatch

**Symptoms**:
```
Failed to execute 'postMessage' on 'DOMWindow':
The target origin provided ('http://localhost:5173') does not match
the recipient window's origin ('http://localhost:5174')
```

**Cause**:
- Frontend running on port 5174
- Backend `appsettings.json` missing port 5174 in `Cors:AllowedOrigins`
- `PopupCallback` origin validation fails and falls back to 5173

**Solution**:
1. Add all development ports to `appsettings.Development.json`
2. Restart backend (Hot Reload doesn't reload config files)

### Problem 2: Popup Redirects to Backend URL

**Symptoms**:
- Login successful but popup shows `http://localhost:5000/api/auth/popup-callback`
- postMessage not sent

**Cause**:
- Origin parameter missing or not passed through
- `OnTicketReceived` event not forwarding origin

**Solution**:
1. Verify frontend sends `login?popup=true&origin=${currentOrigin}`
2. Check `Login` action stores origin in `AuthenticationProperties`
3. Ensure `OnTicketReceived` passes origin as query parameter

### Problem 3: CORS Errors

**Symptoms**:
```
Access to XMLHttpRequest at 'http://localhost:5000/api/auth/status'
from origin 'http://localhost:5174' has been blocked by CORS policy
```

**Cause**:
- Frontend port not in Backend CORS policy

**Solution**:
1. Add port to `appsettings.Development.json` → `Cors:AllowedOrigins`
2. Update `Program.cs` CORS policy
3. Restart backend

### Problem 4: Double Consent Screen (Authway Issue)

**Symptoms**:
- Consent screen appears twice during login

**Cause**:
- Authway SSO auto-login not preserving first consent grant
- Fixed in Authway as of November 2024

**Solution**:
- Use latest Authway version
- Issue resolved in current deployment

### Problem 5: Google OAuth Popup Fails (v0.1.4 Fix)

**Symptoms**:
```
[ConsentPage] window.opener === null (blocked by COOP)
[ConsentPage] ❌ No parent window found for postMessage
```

**Cause**:
- Google OAuth's Cross-Origin-Opener-Policy blocks `window.opener`
- Popup loses reference to main window after Google redirect

**Solution** (Implemented in v0.1.4):
1. Verify `GoogleLoginButton.tsx` sets `sessionStorage.setItem('authway_popup_mode', 'true')`
2. Verify `ConsentPage.tsx` checks sessionStorage: `sessionStorage.getItem('authway_popup_mode')`
3. Verify `callback.html` version is `v0.1.4-iframe-fix`
4. Check browser console for postMessage flow logs

**Expected Logs** (v0.1.4):
```
[GoogleLogin] Popup mode detected - setting sessionStorage flag
[ConsentPage] Popup mode detected via sessionStorage
[ConsentPage] Loading Hydra URL in iframe
[callback.html] VERSION: v0.1.4-iframe-fix
[callback.html] ✅ Running in iframe - sending code to parent via postMessage
[ConsentPage] ✅ Received code from iframe via postMessage
[ConsentPage] ✅ Sent code to parent via postMessage
```

### Problem 6: callback.html Old Version Cached

**Symptoms**:
- Console shows old version: `[callback.html] ✅ Running in iframe - waiting for parent to extract code`
- No postMessage sent from callback.html

**Cause**:
- Browser aggressively caching callback.html

**Solution**:
1. Hard refresh: `Ctrl+Shift+R` (Windows/Linux) or `Cmd+Shift+R` (Mac)
2. Clear browser cache
3. Verify cache-busting headers in callback.html:
```html
<meta http-equiv="Cache-Control" content="no-cache, no-store, must-revalidate">
<meta http-equiv="Pragma" content="no-cache">
<meta http-equiv="Expires" content="0">
```

---

## Security Considerations

### 1. Origin Validation (Required)

**Frontend**:
```typescript
const allowedOrigins = [
  window.location.origin,
  'http://localhost:5000',
  import.meta.env.VITE_API_URL,
].filter(Boolean);

if (!allowedOrigins.some(origin => event.origin === origin)) {
  console.warn('Received message from untrusted origin:', event.origin);
  return; // ⚠️ CRITICAL: Reject messages from unknown origins
}
```

**Backend**:
```csharp
var allowedOrigins = _configuration.GetSection("Cors:AllowedOrigins").Get<string[]>();

if (!allowedOrigins.Contains(origin)) {
    _logger.LogWarning("Popup origin {Origin} not in allowed origins list", origin);
    // Use fallback or reject
}
```

### 2. HTTPS in Production

Development allows HTTP, but **production MUST use HTTPS**:

```csharp
// Program.cs
if (!app.Environment.IsDevelopment())
{
    app.UseHttpsRedirection();
    options.RequireHttpsMetadata = true; // OIDC
}
```

### 3. Timeout Configuration

Prevent infinite waiting:

```typescript
const DEFAULT_OPTIONS = {
  timeout: 300000, // 5 minutes
};
```

### 4. Popup Blocker Handling

```typescript
const popup = window.open(...);

if (!popup) {
  throw new Error('Popup was blocked. Please allow popups for this site.');
}
```

Display popup permission instructions to users.

---

## Testing Checklist

### General Popup Functionality
- [ ] Popup opens centered on screen
- [ ] Popup closes automatically on successful authentication
- [ ] Appropriate error message on authentication failure
- [ ] Error handling when user manually closes popup
- [ ] Popup closes and error shown on timeout
- [ ] postMessage from untrusted origins rejected
- [ ] Works on all development ports without CORS errors
- [ ] No origin validation warnings in backend logs
- [ ] Multiple consecutive login/logout cycles work correctly
- [ ] Works in different browsers (Chrome, Firefox, Edge, Safari)

### v0.1.4 External OAuth Providers
- [ ] Google OAuth popup login completes successfully
- [ ] SessionStorage flag persists across Google redirect
- [ ] callback.html version shows `v0.1.4-iframe-fix` in console
- [ ] Hidden iframe created in ConsentPage/LoginPage
- [ ] postMessage received from iframe to popup
- [ ] postMessage forwarded from popup to main window
- [ ] SessionStorage cleaned up after login
- [ ] Popup closes automatically after external OAuth
- [ ] Works with GitHub OAuth (if configured)
- [ ] Works with Facebook OAuth (if configured)
- [ ] Fallback URL polling works if postMessage fails
- [ ] No cache issues with callback.html (hard refresh test)

---

## Log Verification

### Successful Flow Logs (ASP.NET Backend)

```
info: Storing popup origin: http://localhost:5174
info: User authenticated successfully via Authway OIDC
info: Popup authentication completed successfully
info: Using popup origin from parameter: http://localhost:5174
```

### Successful Flow Logs (v0.1.4 Google OAuth)

```
[GoogleLogin] Popup mode detected - setting sessionStorage flag
[ConsentPage] Popup mode detected via sessionStorage
[ConsentPage] Loading Hydra URL in iframe
[callback.html] VERSION: v0.1.4-iframe-fix
[callback.html] Context check: { isInIframe: true, hasOpener: false }
[callback.html] ✅ Running in iframe - sending code to parent via postMessage
[callback.html] Extracted from iframe URL: { code: "ory_ac_abc...", state: "xyz..." }
[callback.html] ✅ Sent code to parent (ConsentPage/LoginPage) via postMessage
[ConsentPage] Received postMessage: { type: 'authway-callback', code: "...", state: "..." }
[ConsentPage] ✅ Received code from iframe via postMessage
[ConsentPage] ✅ Sent code to parent via postMessage
```

### Problem Logs

**ASP.NET Backend**:
```
warn: Popup origin http://localhost:5174 not in allowed origins list, using fallback
```
→ Check `appsettings.Development.json` → `Cors:AllowedOrigins`.

**v0.1.4 COOP Blocking**:
```
[ConsentPage] window.opener === null (blocked by COOP)
[ConsentPage] sessionStorage.getItem('authway_popup_mode') === null
[ConsentPage] ❌ Not popup mode - doing normal redirect
```
→ Verify GoogleLoginButton sets sessionStorage flag.

**Old callback.html Cached**:
```
[callback.html] ✅ Running in iframe - waiting for parent to extract code
(no postMessage sent)
```
→ Hard refresh browser (Ctrl+Shift+R) or clear cache.

---

## References

- [Authway Documentation](https://authway.iyulab.com)
- [MDN - postMessage](https://developer.mozilla.org/en-US/docs/Web/API/Window/postMessage)
- [ASP.NET Core Authentication](https://learn.microsoft.com/en-us/aspnet/core/security/authentication/)
- [OpenID Connect Specification](https://openid.net/connect/)

---

## License

MIT License

---

**Author**: Authway Team
**Test Environment**: ASP.NET Core 9.0 + React 18 + Authway OIDC
