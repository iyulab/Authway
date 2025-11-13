# Azure Container Apps Deployment with Dynamic CORS

## Overview

This guide covers deploying Authway with Traefik reverse proxy to Azure Container Apps to solve the OAuth 2.0 token endpoint CORS issue while maintaining standards compliance.

## Architecture

```
┌─────────────────────────────────────────────────┐
│ Client (SPA)                                     │
│ https://manuals.alldot.ai                       │
└────────────────┬────────────────────────────────┘
                 │
                 │ POST /oauth2/token
                 │ Origin: https://manuals.alldot.ai
                 ↓
┌─────────────────────────────────────────────────┐
│ Azure Container App: Traefik Reverse Proxy     │
│ https://oauth.authway.in                        │
│                                                  │
│ 1. Extract client_id from request body          │
│ 2. Query PostgreSQL:                            │
│    SELECT allowed_origins                       │
│    FROM clients WHERE client_id = ?            │
│ 3. Validate: origin IN allowed_origins          │
│ 4. Add CORS headers if valid                    │
│ 5. Proxy to Hydra                               │
└────────────────┬────────────────────────────────┘
                 │
                 ↓
┌─────────────────────────────────────────────────┐
│ Azure Container App: Ory Hydra                  │
│ Internal: http://hydra:4444/oauth2/token        │
└─────────────────────────────────────────────────┘
```

## Prerequisites

1. Azure CLI installed and configured
2. Azure subscription with Container Apps enabled
3. PostgreSQL database with `allowed_origins` column
4. Domain names configured (oauth.authway.in, auth.authway.in)

## Step 1: Database Migration

Run the migration to add `allowed_origins` column:

```bash
# Connect to Azure PostgreSQL
az postgres flexible-server connect \
  --name authway-db \
  --admin-user authway \
  --database authway

# Run migration
\i scripts/migrations/002_add_allowed_origins.sql
```

## Step 2: Update Client Configuration

Add allowed origins for your clients:

```sql
-- Update All.Manual client
UPDATE clients
SET allowed_origins = ARRAY[
  'https://manuals.alldot.ai',
  'https://nice-moss-08ac84200.3.azurestaticapps.net'
]
WHERE client_id = 'authway_2qfEM6ccGYfmxh8bC6hjng';

-- Verify
SELECT client_id, name, allowed_origins FROM clients;
```

## Step 3: Deploy Traefik Container App

### 3.1 Create Traefik Configuration Secret

```bash
# Create configuration files as secrets
az containerapp env secret set \
  --name authway-env \
  --resource-group authway \
  --secrets \
    traefik-config="$(cat configs/traefik.yml | base64)" \
    traefik-dynamic="$(cat configs/traefik-dynamic.yml | base64)"
```

### 3.2 Deploy Traefik Container

```bash
az containerapp create \
  --name authway-traefik \
  --resource-group authway \
  --environment authway-env \
  --image traefik:v2.10 \
  --target-port 80 \
  --ingress external \
  --min-replicas 2 \
  --max-replicas 10 \
  --cpu 0.5 \
  --memory 1.0Gi \
  --env-vars \
    POSTGRES_HOST=authway-db.postgres.database.azure.com \
    POSTGRES_PORT=5432 \
    POSTGRES_USER=authway \
    POSTGRES_PASSWORD=secretref:postgres-password \
    POSTGRES_DB=authway \
  --secrets \
    postgres-password="${POSTGRES_PASSWORD}" \
  --registry-server docker.io
```

### 3.3 Configure Custom Domain

```bash
# Add custom domain for oauth.authway.in
az containerapp hostname add \
  --name authway-traefik \
  --resource-group authway \
  --hostname oauth.authway.in

# Bind SSL certificate
az containerapp hostname bind \
  --name authway-traefik \
  --resource-group authway \
  --hostname oauth.authway.in \
  --environment authway-env \
  --validation-method CNAME
```

## Step 4: Update Hydra Container App

Update Hydra to route through Traefik:

```bash
# Update Hydra ingress to internal only
az containerapp ingress update \
  --name authway-hydra \
  --resource-group authway \
  --type internal \
  --target-port 4444

# Update CORS settings (allow all, validation happens at Traefik)
az containerapp update \
  --name authway-hydra \
  --resource-group authway \
  --set-env-vars \
    SERVE_PUBLIC_CORS_ENABLED=true \
    SERVE_PUBLIC_CORS_ALLOWED_ORIGINS="*"
```

## Step 5: Update DNS Records

Configure DNS to point to Traefik:

```bash
# Get Traefik FQDN
TRAEFIK_FQDN=$(az containerapp show \
  --name authway-traefik \
  --resource-group authway \
  --query properties.configuration.ingress.fqdn \
  --output tsv)

# Update DNS records
# oauth.authway.in -> CNAME -> $TRAEFIK_FQDN
# auth.authway.in -> CNAME -> $TRAEFIK_FQDN
```

## Step 6: Test CORS Configuration

### 6.1 Test Allowed Origin

```bash
# Test with allowed origin
curl -X POST https://oauth.authway.in/oauth2/token \
  -H "Origin: https://manuals.alldot.ai" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&client_id=authway_2qfEM6ccGYfmxh8bC6hjng&code=test_code&code_verifier=test_verifier" \
  -v

# Should return:
# Access-Control-Allow-Origin: https://manuals.alldot.ai
# Access-Control-Allow-Credentials: true
```

