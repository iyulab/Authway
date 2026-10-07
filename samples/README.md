# 🔐 Authway Sample Services

Complete OAuth 2.0 test suite for **Authway** featuring three sample service applications to test multi-tenant, Single Sign-On (SSO), and central authentication capabilities.

---

## 📋 Table of Contents

- [Overview](#overview)
- [Services](#services)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Testing Scenarios](#testing-scenarios)
- [Architecture](#architecture)
- [Troubleshooting](#troubleshooting)

---

## 🎯 Overview

This sample suite demonstrates:

- ✅ **OAuth 2.0 Authorization Code Flow**
- ✅ **Single Sign-On (SSO)** across multiple applications
- ✅ **Multi-Tenant** support with tenant isolation
- ✅ **Token Management** (access + refresh tokens)
- ✅ **User Profile** retrieval via OpenID Connect
- ✅ **Secure Session** management

---

## 🍎🍌🍫 Services

### 🍎 Apple Service (Port 9001)
**Color Theme**: Red
**Client ID**: `apple-service-client`
**Callback**: `http://localhost:9001/callback`

### 🍌 Banana Service (Port 9002)
**Color Theme**: Yellow
**Client ID**: `banana-service-client`
**Callback**: `http://localhost:9002/callback`

### 🍫 Chocolate Service (Port 9003)
**Color Theme**: Brown
**Client ID**: `chocolate-service-client`
**Callback**: `http://localhost:9003/callback`

Each service is a fully functional OAuth 2.0 client application demonstrating:
- Login flow with Authway
- User profile display
- Session management
- Logout functionality

---

## 📋 Prerequisites

1. **Authway Server** running on `http://localhost:8080`
   ```powershell
   cd Authway
   .\start-dev.ps1
   ```

2. **Go 1.21+** installed
   ```bash
   go version
   ```

3. **Dependencies** (will be auto-downloaded)
   - `golang.org/x/oauth2`

---

## 🚀 Quick Start

### Step 1: Register OAuth Clients

Run the setup script to register all sample services with Authway:

**Windows (PowerShell)**:
```powershell
cd samples
.\setup-clients.ps1
```

**Linux/Mac (Bash)**:
```bash
cd samples
chmod +x setup-clients.sh
./setup-clients.sh
```

### Step 2: Install Dependencies

```bash
cd shared
go mod download

cd ../AppleService
go mod download

cd ../BananaService
go mod download

cd ../ChocolateService
go mod download
```

### Step 3: Start Sample Services

Open **3 separate terminals** and run:

**Terminal 1 - Apple Service**:
```bash
cd samples/AppleService
go run main.go
```
Output: `🍎 Apple Service starting on http://localhost:9001`

**Terminal 2 - Banana Service**:
```bash
cd samples/BananaService
go run main.go
```
Output: `🍌 Banana Service starting on http://localhost:9002`

**Terminal 3 - Chocolate Service**:
```bash
cd samples/ChocolateService
go run main.go
```
Output: `🍫 Chocolate Service starting on http://localhost:9003`

### Step 4: Test the Services

Open your browser and navigate to:
- 🍎 **Apple Service**: http://localhost:9001
- 🍌 **Banana Service**: http://localhost:9002
- 🍫 **Chocolate Service**: http://localhost:9003

---

## 🧪 Testing Scenarios

### Test 1: Basic OAuth Flow

1. Open **Apple Service** (http://localhost:9001)
2. Click "**Login with Authway**"
3. You'll be redirected to Authway login page
4. Enter credentials and authorize
5. You'll be redirected back to Apple Service
6. ✅ **Success**: User profile displayed

**What to verify**:
- Login redirect works correctly
- Authorization page shows client information
- User profile displays after successful login
- Access token is valid

---

### Test 2: Single Sign-On (SSO)

1. **Login to Apple Service** (http://localhost:9001)
2. Keep the browser window open
3. Open **Banana Service** (http://localhost:9002) in a **new tab**
4. Click "**Login with Authway**"
5. ✅ **Expected**: Auto-login without re-entering credentials

**What to verify**:
- No credential prompt on second service
- User is automatically logged in
- Same user profile appears across services
- Session is shared via Authway

---

### Test 3: Multi-Service Session

1. Login to all 3 services in separate tabs:
   - 🍎 Apple Service
   - 🍌 Banana Service
   - 🍫 Chocolate Service

2. Verify all show the same user profile

3. **Logout from one service** (e.g., Apple Service)

4. ✅ **Expected**:
   - Apple Service logs out locally
   - Other services remain logged in
   - Re-login to Apple Service triggers SSO

**What to verify**:
- Each service maintains independent local session
- Authway maintains central authentication state
- SSO works consistently across services

---

### Test 4: Token Expiration & Refresh

1. Login to Apple Service
2. View **Profile** page to see token expiration time
3. Wait for token to expire (or manually revoke)
4. Refresh the page
5. ✅ **Expected**: Automatic token refresh or re-authentication prompt

**What to verify**:
- Token expiration time is displayed correctly
- Refresh token flow works (if implemented)
- Expired tokens are handled gracefully

---

### Test 5: Multi-Tenant Testing

To test multi-tenancy, you need to create additional tenants:

1. Access Authway Admin Console (http://localhost:3000)
2. Create a new tenant (e.g., "AcmeCorp")
3. Register users for the new tenant
4. Register OAuth clients for the new tenant
5. Update service configuration to use new tenant
6. ✅ **Expected**: Complete isolation between tenants

**What to verify**:
- Tenant A users cannot access Tenant B resources
- OAuth clients are tenant-specific
- User profiles show correct tenant information

---

### Test 6: Authorization Scope Testing

1. Modify service configuration to request different scopes:
   ```go
   Scopes: []string{"openid", "profile"}, // Remove "email"
   ```

2. Restart service and login
3. ✅ **Expected**: Email information not available in profile

**What to verify**:
- Only requested scopes are granted
- User profile reflects granted scopes
- Authorization page shows requested scopes

---

## 🏗️ Architecture

### Flow Diagram

```
┌─────────────┐      ┌─────────────┐      ┌─────────────┐
│   Apple     │      │   Banana    │      │  Chocolate  │
│  Service    │      │  Service    │      │   Service   │
│  :9001      │      │  :9002      │      │   :9003     │
└──────┬──────┘      └──────┬──────┘      └──────┬──────┘
       │                    │                    │
       └────────────────────┼────────────────────┘
                            │
                            │ OAuth 2.0 / OIDC
                            │
                   ┌────────▼────────┐
                   │    Authway      │
                   │   Auth Server   │
                   │     :8080       │
                   └─────────────────┘
                            │
                   ┌────────▼────────┐
                   │   PostgreSQL    │
                   │   (Users DB)    │
                   └─────────────────┘
```

### OAuth 2.0 Flow

1. **Authorization Request**
   ```
   User → Service → Authway /oauth/authorize
   ```

2. **User Authentication**
   ```
   Authway → Login UI → User enters credentials
   ```

3. **Authorization Grant**
   ```
   User authorizes → Authway generates code
   ```

4. **Token Exchange**
   ```
   Service → Authway /oauth/token (with code)
   Authway → Service (access token + refresh token)
   ```

5. **Resource Access**
   ```
   Service → Authway /oauth/userinfo (with access token)
   Authway → Service (user profile)
   ```

---

## 🔧 Configuration

### Service Configuration

Each service can be configured by modifying `main.go`:

```go
const (
	serviceName  = "Apple Service"
	servicePort  = "9001"
	serviceColor = "#FF6B6B"
)

oauthConfig = &shared.OAuthConfig{
	ClientID:     "apple-service-client",
	ClientSecret: "apple-service-secret",
	RedirectURL:  "http://localhost:9001/callback",
	AuthURL:      "http://localhost:8080/oauth/authorize",
	TokenURL:     "http://localhost:8080/oauth/token",
	UserInfoURL:  "http://localhost:8080/oauth/userinfo",
	Scopes:       []string{"openid", "profile", "email"},
}
```

### Shared OAuth Package

Located in `samples/shared/oauth.go`, provides:
- `OAuthConfig` - Configuration management
- `GenerateState()` - CSRF protection
- `ExchangeCode()` - Token exchange
- `GetUserInfo()` - Profile retrieval
- `RefreshAccessToken()` - Token refresh
- `RevokeToken()` - Token revocation

---

## 🐛 Troubleshooting

### Issue: "Client not found" error

**Solution**: Run the setup script to register clients:
```powershell
.\setup-clients.ps1
```

---

### Issue: "Connection refused" to Authway

**Solution**: Make sure Authway is running:
```powershell
cd Authway
.\start-dev.ps1
```

Verify: http://localhost:8080/health

---

### Issue: Services can't start - "port already in use"

**Solution**: Check if ports are available:
```powershell
# Windows
netstat -ano | findstr "9001"
netstat -ano | findstr "9002"
netstat -ano | findstr "9003"

# Kill process using port (replace PID)
taskkill /PID <PID> /F
```

---

### Issue: SSO not working across services

**Possible causes**:
1. **Different browsers/profiles** - SSO requires same browser session
2. **Cleared cookies** - Authway session cookie was deleted
3. **Different tenants** - Services configured for different tenants

**Solution**:
- Use same browser and profile
- Don't clear cookies between tests
- Verify all services use same tenant

---

### Issue: Token expired error

**Solution**: This is expected behavior. The service should:
1. Detect expired token
2. Use refresh token to get new access token
3. Retry the request

If refresh fails, user will be redirected to login.

---

## 📚 Additional Resources

- **Authway Documentation**: [../README.md](../README.md)
- **OAuth 2.0 Spec**: https://oauth.net/2/
- **OpenID Connect**: https://openid.net/connect/
- **Go OAuth2 Package**: https://pkg.go.dev/golang.org/x/oauth2

---

## 🎓 Learning Objectives

After completing these tests, you should understand:

- ✅ OAuth 2.0 Authorization Code Flow
- ✅ How SSO works across multiple applications
- ✅ Token lifecycle (access + refresh)
- ✅ Scope-based authorization
- ✅ Multi-tenant architecture patterns
- ✅ Secure session management
- ✅ PKCE flow (if implemented)

---

## 📝 Notes

- **Development Only**: These samples use HTTP (not HTTPS) for localhost testing
- **In-Memory Sessions**: Services use simple in-memory session storage
- **Production**: Real applications should:
  - Use HTTPS exclusively
  - Implement persistent session storage (Redis, database)
  - Add PKCE for public clients
  - Implement proper error handling
  - Add logging and monitoring
  - Use environment variables for configuration

---

**Happy Testing!** 🚀
