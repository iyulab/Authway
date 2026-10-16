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

## 5. Asking the user to approve one action

Some actions should not run on the delegation alone: the user is to say yes
to this one. The client asks; the user answers on Authway's approval screen;
the client polls and receives a token bound to that approval.
`capabilities.approval_requests` is `true` where this is offered. It is not
OpenID CIBA: no ID token is issued (`capabilities.ciba` stays `false`).

**Ask.** `POST /oauth2/approval-requests` on the API (form), with the client's
authentication as at the token endpoint — `client_secret_post`, or
`private_key_jwt` with an assertion addressed to the token endpoint (each
assertion once):

| Parameter | |
|---|---|
| `subject_token` | the delegation handle (`delegation_grant`) |
| `subject_token_type` | `urn:authway:params:oauth:token-type:delegation-grant` |
| `binding_message` | a short code, up to 200 characters, the user compares with what your application shows — not a description of the action |
| `authorization_details` | optional: a JSON array of objects with a `type` (RFC 9396). Kept and put in the token as given; the approval screen shows only each `type`. Use an opaque reference to the action, so a resource server can check from the token alone that the approval is for that action |
| `scope` | optional: within the delegation |
| `requested_expiry` | optional: 30 to 600 seconds, 300 by default |

The answer is `auth_req_id` (the polling secret — keep it), `approve_url`,
`expires_in` and `interval`. **Bring `approve_url` to the user yourself** — a
notification, a message, a link on your own screen, with the same code.
Authway does not contact the user.

**The user answers.** `approve_url` opens the approval screen. The user signs
in to their account there (no screen when their session is still there),
compares the code, sees the kinds of action (`type`) and the permissions, and
approves or denies. Answering needs a sign-in within the last ten minutes; a
token your application holds for the user cannot answer.

**Poll.** The token endpoint, with
`grant_type=urn:authway:params:oauth:grant-type:approval`, `auth_req_id` and
the client's authentication, no more often than `interval`:

| Answer | |
|---|---|
| `authorization_pending` | not answered yet |
| `slow_down` | asked within `interval` |
| `access_denied` | the user denied |
| `expired_token` | not answered in time — take it as a denial |
| `invalid_grant` | the token was already issued, or the request is not this client's |
| 200 | one access token: `sub` the user, `act.sub` your client, `approval.id`, `authorization_details` as you sent them; at most five minutes, no refresh token |

The token is issued once. A resource server checks `approval.id` or
`authorization_details` against the action it is asked to perform.

The audit log records `approval.requested`, `approval.approved`,
`approval.denied` and `approval.exchanged`.

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
