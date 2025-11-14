# Logout Redirect Policy Configuration

## Overview

Authway provides configurable logout redirect URI validation policies to balance security requirements with development flexibility. This feature implements **OpenID Connect RP-Initiated Logout** with three distinct policy levels that can be configured per OAuth client.

## Policy Levels

### 1. Strict Policy (Default, Production Recommended)

**Behavior:**
- `post_logout_redirect_uri` parameter is **required** in logout requests
- Provided URI **must** be whitelisted in the client's `post_logout_redirect_uris` list
- Logout request fails if URI is missing or not whitelisted
- Maximum security for production environments

**Use Cases:**
- Production environments
- Public-facing applications
- Applications requiring strict security compliance
- Multi-tenant environments with external clients

**Example:**
```json
{
  "client_id": "prod-web-app",
  "name": "Production Web Application",
  "logout_redirect_policy": "strict",
  "post_logout_redirect_uris": [
    "https://example.com/logout-success",
    "https://example.com/goodbye"
  ]
}
```

**Logout Request:**
```http
GET /oauth2/sessions/logout?post_logout_redirect_uri=https://example.com/logout-success
```

**Result:** ✅ Success (URI is whitelisted)

```http
GET /oauth2/sessions/logout
```

**Result:** ❌ Error - `post_logout_redirect_uri` required

### 2. Lenient Policy (Development/Staging)

**Behavior:**
- `post_logout_redirect_uri` parameter is **optional**
- If provided, URI **must** be whitelisted (same validation as strict)
- If omitted, falls back to `default_logout_uri`
- If `default_logout_uri` is not configured, falls back to first URI in `post_logout_redirect_uris`
- Provides flexibility while maintaining validation when URIs are provided

**Use Cases:**
- Development environments
- Staging/QA environments
- Internal applications
- Prototyping and testing
- Applications with predictable logout flows

**Example:**
```json
{
  "client_id": "dev-web-app",
  "name": "Development Web Application",
  "logout_redirect_policy": "lenient",
  "post_logout_redirect_uris": [
    "http://localhost:3000/logout",
    "http://localhost:3001/logout"
  ],
  "default_logout_uri": "http://localhost:3000/logout"
}
```

**Logout Request (with URI):**
```http
GET /oauth2/sessions/logout?post_logout_redirect_uri=http://localhost:3000/logout
```

**Result:** ✅ Success (URI is whitelisted)

**Logout Request (without URI):**
```http
GET /oauth2/sessions/logout
```

**Result:** ✅ Success (redirects to `default_logout_uri`)

**Logout Request (invalid URI):**
```http
GET /oauth2/sessions/logout?post_logout_redirect_uri=http://malicious.com
```

**Result:** ❌ Error - URI not whitelisted

### 3. Disabled Policy (Local Development Only)

**Behavior:**
- **No validation** performed on `post_logout_redirect_uri`
- Allows any URI without whitelist checking
- **Blocked in production environments** via `AUTHWAY_ENV` check
- Maximum flexibility for local development and debugging

**Use Cases:**
- Local development only
- Rapid prototyping
- Debugging logout flows
- Testing with dynamic redirect URIs

**Production Safety:**
```go
// Automatic environment check prevents disabled policy in production
if normalizedPolicy == PolicyDisabled && os.Getenv("AUTHWAY_ENV") == "production" {
    return errors.New("disabled logout redirect policy is not allowed in production environment")
}
```

**Example:**
```json
{
  "client_id": "local-dev-app",
  "name": "Local Development Application",
  "logout_redirect_policy": "disabled",
  "post_logout_redirect_uris": []
}
```

**Logout Request (any URI):**
```http
GET /oauth2/sessions/logout?post_logout_redirect_uri=http://any-uri.com
```

**Result:** ✅ Success (no validation) - **Only works when `AUTHWAY_ENV != "production"`**

## Wildcard Pattern Support

All policy levels (except disabled) support wildcard patterns in `post_logout_redirect_uris` when `allow_wildcard_logout` is enabled.

### Supported Wildcard Patterns

#### 1. Port Wildcards
Match any port on a specific host:

