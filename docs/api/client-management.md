# Client Management API

The `/api/v1/clients/*` endpoints manage OAuth2/OIDC clients. Most of them
require admin authentication — either the long-lived `AUTHWAY_ADMIN_API_KEY`
(via `Authorization: Bearer …`) or an admin session token issued by
`/admin/login`.

`POST /api/v1/clients` (client creation) additionally accepts a third method:
a scoped **service client** credential — a tenant-scoped `client_credentials`
bearer token obtained from Hydra. A request authenticated this way may only
create clients in the tenant the credential was provisioned for, and is
restricted to a reduced field set (no provider secrets, no MFA/logout policy,
no custom `client_id`/`client_secret`). See
[Service Clients](./service-clients.md) for provisioning, token acquisition,
and the exact request restrictions. Every other `/clients*` route (`GET`,
`PUT`, `DELETE`, `regenerate-secret`, …) is unchanged and still accepts only
the two admin methods above — except the public `GET
/clients/{client_id}/config` endpoint, which requires no authentication at
all.

This document covers two semantics that are easy to get wrong: **Hydra sync
visibility** and **client-config validation**.

---

## Hydra sync — best-effort by default, strict on demand

Authway stores OAuth client configuration in its own database and replicates
relevant fields (`redirect_uris`, `grant_types`, secret, …) to Ory Hydra.
Hydra is the actual OAuth2 authorization server, so a config change isn't
"live" until Hydra sees it.

### What you used to see (≤ 0.2.x)

```http
PUT /api/v1/clients/{id}
Content-Type: application/json

{ "redirect_uris": ["https://app.example/new-callback"] }

→ HTTP/1.1 200 OK
{ "message": "Client updated successfully", "client": {...} }
```

If Hydra rejected the change (network failure, schema mismatch, 5xx), the
response was **identical** — only a server log line indicated the failure.
First-time observable symptom was an `invalid_redirect_uri` error during the
next OAuth flow. This is the issue that motivated the change.

### What you see now (≥ 0.3.0) — best-effort + visibility

Every mutation includes a `sync_status` envelope:

```json
{
  "message": "Client updated successfully",
  "client": { "...": "..." },
  "sync_status": { "state": "ok" }
}
```

When Hydra rejects the change, the DB is still updated (best-effort) but
`sync_status` reports the upstream failure:

```json
{
  "message": "Client updated successfully",
  "client": { "...": "..." },
  "sync_status": {
    "state": "failed",
    "error": "hydra: 502 Bad Gateway: connection refused"
  }
}
```

Callers MUST inspect `sync_status.state`. Treat anything other than `"ok"` or
`"skipped"` as drift between Authway and Hydra; either retry the mutation or
trigger a reconciliation via `POST /api/v1/clients/sync-hydra`.

### Strict mode — fail loudly

For migration scripts, CI, or anything where silent drift is unacceptable,
add `?strict_sync=true`:

```http
PUT /api/v1/clients/{id}?strict_sync=true
```

When the upstream sync fails:

```http
HTTP/1.1 502 Bad Gateway
Content-Type: application/json

{
  "error": "Upstream OAuth provider sync failed",
  "sync_status": { "state": "failed", "error": "..." },
  "hint": "Authway DB was updated but Hydra rejected the change. Retry, or omit ?strict_sync=true to accept best-effort sync (drift visible in sync_status)."
}
```

The DB write is **not rolled back** — the change is in Authway but not in
Hydra. Run the operation again after addressing the underlying issue, or
explicitly accept the drift by re-issuing without `strict_sync`.

### When to use which

| Caller                       | Recommended      |
|------------------------------|------------------|
| Admin Console UI             | best-effort + show `sync_status` warning if drift |
| Programmatic client config edits at runtime | best-effort + monitoring on `state != "ok"` |
| Migration scripts / Terraform | `strict_sync=true` |
| CI smoke tests               | `strict_sync=true` |

Endpoints affected: `PUT /clients/{id}`, `DELETE /clients/{id}`,
`POST /clients/{id}/regenerate-secret`. (`POST /clients` already returns 5xx
on Hydra failure with DB rollback — strict by construction.)

---

## Client config validation

Create/Update reject internally inconsistent client configurations with a
`400 Bad Request` and a structured body. Each error has a stable `code`,
the offending `field`, a human `message`, and an actionable `hint`.

```json
{
  "error": "Public clients must not be assigned a client_secret",
  "code": "public_client_with_secret",
  "field": "client_secret",
  "hint": "Either set public=false (confidential, recommended for ASP.NET / Next.js / Blazor Server / any backend), or omit client_secret and use PKCE (recommended for SPA / native / mobile)."
}
```

### Codes

| Code | Trigger | Fix |
|------|---------|-----|
| `public_client_with_secret` | `public=true` and `client_secret` non-empty | Either remove the secret (use PKCE) or set `public=false` |
| `public_client_with_client_credentials` | `public=true` and `grant_types` includes `client_credentials` | M2M clients must be confidential |
| `public_client_with_password_grant` | `public=true` and `grant_types` includes `password` | Use authorization_code + PKCE |
| `public_client_missing_allowed_origins` | SPA config without CORS allow-list | Set `allowed_origins` to the SPA origin(s) |
| `confidential_client_unsupported_grants` | `public=false` with no credential-bearing grant | Use one of: authorization_code, client_credentials, refresh_token, password |

### Application-type matrix

| Stack                       | `public` | `grant_types` example | Notes |
|----------------------------|----------|----------------------|-------|
| ASP.NET Core (server-side) | `false`  | `["authorization_code", "refresh_token"]` | Sends `client_secret_post` automatically |
| Next.js (auth.js)          | `false`  | `["authorization_code", "refresh_token"]` | Server-side session — confidential |
| Blazor Server              | `false`  | `["authorization_code", "refresh_token"]` | |
| Blazor WebAssembly         | `true`   | `["authorization_code"]` + PKCE | Set `allowed_origins` |
| React / Vue / Angular SPA  | `true`   | `["authorization_code"]` + PKCE | Set `allowed_origins` |
| Native / mobile            | `true`   | `["authorization_code"]` + PKCE | Use system browser; `allowed_origins` not strictly needed |
| M2M / service-to-service   | `false`  | `["client_credentials"]`         | No user, no redirect_uri |