### 6.2 Test Disallowed Origin

```bash
# Test with disallowed origin
curl -X POST https://oauth.authway.in/oauth2/token \
  -H "Origin: https://evil.com" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&client_id=authway_2qfEM6ccGYfmxh8bC6hjng&code=test_code&code_verifier=test_verifier" \
  -v

# Should return:
# HTTP/1.1 403 Forbidden
```

### 6.3 Test Preflight Request

```bash
# Test OPTIONS preflight
curl -X OPTIONS https://oauth.authway.in/oauth2/token \
  -H "Origin: https://manuals.alldot.ai" \
  -H "Access-Control-Request-Method: POST" \
  -H "Access-Control-Request-Headers: Content-Type" \
  -v

# Should return:
# Access-Control-Allow-Origin: https://manuals.alldot.ai
# Access-Control-Allow-Methods: POST, OPTIONS
# Access-Control-Max-Age: 3600
```

## Step 7: Update SDK Configuration (Optional)

If using `@authway/client` SDK, no changes needed! The SDK already calls `/oauth2/token` directly.

```typescript
// SDK configuration remains the same
const authClient = new AuthClient({
  domain: 'https://auth.authway.in',
  oauthUrl: 'https://oauth.authway.in',  // Now proxied by Traefik
  clientId: 'authway_2qfEM6ccGYfmxh8bC6hjng',
  redirectUri: 'https://manuals.alldot.ai/my-manuals'
});
```

## Step 8: Monitoring and Logging

### 8.1 Enable Application Insights

```bash
# Create Application Insights
az monitor app-insights component create \
  --app authway-traefik-insights \
  --location eastus \
  --resource-group authway \
  --application-type web

# Get instrumentation key
INSTRUMENTATION_KEY=$(az monitor app-insights component show \
  --app authway-traefik-insights \
  --resource-group authway \
  --query instrumentationKey \
  --output tsv)

# Update Traefik with Application Insights
az containerapp update \
  --name authway-traefik \
  --resource-group authway \
  --set-env-vars \
    APPLICATIONINSIGHTS_CONNECTION_STRING="InstrumentationKey=$INSTRUMENTATION_KEY"
```

### 8.2 Query Logs

```bash
# View CORS validation logs
az monitor app-insights query \
  --app authway-traefik-insights \
  --resource-group authway \
  --analytics-query "traces | where message contains 'CORS' | order by timestamp desc | take 100"
```

## Troubleshooting

### CORS Still Not Working

1. **Check client allowed_origins**:
   ```sql
   SELECT client_id, allowed_origins FROM clients WHERE client_id = 'your_client_id';
   ```

2. **Check Traefik logs**:
   ```bash
   az containerapp logs show \
     --name authway-traefik \
     --resource-group authway \
     --tail 100
   ```

3. **Verify DNS**:
   ```bash
   nslookup oauth.authway.in
   ```

4. **Test database connectivity from Traefik**:
   ```bash
   az containerapp exec \
     --name authway-traefik \
     --resource-group authway \
     --command "pg_isready -h authway-db.postgres.database.azure.com -p 5432"
   ```

### Performance Issues

1. **Enable database connection pooling**
2. **Increase Traefik cache TTL** (default: 5 minutes)
3. **Scale Traefik replicas**:
   ```bash
   az containerapp update \
     --name authway-traefik \
     --resource-group authway \
     --min-replicas 3 \
     --max-replicas 20
   ```

## Rollback Plan

If issues occur, rollback using:

```bash
# Remove Traefik
az containerapp delete \
  --name authway-traefik \
  --resource-group authway \
  --yes

# Restore Hydra direct access
az containerapp ingress update \
  --name authway-hydra \
  --resource-group authway \
  --type external \
  --target-port 4444

# Temporarily allow all origins (NOT FOR PRODUCTION)
az containerapp update \
  --name authway-hydra \
  --resource-group authway \
  --set-env-vars \
    SERVE_PUBLIC_CORS_ALLOWED_ORIGINS="*"

# Rollback database
psql -U authway -d authway < scripts/migrations/ROLLBACK_002.sql
```

## Cost Estimation

**Azure Container Apps** (per month):
- Traefik: 2-10 replicas @ 0.5 vCPU, 1GB RAM ~ $30-150
- Hydra: Unchanged
- PostgreSQL queries: Negligible (cached)

**Total Additional Cost**: ~$30-150/month (minimal for production auth service)

## Security Considerations

1. **PostgreSQL Connection**: Use SSL and connection pooling
2. **Cache Management**: Implement TTL and cache invalidation
3. **Rate Limiting**: Configure per-client rate limits in Traefik
4. **Monitoring**: Set up alerts for:
   - High CORS rejection rate
   - Database connection failures
   - Traefik downtime

## Next Steps

1. Run load testing to validate performance
2. Set up monitoring dashboards
3. Document client onboarding process with `allowed_origins`
4. Update client registration UI to include origin management

---

**Last Updated**: 2025-11-13
**Authway Version**: 0.1.0+cors-fix