```json
{
  "post_logout_redirect_uris": [
    "http://localhost:*"
  ],
  "allow_wildcard_logout": true
}
```

**Matches:**
- `http://localhost:3000`
- `http://localhost:8080`
- `http://localhost:9999`

#### 2. Subdomain Wildcards
Match any subdomain:

```json
{
  "post_logout_redirect_uris": [
    "https://*.example.com"
  ],
  "allow_wildcard_logout": true
}
```

**Matches:**
- `https://app.example.com`
- `https://staging.example.com`
- `https://dev.example.com`

#### 3. Combined Patterns
Use multiple wildcard patterns:

```json
{
  "post_logout_redirect_uris": [
    "http://localhost:*",
    "https://*.dev.example.com",
    "https://example.com"
  ],
  "allow_wildcard_logout": true
}
```

### Security Considerations for Wildcards

⚠️ **Use wildcards carefully:**
- Wildcards increase the attack surface by allowing multiple URIs
- Use specific patterns rather than overly broad ones
- Consider using wildcards only in lenient policy for non-production environments
- For strict policy in production, prefer explicit URI lists

**Good Practice:**
```json
"post_logout_redirect_uris": ["http://localhost:*"]  // Limited to localhost
```

**Bad Practice:**
```json
"post_logout_redirect_uris": ["https://*"]  // Too broad, security risk
```

## Configuration Examples

### Production Setup (Strict Policy)

```json
{
  "client_id": "prod-web-client",
  "name": "Production Web Client",
  "logout_redirect_policy": "strict",
  "post_logout_redirect_uris": [
    "https://example.com/logout-success",
    "https://example.com/goodbye",
    "https://example.com/signed-out"
  ],
  "default_logout_uri": null,
  "allow_wildcard_logout": false
}
```

### Development Setup (Lenient Policy)

```json
{
  "client_id": "dev-web-client",
  "name": "Development Web Client",
  "logout_redirect_policy": "lenient",
  "post_logout_redirect_uris": [
    "http://localhost:*",
    "https://dev.example.com/logout"
  ],
  "default_logout_uri": "http://localhost:3000/logout",
  "allow_wildcard_logout": true
}
```

### Local Development Setup (Disabled Policy)

```json
{
  "client_id": "local-dev-client",
  "name": "Local Development Client",
  "logout_redirect_policy": "disabled",
  "post_logout_redirect_uris": [],
  "default_logout_uri": null,
  "allow_wildcard_logout": false
}
```

**Note:** Only works when `AUTHWAY_ENV != "production"`

### Multi-Environment Single Client

For clients that need to work across environments, use lenient policy with wildcards:

```json
{
  "client_id": "multi-env-client",
  "name": "Multi-Environment Client",
  "logout_redirect_policy": "lenient",
  "post_logout_redirect_uris": [
    "http://localhost:*",
    "https://*.staging.example.com",
    "https://example.com/logout"
  ],
  "default_logout_uri": "https://example.com/logout",
  "allow_wildcard_logout": true
}
```

## API Reference

### Client Creation

**POST** `/api/v1/clients`

```json
{
  "tenant_id": "tenant-uuid",
  "name": "My Application",
  "redirect_uris": ["https://example.com/callback"],
  "post_logout_redirect_uris": [
    "https://example.com/logout-success"
  ],
  "logout_redirect_policy": "strict",
  "default_logout_uri": null,
  "allow_wildcard_logout": false,
  "grant_types": ["authorization_code"],
  "scopes": ["openid", "profile"],
  "public": false
}
```

### Client Update

**PUT** `/api/v1/clients/{client_id}`

```json
{
  "logout_redirect_policy": "lenient",
  "post_logout_redirect_uris": [
    "http://localhost:*"
  ],
  "default_logout_uri": "http://localhost:3000/logout",
  "allow_wildcard_logout": true
}
```

### Field Descriptions

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `post_logout_redirect_uris` | `string[]` | No | `[]` | Whitelisted URIs for post-logout redirection |
| `logout_redirect_policy` | `string` | No | `"strict"` | Validation policy: `strict`, `lenient`, or `disabled` |
| `default_logout_uri` | `string` | No | `null` | Default URI for lenient policy when `post_logout_redirect_uri` is not provided |
| `allow_wildcard_logout` | `boolean` | No | `false` | Allow wildcard patterns in `post_logout_redirect_uris` |

