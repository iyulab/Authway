# Authway Configuration Guide

Complete guide for configuring Authway in different environments.

## Quick Start

### Development (Local)

```bash
# 1. Copy environment template
cp .env.example .env

# 2. Start development environment
./start-dev.ps1  # Windows PowerShell
# or
./start-dev.sh   # Linux/Mac

# 3. Access services
# - Admin Dashboard: http://localhost:3000 (password: admin123)
# - Backend API: http://localhost:8080
# - MailHog UI: http://localhost:8025
```

### Production

```bash
# 1. Copy production template
cp .env.production.example .env.production

# 2. Update ALL values in .env.production
# - Change all passwords and secrets
# - Set production domain and SSL settings
# - Configure production database and Redis
# - Set up production SMTP service

# 3. Deploy using Docker Compose or your orchestration platform
```

## Configuration Categories

### 🔴 Required (Must Configure)

These settings **MUST** be configured for the application to run:

| Variable | Description | Default | Production Requirement |
|----------|-------------|---------|----------------------|
| `AUTHWAY_DATABASE_HOST` | PostgreSQL host | `localhost` | Production DB host |
| `AUTHWAY_DATABASE_USER` | Database user | `authway` | Secure username |
| `AUTHWAY_DATABASE_NAME` | Database name | `authway` | Production DB name |
| `AUTHWAY_DATABASE_PASSWORD` | Database password | `authway` | **Strong password required** |

### 🟡 Required in Production

These have development defaults but **MUST** be changed for production:

| Variable | Description | Default | Production Requirement |
|----------|-------------|---------|----------------------|
| `AUTHWAY_ADMIN_PASSWORD` | Admin console password | `admin123` | **20+ character strong password** |
| `AUTHWAY_ADMIN_API_KEY` | Key for the management API | Generated in development | **Random key** — the API refuses to start without it |
| `AUTHWAY_TOTP_ENCRYPTION_KEY` | Encrypts stored two-factor secrets and provider credentials (AES-256-GCM) | None (plaintext, development only) | **Base64 of exactly 32 random bytes** — the API refuses to start without it |
| `AUTHWAY_APP_ENVIRONMENT` | Environment name | `development` | **Must be `production`** |
| `AUTHWAY_DATABASE_SSL_MODE` | DB SSL mode | `disable` | **Must be `require`** |

### 🟢 Optional (Has Sensible Defaults)

These settings have sensible defaults and can be customized as needed:

| Variable | Description | Default |
|----------|-------------|---------|
| `AUTHWAY_APP_NAME` | Application name | `Authway` |
| `AUTHWAY_APP_PORT` | Server port | `8080` |
| `AUTHWAY_REDIS_HOST` | Redis host — the API does not start without Redis | `localhost` |
| `AUTHWAY_REDIS_PORT` | Redis port | `6379` |
| `AUTHWAY_CORS_ALLOWED_ORIGINS` | CORS origins | `http://localhost:3000,...` |

### ⚪ Fully Optional

These features are disabled by default and optional:

| Variable | Description | Default |
|----------|-------------|---------|
| `AUTHWAY_GOOGLE_ENABLED` | Enable Google OAuth | `false` |
| `AUTHWAY_GITHUB_ENABLED` | Enable GitHub OAuth | `false` |
| `AUTHWAY_ADMIN_API_KEY` | Admin API access key | Empty |
| `AUTHWAY_TENANT_SINGLE_TENANT_MODE` | Single tenant mode | `false` |

## Environment Variables Reference

### Application Settings

```bash
# Application identity and behavior
AUTHWAY_APP_NAME=Authway                    # Display name
AUTHWAY_APP_VERSION=1.0.0                   # Version number
AUTHWAY_APP_ENVIRONMENT=development         # development|staging|production
AUTHWAY_APP_PORT=8080                       # HTTP port
AUTHWAY_APP_BASE_URL=http://localhost:8080      # This API's own public URL (discovery `api_server`)
AUTHWAY_APP_FRONTEND_URL=http://localhost:3001  # The auth UI's public URL
```

`BASE_URL` and `FRONTEND_URL` are different hosts and must not be set to the
same value — startup fails if they are. Everything a person clicks is built from
`FRONTEND_URL`: invitation accept, magic link, email verification, password
reset. Pointing those at the API returns 404, and nothing else in the system
notices — only the recipient of the mail does.

### Database Configuration

```bash
# PostgreSQL connection
AUTHWAY_DATABASE_HOST=localhost             # Database server host
AUTHWAY_DATABASE_PORT=5432                  # PostgreSQL port
AUTHWAY_DATABASE_USER=authway               # Database username
AUTHWAY_DATABASE_PASSWORD=authway           # Database password
AUTHWAY_DATABASE_NAME=authway               # Database name
AUTHWAY_DATABASE_SSL_MODE=disable           # disable|require|verify-ca|verify-full
```

### Redis Configuration

