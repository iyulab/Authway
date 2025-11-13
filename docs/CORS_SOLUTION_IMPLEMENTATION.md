# OAuth 2.0 Compliant CORS Solution Implementation

## 📋 Executive Summary

**Problem**: Browser-based OAuth 2.0 token exchange fails due to CORS restrictions on Hydra's `/oauth2/token` endpoint, blocking production usage for all SPA clients.

**Root Cause**: Hydra cannot dynamically validate origins per client, forcing either:
- Wildcard CORS (`*`) → Security risk
- Static origins → Doesn't scale for multi-tenant

**Solution**: Reverse proxy (Traefik) with dynamic CORS validation against database-stored `allowed_origins`, maintaining full OAuth 2.0 RFC compliance.

**Status**: Implementation complete, ready for testing and deployment.

---

## 🎯 Solution Architecture

### Standards Compliance Analysis

✅ **OAuth 2.0 RFC 6749 Compliant**:
- Token endpoint URL unchanged (`/oauth2/token`)
- Client behavior unchanged (direct endpoint call)
- PKCE flow (RFC 7636) unchanged
- All OAuth client libraries remain compatible

✅ **Industry Standard Pattern**:
- Same architecture as Auth0, Okta, Keycloak
- Reverse proxy handles CORS, not application layer
- Dynamic origin validation per client

❌ **Alternative Rejected** (Token Exchange Proxy):
- Would violate RFC 6749 (token endpoint URL change)
- Breaks standard OAuth library compatibility
- Contradicts "standards-compliant provider" identity

### Architecture Diagram

```
┌──────────────────────────────────────────────────────┐
│ OAuth 2.0 Client (SPA)                                │
│ https://manuals.alldot.ai                            │
└────────────────┬─────────────────────────────────────┘
                 │
                 │ POST /oauth2/token
                 │ Origin: https://manuals.alldot.ai
                 │ Body: grant_type, code, code_verifier, client_id
                 ↓
┌──────────────────────────────────────────────────────┐
│ Traefik Reverse Proxy (New Component)               │
│ https://oauth.authway.in                             │
│                                                       │
│ CORS Validation Logic:                               │
│ 1. Extract origin from request header                │
│ 2. Extract client_id from request body               │
│ 3. Query: SELECT allowed_origins                     │
│           FROM clients WHERE client_id = ?           │
│           (with 5-min cache)                         │
│ 4. IF origin IN allowed_origins:                     │
│       Add CORS headers, proxy to Hydra               │
│    ELSE:                                              │
│       Return 403 Forbidden                           │
└────────────────┬─────────────────────────────────────┘
                 │
                 │ (If origin allowed)
                 ↓
┌──────────────────────────────────────────────────────┐
│ Ory Hydra (OAuth 2.0 Server)                         │
│ http://hydra:4444/oauth2/token (internal)            │
│                                                       │
│ CORS: Disabled (handled by Traefik)                  │
└──────────────────────────────────────────────────────┘
```

---

## 🔧 Implementation Details

### 1. Database Changes

**Migration**: `scripts/migrations/002_add_allowed_origins.sql`

```sql
-- Add allowed_origins column to clients table
ALTER TABLE clients
ADD COLUMN IF NOT EXISTS allowed_origins TEXT[] DEFAULT '{}';

-- Create GIN index for fast lookups
CREATE INDEX IF NOT EXISTS idx_clients_allowed_origins
ON clients USING GIN (allowed_origins);
```

**Rollback**: `scripts/migrations/ROLLBACK_002.sql`

**Model Update**: `apps/central/api/pkg/client/models.go`

```go
type Client struct {
    // ... existing fields
    AllowedOrigins pq.StringArray `json:"allowed_origins" gorm:"type:text[];column:allowed_origins;default:'{}'"`
}

type CreateClientRequest struct {
    // ... existing fields
    AllowedOrigins []string `json:"allowed_origins" validate:"omitempty,dive,url"`
}
```

**Service Update**: Automatic handling in create/update operations.

### 2. Reverse Proxy Configuration

