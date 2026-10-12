# Custom claims

An application's backend can store claims for a user — a plan, a role, an
organization — and Authway carries them in the tokens it issues to that user.
[`packages/contract/openapi.yaml`](../../packages/contract/openapi.yaml)
describes the endpoints (`getUserClaims`, `patchUserClaims`);
`GET /api/v1/capabilities` reports them as `custom_claims`.

**Only a trusted party writes these claims.** A relying party may decide what
the user is allowed to do from them, so the user — and anything holding the
user's access token, such as a browser app — cannot. The callers are:

- the admin API key or an admin console session, for any user;
- a tenant's service credential granted the `admin.claims:write` scope, for
  that tenant's users only (a user of another tenant answers `404`).

A service credential is created by an administrator with
`POST /api/v1/tenants/{id}/service-clients` and `"scopes": ["admin.claims:write"]`;
the backend exchanges it for an access token with the `client_credentials`
grant and calls the endpoints below with that token.

---

## 1. Reading a user's claims

```http
GET /api/v1/users/{id}/claims
Authorization: Bearer <admin key, or the service credential's access token>
```

answers `{ "claims": { … } }`. `{id}` is the user's id — the `sub` of their tokens.

---

## 2. Setting and removing claims

```http
PATCH /api/v1/users/{id}/claims
Authorization: Bearer <admin key, or the service credential's access token>
Content-Type: application/json

{ "claims": { "plan": "pro", "seats": 5, "trial": null } }
```

`claims` is a JSON merge patch (RFC 7396): a value sets the claim, `null`
removes it, a claim not named is left alone. The answer is the claims stored
afterwards, `{ "claims": { … } }`. Each change is recorded in the audit log as
`user.claims_updated`, with the names set and removed (not the values).

A patch is refused as a whole, changing nothing, when it:

| Answer | `code` | Why |
|---|---|---|
| `400` | `reserved_claim` | It sets a claim Authway asserts (`tenant_id`, `email`, `name`, `email_verified`), one OAuth or OpenID Connect defines (`sub`, `aud`, `exp`, `auth_time`, `amr`, `scope`, …), or one starting with `_`. `keys` lists them. Removing such a claim (`null`) is allowed. |
| `400` | `invalid_request` | A claim name is empty or longer than 255 characters, or the body has no `claims` object. |
| `413` | `claims_too_large` | The user's claims would take more than 8 KB as JSON — they travel in every token. |

---

## 3. Where the claims appear

Stored claims go into the tokens issued **from the user's next authorization
on**; tokens issued earlier, and tokens refreshed from them, keep the claims
they had. After changing a claim the application sends the user through
authorization again. With the user's session still active there is no sign-in
form; the consent screen appears as it would for any authorization — not at
all for a client that skips it or while the user's earlier consent is
remembered. Stored claims never change whether the user is asked. For a
silent renewal use `prompt=none`.

- **ID token**: at the top level.
- **Access token**: under `ext` (Authway's authorization server nests session
  claims there). A deployment that lists a claim's name in
  `HYDRA_ALLOWED_TOP_LEVEL_CLAIMS` also gets it at the top level of a JWT
  access token.

Authway's own claims (`tenant_id`, `email`, `name`, `email_verified`) are
always Authway's: a stored claim never replaces them.
