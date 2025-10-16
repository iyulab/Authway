# Ory Hydra Azure Deployment

## Current Status

✅ **Completed**:
- Hydra deployed to Azure Container Apps (`authway-hydra`)
- Database migration completed successfully
- Public API (OAuth2/OIDC) accessible at: `https://authway-hydra.lemonfield-03f5fb88.koreacentral.azurecontainerapps.io`
- OpenID Connect discovery endpoint working: `/.well-known/openid-configuration`
- Authway backend configured with Hydra URLs

⚠️ **Known Issue**:
- Hydra Admin API (port 4445) is NOT accessible from authway-api
- Reason: Azure Container Apps only exposes ports configured via ingress
- Current ingress only exposes port 4444 (public API)

## Architecture

```
┌─────────────────────┐
│   OAuth Clients     │ (ASP.NET, React, etc.)
│  (External Traffic) │
└──────────┬──────────┘
           │ HTTPS
           ▼
┌─────────────────────────────────────┐
│  Azure Container Apps (authway-env) │
│  ┌───────────────────────────────┐  │
│  │   authway-hydra               │  │
│  │   ┌─────────────────────────┐ │  │
│  │   │  Hydra Container        │ │  │
│  │   │  - Port 4444: Public ✅ │ │  │ <-- Accessible (ingress)
│  │   │  - Port 4445: Admin  ❌ │ │  │ <-- NOT accessible (no ingress)
│  │   └─────────────────────────┘ │  │
│  └───────────────────────────────┘  │
│  ┌───────────────────────────────┐  │
│  │   authway-api                 │  │
│  │   (needs admin API access)    │  │
│  └───────────────────────────────┘  │
└─────────────────────────────────────┘
```

## Test Results

**DNS Resolution**: ✅ Working
```
authway-hydra resolves to 100.100.246.69 (internal IP)
```

**Port 4444 (Public API)**: ✅ Accessible
```bash
curl https://authway-hydra.lemonfield-03f5fb88.koreacentral.azurecontainerapps.io/.well-known/openid-configuration
# Returns valid OIDC configuration
```

**Port 4445 (Admin API)**: ❌ Connection Timeout
```bash
# From inside authway-api container:
wget http://authway-hydra:4445/health/ready
# Result: Connection timeout (port not exposed)
```

## Solution Options

### Option 1: Nginx Sidecar (Recommended)

Add nginx as a sidecar container to route both APIs through a single port.

**Pros**:
- Secure (admin API not publicly exposed)
- Cost-effective (minimal overhead)
- Clean architecture

**Cons**:
- Requires custom nginx configuration
- Adds complexity

**Implementation**:
```yaml
containers:
  - name: nginx
    image: nginx:alpine
    resources:
      cpu: 0.25
      memory: 0.5Gi
    volumeMounts:
      - name: nginx-config
        mountPath: /etc/nginx/nginx.conf
        subPath: nginx.conf
  - name: hydra
    image: oryd/hydra:v2.2
    # ... existing config
```

**Nginx Config**:
```nginx
server {
    listen 8080;

    # Public API (OAuth2/OIDC)
    location / {
        proxy_pass http://localhost:4444;
    }

    # Admin API (internal use)
    location /admin/ {
        proxy_pass http://localhost:4445/;
    }
}
```

**Updated URLs**:
- Public: `https://authway-hydra.../` → Hydra :4444
- Admin: `http://authway-hydra.internal.../admin` → Hydra :4445

### Option 2: Separate Container Apps

Create two Container Apps sharing the same Hydra database.

**Pros**:
- Simple configuration
- Clear separation

**Cons**:
- ❌ **Does NOT work**: Hydra requires both APIs in SAME process
- Doubles compute costs
- State synchronization issues

**Status**: ⛔ Not viable

### Option 3: Expose Admin Port Publicly with IP Restrictions

Add IP security restrictions to limit admin API access.

**Pros**:
- Simple to implement
- Works immediately

**Cons**:
- Less secure (admin API exposed)
- Requires maintenance of IP allowlist
- Azure Container Apps IP restrictions are coarse-grained

**Implementation**:
```bash
# Update ingress to expose port 4445
az containerapp update \
  --name authway-hydra-admin \
  --resource-group authway \
  --set-env-vars "TARGET_PORT=4445" \
  --ingress external \
  --target-port 4445

# Add IP restrictions (authway-api outbound IPs)
# Configure via Azure Portal or CLI
```

**Status**: ⚠️ Temporary workaround only

### Option 4: Dapr Service Mesh

Use Dapr for service-to-service communication.

**Pros**:
- Azure Container Apps native support
- Advanced features (retries, circuit breaker)

**Cons**:
- Overkill for this use case
- Additional complexity
- Learning curve

**Status**: 🤔 Consider for future

## Recommended Action

**Implement Option 1 (Nginx Sidecar)** for production-ready deployment.

**Steps**:
1. Create nginx configuration file
2. Update `hydra-containerapp.yaml` to include nginx sidecar
3. Mount nginx config as volume
4. Update ingress target port to 8080 (nginx)
5. Update `AUTHWAY_HYDRA_ADMIN_URL` to use nginx admin path
6. Redeploy Hydra Container App

## Current Configuration

**Environment**: `authway-env` (Korea Central)

**Hydra Public API**:
- URL: `https://authway-hydra.lemonfield-03f5fb88.koreacentral.azurecontainerapps.io`
- Port: 4444 (exposed via ingress)
- Target: OAuth clients, ASP.NET samples

**Hydra Admin API**:
- URL: `http://authway-hydra:4445` (configured but not accessible)
- Port: 4445 (NOT exposed)
- Target: authway-api (login/consent flows)

**Authway Backend**:
- `AUTHWAY_HYDRA_PUBLIC_URL`: https://authway-hydra.lemonfield-03f5fb88.koreacentral.azurecontainerapps.io
- `AUTHWAY_HYDRA_ADMIN_URL`: http://authway-hydra:4445 ⚠️ (not working)

**Database**:
- Host: authway-postgres.postgres.database.azure.com
- Database: authway (shared with Authway backend)
- Schema: Hydra tables migrated successfully

**Secrets**:
- `SECRETS_SYSTEM`: b2ea992899147b0d712ddff895c00c54ced6c2b16628892aebba6e88ce3dbd5d

## Resources

- Hydra YAML: `scripts/hydra-containerapp.yaml`
- Container App: `authway-hydra`
- Resource Group: `authway`
- Environment: `authway-env`

## Next Steps

1. **Decision Required**: Choose solution approach (recommend Option 1)
2. **Implement Solution**: Update configuration based on chosen option
3. **Test OAuth Flow**: Verify end-to-end authentication works
4. **Update ASP.NET Sample**: Update README to point to Azure deployment
5. **Monitor**: Check Hydra logs and performance

## Related Documentation

- [Ory Hydra Documentation](https://www.ory.sh/docs/hydra)
- [Azure Container Apps Networking](https://learn.microsoft.com/azure/container-apps/networking)
- [ASP.NET Sample README](../samples/asp-sample/README.md)
