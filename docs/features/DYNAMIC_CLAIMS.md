# Dynamic Claims & Token Refresh Guide

**Feature Version**: 0.1.0
**Status**: ✅ Production Ready

---

## 📚 Overview

Dynamic Claims is a powerful feature that allows you to **update user claims in real-time** and **automatically refresh OAuth tokens** without requiring users to log out and log back in.

### Key Features

✅ **Real-time Claim Updates** - Change user context instantly
✅ **Automatic Token Refresh** - Silent re-authentication with `prompt=none`
✅ **Permanent & Session Claims** - Choose persistence level
✅ **Multi-tenant Support** - Isolated claims per tenant
✅ **Redis-backed** - Fast pending claims with PostgreSQL persistence

---

## 🎯 Use Cases

### 1. Workspace/Context Switching

User switches between different workspaces or organizational contexts.

```javascript
// User selects a different workspace
await updateClaims({
  workspace_id: 'ws-production',
  workspace_name: 'Production Environment',
  permissions: ['deploy', 'read', 'write']
});
```

### 2. Role Changes

Administrator grants or revokes permissions.

```javascript
// Promote user to admin
await updateClaims({
  role: 'admin',
  permissions: ['manage_users', 'manage_clients', 'view_analytics']
}, { permanent: true });
```

### 3. Feature Flags

Enable/disable features dynamically.

```javascript
// Enable beta features
await updateClaims({
  features: {
    beta_dashboard: true,
    advanced_analytics: true
  }
});
```

### 4. Temporary Elevations

Grant temporary access for support or auditing.

```javascript
// 5-minute elevated access
await updateClaims({
  support_mode: true,
  elevated_until: Date.now() + 300000
});
```

---

## 🔧 How It Works

### Architecture

```
┌─────────────┐
│   Client    │
│ Application │
└──────┬──────┘
       │
       │ 1. POST /api/v1/claims
       │    { workspace_id: "ws-prod" }
       ▼
┌─────────────────┐
│  Authway API    │
│  Claims Service │
└────┬───────┬────┘
     │       │
     │       │ 2. Store pending claims (Redis)
     │       ▼
     │    ┌───────┐
     │    │ Redis │
     │    └───────┘
     │
     │ 3. Store permanent claims (PostgreSQL)
     ▼
┌──────────────┐
│  PostgreSQL  │
│ user_claims  │
└──────────────┘
       │
       │ 4. Return re-auth URL
       │    (with prompt=none)
       ▼
┌─────────────┐
│   Client    │ ──► 5. Silent re-authentication
│ (iframe/    │     GET /oauth2/auth?prompt=none
│  redirect)  │
└─────────────┘
       │
       │ 6. New token with updated claims
       ▼
┌─────────────┐
│ Application │
│ (New token) │
└─────────────┘
```

### Flow Details

1. **Client Updates Claims** → POST to `/api/v1/claims`
2. **Authway Stores Claims**:
   - **Redis**: Pending claims (5-minute TTL)
   - **PostgreSQL**: Permanent claims (if requested)
3. **Authway Returns Re-auth URL** → `prompt=none` for silent flow
4. **Client Re-authenticates** → iframe or redirect to re-auth URL
5. **Authway Injects Claims** → During OAuth consent flow
6. **Client Receives New Token** → With updated claims in JWT

---

## 📖 API Reference

### Update Claims

**Endpoint**: `POST /api/v1/claims/update`
**Auth Required**: ✅ Bearer Token

#### Request

```json
{
  "claims": {
    "workspace_id": "ws-production",
    "workspace_name": "Production",
    "role": "developer",
    "permissions": ["read", "write", "deploy"]
  },
  "permanent": false,
  "client_id": "your-client-id",
  "redirect_uri": "http://localhost:3000/callback"
}
```

**Parameters**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `claims` | Object | ✅ | Key-value pairs of claims to update |
| `permanent` | Boolean | ❌ | Store in database (default: `false`) |
| `client_id` | String | ✅ | OAuth client ID for re-auth |
| `redirect_uri` | String | ✅ | Callback URL after re-auth |

#### Response

```json
{
  "success": true,
  "auth_url": "http://localhost:4444/oauth2/auth?response_type=code&client_id=...",
  "message": "Claims updated. Re-authenticate to receive new token with updated claims."
}
```