**Component**: Traefik v2.10

**Configuration Files**:
- `configs/traefik.yml` - Static configuration
- `configs/traefik-dynamic.yml` - Dynamic routing and CORS
- `scripts/traefik-cors-plugin.go` - Custom dynamic CORS plugin

**Key Features**:
- Database connection pooling
- 5-minute result caching
- Wildcard subdomain support (`*.example.com`)
- Automatic preflight (OPTIONS) handling
- Rate limiting (10 req/sec per client)

**Docker Compose**: `docker-compose.proxy.yml`

### 3. CORS Validation Logic

```go
func isOriginAllowed(clientID, origin string) (bool, error) {
    // 1. Check cache (5-min TTL)
    if entry, ok := cache[clientID]; ok && !expired(entry) {
        return contains(entry.origins, origin), nil
    }

    // 2. Query database
    var allowedOrigins []string
    rows := db.Query(`
        SELECT unnest(allowed_origins)
        FROM clients
        WHERE client_id = $1 AND active = true
    `, clientID)

    // 3. Cache result
    cache[clientID] = {origins: allowedOrigins, expiresAt: now + 5min}

    // 4. Validate
    return contains(allowedOrigins, origin), nil
}
```

### 4. Client Registration Flow

**API Endpoint**: `POST /api/v1/clients`

```json
{
  "tenant_id": "uuid",
  "name": "All.Manual",
  "public": true,
  "redirect_uris": ["https://manuals.alldot.ai/callback"],
  "allowed_origins": [
    "https://manuals.alldot.ai",
    "https://nice-moss-08ac84200.3.azurestaticapps.net"
  ],
  "grant_types": ["authorization_code", "refresh_token"],
  "scopes": ["openid", "profile", "email"]
}
```

**Response**: Includes `allowed_origins` in public client data.

---

## 📦 Deployment Guide

### Local Development

```bash
# 1. Run database migration
psql -U authway -d authway < scripts/migrations/002_add_allowed_origins.sql

# 2. Start with Traefik proxy
docker-compose -f docker-compose.dev.yml -f docker-compose.proxy.yml up -d

# 3. Update /etc/hosts (Windows: C:\Windows\System32\drivers\etc\hosts)
127.0.0.1 oauth.authway.local
127.0.0.1 auth.authway.local
127.0.0.1 login.authway.local
127.0.0.1 admin.authway.local

# 4. Test CORS
curl -X OPTIONS http://oauth.authway.local/oauth2/token \
  -H "Origin: http://localhost:3000" \
  -H "Access-Control-Request-Method: POST" \
  -v
```

### Azure Container Apps

**Full Guide**: `docs/deployment/AZURE_CORS_DEPLOYMENT.md`

**Quick Steps**:
1. Run database migration on Azure PostgreSQL
2. Deploy Traefik container app
3. Update Hydra ingress to internal-only
4. Configure DNS (oauth.authway.in → Traefik)
5. Test with production clients

**Estimated Cost**: +$30-150/month for Traefik (2-10 replicas)

---

## 🧪 Testing Checklist

### Functional Tests

- [ ] Allowed origin returns CORS headers
- [ ] Disallowed origin returns 403
- [ ] OPTIONS preflight works correctly
- [ ] Token exchange succeeds with allowed origin
- [ ] Cache reduces database queries (check logs)
- [ ] Wildcard origins work (`*.example.com`)
- [ ] Multiple origins per client work

### Integration Tests

- [ ] `@authway/client` SDK works without changes
- [ ] Standard OAuth libraries (oauth4webapi) compatible
- [ ] All.Manual production flow works end-to-end
- [ ] Popup and redirect modes both work
- [ ] Token refresh works

### Performance Tests

- [ ] <50ms CORS validation latency (cached)
- [ ] <200ms CORS validation latency (uncached)
- [ ] 1000 req/sec sustained load
- [ ] Cache hit rate >90% after warmup
- [ ] Database connection pool stable

### Security Tests