## Migration Guide

### For Existing Clients

When you upgrade to version 0.1.5+, the migration script automatically:

1. **Copies `redirect_uris` to `post_logout_redirect_uris`**
   - Ensures backward compatibility
   - Existing clients continue to work without changes

2. **Sets `logout_redirect_policy` to `strict`**
   - Maintains existing security behavior
   - No behavioral changes for existing clients

3. **Sets first `redirect_uri` as `default_logout_uri`**
   - Provides fallback for lenient policy if you switch later
   - Optional field, only used in lenient policy

4. **Sets `allow_wildcard_logout` to `false`**
   - Maintains strict matching behavior by default
   - Can be enabled per client as needed

### Migration SQL

```sql
-- Migration: 003_add_logout_policy.sql
ALTER TABLE clients
ADD COLUMN IF NOT EXISTS post_logout_redirect_uris TEXT[] DEFAULT '{}',
ADD COLUMN IF NOT EXISTS logout_redirect_policy VARCHAR(20) DEFAULT 'strict',
ADD COLUMN IF NOT EXISTS default_logout_uri VARCHAR(512) NULL,
ADD COLUMN IF NOT EXISTS allow_wildcard_logout BOOLEAN DEFAULT false;

-- Migrate existing clients
UPDATE clients
SET
  post_logout_redirect_uris = CASE
    WHEN redirect_uris IS NOT NULL AND array_length(redirect_uris, 1) > 0
    THEN redirect_uris
    ELSE '{}'
  END,
  default_logout_uri = CASE
    WHEN redirect_uris IS NOT NULL AND array_length(redirect_uris, 1) > 0
    THEN redirect_uris[1]
    ELSE NULL
  END,
  logout_redirect_policy = 'strict',
  allow_wildcard_logout = false
WHERE post_logout_redirect_uris IS NULL;
```

### Manual Migration Steps

If you want to optimize policies for existing clients:

1. **Review each client's environment:**
   ```bash
   # Development/staging clients
   curl -X PUT https://authway.example.com/api/v1/clients/{client-id} \
     -H "Authorization: Bearer $TOKEN" \
     -d '{
       "logout_redirect_policy": "lenient",
       "allow_wildcard_logout": true
     }'
   ```

2. **Configure wildcards for localhost clients:**
   ```bash
   curl -X PUT https://authway.example.com/api/v1/clients/{client-id} \
     -H "Authorization: Bearer $TOKEN" \
     -d '{
       "post_logout_redirect_uris": ["http://localhost:*"],
       "allow_wildcard_logout": true
     }'
   ```

3. **Set default logout URIs for lenient clients:**
   ```bash
   curl -X PUT https://authway.example.com/api/v1/clients/{client-id} \
     -H "Authorization: Bearer $TOKEN" \
     -d '{
       "logout_redirect_policy": "lenient",
       "default_logout_uri": "https://example.com/logout"
     }'
   ```

## Admin UI Configuration

The Admin UI provides form fields for configuring logout policies:

### Creating a New Client

1. Navigate to **Clients** page
2. Click **Create Client**
3. Configure logout policy fields:
   - **Logout Redirect Policy**: Select from dropdown (Strict/Lenient/Disabled)
   - **Post-Logout Redirect URIs**: Enter one URI per line
   - **Default Logout URI**: Enter fallback URI (for lenient policy)
   - **Allow Wildcard Logout**: Enable checkbox for wildcard pattern support

### Editing an Existing Client

1. Navigate to **Clients** page
2. Click **Edit** on desired client
3. Modify logout policy fields as needed
4. Click **Save Changes**

### UI Field Descriptions

- **Logout Redirect Policy Dropdown:**
  - **Strict (기본값)**: URI 필수 + 검증 - Production recommended
  - **Lenient**: URI 선택적 + 검증 - Development/staging
  - **Disabled**: 검증 비활성화 (개발 환경만) - Local development only

- **Default Logout URI:**
  - Only used in lenient policy
  - Fallback when `post_logout_redirect_uri` is not provided
  - Must be a valid URL