**Status Codes**:
- `200 OK` - Claims updated successfully
- `400 Bad Request` - Invalid request body
- `401 Unauthorized` - Missing or invalid token
- `500 Internal Server Error` - Server error

---

### Get Current Claims

**Endpoint**: `GET /api/v1/claims`
**Auth Required**: ✅ Bearer Token

#### Response

```json
{
  "user_id": "user-uuid",
  "tenant_id": "tenant-uuid",
  "claims": {
    "workspace_id": "ws-production",
    "workspace_name": "Production",
    "role": "developer"
  },
  "permanent_claims": ["role"],
  "session_claims": ["workspace_id", "workspace_name"]
}
```

---

### Delete Claim

**Endpoint**: `DELETE /api/v1/claims/:claim_key`
**Auth Required**: ✅ Bearer Token

#### Response

```json
{
  "success": true,
  "message": "Claim 'workspace_id' deleted successfully"
}
```

---

## 💻 Implementation Examples

### JavaScript / React

```javascript
// claims-service.js
export async function updateClaims(claims, options = {}) {
  const { permanent = false } = options;

  const response = await fetch('http://localhost:8080/api/v1/claims', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${getAccessToken()}`
    },
    body: JSON.stringify({
      claims,
      permanent,
      client_id: process.env.REACT_APP_CLIENT_ID,
      redirect_uri: `${window.location.origin}/callback`
    })
  });

  const data = await response.json();

  if (!data.success) {
    throw new Error('Failed to update claims');
  }

  return data;
}

// Silent re-authentication in hidden iframe
export function silentReauth(authUrl) {
  return new Promise((resolve, reject) => {
    const iframe = document.createElement('iframe');
    iframe.style.display = 'none';
    iframe.src = authUrl;

    // Listen for the callback
    window.addEventListener('message', function handler(event) {
      if (event.data.type === 'auth_complete') {
        window.removeEventListener('message', handler);
        document.body.removeChild(iframe);
        resolve(event.data.tokens);
      }
    });

    iframe.onerror = () => {
      document.body.removeChild(iframe);
      reject(new Error('Silent re-authentication failed'));
    };

    document.body.appendChild(iframe);
  });
}

// Usage in React component
function WorkspaceSwitcher() {
  const [loading, setLoading] = useState(false);

  async function handleWorkspaceChange(workspaceId) {
    setLoading(true);

    try {
      // Update claims
      const { auth_url } = await updateClaims({
        workspace_id: workspaceId,
        workspace_name: getWorkspaceName(workspaceId)
      });

      // Silent re-authentication
      const tokens = await silentReauth(auth_url);

      // Update local token storage
      setAccessToken(tokens.access_token);
      setRefreshToken(tokens.refresh_token);

      // Refresh UI
      window.location.reload();
    } catch (error) {
      console.error('Workspace switch failed:', error);
      alert('Failed to switch workspace');
    } finally {
      setLoading(false);
    }
  }

  return (
    <select onChange={e => handleWorkspaceChange(e.target.value)} disabled={loading}>
      <option value="ws-dev">Development</option>
      <option value="ws-prod">Production</option>
    </select>
  );
}
```

---

### Python / Flask

```python
# claims_service.py
import requests
from typing import Dict, Any, Optional

class ClaimsService:
    def __init__(self, api_base_url: str, client_id: str, redirect_uri: str):
        self.api_base_url = api_base_url
        self.client_id = client_id
        self.redirect_uri = redirect_uri

    def update_claims(
        self,
        access_token: str,
        claims: Dict[str, Any],
        permanent: bool = False
    ) -> Dict[str, Any]:
        """Update user claims and get re-authentication URL"""

        response = requests.post(
            f'{self.api_base_url}/api/v1/claims',
            headers={
                'Content-Type': 'application/json',
                'Authorization': f'Bearer {access_token}'
            },
            json={
                'claims': claims,
                'permanent': permanent,
                'client_id': self.client_id,
                'redirect_uri': self.redirect_uri
            }
        )

        response.raise_for_status()
        return response.json()

    def get_claims(self, access_token: str) -> Dict[str, Any]:
        """Get current user claims"""

        response = requests.get(
            f'{self.api_base_url}/api/v1/claims',
            headers={'Authorization': f'Bearer {access_token}'}
        )

        response.raise_for_status()
        return response.json()

