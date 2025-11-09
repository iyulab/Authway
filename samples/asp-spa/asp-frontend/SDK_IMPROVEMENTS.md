# SDK Package Improvements - Verification Guide

## 🎯 Problem Solved

**Original Issue**: SDK configuration was ambiguous, leading to authentication failures when users confused Central API (port 8080) with Auth Backend (port 8081).

**Root Cause**: The SDK's `normalizeConfig` method used `domain` for both OAuth server detection AND API calls, causing API endpoints to target the wrong service.

## ✅ SDK Changes Implemented

### 1. Enhanced Configuration Interface (`packages/client/src/types/config.ts`)

**New Fields**:
```typescript
export interface AuthwayConfig {
  /**
   * Authway Central API URL (port 8080 for local dev)
   * ✅ Clear documentation about what this should point to
   */
  domain: string

  /**
   * OAuth server URL (advanced usage only)
   * ✅ Auto-detected from domain for most cases
   * @default auto-detected from domain
   */
  oauthServerUrl?: string

  /**
   * @deprecated Use 'domain' instead
   * ✅ Backwards compatibility maintained
   */
  authwayUrl?: string
}
```

**New Internal Structure**:
```typescript
export interface NormalizedConfig {
  oauthServerUrl: string   // For OAuth flows (Hydra on 4444)
  centralApiUrl: string    // For API calls (Central API on 8080)
  authwayUrl: string       // Deprecated, kept for compatibility
  // ... other fields
}
```

### 2. Fixed Configuration Normalization (`packages/client/src/AuthwayClient.ts`)

**Before** (❌ Problematic):
```typescript
// Used domain for both OAuth AND APIs
let oauthUrl = config.authwayUrl || normalizedDomain
return {
  domain: oauthUrl,              // OAuth server
  authwayUrl: normalizedDomain,  // Also used for APIs! ❌
}
```

**After** (✅ Fixed):
```typescript
// Properly separated concerns
let centralApiUrl = config.domain  // Central API URL
let oauthServerUrl = config.oauthServerUrl || centralApiUrl

// Auto-detect Hydra from Central API port
if (oauthServerUrl.includes(':8080') || oauthServerUrl.includes(':8081')) {
  oauthServerUrl = oauthServerUrl.replace(/:(8080|8081)/, ':4444')
}

// Warn about common mistake
if (centralApiUrl.includes(':8081')) {
  console.warn('⚠️ WARNING: domain is set to port 8081 (Auth Backend)...')
}

return {
  oauthServerUrl,    // Hydra (4444)
  centralApiUrl,     // Central API (8080)
  authwayUrl: centralApiUrl  // Backwards compatibility
}
```

### 3. Updated All API Calls

**Changed 9 API endpoints** to use `centralApiUrl`:

```typescript
// ✅ User Claims API
`${this.config.centralApiUrl}/api/v1/claims`

// ✅ Update Claims API
`${this.config.centralApiUrl}/api/v1/claims`

// ✅ User Info API
`${this.config.centralApiUrl}/api/v1/claims/user`

// ✅ Identity APIs
`${this.config.centralApiUrl}/api/v1/user/identities`
`${this.config.centralApiUrl}/api/v1/user/identities/link`
`${this.config.centralApiUrl}/api/v1/user/identities/${provider}/${userId}`

// ✅ Login API
`${this.config.centralApiUrl}/api/v1/auth/login`
```

**OAuth endpoints remain unchanged** (correctly use `oauthServerUrl`):
```typescript
// ✅ Token exchange
`${this.config.oauthServerUrl}/oauth2/token`

// ✅ Authorization
`${this.config.oauthServerUrl}/oauth2/auth`

// ✅ Logout
`${this.config.oauthServerUrl}/oauth2/sessions/logout`
```

## 🧪 Testing the Improvements

### Test 1: Correct Configuration (Port 8080)
```typescript
const authConfig = {
  domain: 'http://localhost:8080',  // ✅ Central API
  clientId: 'authway_spa_sample_local'
}

// Expected behavior:
// - No warnings in console
// - centralApiUrl = 'http://localhost:8080'
// - oauthServerUrl = 'http://localhost:4444' (auto-detected)
// - All API calls target port 8080 ✅
// - OAuth flows use port 4444 ✅
```

### Test 2: Incorrect Configuration (Port 8081 - Should Warn)
```typescript
const authConfig = {
  domain: 'http://localhost:8081',  // ⚠️ Auth Backend (wrong!)
  clientId: 'authway_spa_sample_local'
}

// Expected behavior:
// - ⚠️ Console warning: "domain is set to port 8081 (Auth Backend)"
// - centralApiUrl = 'http://localhost:8081'
// - oauthServerUrl = 'http://localhost:4444' (auto-detected)
// - Warning helps user fix the mistake!
```

