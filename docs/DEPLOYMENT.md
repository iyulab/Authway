# Authway Deployment Guide

**Version**: 0.2.0
**Last Updated**: 2025-11-17

Complete guide for deploying Authway to production environments.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Azure Deployment](#azure-deployment)
- [Docker Deployment](#docker-deployment)
- [CORS Configuration](#cors-configuration)
- [Environment Variables](#environment-variables)
- [Database Migrations](#database-migrations)
- [Production Checklist](#production-checklist)

---

## Prerequisites

- **PostgreSQL** 15+ (Azure Database, RDS, or self-hosted)
- **Container Registry** (Azure Container Registry, Docker Hub, GitHub Packages)
- **Domain & SSL** certificates
- **OAuth Provider** (Ory Hydra deployed)

---

## Azure Deployment

### Architecture

```
Internet → Azure Front Door → Container Apps Environment
  ├─ Hydra (OAuth Server)
  ├─ Central API (Public — admin API and the login screens' backend)
  └─ Static Web Apps (Admin Dashboard, Auth UI)

PostgreSQL ← Container Apps
```

### 1. Create Resources

```bash
# Resource Group
az group create --name authway --location koreacentral

# Container Apps Environment
az containerapp env create \
  --name authway-env \
  --resource-group authway \
  --location koreacentral

# PostgreSQL
az postgres flexible-server create \
  --name authway-db \
  --resource-group authway \
  --location koreacentral \
  --admin-user authwayadmin \
  --admin-password <secure-password> \
  --sku-name Standard_B1ms \
  --tier Burstable \
  --version 15
```

### 2. Deploy Hydra

```bash
az containerapp create \
  --name authway-hydra \
  --resource-group authway \
  --environment authway-env \
  --image oryd/hydra:v26.2.0 \
  --target-port 4444 \
  --ingress external \
  --env-vars \
    DSN="postgres://user:password@authway-db.postgres.database.azure.com/hydra?sslmode=require" \
    SECRETS_SYSTEM="<random-32-char-string>" \
    URLS_SELF_ISSUER="https://oauth.authway.in" \
    URLS_LOGIN="https://api.authway.in/login" \
    URLS_CONSENT="https://api.authway.in/consent" \
    URLS_LOGOUT="https://api.authway.in/logout"
```

### 3. Deploy Central API

```bash
# Build and push Docker image
docker build -t authway-api:latest apps/central/api
docker tag authway-api:latest <registry>/authway-api:latest
docker push <registry>/authway-api:latest

# Deploy to Azure
az containerapp create \
  --name authway-api \
  --resource-group authway \
  --environment authway-env \
  --image <registry>/authway-api:latest \
  --target-port 8080 \
  --ingress external \
  --env-vars \
    DATABASE_URL="postgres://user:password@authway-db.postgres.database.azure.com/authway?sslmode=require" \
    HYDRA_ADMIN_URL="http://authway-hydra:4445" \
    HYDRA_PUBLIC_URL="https://oauth.authway.in"
```

The central API is the login screens' backend too, so it takes external
traffic: the authorization server sends browsers to its `/login`, `/consent`
and `/logout`, which hand the login UI a flow id.

### 4. Deploy Static Web Apps

```bash
# Admin Dashboard
az staticwebapp create \
  --name authway-admin \
  --resource-group authway \
  --location koreacentral

# Auth UI
az staticwebapp create \
  --name authway-auth-ui \
  --resource-group authway \
  --location koreacentral
```

The landing page (apex domain marketing site) is **not** an Azure Static Web
App — see the Cloudflare Pages subsection below.

### 5. Custom Domains & SSL

```bash
# Add custom domain
az containerapp hostname add \
  --name authway-api \
  --resource-group authway \
  --hostname api.authway.in

# Bind SSL certificate
az containerapp hostname bind \
  --name authway-api \
  --resource-group authway \
  --hostname api.authway.in \
  --certificate <certificate-id>
```

#### DNS: `authway.in` is on Cloudflare, not the registrar

As of 2026-08-20, `authway.in`'s nameservers were migrated from the
registrar (GoDaddy) to Cloudflare — the domain is registered at GoDaddy, but
Cloudflare is now authoritative for all DNS records (`auth.`, `admin.`,
`api.`, `oauth.`, `auth-api.`, MX/SPF/DKIM for M365 mail, everything). DNS
changes for this domain happen in the Cloudflare dashboard/API from now on,
not at the registrar.

#### Landing page apex domain (`authway.in`) — Cloudflare Pages

The landing page is a **Cloudflare Pages** project (`authway-landing`), not
an Azure Static Web App — Azure's Static Web Apps Free-tier quota was
exhausted on this subscription when this page was set up, and since the
zone had just moved to Cloudflare, Cloudflare Pages was the better fit
anyway (apex domains there just need a CNAME — Cloudflare flattens it
automatically — no separate TXT-verification dance the way Azure's apex
binding needs).

Already done (nothing further to set up):
1. Cloudflare Pages project `authway-landing` created, static site deployed
   from `apps/marketing/landing/`.
2. Custom domains `authway.in` and `www.authway.in` bound to the project —
   both active over HTTPS.
3. `authway.in`'s DNS record is a proxied `CNAME` → `authway-landing.pages.dev`
   (the old GoDaddy-parking `A` records were removed).
4. `scripts/deploy/prod/.env` has `CLOUDFLARE_API_TOKEN`,
   `CLOUDFLARE_ACCOUNT_ID`, and `CLOUDFLARE_PAGES_PROJECT_LANDING=authway-landing`.

To redeploy after changing `apps/marketing/landing/` content:
`scripts/deploy/prod/publish-landing.ps1`.

To manage DNS for this zone going forward, use the Cloudflare dashboard
(dns.cloudflare.com → `authway.in`) or the Cloudflare API — not the GoDaddy
DNS panel, which is no longer authoritative.

---

## Docker Deployment

### Docker Compose

**`docker-compose.yml`**:
```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: authway
      POSTGRES_USER: authway
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"

  hydra:
    image: oryd/hydra:v26.2.0
    command: serve all --dev
    environment:
      DSN: postgres://authway:${DB_PASSWORD}@postgres:5432/hydra?sslmode=disable
      SECRETS_SYSTEM: ${HYDRA_SYSTEM_SECRET}
      URLS_SELF_ISSUER: https://oauth.example.com
      URLS_LOGIN: https://api.example.com/login
      URLS_CONSENT: https://api.example.com/consent
      URLS_LOGOUT: https://api.example.com/logout
    ports:
      - "4444:4444"
      - "4445:4445"
    depends_on:
      - postgres

  central-api:
    build: ./apps/central/api
    environment:
      DATABASE_URL: postgres://authway:${DB_PASSWORD}@postgres:5432/authway?sslmode=disable
      HYDRA_ADMIN_URL: http://hydra:4445
      HYDRA_PUBLIC_URL: https://oauth.example.com
    ports:
      - "8080:8080"
    depends_on:
      - postgres
      - hydra

volumes:
  postgres_data:
```

**`.env`**:
```bash
DB_PASSWORD=secure-db-password
HYDRA_SYSTEM_SECRET=at-least-32-random-characters
AUTH_BACKEND_SECRET=another-secure-secret
```

**Deploy**:
```bash
docker-compose up -d
```

---

## CORS Configuration

### Overview

CORS (Cross-Origin Resource Sharing) allows your frontend to communicate with the API from different domains.

### Database Configuration (Recommended)

**Per-Client CORS**:
```sql
UPDATE clients
SET allowed_origins = ARRAY[
  'https://app.example.com',
  'https://www.example.com',
  'http://localhost:3000'
]
WHERE client_id = 'my_app';
```

**Query**:
```sql
SELECT client_id, allowed_origins FROM clients;
```

### Environment Variable Configuration

**Central API `.env`**:
```bash
ALLOWED_ORIGINS=https://app.example.com,https://www.example.com,http://localhost:3000
```

**Code Implementation**:
```go
// Automatic CORS middleware
allowedOrigins := strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",")

router.Use(cors.New(cors.Config{
  AllowOrigins:     allowedOrigins,
  AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
  AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
  AllowCredentials: true,
  MaxAge:           12 * time.Hour,
}))
```

### Preflight Requests

**OPTIONS Request** (automatic):
```http
OPTIONS /api/v1/users HTTP/1.1
Origin: https://app.example.com
Access-Control-Request-Method: POST
Access-Control-Request-Headers: Content-Type, Authorization

Response:
Access-Control-Allow-Origin: https://app.example.com
Access-Control-Allow-Methods: POST, GET, OPTIONS
Access-Control-Allow-Headers: Content-Type, Authorization
Access-Control-Allow-Credentials: true
Access-Control-Max-Age: 43200
```

### Troubleshooting

**Issue: "CORS policy blocked"**
- Add origin to `allowed_origins` in client config
- Verify CORS middleware is enabled
- Check protocol (http vs https)

**Issue: "Credentials mode requires exact origin"**
- Don't use wildcard `*` when credentials are required
- Specify exact origins

---

## Environment Variables

### Central API

```bash
# Required
DATABASE_URL=postgres://user:password@host:5432/authway?sslmode=require
HYDRA_ADMIN_URL=http://hydra:4445
HYDRA_PUBLIC_URL=https://oauth.authway.in

# Optional
PORT=8080
LOG_LEVEL=info
ALLOWED_ORIGINS=https://app.example.com,https://www.example.com
```

### Hydra

```bash
# Required
DSN=postgres://user:password@host:5432/hydra?sslmode=require
SECRETS_SYSTEM=<at-least-32-random-characters>
URLS_SELF_ISSUER=https://oauth.authway.in
URLS_LOGIN=https://api.authway.in/login      # the API hands the login UI an opaque flow id
URLS_CONSENT=https://api.authway.in/consent
URLS_LOGOUT=https://api.authway.in/logout

# Optional
LOG_LEVEL=info
SERVE_COOKIES_SAME_SITE_MODE=Lax
```

---

## Database Migrations

The API applies pending migrations itself when it starts — all of them in one
transaction, and it exits rather than serve a half-migrated schema. Deploying the
API image is what migrates the database; there is no separate migration step.

Check a deployed environment (read-only):

```powershell
.\scripts\deploy\staging\check-migration-status.ps1
.\scripts\deploy\prod\check-migration-status.ps1
```

Dropping a column or table is a two-deployment change — see
[DATABASE.md](./DATABASE.md#changes-that-cannot-be-undone).

---

## Production Checklist

### Security

- [ ] **HTTPS Everywhere**: All endpoints use HTTPS
- [ ] **Secure Secrets**: Use environment variables or Azure Key Vault
- [ ] **CORS Configured**: Allowed origins whitelisted
- [ ] **Cookie Security**: `Secure`, `HttpOnly`, `SameSite=Strict`
- [ ] **Rate Limiting**: API rate limits configured
- [ ] **SQL Injection**: Parameterized queries used
- [ ] **XSS Protection**: Content Security Policy headers

### Performance

- [ ] **CDN**: Static assets served via CDN
- [ ] **Caching**: Redis for session/token caching
- [ ] **Database**: Connection pooling configured
- [ ] **Horizontal Scaling**: Multiple instances for high availability
- [ ] **Health Checks**: Liveness/readiness probes configured

### Monitoring

- [ ] **Logging**: Centralized logging (Application Insights, CloudWatch)
- [ ] **Metrics**: Performance metrics tracked
- [ ] **Alerts**: Critical error notifications
- [ ] **Uptime Monitoring**: External health checks

### Backup

- [ ] **Database Backups**: Automated daily backups
- [ ] **Disaster Recovery**: Backup restoration tested
- [ ] **Config Backup**: Environment variables documented

### Compliance

- [ ] **GDPR**: Data retention policies configured
- [ ] **Privacy Policy**: Terms of service published
- [ ] **Audit Logs**: User activity tracked

---

## Deployment Scripts

### Azure Deployment

**Full Deployment**:
```powershell
# Deploy all services (prod/ or staging/ wrappers; see scripts/deploy/README.md)
.\scripts\deploy\prod\deploy-all.ps1

# Deploy individual services
.\scripts\deploy\prod\publish-hydra.ps1
.\scripts\deploy\prod\publish-api.ps1
.\scripts\deploy\prod\publish-auth-ui.ps1
.\scripts\deploy\prod\publish-admin.ps1
```

### Health Checks

**Check Service Status**:
```bash
# Hydra
curl https://oauth.authway.in/health/ready

# Central API (also serves the SDK bootstrap document)
curl https://api.authway.in/health
curl https://api.authway.in/.well-known/authway-config
```

---

## Troubleshooting

### Common Issues

**Issue: Database connection timeout**
- Check firewall rules
- Verify connection string
- Test network connectivity

**Issue: Hydra not starting**
- Verify DSN format
- Check SECRETS_SYSTEM length (min 32 chars)
- Review Hydra logs

**Issue: CORS errors in production**
- Verify `allowed_origins` includes production domain
- Check HTTPS vs HTTP protocol mismatch
- Confirm credentials mode configuration

**Issue: SSL certificate errors**
- Verify certificate is valid and not expired
- Check domain name matches certificate
- Ensure certificate chain is complete

---

## Next Steps

- **[Database Migrations](./DATABASE.md)** - Schema management
- **[Features Guide](./FEATURES.md)** - Configure advanced features
- **[Backend Integration](./BACKEND_INTEGRATION.md)** - Protect APIs

---

**Need Help?**
- GitHub Issues: [github.com/iyulab/authway/issues](https://github.com/iyulab/authway/issues)
- Documentation: [./README.md](./README.md)
