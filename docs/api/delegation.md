# Acting for a user who is away (delegation)

An agent, a scheduled job or a connector sometimes has to act for a user who
is not at the screen. Authway lets the user grant that once, and lets the
client turn the grant into a short-lived access token whenever it needs one —
an access token whose subject is the user and whose actor is the client, so a
resource server can tell "the user did this" from "a client did this for the
user" (RFC 8693, OAuth 2.0 Token Exchange).

`capabilities.token_exchange` is `true` where this is offered.

## 1. Register the client

The client authenticates when it exchanges, so it is confidential. It needs
the token exchange grant and may ask for the `delegation` scope; it also
signs users in itself (`authorization_code`) to be granted the delegation:

```http
POST /api/v1/clients
Authorization: Bearer <admin key>

{
  "tenant_id": "…",
  "name": "Orders agent",
  "public": false,
  "redirect_uris": ["https://agent.example.com/callback"],
  "grant_types": ["authorization_code", "urn:ietf:params:oauth:grant-type:token-exchange"],
  "scopes": ["openid", "profile", "orders.read", "delegation"],
  "audience": ["https://orders.example.com"]
}
```

A public client with the grant is `400 public_client_with_token_exchange`;
the `delegation` scope without it is `400 delegation_without_token_exchange`.

## 2. Ask the user

Send the user through an ordinary authorization request that includes the
`delegation` scope:

```
scope=openid profile orders.read delegation
```

The consent screen tells the user that the application will act for them
while they are not signed in, with the other permissions listed, until they
revoke it. It asks even when the client is registered to skip consent,
unless the user's earlier answer to it is remembered.

If the user consents, the ID token of that sign-in carries the delegation's
**handle** in the claim `delegation_grant`. It is shown this once; keep it
with the client's secret. The handle is no use to anyone but this client: it
is only accepted together with the client's own authentication.

The delegation's ceiling is the other scopes the user granted in that
consent (`delegation` itself excluded). Each consent to the scope grants a new
delegation with a new handle; a user has at most 20 in force per client, the
oldest being revoked past that.

## 3. Exchange the handle

Whenever the client needs to act, it exchanges the handle at the token
endpoint discovery names (`token_endpoint`):

```http
POST {token_endpoint}
Content-Type: application/x-www-form-urlencoded

grant_type=urn:ietf:params:oauth:grant-type:token-exchange
&subject_token=<handle>
&subject_token_type=urn:authway:params:oauth:token-type:delegation-grant
&client_id=<client_id>&client_secret=<client_secret>
&scope=orders.read                       (optional — within the delegation)
&audience=https://orders.example.com     (optional — among the registered audiences)
```

```json
{
  "access_token": "…",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "token_type": "bearer",
  "expires_in": 299,
  "scope": "orders.read"
}
```

There is no refresh token: the token lasts at most five minutes, and the
client exchanges again when it needs another. Refusals follow RFC 6749/8693:

| Error | When |
|---|---|
| `invalid_request` | `subject_token_type` is not the delegation type, an `actor_token` is sent, or another `requested_token_type` |
| `invalid_client` | the client's authentication fails |
| `invalid_grant` | the handle is unknown, revoked, another client's, or its user is suspended / tenant or client turned off |
| `invalid_scope` | a scope outside the delegation |
| `invalid_target` | an audience the client is not registered for |
| `unauthorized_client` | the client is not registered for token exchange |

## 4. At the resource server

Validate the access token as any other (the resource server profile in the
contract): issuer, signature, expiry, and that `aud` **contains** your
identifier. Then:

- `sub` is the user the client acts for.
- `act.sub` is the client acting (`client_id` says the same).
- `tenant_id` is the user's tenant.

A token without `act` is the user's own. Introspection
(`POST /api/v1/oauth2/introspect`) answers `act` too.

## Revoking

- The user: `GET /api/v1/users/delegations`, `DELETE /api/v1/users/delegations/{id}`
  (their own access token).
- An administrator: `GET /api/v1/delegations?user_id=…&client_id=…`,
  `DELETE /api/v1/delegations/{id}` (tenant-scoped).

A revoked delegation cannot be exchanged again, and the user's remembered
consent to the client is withdrawn with it — along with the client's tokens
for the user — so the client asks before it is granted another. Access tokens
already exchanged run out within their five minutes.

The audit log records `delegation.granted`, `delegation.revoked` and
`delegation.exchanged`.
