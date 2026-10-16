# Authway Deployment Guide

Complete guide for deploying Authway to production environments.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Azure Deployment](#azure-deployment)
- [Docker Deployment](#docker-deployment)
- [CORS Configuration](#cors-configuration)
- [Environment Variables](#environment-variables)
- [Redis](#redis)
- [Changing secrets](#changing-secrets)
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
  --admin-user <admin-user> \
  --admin-password <secure-password> \
  --sku-name Standard_B1ms \
  --tier Burstable \
  --version 15
```

#### Database and role

The API and the authorization server connect as a role of their own, not as
the server's administrator login. The administrator can reach every database
on the server; the application role reaches only its own. The role owns the
database and everything in it, because the API's migrations and the
authorization server's (`hydra migrate sql`) create and alter tables — run
both with the role's credentials:

```sql
-- as the server administrator
SET createrole_self_grant = 'set, inherit';  -- lets the administrator hand objects to the role
CREATE ROLE authway LOGIN PASSWORD '<application-password>';
CREATE DATABASE authway OWNER authway;
\c authway
ALTER SCHEMA public OWNER TO authway;
-- anything the administrator creates here later stays usable by the application
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO authway;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO authway;
```

Use letters and digits for the password: it is placed in the authorization
server's `DSN` URL as it is.

To move an existing database that the administrator owns to such a role,
change the owner of the database, the `public` schema and each table and
function in it explicitly (`ALTER ... OWNER TO`). Avoid `REASSIGN OWNED`: it
also moves objects outside this database that the administrator owns. Running
processes keep working through the administrator's membership in the role, so
each one can then be switched to the role's credentials on its own deploy.

### 2. Deploy Hydra

```bash
az containerapp create \
  --name authway-hydra \
  --resource-group authway \
  --environment authway-env \
  --image oryd/hydra:v26.2.0 \
  --target-port 4444 \
  --ingress external \
  --secrets \
    dsn="postgres://user:password@authway-db.postgres.database.azure.com/hydra?sslmode=require" \
    secrets-system="<random-32-char-string>" \
  --env-vars \
    DSN=secretref:dsn \
    SECRETS_SYSTEM=secretref:secrets-system \
    URLS_SELF_ISSUER="https://oauth.authway.in" \
    URLS_LOGIN="https://api.authway.in/login" \
    URLS_CONSENT="https://api.authway.in/consent" \
    URLS_LOGOUT="https://api.authway.in/logout"     WEBFINGER_OIDC_DISCOVERY_TOKEN_URL="https://api.authway.in/oauth2/token"     OAUTH2_TOKEN_HOOK_URL="https://<api app address>/internal/token-hook"     CLIENTS_HTTP_PRIVATE_IP_EXCEPTION_URLS="https://<api app address>/internal/token-hook"     OAUTH2_TOKEN_HOOK_AUTH_CONFIG_VALUE=secretref:token-hook-key
```

(plus `--secrets token-hook-key="<openssl rand -hex 32>"`, and the settings
every deployment shares from `configs/hydra.shared.env`.)

**Token endpoint and token hook.** Discovery's `token_endpoint` is the API's
`/oauth2/token`: the API passes every grant the authorization server knows on
to it unchanged (body, client authentication, answer) and answers the grants
Authway assembles itself. A client that signs an assertion
(`private_key_jwt`) addresses it to that URL; the authorization server
accepts it because it is the token endpoint its discovery names. Before it
issues any token the authorization server calls the API's token hook,
presenting the key in `X-Authway-Hook-Key`; the API holds the same key
(`AUTHWAY_HYDRA_TOKEN_HOOK_KEY`). The hook address is the API's Container Apps
address, so the call stays inside the environment; that address may resolve
to a private one, which `CLIENTS_HTTP_DISALLOW_PRIVATE_IP_RANGES` refuses — the
hook address, and only it, is listed as the exception. If the hook does not
answer, the authorization server issues no token — keep the API at one replica
or more (`--min-replicas 1`): a request made at the authorization server's own
token endpoint while the API is starting from zero can fail. Requests at the
API's endpoint start it first. The hook settings go to the public process
only: the authorization server refuses to start with the hook's key settings
but no address.

Pass passwords, keys and connection strings with `--secrets` and refer to
them with `secretref:`. A plain `--env-vars` value is part of the app
definition: anyone who can read the app can read it, and every revision keeps
a copy that cannot be deleted.

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
  --secrets \
    database-password="<database-password>" \
    admin-password="<admin-password>" \
    admin-api-key="<admin-api-key>" \
    totp-encryption-key="<base64 of 32 random bytes>" \
  --env-vars \
    AUTHWAY_APP_ENVIRONMENT=production \
    AUTHWAY_DATABASE_HOST=authway-db.postgres.database.azure.com \
    AUTHWAY_DATABASE_USER=<database-user> \
    AUTHWAY_DATABASE_NAME=authway \
    AUTHWAY_DATABASE_PASSWORD=secretref:database-password \
    AUTHWAY_DATABASE_SSL_MODE=require \
    AUTHWAY_ADMIN_PASSWORD=secretref:admin-password \
    AUTHWAY_ADMIN_API_KEY=secretref:admin-api-key \
    AUTHWAY_TOTP_ENCRYPTION_KEY=secretref:totp-encryption-key \
    AUTHWAY_HYDRA_ADMIN_URL="http://authway-hydra:4445" \
    AUTHWAY_HYDRA_PUBLIC_URL="https://oauth.authway.in"     AUTHWAY_HYDRA_PUBLIC_BACKEND_URL="https://<hydra public app address>"     AUTHWAY_HYDRA_TOKEN_HOOK_KEY=secretref:token-hook-key
```

`AUTHWAY_HYDRA_PUBLIC_BACKEND_URL` is where the API passes token requests to
the authorization server — its Container Apps address, not the public one, so
a token request does not leave the environment and come back through what
stands in front of it.

Redis and the auth UI address are required as well — the full list, and what
the API refuses to start without, is in [CONFIGURATION.md](../CONFIGURATION.md).

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

#### Discovery document

The authorization server's own discovery document cannot describe the
deployment fully: its response type, grant and logout lists are fixed in its
code (it advertises `implicit`, front-channel logout and request URIs, which
no client can be registered for), and it has no place for the introspection
endpoint or the grants Authway answers at the token endpoint (token exchange,
approval polling). The API serves the document as the deployment offers it at
`/.well-known/openid-configuration` and `/.well-known/oauth-authorization-server`
— the authorization server's document with those corrections, `issuer`
unchanged.

A client reads discovery at the issuer's address, so the document reaches
clients only where the deployment routes those two paths of the issuer's host
to the API (a reverse proxy in front of both, or an edge rule on the issuer's
domain). Without that route, clients see the authorization server's document;
everything they can register still works, and the introspection endpoint is
`{API}/api/v1/oauth2/introspect`. Read at the API's own address, the document
fails a client's issuer check (OpenID Connect Discovery 1.0 §4.3) — the API is
not the issuer.

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

Which pages may call the API from a browser is decided in three ways:

| Caller | What it may call | How it is admitted |
|---|---|---|
| A page on **any origin** | The documents that describe the deployment: `/.well-known/authway-config`, `/api/v1/capabilities` | Always (`Access-Control-Allow-Origin: *`) |
| An **application's** pages | The routes a signed-in user calls with their own access token: `/api/v1/profile/*`, `/api/v1/users/mfa/*`, `POST /api/v1/logout` | The origin is listed in `allowed_origins` on one of the deployment's active clients. No cookies are accepted from it — the token is the credential. |
| The deployment's **own** login screens and admin console | Everything, with cookies | The origin is listed in `AUTHWAY_CORS_ALLOWED_ORIGINS` (comma-separated) |

So a new browser application needs no change to the deployment: register its
origin on its client and its pages can call the user-token routes. A change to
`allowed_origins` takes effect within a minute.

The admin API is never open to an application's origin. Call it from a backend.

The OIDC endpoints (authorization, token, discovery, JWKS, userinfo) are served
by the authorization server, which has its own CORS setting.

### Deployment setting

```bash
# The origins of the login screens and the admin console
AUTHWAY_CORS_ALLOWED_ORIGINS=https://auth.example.com,https://admin.example.com
```

### Troubleshooting

**Issue: "CORS policy blocked" in an application**
- Add the page's origin to `allowed_origins` on the application's client. It must match exactly — scheme, host and port
- The route must be one a user calls with their own token; the admin API is not open to application origins

**Issue: the login screen or console cannot call the API**
- Its origin must be in `AUTHWAY_CORS_ALLOWED_ORIGINS`; restart the API after changing it

---

## Environment Variables

### Central API

```bash
# Required
DATABASE_URL=postgres://user:password@host:5432/authway?sslmode=require
HYDRA_ADMIN_URL=http://hydra:4445
HYDRA_PUBLIC_URL=https://oauth.authway.in
AUTHWAY_HYDRA_TOKEN_HOOK_KEY=<the key the authorization server presents to the token hook>

# Optional
AUTHWAY_HYDRA_PUBLIC_BACKEND_URL=https://<hydra public app address>   # default: HYDRA_PUBLIC_URL
PORT=8080
LOG_LEVEL=info
AUTHWAY_CORS_ALLOWED_ORIGINS=https://auth.example.com,https://admin.example.com
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
WEBFINGER_OIDC_DISCOVERY_TOKEN_URL=https://api.authway.in/oauth2/token   # the API's token endpoint
OAUTH2_TOKEN_HOOK_URL=https://<api app address>/internal/token-hook
CLIENTS_HTTP_PRIVATE_IP_EXCEPTION_URLS=https://<api app address>/internal/token-hook
OAUTH2_TOKEN_HOOK_AUTH_CONFIG_VALUE=<the same key as the API's AUTHWAY_HYDRA_TOKEN_HOOK_KEY>

# Optional
LOG_LEVEL=info
SERVE_COOKIES_SAME_SITE_MODE=Lax
```

---

## Redis

The API does not start without Redis. Nothing in it is a record: every key
has a lifetime, and losing all of them costs users a retry, not data.

| Keys | What | Kept for | If lost |
|------|------|----------|---------|
| `auth_time:<session>` | When and how the user of a browser session last signed in — the `auth_time` of access tokens and the `amr` of ID tokens issued on a reused session, and the check before deleting an account | 30 days | Until the user signs in again, access tokens issued on that session carry no `auth_time` and ID tokens no `amr`; an operation that needs a recent sign-in asks for one |
| `mfa_challenge:<id>` | A sign-in that passed its first factor and owes the second | 10 minutes | The user starts the sign-in again |
| `oauth:state:<state>` | A sign-in sent to a social provider and not yet back | 10 minutes | The user starts the sign-in again |
| `ratelimit:…` | Attempt counters and blocks, per route and address | 1 minute to 1 hour | Counters start from zero |

### Size

Each key is a few hundred bytes and is written once per sign-in or per
attempt. A deployment with thousands of sign-ins a day stays in the tens of
megabytes; the smallest instance a provider offers is enough until measured
otherwise. Choose for availability, not for memory: Redis is on the sign-in
path, so an instance without a replica makes its restarts sign-in outages.

### What the API asks of it

One node, or a Redis Cluster. Every command names a single key —
transactions and the one script included — so nothing depends on keys
sharing a slot. No keyspace notifications, no `KEYS` or `SCAN`.

A Redis that shards keys over nodes and answers a key held elsewhere with
`MOVED` — a managed Redis with the OSS cluster policy, or a self-run cluster —
needs `AUTHWAY_REDIS_CLUSTER=true`: the API then speaks the cluster protocol,
learns the nodes from the one in `AUTHWAY_REDIS_HOST`/`PORT`, and sends each
command to the node that holds its key. A clustered Redis that hides its
nodes behind one endpoint (an enterprise or proxy policy) is a single node to
the API and needs nothing. A cluster has database 0 only: the API refuses to
start with `AUTHWAY_REDIS_CLUSTER=true` and a database number.

### Sharing one Redis between deployments

Give each deployment a prefix: `AUTHWAY_REDIS_KEY_PREFIX=staging` puts all of
its keys under `staging:`. A database number (`AUTHWAY_REDIS_DB`) separates
deployments too, but only where numbered databases exist — a clustered or
managed Redis has database 0 alone.

The prefix is applied to the connection, not key by key: a command the API
has not been taught to place under the prefix is refused rather than sent, so
a deployment cannot write outside its share by oversight.

Turning the prefix on, or changing it, makes the keys written before
invisible: the effect is that of losing them (the table above), once.

### Moving to another instance

Nothing has to be copied. Point the API at the new instance and deploy; the
effects are those of losing every key, once. To make the move unnoticed for
signed-in users, do it when a restart is acceptable anyway — sign-ins in
flight at that moment are started again.

1. Create the instance; note host, port, password, whether it requires TLS,
   and whether its clustering needs a cluster client (above).
2. If deployments will share it, choose a prefix for each.
3. Set `AUTHWAY_REDIS_*` for the deployment and deploy the API.
4. Sign in once and check the new instance has keys under the expected
   prefix (`redis-cli --scan --pattern '<prefix>:*'`).
5. Remove the old instance when no deployment points at it.

## Changing secrets

Each secret can be changed on its own. What a change costs differs:

| Secret | Who else holds it | How |
|--------|-------------------|-----|
| `AUTHWAY_ADMIN_API_KEY` | Every script or service that calls the management API with it | Set the new value and redeploy; give the callers the new value. Calls with the old one get 401 from then on. |
| `AUTHWAY_ADMIN_PASSWORD` | The people who sign in to the admin console | Set and redeploy. |
| `AUTHWAY_TOTP_ENCRYPTION_KEY` | Nobody — it seals values at rest | Change it with the previous key listed, then remove that ([CONFIGURATION.md](../CONFIGURATION.md)). Nothing sealed is lost. |
| Database password | The API and both authorization server processes (their `DSN`) | Below. |
| Token hook key (`AUTHWAY_HYDRA_TOKEN_HOOK_KEY`) | The authorization server's public process (`OAUTH2_TOKEN_HOOK_AUTH_CONFIG_VALUE`) | The API accepts every key in a comma-separated list: deploy the API with `<new>,<old>`, then the authorization server with `<new>`, then the API with `<new>`. No token request fails on the way. |
| Redis password | The API | Set and redeploy; or move to a new instance ([Moving to another instance](#moving-to-another-instance)). |
| Authorization server `SECRETS_SYSTEM` | The authorization server's own stored keys and issued tokens | Below — in stages. |

On Azure Container Apps a secret's value reaches a replica when the replica
starts. The deploy scripts restart the app when a deploy changes a secret and
nothing else; when changing a secret by hand, restart the active revision.

### Database password

Connections already open stay open when the password changes; new ones need
the new value. To keep the window short, give every process the new value
first, then change the password, then restart them:

1. Set the new password in the API's configuration and in the `DSN` of both
   authorization server processes, without restarting.
2. Change the password in the database.
3. Restart the API and both authorization server processes at once.

Until step 3, a process that starts — a scale-out, a crash — fails to
connect, so do steps 2 and 3 together.

### Authorization server system secret

`SECRETS_SYSTEM` takes a comma-separated list. The first value encrypts and
signs what the server writes from then on; every value in the list is tried
when reading. The server's signing keys are stored encrypted with the value
in use when they were made, and adding a value does not re-encrypt them —
remove the old value while keys made under it remain, and the server cannot
publish its keys or issue tokens.

So a change takes four steps, the last two after a wait:

1. **New value first**: `SECRETS_SYSTEM=<new>,<old>`, restart. Tokens already
   issued stay valid.
2. **New signing keys**: add a key to each signing key set
   (`hydra create jwk hydra.jwt.access-token <kid> --public` and the same for
   `hydra.openid.id-token`, against the admin endpoint — `--public` keeps the
   private key out of the command's output and logs). They are encrypted
   with the new value and the server signs with them at once; the old keys
   stay published, so tokens signed with them still verify.
3. **Remove the old keys**, once every token signed with them has expired
   (the longest of the access token and ID token lifetimes). From then on
   nothing the old value can decrypt is a key the server publishes.
4. **Remove the old value** from the list, once the refresh token lifetime has
   passed since step 1. A refresh token issued before step 1 is checked
   against the old value; removing it earlier makes those users sign in again.

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
- [ ] **Database Role**: The API and the authorization server connect as an application role that owns only their database, not as the server administrator ([Database and role](#database-and-role))
- [ ] **CORS Configured**: `AUTHWAY_CORS_ALLOWED_ORIGINS` lists the login screens and console; each browser application has its origin in `allowed_origins`
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
- Verify the application's client lists the production origin in `allowed_origins` (exact match)
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
