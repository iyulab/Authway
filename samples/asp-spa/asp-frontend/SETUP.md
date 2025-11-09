# ASP.NET SPA Sample - Setup Guide

## Prerequisites

1. Authway services running:
   - Central API (port 8080)
   - Auth Backend (port 8081)
   - Hydra (ports 4444, 4445)

2. ASP.NET backend running (port 5222)

## Client Registration

The client `authway_spa_sample_local` must be registered in Hydra with the correct redirect URIs.

### Automatic Setup (Recommended)

Run the setup script from the samples directory:

```bash
cd D:\data\Authway\samples\asp-spa
bash setup-asp-spa-client.sh
```

If the script shows "Client already exists in Hydra", you need to update it manually:

### Manual Setup

Update the Hydra client to include `callback.html`:

```bash
curl -X PUT http://localhost:4445/admin/clients/authway_spa_sample_local \
  -H "Content-Type: application/json" \
  -d '{
    "client_id": "authway_spa_sample_local",
    "client_name": "Authway SPA Sample (Local Development)",
    "redirect_uris": [
      "http://localhost:5173",
      "http://localhost:5173/",
      "http://localhost:5173/callback.html",
      "http://localhost:4173",
      "http://localhost:4173/"
    ],
    "grant_types": ["authorization_code", "refresh_token"],
    "response_types": ["code"],
    "scope": "openid profile email offline_access",
    "token_endpoint_auth_method": "none",
    "skip_consent": true,
    "skip_logout_consent": true
  }'
```

### Verify Registration

Check that `callback.html` is in the redirect URIs:

```bash
curl -s http://localhost:4445/admin/clients/authway_spa_sample_local | grep redirect_uris
```

You should see:
```json
"redirect_uris":["http://localhost:5173","http://localhost:5173/","http://localhost:5173/callback.html","http://localhost:4173","http://localhost:4173/"]
```

## Install Dependencies

```bash
cd D:\data\Authway\samples\asp-spa\asp-frontend
pnpm install
```

## Start Development Server

```bash
pnpm dev
```

The app will be available at http://localhost:5173

## Testing

### Redirect Login
1. Click "Redirect Login" button
2. Browser navigates to Auth Backend
3. Login with Google
4. Redirect back to app

### Popup Login
1. Click "Popup Login" button
2. Popup window opens
3. Login with Google in popup
4. Popup closes automatically
5. Main window shows logged-in state

### Dynamic Claims
1. After login, go to "Dynamic Claims" tab
2. Enter claim key/value (e.g., `role: admin`)
3. Click "Update Claim"
4. Token automatically refreshes
5. New claim appears in claims viewer

## Troubleshooting

### "redirect_uri does not match" Error

**Symptom**: Login fails with redirect_uri mismatch error

**Cause**: Hydra client doesn't have `callback.html` registered

**Solution**: Run the manual setup command above to update redirect URIs

### "Popup was closed by user" Error

**Symptom**: Popup closes but shows error message

**Cause**: Message type mismatch between callback.html and SDK

**Solution**: Verify `callback.html` sends `type: 'authway-callback'` (already fixed in this version)

### "Missing code or state parameter" Error

**Symptom**: Popup closes successfully but shows missing code/state error

**Cause**: callback.html not parsing and sending OAuth parameters correctly

**Solution**: Verify `callback.html` parses URL params and sends `code` and `state` in postMessage (already fixed in this version)

### Popup Blocked

**Symptom**: No popup window appears

**Solution**: Allow popups for `localhost:5173` in browser settings

### CORS Errors

**Symptom**: API calls fail with CORS errors

**Solution**: Ensure ASP.NET backend has CORS configured for `http://localhost:5173`

### Dependencies Not Found

**Symptom**: Vite can't resolve `@authway/react`

**Solution**: Run `pnpm install` from the workspace root to link packages

## Port Configuration

If you need to use different ports, update these locations:

1. **Vite dev server**: `package.json` scripts section
2. **Redirect URIs**: Hydra client configuration (see manual setup above)
3. **Central API**: `src/App.tsx` authConfig.domain ⚠️ **MUST be port 8080, NOT 8081!**
4. **API Backend**: `src/config.ts` API_BASE_URL

### ⚠️ Critical Configuration

**IMPORTANT**: The `domain` in `authConfig` MUST point to Central API (port 8080), NOT Auth Backend (port 8081)!

```typescript
// ✅ CORRECT
const authConfig = {
  domain: 'http://localhost:8080',  // Central API
  clientId: 'authway_spa_sample_local'
}

// ❌ WRONG - Will fail with "Failed to authenticate user with central API"
const authConfig = {
  domain: 'http://localhost:8081',  // Auth Backend (WRONG!)
  clientId: 'authway_spa_sample_local'
}
```

**Why?**: The SDK fetches user claims from `${domain}/api/v1/claims`, which exists on Central API (8080), not Auth Backend (8081). OAuth server (Hydra on 4444) is auto-detected.

## Notes

- Hydra stores clients in memory by default. Restart Hydra = re-register client.
- For production, configure Hydra with persistent storage (PostgreSQL).
- The client secret is `asp-spa-secret-dev-only` (dev only, not for production).
- **Domain Configuration**: Always use Central API URL (port 8080) in authConfig!