# Usage in Flask app
from flask import Flask, session, redirect

app = Flask(__name__)
claims_service = ClaimsService(
    api_base_url='http://localhost:8080',
    client_id='your-client-id',
    redirect_uri='http://localhost:5000/callback'
)

@app.route('/switch-workspace', methods=['POST'])
def switch_workspace():
    workspace_id = request.json.get('workspace_id')
    access_token = session.get('access_token')

    # Update claims
    result = claims_service.update_claims(
        access_token=access_token,
        claims={
            'workspace_id': workspace_id,
            'workspace_name': get_workspace_name(workspace_id)
        }
    )

    # Redirect to re-authentication URL
    return redirect(result['auth_url'])
```

---

### Go

```go
// claims.go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

type ClaimsService struct {
    APIBaseURL  string
    ClientID    string
    RedirectURI string
}

type UpdateClaimsRequest struct {
    Claims      map[string]interface{} `json:"claims"`
    Permanent   bool                   `json:"permanent"`
    ClientID    string                 `json:"client_id"`
    RedirectURI string                 `json:"redirect_uri"`
}

type UpdateClaimsResponse struct {
    Success bool   `json:"success"`
    AuthURL string `json:"auth_url"`
    Message string `json:"message"`
}

func (s *ClaimsService) UpdateClaims(accessToken string, claims map[string]interface{}, permanent bool) (*UpdateClaimsResponse, error) {
    reqBody := UpdateClaimsRequest{
        Claims:      claims,
        Permanent:   permanent,
        ClientID:    s.ClientID,
        RedirectURI: s.RedirectURI,
    }

    jsonData, err := json.Marshal(reqBody)
    if err != nil {
        return nil, err
    }

    req, err := http.NewRequest("POST", s.APIBaseURL+"/api/v1/claims", bytes.NewBuffer(jsonData))
    if err != nil {
        return nil, err
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+accessToken)

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("API error: %d", resp.StatusCode)
    }

    var result UpdateClaimsResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    return &result, nil
}

// Usage example
func handleWorkspaceSwitch(w http.ResponseWriter, r *http.Request) {
    claimsService := &ClaimsService{
        APIBaseURL:  "http://localhost:8080",
        ClientID:    "your-client-id",
        RedirectURI: "http://localhost:9001/callback",
    }

    accessToken := getAccessToken(r)

    result, err := claimsService.UpdateClaims(
        accessToken,
        map[string]interface{}{
            "workspace_id":   "ws-production",
            "workspace_name": "Production",
        },
        false, // Not permanent
    )

    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Redirect to re-auth URL
    http.Redirect(w, r, result.AuthURL, http.StatusFound)
}
```

---

### C# / .NET

```csharp
// ClaimsService.cs
using System;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Threading.Tasks;

public class ClaimsService
{
    private readonly HttpClient _httpClient;
    private readonly string _clientId;
    private readonly string _redirectUri;

    public ClaimsService(string apiBaseUrl, string clientId, string redirectUri)
    {
        _httpClient = new HttpClient { BaseAddress = new Uri(apiBaseUrl) };
        _clientId = clientId;
        _redirectUri = redirectUri;
    }

    public async Task<UpdateClaimsResponse> UpdateClaimsAsync(
        string accessToken,
        Dictionary<string, object> claims,
        bool permanent = false)
    {
        var request = new UpdateClaimsRequest
        {
            Claims = claims,
            Permanent = permanent,
            ClientId = _clientId,
            RedirectUri = _redirectUri
        };

        var json = JsonSerializer.Serialize(request);
        var content = new StringContent(json, Encoding.UTF8, "application/json");

        var httpRequest = new HttpRequestMessage(HttpMethod.Post, "/api/v1/claims")
        {
            Content = content
        };
        httpRequest.Headers.Authorization = new AuthenticationHeaderValue("Bearer", accessToken);

        var response = await _httpClient.SendAsync(httpRequest);
        response.EnsureSuccessStatusCode();

        var responseBody = await response.Content.ReadAsStringAsync();
        return JsonSerializer.Deserialize<UpdateClaimsResponse>(responseBody);
    }
}

public class UpdateClaimsRequest
{
    [JsonPropertyName("claims")]
    public Dictionary<string, object> Claims { get; set; }

