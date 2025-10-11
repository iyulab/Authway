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
| `AUTHWAY_JWT_ACCESS_TOKEN_SECRET` | JWT signing secret | Dev placeholder | **64+ character random string** |
| `AUTHWAY_JWT_REFRESH_TOKEN_SECRET` | Refresh token secret | Dev placeholder | **64+ character random string** |
| `AUTHWAY_ADMIN_PASSWORD` | Admin console password | `admin123` | **20+ character strong password** |
| `AUTHWAY_APP_ENVIRONMENT` | Environment name | `development` | **Must be `production`** |
| `AUTHWAY_DATABASE_SSL_MODE` | DB SSL mode | `disable` | **Must be `require`** |

### 🟢 Optional (Has Sensible Defaults)

These settings have sensible defaults and can be customized as needed:

| Variable | Description | Default |
|----------|-------------|---------|
| `AUTHWAY_APP_NAME` | Application name | `Authway` |
| `AUTHWAY_APP_PORT` | Server port | `8080` |
| `AUTHWAY_JWT_ACCESS_TOKEN_EXPIRY` | Access token lifetime | `15m` |
| `AUTHWAY_JWT_REFRESH_TOKEN_EXPIRY` | Refresh token lifetime | `7d` |
| `AUTHWAY_REDIS_HOST` | Redis host (optional) | `localhost` |
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
AUTHWAY_APP_BASE_URL=http://localhost:8080  # Public URL for redirects
```

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

### Redis Configuration (Optional)

```bash
# Redis for session storage and caching
AUTHWAY_REDIS_HOST=localhost                # Redis server host
AUTHWAY_REDIS_PORT=6379                     # Redis port
AUTHWAY_REDIS_PASSWORD=                     # Redis password (if auth enabled)
AUTHWAY_REDIS_DB=0                          # Redis database number (0-15)
```

### JWT Configuration

```bash
# JSON Web Token settings
AUTHWAY_JWT_ACCESS_TOKEN_SECRET=...         # Signing secret (64+ chars in prod)
AUTHWAY_JWT_REFRESH_TOKEN_SECRET=...        # Refresh secret (64+ chars in prod)
AUTHWAY_JWT_ACCESS_TOKEN_EXPIRY=15m         # Access token lifetime
AUTHWAY_JWT_REFRESH_TOKEN_EXPIRY=7d         # Refresh token lifetime
AUTHWAY_JWT_ISSUER=authway                  # JWT issuer claim
```

### Email Configuration

```bash
# SMTP settings for email delivery
AUTHWAY_EMAIL_SMTP_HOST=localhost           # SMTP server
AUTHWAY_EMAIL_SMTP_PORT=1025                # SMTP port (587 for TLS, 465 for SSL)
AUTHWAY_EMAIL_SMTP_USER=                    # SMTP username
AUTHWAY_EMAIL_SMTP_PASSWORD=                # SMTP password
AUTHWAY_EMAIL_FROM_EMAIL=noreply@authway.dev # From address
AUTHWAY_EMAIL_FROM_NAME=Authway             # From display name
```

### Admin Console

```bash
# Admin access credentials
AUTHWAY_ADMIN_PASSWORD=admin123             # Admin console password
AUTHWAY_ADMIN_API_KEY=                      # Optional API key
```

## Configuration Validation

Authway validates configuration on startup and will **fail to start** if:

1. **Missing Required Settings**: Database host, user, or name is empty
2. **Production Security Issues**: Using default secrets in production environment
3. **Invalid Configuration**: Malformed values or conflicting settings

## Generating Secrets

### JWT Secrets (64 characters)

```bash
# Linux/Mac
openssl rand -base64 64

# Windows (PowerShell)
$bytes = [System.Byte[]]::new(64)
[System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
[Convert]::ToBase64String($bytes)
```

### Admin API Key (32 bytes hex)

```bash
# Linux/Mac
openssl rand -hex 32

# Windows (PowerShell)
-join ((48..57) + (97..102) | Get-Random -Count 64 | ForEach-Object {[char]$_})
```

## Docker Compose Configuration

### Development Mode (Local Backend/Frontend)

Uses environment variables from `.env` file:

```bash
# Start infrastructure only (PostgreSQL, Redis, MailHog)
./start-dev.ps1
```

### Full Docker Mode (Everything in Docker)

Override environment variables:

```bash
# Use default values from docker-compose.dev.yml
docker-compose -f docker-compose.dev.yml --profile full up

# Override with custom values
AUTHWAY_ADMIN_PASSWORD=mypassword docker-compose -f docker-compose.dev.yml --profile full up
```

## Production Deployment

### Security Checklist

- [ ] **Secrets**: Changed ALL default passwords and secrets
- [ ] **JWT**: Generated unique secrets (64+ characters)
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

### JWT Token Issues

**Problem**: Tokens invalid or not recognized

**Solution**:
1. Ensure `AUTHWAY_JWT_ACCESS_TOKEN_SECRET` is set and consistent
2. Verify tokens haven't expired (check expiry settings)
3. In production, ensure secrets are sufficiently long (64+ chars)
4. Don't change secrets after tokens are issued (invalidates all tokens)

## Support

For configuration help:
- Documentation: [https://docs.authway.dev](https://docs.authway.dev)
- GitHub Issues: [https://github.com/yourusername/authway/issues](https://github.com/yourusername/authway/issues)