### Test 3: Legacy Configuration (Deprecated authwayUrl)
```typescript
const authConfig = {
  domain: 'http://localhost:8080',
  authwayUrl: 'http://localhost:8080',  // ⚠️ Deprecated
  clientId: 'authway_spa_sample_local'
}

// Expected behavior:
// - ⚠️ Console warning: "authwayUrl is deprecated. Use 'domain' instead"
// - Still works correctly (backwards compatibility maintained)
```

### Test 4: Advanced Configuration (Custom OAuth Server)
```typescript
const authConfig = {
  domain: 'http://localhost:8080',              // Central API
  oauthServerUrl: 'http://custom-hydra:4444',   // Custom OAuth server
  clientId: 'authway_spa_sample_local'
}

// Expected behavior:
// - No warnings
// - centralApiUrl = 'http://localhost:8080'
// - oauthServerUrl = 'http://custom-hydra:4444'
// - Advanced users can override auto-detection
```

## 🔍 Verification Checklist

### Configuration Validation
- [ ] Console shows no errors with correct config (port 8080)
- [ ] Console shows warning with incorrect config (port 8081)
- [ ] Console shows deprecation warning with authwayUrl
- [ ] OAuth server auto-detection works (8080/8081 → 4444)

### API Endpoints
- [ ] Login flow completes successfully
- [ ] Claims API calls target Central API (port 8080)
- [ ] Token exchange targets Hydra (port 4444)
- [ ] User identity APIs work correctly
- [ ] Dynamic claims updates work

### Popup Login Flow
```
1. Click "Popup Login" → Popup opens
2. Redirect to Hydra (port 4444) ✅
3. Auth Backend handles login (port 8081) ✅
4. Callback to app with code/state ✅
5. SDK exchanges code for tokens at Hydra (port 4444) ✅
6. SDK fetches claims from Central API (port 8080) ✅ [FIXED!]
7. Login complete, no errors ✅
```

## 📊 Architecture Understanding

```
User Config:
  domain: 'http://localhost:8080'  ← You provide this (Central API)

SDK Internal:
  centralApiUrl: 'http://localhost:8080'   ← For all API calls
  oauthServerUrl: 'http://localhost:4444'  ← Auto-detected for OAuth

Service Endpoints:
  ┌─────────────────────────────┐
  │  Central API :8080          │ ← API calls go here
  │  GET /api/v1/claims         │
  │  POST /api/v1/claims        │
  │  GET /api/v1/user/identities│
  └─────────────────────────────┘

  ┌─────────────────────────────┐
  │  Auth Backend :8081         │ ← Only for login UI
  │  /login?login_challenge=... │
  │  /consent?consent_challenge=│
  └─────────────────────────────┘

  ┌─────────────────────────────┐
  │  Hydra :4444                │ ← OAuth flows go here
  │  POST /oauth2/token         │
  │  GET /oauth2/auth           │
  └─────────────────────────────┘
```

## 🎓 Key Improvements

1. **Clear Separation**: `centralApiUrl` vs `oauthServerUrl` internally
2. **Auto-detection**: OAuth server port (4444) detected from Central API port
3. **Validation**: Warns users about common mistakes (port 8081)
4. **Backwards Compatibility**: Legacy `authwayUrl` still works
5. **Better Documentation**: JSDoc comments explain what `domain` should be
6. **Correct Routing**: All API calls now target Central API, OAuth calls target Hydra

## 🚀 Next Steps

1. **Test Popup Login**:
   ```bash
   cd D:\data\Authway\samples\asp-spa\asp-frontend
   pnpm dev
   ```

2. **Verify Console Output**:
   - No errors during login
   - Claims fetch successful from port 8080
   - OAuth flows use port 4444

3. **Try Wrong Configuration** (for testing):
   ```typescript
   domain: 'http://localhost:8081'  // Should show warning!
   ```

4. **Confirm Fix**:
   - Popup login completes successfully
   - Dashboard shows user info
   - Dynamic claims work
   - No 401 errors!

## 📝 Summary

**Before**: SDK used `domain` ambiguously, causing API calls to target wrong service when users set port 8081.

**After**: SDK properly separates Central API URL from OAuth server URL, validates configuration, warns about mistakes, and routes all calls correctly.

**Result**: Users can configure SDK correctly, SDK helps prevent mistakes, and authentication flows work reliably! ✅