The API needs Redis and does not start without it. What it keeps there is
short-lived — see [Redis](docs/DEPLOYMENT.md#redis).

```bash
AUTHWAY_REDIS_HOST=localhost                # Redis server host
AUTHWAY_REDIS_PORT=6379                     # Redis port
AUTHWAY_REDIS_PASSWORD=                     # Redis password (if auth enabled)
AUTHWAY_REDIS_TLS_ENABLED=false             # true for a Redis that only speaks TLS
AUTHWAY_REDIS_DB=0                          # Redis database number (0-15)
AUTHWAY_REDIS_KEY_PREFIX=                   # this deployment's share of a Redis others use too
```

`AUTHWAY_REDIS_KEY_PREFIX` puts every key the deployment reads or writes
under `<prefix>:`. Set it when two deployments share one Redis — it works
where only database 0 exists, as on a clustered or managed Redis. Up to 32
letters, digits, `.`, `_` or `-`; the API refuses to start on anything else.

Tokens are issued and signed by the authorization server, not by the API, so
the API has no token secrets or lifetimes to configure.

### Email Configuration

```bash
# SMTP settings for email delivery
AUTHWAY_EMAIL_SMTP_HOST=localhost           # SMTP server
AUTHWAY_EMAIL_SMTP_PORT=1025                # SMTP port (587 for TLS, 465 for SSL)
AUTHWAY_EMAIL_SMTP_USER=                    # SMTP username
AUTHWAY_EMAIL_SMTP_PASSWORD=                # SMTP password
AUTHWAY_EMAIL_FROM_EMAIL=noreply@authway.in # From address
AUTHWAY_EMAIL_FROM_NAME=Authway             # From display name
```

### Admin Console

```bash
# Admin access credentials
AUTHWAY_ADMIN_PASSWORD=admin123             # Admin console password
AUTHWAY_ADMIN_API_KEY=                      # Management API key (required outside development)
AUTHWAY_TOTP_ENCRYPTION_KEY=                # At-rest encryption key (required outside development)
```

## Configuration Validation

Authway validates configuration on startup and will **fail to start** if:

1. **Missing Required Settings**: Database host, user, or name is empty
2. **Production Security Issues**: Using default secrets in production environment
3. **Invalid Configuration**: Malformed values or conflicting settings

## Generating Secrets

### Admin API Key (32 bytes hex)

```bash
# Linux/Mac
openssl rand -hex 32

# Windows (PowerShell 7+)
[Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLower()
```

### At-Rest Encryption Key (32 bytes, base64)

```bash
# Linux/Mac
openssl rand -base64 32

# Windows (PowerShell 7+)
[Convert]::ToBase64String([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
```

Changing this key makes everything encrypted under the old one unreadable —
two-factor enrollments and stored provider credentials.

### Where secrets live in a deployment

Pass passwords and keys to the container as secrets of your platform (for
example Container Apps secrets referenced with `secretref:`, or a Kubernetes
Secret), not as plain environment values. A plain value is part of the app
definition: anyone who can read the app can read it, and its revision history
keeps every past value.

## Docker Compose Configuration

### Development Mode (Local Backend/Frontend)

Uses environment variables from `.env` file:

```bash
# Start infrastructure only (PostgreSQL, Redis, MailHog)
./start-dev.ps1
```

### Backing Services Only

`docker-compose.yml` provides Postgres, Redis, MailHog and Hydra. The APIs and
UIs run natively, so their environment comes from `.env` (or the shell), not
from compose:

```bash
docker compose up -d

# Override for a single run
AUTHWAY_ADMIN_PASSWORD=mypassword go run ./cmd/
```

## Production Deployment

### Security Checklist

- [ ] **Secrets**: Changed ALL default passwords and secrets
- [ ] **Keys**: Generated a unique admin API key and at-rest encryption key
- [ ] **Secret storage**: Passwords and keys passed as platform secrets, not plain environment values
- [ ] **Admin**: Set strong password (20+ characters)
- [ ] **Database**: Enabled SSL (`ssl_mode=require`)
- [ ] **HTTPS**: Using HTTPS for all URLs
- [ ] **CORS**: Restricted to specific production domains
- [ ] **SMTP**: Configured production email service

### Infrastructure Checklist

- [ ] **Database**: Production PostgreSQL with automated backups
- [ ] **Redis**: Production instance or cluster for HA
- [ ] **SSL/TLS**: Valid certificates installed
- [ ] **Monitoring**: Application logging and alerts configured
- [ ] **Backups**: Daily automated backups verified

## Troubleshooting

### Configuration Not Loading

**Problem**: Environment variables not being read

**Solution**:
1. Check `.env` file exists in the correct location
2. Verify environment variable names start with `AUTHWAY_`
3. Restart the application after changing `.env`

### Database Connection Failed

**Problem**: Cannot connect to PostgreSQL

**Solution**:
1. Verify database is running: `docker ps` or `pg_isready`
2. Check host/port/credentials in configuration
3. Ensure database name exists
4. Check SSL mode matches server configuration

### Admin Console Login Failed

**Problem**: Admin password not working

**Solution**:
1. Check `AUTHWAY_ADMIN_PASSWORD` is set in `.env`
2. Restart backend after changing password
3. Default development password is `admin123`
4. In production, password must be explicitly set

## Support

For configuration help:
- Documentation: [https://docs.authway.in](https://docs.authway.in)
- GitHub Issues: [https://github.com/iyulab/authway/issues](https://github.com/iyulab/authway/issues)
