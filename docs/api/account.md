# The user's own account

An application acts for its signed-in user with the **user's access token** —
the one it received from the authorization server — as a bearer token. These
calls need no admin key. [`packages/contract/openapi.yaml`](../../packages/contract/openapi.yaml)
describes them under the `account` tag.

---

## 1. Reading the profile

```http
GET /api/v1/profile/me
Authorization: Bearer <the user's access token>
```

answers `{ "id", "email", "name", "email_verified", "created_at", "updated_at" }`.
`id` is the `sub` of the user's tokens.

---

## 2. Deleting the account

```http
DELETE /api/v1/profile/me
Authorization: Bearer <the user's access token>
```

On `200` the account is gone:

- the user's sign-in sessions, consents and every token issued under them are
  ended at the authorization server — the access token used for this call
  stops working, as does any refresh token;
- the account is deleted;
- [`user.deleted`](webhooks.md#events) is sent, with the user as both
  `resource` and `actor`, so services that keep data about the user can remove
  it. The event carries the user's id (their `sub`) and nothing else.

A deployment offers this when `GET /api/v1/capabilities` answers
`"account_deletion": true`.

### The user must have signed in recently

Deleting an account cannot be undone, so a token from a session that has been
open for days — or a token someone else got hold of — is not enough. The user
must have **signed in within the last 10 minutes**.

Access tokens carry `auth_time`: when the user of the session the token came
from last signed in. Refreshing a token, or getting a new one from a session
that is still open, does not change it; only signing in does.

When it is too old (or unknown), the answer is the step-up challenge of
[RFC 9470](https://www.rfc-editor.org/rfc/rfc9470):

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer error="insufficient_user_authentication", error_description="A more recent sign-in is required", max_age="600"

{ "error": "Sign in again to delete your account.", "code": "insufficient_user_authentication", "max_age": 600 }
```

Send the user through authorization again with that `max_age` — the
authorization server asks them to sign in if their last sign-in is older —
and retry with the token you get back:

```
GET <authorization_endpoint>?client_id=…&response_type=code&scope=openid&max_age=600&…
```

A typical "delete my account" button therefore does:

1. `DELETE /api/v1/profile/me`.
2. On `401 insufficient_user_authentication`: start authorization with
   `max_age` from the answer, exchange the code, then repeat step 1 with the
   new access token.
3. On `200`: discard the tokens you hold and show the signed-out state.

### Other answers

| Status | `code` | Meaning |
|--------|--------|---------|
| `401` | `unauthorized` | No token, or one that is no longer valid |
| `404` | `not_found` | The account no longer exists |
| `502` | `authorization_server_unavailable` | The sessions could not be ended; **nothing was deleted** — try again |

An administrator deleting a user (`DELETE /api/v1/users/{id}`) ends that
user's sessions and tokens in the same way.
