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
all. A missing or rejected credential answers `401` with
`{"error": "...", "code": "unauthorized"}`.

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
  "code": "upstream_sync_failed",
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
| `public_client_missing_allowed_origins` | `public=true` with `authorization_code`, a web redirect URI (`https://…`, or `http://` to a named host such as `localhost`) and no `allowed_origins` | Set `allowed_origins` to the SPA origin(s). A native app needs none — see below |
| `invalid_redirect_uri` | A `redirect_uris` entry is not an absolute URI, or carries a fragment | Use `https://app.example.com/callback`, `http://127.0.0.1/callback` or `com.example.app:/callback` |
| `confidential_client_unsupported_grants` | `public=false` with no credential-bearing grant | Use one of: authorization_code, client_credentials, refresh_token, password |
| `public_client_has_no_secret` | `POST /clients/{id}/regenerate-secret` on a public client | A public client uses PKCE; register a confidential client if it can keep a secret |
| `invalid_backchannel_logout_uri` | `backchannel_logout_uri` is not an absolute `https` URL, has a fragment, or points at a local or private address (as written, or by what its host name resolves to) | Give the URL of an endpoint on the application's backend, e.g. `https://app.example.com/backchannel-logout` |
| `invalid_audience` | An `audience` entry is empty, longer than 255 characters, contains whitespace or is repeated | Name each API by one identifier, usually its URL |
| `invalid_token_endpoint_auth_method` | `token_endpoint_auth_method` is not one of `client_secret_post`, `private_key_jwt`, `none`; or does not fit the kind of client (`none` for a confidential one, a secret or a key for a public one); or a `client_secret` is given to a client that authenticates with a key | Leave it out for the default, or see [below](#clients-that-hold-a-key-instead-of-a-secret-private_key_jwt) |
| `invalid_jwks` | A client that authenticates with a key gives neither or both of `jwks` and `jwks_uri`; `jwks` is not a set of 1 to 10 RSA, EC or OKP public keys, or holds a private key; or keys are given to a client that does not authenticate with one | Register the public half only: `{"keys": [{"kty": "RSA", "kid": "…", "n": "…", "e": "AQAB"}]}` |
| `invalid_jwks_uri` | `jwks_uri` is not an absolute `https` URL, has a fragment, or points at a local or private address | Give the public address the key set is served at |
| `token_endpoint_auth_method_is_fixed` | An update names a different `token_endpoint_auth_method`, or would make a client that authenticates with a key public | Register a new client and move the application to it |
| `client_has_no_secret` | `POST /clients/{id}/regenerate-secret` on a client that authenticates with a key | Update `jwks` or `jwks_uri` to change its key |

### Application-type matrix

| Stack                       | `public` | `grant_types` example | Notes |
|----------------------------|----------|----------------------|-------|
| ASP.NET Core (server-side) | `false`  | `["authorization_code", "refresh_token"]` | Sends `client_secret_post` automatically |
| Next.js (auth.js)          | `false`  | `["authorization_code", "refresh_token"]` | Server-side session — confidential |
| Blazor Server              | `false`  | `["authorization_code", "refresh_token"]` | |
| Blazor WebAssembly         | `true`   | `["authorization_code"]` + PKCE | Set `allowed_origins` |
| React / Vue / Angular SPA  | `true`   | `["authorization_code"]` + PKCE | Set `allowed_origins` |
| Desktop / CLI / mobile     | `true`   | `["authorization_code"]` + PKCE | System browser; loopback or private-use scheme redirect, no `allowed_origins` — see below |
| M2M / service-to-service   | `false`  | `["client_credentials"]`         | No user, no redirect_uri |

### Native apps: desktop, command line, mobile

A native app signs the user in with the system browser and receives the
authorization code in one of two ways ([RFC 8252](https://www.rfc-editor.org/rfc/rfc8252)):

- **Loopback address** — the app listens on a port the operating system picks
  when the sign-in starts. Register the redirect URI **without a port**, on the
  IP literal: `http://127.0.0.1/callback` (and `http://[::1]/callback` for
  IPv6). An authorization request whose `redirect_uri` differs from the
  registered one only in its port is accepted, so
  `http://127.0.0.1:51234/callback` works without registering any port. The
  path must match exactly. `http://localhost/…` does not get this treatment —
  use the IP literal.
- **Private-use URI scheme** — `com.example.app:/oauth2redirect`, registered
  with the operating system by the app.

```json
{
  "name": "Example Desktop",
  "public": true,
  "redirect_uris": ["http://127.0.0.1/callback", "http://[::1]/callback"],
  "grant_types": ["authorization_code", "refresh_token"],
  "scopes": ["openid", "profile", "email"]
}
```

No `allowed_origins` is needed: the token request comes from the app, not from
a page in a browser, so there is no origin to allow. Use PKCE (`S256`) on every
request; a public client has no secret.

### Applications that keep their own session: `backchannel_logout_uri`

An application with a server-side session registers where it wants to be told
that the user's session at the provider has ended:

```json
{ "backchannel_logout_uri": "https://app.example.com/backchannel-logout" }
```

The provider POSTs a logout token there whenever a session of a user signed in
to the application ends. On update, an empty string clears it; leaving the
field out keeps it.

The call is made from inside the deployment, so the address has to be one the
deployment may call on a client's say-so: **`https`, at a public address**.
`localhost`, loopback, private, shared and link-local addresses are refused —
written as such or behind a host name, which is looked up when the client is
saved — and so is plain `http`, which would carry the logout token in the
clear. A deployment whose applications run on its own network, or a developer
with a receiver on their machine, sets `AUTHWAY_ALLOW_PRIVATE_TARGETS=true`;
link-local addresses (cloud metadata endpoints among them) stay refused even
then. What the application does with the token is in
[FEATURES.md](../FEATURES.md#telling-other-applications-about-a-logout).

### Clients whose tokens go to an API: `audience`

When an application calls an API of yours with the user's access token, the
API has to know the token was issued **for it** — otherwise it accepts the
token of every other application of the same issuer. Name the API on the
client:

```json
{
  "name": "Orders app",
  "public": true,
  "redirect_uris": ["https://orders.example.com/callback"],
  "allowed_origins": ["https://orders.example.com"],
  "grant_types": ["authorization_code", "refresh_token"],
  "scopes": ["openid", "profile", "email"],
  "access_token_strategy": "jwt",
  "audience": ["https://orders-api.example.com"]
}
```

- `audience` is a list of identifiers, each the name an API knows itself by —
  by convention its URL. Authway does not call it.
- The client's access tokens carry them as the `aud` claim, with no change to
  the application's authorization request. A request that names some of them
  (`audience` parameter) gets those only; one it is not registered for is
  never granted.
- The API checks that `aud` contains its own identifier. Every JWT library has
  this as its *audience* setting.
- A client registered for none gets tokens with an empty `aud`, which an API
  that checks its audience refuses.
- `PUT /api/v1/clients/{id}` with `"audience": []` clears it; leaving the field
  out keeps it.

This applies to tokens issued to a user (authorization code). A
machine-to-machine client (`client_credentials`) names the audience it wants in
its token request (`audience` parameter).

### Clients that hold a key instead of a secret: `private_key_jwt`

A confidential client is given a secret, and sends it to the token endpoint.
A client that can keep a key need not share anything secret with the
provider: it registers the public half and proves who it is by signing a
short-lived assertion with the private half (RFC 7523). Nothing that would
let someone act as the client is stored by the provider, and the key never
leaves where the client runs.

```json
{
  "name": "Orders gateway",
  "public": false,
  "grant_types": ["client_credentials"],
  "token_endpoint_auth_method": "private_key_jwt",
  "jwks": { "keys": [ { "kty": "RSA", "kid": "2026-10", "use": "sig", "n": "…", "e": "AQAB" } ] }
}
```

- Give `jwks` (the key set itself) or `jwks_uri` (a public https address the
  provider fetches it from), not both. The answer carries no `client_secret`.
- At the token endpoint the client sends, in place of a secret,
  `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer`
  and `client_assertion=<JWT>`. The JWT has `iss` and `sub` equal to the
  `client_id`, `aud` equal to the token endpoint's URL, a `jti` used once and
  an `exp` a few minutes ahead, and names its key in the `kid` header.
- **Changing the key:** `PUT /clients/{id}` with a new `jwks` (or
  `jwks_uri`) replaces the set. To change keys without a gap, register the
  new key alongside the old one, move the client to it, then remove the old
  one. An assertion signed with a key no longer registered is refused.
- How a client authenticates is decided when it is registered. To move an
  application from a secret to a key, register a new client.
- The method is offered where the provider's discovery document lists
  `private_key_jwt` under `token_endpoint_auth_methods_supported`.
- `POST /api/v1/oauth2/introspect` authenticates its caller with a client
  secret: a client that holds a key cannot call it.
