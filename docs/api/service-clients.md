# Service Clients

A **service client** is a tenant-scoped, revocable machine-to-machine (M2M)
credential that lets a consumer application create OAuth clients
programmatically — without holding the long-lived `AUTHWAY_ADMIN_API_KEY` or
an admin session.

Under the hood a service client is a Hydra `client_credentials` OAuth2Client
paired with an Authway-side row that records which tenant it belongs to and
which scopes it was granted. Hydra is the credential store (`client_id` /
`client_secret`); Authway only tracks the tenant + scope mapping and whether
the credential has been revoked.

Today the only scope a service client can be granted is
`admin.clients:write`, which authorizes exactly one action: creating OAuth
clients in the credential's own tenant via `POST /api/v1/clients`.

---

## 1. Provisioning a service client

```http
POST /api/v1/tenants/{tenant_id}/service-clients
Authorization: Bearer <AUTHWAY_ADMIN_API_KEY or admin session token>
Content-Type: application/json

{
  "name": "my-app-provisioner",
  "scopes": ["admin.clients:write"]
}
```

This endpoint is **admin-only** — it mints a new M2M credential, so it
requires the same authentication as other tenant management routes (the
admin API key or an admin session token), never a service client's own
credential.

Response (`201 Created`):

```json
{
  "message": "Service client created successfully",
  "service_client": {
    "id": "b6e2b6b0-....",
    "tenant_id": "1c2b3a4d-....",
    "hydra_client_id": "authway_svc_....",
    "name": "my-app-provisioner",
    "granted_scopes": ["admin.clients:write"],
    "revoked_at": null,
    "created_at": "2026-08-21T00:00:00Z"
  },
  "credentials": {
    "client_id": "authway_svc_....",
    "client_secret": "..."
  }
}
```

`credentials.client_secret` is shown **exactly once**, in this response. It
is generated with `crypto/rand`, handed to Hydra at registration time, and
never stored by Authway — there is no "reveal secret" endpoint and no
recovery path. If it is lost, revoke the service client (below) and
provision a new one.

---

## 2. Getting an access token

Use the standard OAuth2 `client_credentials` grant against Hydra's **public**
token endpoint (not the Authway API, and not Hydra's admin endpoint):

```http
POST {hydra_public_url}/oauth2/token
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials
&client_id=authway_svc_....
&client_secret=...
&scope=admin.clients:write
```

`hydra_public_url` is the same Hydra public URL Authway itself uses for
authorization-code flows (`AUTHWAY_HYDRA_PUBLIC_URL`, default
`http://localhost:4444` in local/dev setups). The service client was
registered in Hydra with `token_endpoint_auth_method: client_secret_post`
(provisioning step 1), so `client_id`/`client_secret` go in the form body as
shown above — not as an HTTP Basic `Authorization` header. Hydra returns a
standard token response; the `access_token` is the bearer token used in the
next step.

---

## 3. Using the token to create a client

```http
POST /api/v1/clients
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "name": "my-app",
  "public": false,
  "redirect_uris": ["https://my-app.example/callback"],
  "grant_types": ["authorization_code", "refresh_token"],
  "scopes": ["openid", "profile", "email"],
  "allowed_origins": []
}
```

Authway detects that the bearer token belongs to a service client (via Hydra
token introspection) and routes the request through a reduced code path.
Compared to an admin-authenticated `POST /clients`, this path:

- Accepts **exactly six** body fields: `name`, `public`, `redirect_uris`,
  `grant_types`, `scopes`, `allowed_origins`. Every other admin-only field
  (Google/GitHub/Microsoft/Apple provider secrets, MFA settings, logout
  redirect policy, a caller-supplied `client_id`/`client_secret`, …) is
  unavailable on this path — even if included in the body, it is ignored.
- Restricts `grant_types` to `authorization_code` and `refresh_token` only.
  `client_credentials` and `implicit` are rejected — a service client must
  never be able to mint another M2M credential.
- Ignores any `tenant_id` in the body. The new client is always created in
  the tenant the service client was provisioned for (from step 1), regardless
  of what the request body says.

The response shape matches a normal client-creation response (`client` +
`credentials`, with the new client's own one-time secret).

Authentication for this route is layered cheap-to-expensive: it first tries
the admin API key, then an admin session token, and only if both miss does
it fall back to Hydra token introspection for a service client credential —
so admin callers pay no extra cost. A token that fails all three, or whose
service client has been revoked, or whose granted scopes don't include
`admin.clients:write`, is rejected with `401 Unauthorized` (or `403
Forbidden` for a valid-but-under-scoped credential).

---

## 4. Revoking a service client

```http
DELETE /api/v1/tenants/{tenant_id}/service-clients/{service_client_id}
Authorization: Bearer <AUTHWAY_ADMIN_API_KEY or admin session token>
```

Admin-only, for the same reason as provisioning. Revocation is immediate:

- The service client's Authway-side record is marked revoked. Every
  subsequent request authenticated with that credential — including one
  using an access token minted before the revoke — is rejected on its very
  next request, because the auth path checks revocation status on every
  introspection rather than trusting Hydra token validity alone.
- The underlying Hydra OAuth2Client is deleted (best-effort — if the Hydra
  call itself fails, the Authway-side revocation above still blocks the
  credential; a warning is logged so the orphaned Hydra client can be
  cleaned up separately). Once deleted, Hydra can no longer mint any new
  access token for that `client_id` at all.

Response (`200 OK`):

```json
{ "message": "Service client revoked successfully" }
```