- **Allow Wildcard Logout Checkbox:**
  - Enables wildcard pattern matching
  - Use for `http://localhost:*` or `https://*.example.com` patterns

## Security Best Practices

### Policy Selection Guidelines

| Environment | Recommended Policy | Wildcard | Reasoning |
|-------------|-------------------|----------|-----------|
| Production | **Strict** | No | Maximum security, explicit whitelist |
| Staging | Lenient | Optional | Flexibility with validation |
| Development | Lenient | Yes | Convenience with security |
| Local Dev | Disabled | N/A | Maximum convenience, blocked in prod |

### Security Checklist

✅ **DO:**
- Use strict policy for production environments
- Explicitly whitelist all production logout URIs
- Use lenient policy for staging with default fallback
- Enable wildcards only for trusted patterns (e.g., `localhost:*`)
- Set `default_logout_uri` to a safe, known endpoint
- Review and audit client configurations regularly

❌ **DON'T:**
- Use disabled policy in production (automatically blocked)
- Use overly broad wildcard patterns like `https://*`
- Share clients between production and development environments
- Allow user-controlled input in logout URIs without validation
- Forget to whitelist new logout endpoints when deploying

### Open Redirect Prevention

All policy levels (except disabled) protect against open redirect attacks:

1. **Whitelist Enforcement:** Only pre-approved URIs are allowed
2. **Pattern Validation:** Wildcard patterns must match specific rules
3. **Environment Restrictions:** Disabled policy blocked in production
4. **Explicit Configuration:** No automatic URI acceptance

**Example Attack Prevention:**

```http
# Attacker tries to redirect to malicious site
GET /oauth2/sessions/logout?post_logout_redirect_uri=https://evil.com

# Result: ❌ Error - URI not whitelisted (strict/lenient)
# Result: ✅ Allowed only if policy is disabled (local dev only)
```

## Troubleshooting

### Common Issues

#### 1. "post_logout_redirect_uri is not whitelisted"

**Cause:** URI is not in client's `post_logout_redirect_uris` list

**Solutions:**
- Add the URI to the whitelist via Admin UI or API
- Switch to lenient policy and configure `default_logout_uri`
- For local dev, use disabled policy (not for production)

#### 2. "Logout failed because query parameter post_logout_redirect_uri is missing"

**Cause:** Strict policy requires the parameter, but it's not provided

**Solutions:**
- Provide `post_logout_redirect_uri` in logout request
- Switch to lenient policy with configured `default_logout_uri`
- Update application code to include the parameter

#### 3. "disabled logout redirect policy is not allowed in production environment"

**Cause:** Attempting to use disabled policy when `AUTHWAY_ENV=production`

**Solutions:**
- Use strict or lenient policy instead
- Only use disabled policy in local development environments
- Set `AUTHWAY_ENV` to non-production value for development

#### 4. Wildcard patterns not matching

**Cause:** `allow_wildcard_logout` is `false` or pattern syntax is incorrect

**Solutions:**
- Enable `allow_wildcard_logout` in client configuration
- Verify pattern syntax: `http://localhost:*` or `https://*.example.com`
- Check that wildcard is in the correct position (port or subdomain)

### Debug Mode

Enable debug logging to troubleshoot logout validation:

```bash
export AUTHWAY_LOG_LEVEL=debug
```

Debug logs will show:
- Policy being applied
- URI matching attempts
- Whitelist comparison results
- Wildcard pattern matching

## Version Compatibility

- **Introduced:** v0.1.5
- **Migration Required:** Yes (automatic via `003_add_logout_policy.sql`)
- **Breaking Changes:** None (backward compatible)
- **Database Schema:** PostgreSQL 12+

## References

- [OpenID Connect RP-Initiated Logout](https://openid.net/specs/openid-connect-rpinitiated-1_0.html)
- [OAuth 2.0 Security Best Current Practice](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-security-topics)
- [Ory Hydra Post-Logout Redirect](https://www.ory.sh/docs/hydra/guides/logout#post-logout-redirect)

## Support

For issues or questions:
- GitHub Issues: https://github.com/yourusername/authway/issues
- Documentation: https://github.com/yourusername/authway/docs