    [JsonPropertyName("permanent")]
    public bool Permanent { get; set; }

    [JsonPropertyName("client_id")]
    public string ClientId { get; set; }

    [JsonPropertyName("redirect_uri")]
    public string RedirectUri { get; set; }
}

public class UpdateClaimsResponse
{
    [JsonPropertyName("success")]
    public bool Success { get; set; }

    [JsonPropertyName("auth_url")]
    public string AuthUrl { get; set; }

    [JsonPropertyName("message")]
    public string Message { get; set; }
}

// Usage in ASP.NET Core
[HttpPost("switch-workspace")]
public async Task<IActionResult> SwitchWorkspace([FromBody] SwitchWorkspaceRequest request)
{
    var claimsService = new ClaimsService(
        "http://localhost:8080",
        "your-client-id",
        "http://localhost:5000/callback"
    );

    var accessToken = HttpContext.Session.GetString("access_token");

    var result = await claimsService.UpdateClaimsAsync(
        accessToken,
        new Dictionary<string, object>
        {
            ["workspace_id"] = request.WorkspaceId,
            ["workspace_name"] = request.WorkspaceName
        }
    );

    return Redirect(result.AuthUrl);
}
```

---

## 🔒 Security Considerations

### 1. Token Lifetime

- **Access tokens**: 15 minutes (recommended)
- **Pending claims**: 5 minutes (Redis TTL)
- **Login claims**: 10 minutes (Redis TTL)

### 2. Claim Validation

Always validate claims on the server side:

```javascript
// ❌ DON'T trust claims blindly
if (claims.role === 'admin') {
  // Grant admin access
}

// ✅ DO validate against database/permissions
async function canAccessAdminPanel(userId, claims) {
  const user = await db.users.findById(userId);
  return user.role === 'admin' && claims.role === 'admin';
}
```

### 3. Silent Re-authentication

Use `prompt=none` carefully:

- ✅ Good for workspace switching
- ✅ Good for claim updates
- ❌ Bad for initial login
- ❌ Bad for sensitive operations

### 4. CSRF Protection

Validate `state` parameter in OAuth flow:

```javascript
// Generate and store state
const state = crypto.randomUUID();
sessionStorage.setItem('oauth_state', state);

// Validate on callback
const receivedState = new URLSearchParams(window.location.search).get('state');
const storedState = sessionStorage.getItem('oauth_state');

if (receivedState !== storedState) {
  throw new Error('CSRF attack detected');
}
```

---

## 🧪 Testing

### Manual Testing

1. **Start Sample Service**:
```bash
cd samples/AppleService
go run main.go
```

2. **Open Test Page**:
```
http://localhost:9001/test-workspace
```

3. **Select Workspace** → Observe token refresh

### Automated Testing

```javascript
// claims.test.js
describe('Dynamic Claims', () => {
  it('should update claims and receive new token', async () => {
    const result = await updateClaims({
      workspace_id: 'ws-test'
    });

    expect(result.success).toBe(true);
    expect(result.auth_url).toContain('prompt=none');

    // Perform silent re-auth
    const tokens = await silentReauth(result.auth_url);
    const decodedToken = jwt.decode(tokens.access_token);

    expect(decodedToken.workspace_id).toBe('ws-test');
  });
});
```

---

## ❓ FAQ

### Q: What happens if Redis is down?

**A**: Pending claims won't work, but permanent claims will still be available from PostgreSQL on next login.

### Q: Can I update claims without triggering re-authentication?

**A**: No. To ensure security and token consistency, re-authentication is required.

### Q: What's the difference between permanent and session claims?

**A**:
- **Permanent claims**: Stored in PostgreSQL, survive across sessions
- **Session claims**: Stored in Redis, only for current session (5-minute TTL)

### Q: Can I update another user's claims?

**A**: No. Claims are scoped to the authenticated user's token.

### Q: Does `prompt=none` work on mobile?

**A**: It works best in web browsers. For mobile apps, consider using refresh tokens instead.

---

## 📚 Related Documentation

- [API Introduction](./API_INTRODUCTION.md)
- [Multi-Tenancy Architecture](./architecture/multi-tenancy.md)
- [OAuth 2.0 Integration Guide](./INTEGRATION_GUIDE.md)

---

**Last Updated**: 2025-10-18
**Maintained by**: Authway Team