- [ ] Disallowed origins blocked
- [ ] SQL injection attempts fail
- [ ] Rate limiting works (10 req/sec)
- [ ] Cache poisoning attempts fail
- [ ] Invalid client_id handled gracefully

---

## 📊 Comparison: Rejected vs. Accepted Solution

| Aspect | ❌ Token Proxy (Rejected) | ✅ Reverse Proxy (Accepted) |
|--------|---------------------------|----------------------------|
| **RFC 6749 Compliance** | Violates (URL change) | Compliant |
| **Token Endpoint URL** | `/api/oauth/token` (custom) | `/oauth2/token` (standard) |
| **OAuth Library Compat** | Breaks standard libraries | All libraries work |
| **SDK Required** | Authway SDK mandatory | SDK optional |
| **Identity Alignment** | "Custom auth system" | "Standard OAuth provider" |
| **Auth0/Okta Pattern** | Different | Same |
| **Implementation** | Backend code change | Infrastructure config |
| **Rollback Complexity** | High (SDK changes) | Low (remove Traefik) |
| **Long-term Flexibility** | Vendor lock-in | Standard-compliant |

---

## 🚀 Migration Path

### Phase 1: Database & API (Complete)
- ✅ Add `allowed_origins` column
- ✅ Update Client model
- ✅ Update service layer
- ✅ Update API requests/responses

### Phase 2: Local Testing
- [ ] Deploy Traefik locally
- [ ] Test with sample clients
- [ ] Validate cache performance
- [ ] Document any issues

### Phase 3: Staging Deployment
- [ ] Deploy to Azure staging environment
- [ ] Test with All.Manual staging app
- [ ] Load testing
- [ ] Security audit

### Phase 4: Production Rollout
- [ ] Deploy Traefik to production
- [ ] Update DNS gradually (canary)
- [ ] Monitor error rates
- [ ] Full cutover

### Phase 5: Client Migration
- [ ] Update existing clients with `allowed_origins`
- [ ] Notify clients of changes
- [ ] Update documentation
- [ ] Provide migration support

---

## 📝 API Usage Examples

### Create Client with Allowed Origins

```bash
curl -X POST https://api.authway.in/api/v1/clients \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "a1b2c3d4-...",
    "name": "My SPA App",
    "public": true,
    "redirect_uris": ["https://app.example.com/callback"],
    "allowed_origins": [
      "https://app.example.com",
      "https://staging.app.example.com"
    ],
    "grant_types": ["authorization_code", "refresh_token"],
    "scopes": ["openid", "profile", "email"]
  }'
```

### Update Client Origins

```bash
curl -X PUT https://api.authway.in/api/v1/clients/$CLIENT_ID \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "allowed_origins": [
      "https://app.example.com",
      "https://staging.app.example.com",
      "https://dev.app.example.com"
    ]
  }'
```

### Query Client Origins

```bash
curl https://api.authway.in/api/v1/clients/$CLIENT_ID \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Response includes:
{
  "client_id": "authway_...",
  "allowed_origins": ["https://app.example.com"],
  ...
}
```

---

## 🔧 Troubleshooting

### Common Issues

**CORS still fails after deployment**:
1. Check `allowed_origins` in database
2. Verify Traefik is proxying correctly
3. Check Traefik logs for errors
4. Verify DNS points to Traefik, not Hydra

**Database connection errors**:
1. Check Traefik environment variables
2. Verify PostgreSQL allows Traefik IP
3. Check connection pooling limits

**Cache not working**:
1. Verify cache TTL configuration
2. Check for cache invalidation bugs
3. Monitor cache hit rate metrics

---

## 📚 References

- **OAuth 2.0 RFC 6749**: https://datatracker.ietf.org/doc/html/rfc6749
- **PKCE RFC 7636**: https://datatracker.ietf.org/doc/html/rfc7636
- **Traefik Documentation**: https://doc.traefik.io/traefik/
- **Ory Hydra CORS**: https://www.ory.sh/docs/hydra/reference/configuration

---

**Document Version**: 1.0
**Last Updated**: 2025-11-13
**Author**: Authway Development Team
**Review Status**: Ready for Implementation
