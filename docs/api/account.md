# The user's own account

An application acts for its signed-in user with the **user's access token** —
the one it received from the authorization server — as a bearer token. These
calls need no admin key. An application that would rather not build these
screens links its users to [the account screen](#the-account-screen) instead. [`packages/contract/openapi.yaml`](../../packages/contract/openapi.yaml)
describes the profile calls (sections 1 and 2) and the two-factor calls
(section 3) under the `account` tag; the sign-out call (section 4) is not part
of that contract.

---

## 1. Reading the profile

```http
GET /api/v1/profile/me
Authorization: Bearer <the user's access token>
```

answers `{ "id", "email", "name", "email_verified", "has_password",
"created_at", "updated_at" }`. `id` is the `sub` of the user's tokens;
`has_password` is false for an account that signs in only by an emailed link
or another provider.

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

{ "type": "about:blank", "title": "Unauthorized", "status": 401, "detail": "Sign in again to delete your account.", "code": "insufficient_user_authentication", "max_age": 600 }
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

---

## 3. Two-factor authentication

A user adds an authenticator app (TOTP) as a second factor with the calls
below. A deployment offers them when `GET /api/v1/capabilities` lists `"totp"`
under `mfa`. Each call takes the user's access token and acts on that user
only.

### Turning it on

```http
POST /api/v1/users/mfa/setup
Authorization: Bearer <the user's access token>
```

answers `{ "secret", "otpauth_uri", "qr_image", "issuer", "account_name" }`.
`secret` is the TOTP secret (base32), for typing into an authenticator by
hand; `otpauth_uri` is the same secret as an `otpauth://` URI — show it as a
QR code, or show `qr_image`, the same URI already drawn as a PNG `data:` URL
for an `<img>`. `account_name` is the user's email address. Nothing is turned on yet,
and until it is, each call issues a new secret in place of the previous one.
Once the second factor is on, `setup` answers `409` (`mfa_already_enabled`):
the factor in use is not replaced by a call that needs only the user's token.
To change authenticators, turn it off first.

```http
POST /api/v1/users/mfa/verify
Authorization: Bearer <the user's access token>
Content-Type: application/json

{ "code": "123456" }
```

`code` is the current 6-digit code from the authenticator. On `200` the second
factor is on — every sign-in now asks the user for a code, whether it starts
with a password, an emailed link or another identity provider (`next: mfa`
in the [login flow](login-flows.md#login)) — and the answer is
`{ "recovery_codes": ["ABCD-EFGH-IJKL", …], "message", "sessions_ended" }`:
eight single-use recovery codes. They are shown only here; have the user save
them.

Turning the factor on **ends the user's other sessions**, and the tokens
issued in them: they were opened on one factor, and whoever held one would
otherwise keep the account past the second. Applications signed in on those
sessions are told (back-channel logout). The session of the request stays —
its user has just given a code and is looking at the recovery codes.
`sessions_ended` says what happened:

| `sessions_ended` | |
|---|---|
| `others` | Every other session of the user has ended; the token used here still works |
| `all` | Every session has ended, **this one too**: the access token did not say which session it came from (it was issued before sessions were named in tokens). Show the recovery codes from this answer, then send the user to sign in again |
| `none` | The factor is on, but the sessions could not be ended (the authorization server did not answer). The user can sign out everywhere themselves |

A refusal is problem details with a `code`, like every other call ([Errors](errors.md)):

| `code` | Status | When |
|---|---|---|
| `mfa_already_enabled` | 409 | `setup` or `verify` while the factor is on |
| `mfa_not_set_up` | 409 | `verify` before `setup` |
| `invalid_code` | 400 | `verify` with a code that is not current, or already used |
| `mfa_code_required` | 403 | Turning the factor off or regenerating recovery codes without proof |
| `mfa_not_enabled` | 409 | Turning off or regenerating while the factor is off |

### Status

```http
GET /api/v1/users/mfa/status
Authorization: Bearer <the user's access token>
```

answers `{ "enabled", "enabled_at", "recovery_codes_left" }`. `enabled_at` is
left out while the second factor is off.

### Recovery codes

```http
POST /api/v1/users/mfa/recovery/regenerate
Authorization: Bearer <the user's access token>
Content-Type: application/json

{ "code": "123456" }
```

replaces the user's recovery codes with eight new ones and answers
`{ "recovery_codes": […], "message" }`. The earlier codes stop working.
`code` is proof of the second factor: a current code from the authenticator,
or an unused recovery code (which is used up). Without it the answer is `403`
(`mfa_code_required`).

### Turning it off

```http
DELETE /api/v1/users/mfa
Authorization: Bearer <the user's access token>
Content-Type: application/json

{ "code": "123456" }
```

`code` is proof of the second factor, as for regenerating recovery codes: a
current authenticator code or an unused recovery code. The user's token alone
is not enough — it is one factor, and must not be able to remove the other.
Without valid proof the answer is `403` (`mfa_code_required`), and repeated
wrong codes are rate limited like failed sign-ins.

On `200` (`{ "message" }`) the second factor is off, and the secret and the
recovery codes are removed. To turn it on again, start from `setup`.

### Other answers

| Status | `code` | Meaning |
|--------|--------|---------|
| `400` | `invalid_request` | The body could not be read as JSON |
| `401` | `unauthorized` | No token, or one that is no longer valid |
| `500` | `internal_error` | The call failed for a reason the caller cannot act on |

---

## 4. Changing the password

```http
POST /api/v1/users/password
Authorization: Bearer <the user's access token>
Content-Type: application/json

{ "current_password": "…", "new_password": "…" }
```

An account with a password proves it with `current_password` — a token alone
does not change it. An account without one (`has_password: false`) leaves
`current_password` out and sets its first password; for that the user must
have signed in recently, as for [deleting the account](#the-user-must-have-signed-in-recently).
The new password follows the tenant's rule.

On `200` the answer is `{ "message", "sessions_ended" }`: the user's other
sessions — every device, every application — have ended, with the tokens
issued in them, and applications that registered a `backchannel_logout_uri`
are told. `sessions_ended` is `others` (this session stays), `all` (the token
named no session, so this one ended too — sign the user in again) or `none`
(they could not be ended; the password is changed all the same). The webhook
event is `user.password_changed`.

| Status | `code` | Meaning |
|--------|--------|---------|
| `400` | `invalid_current_password` | The current password is not correct. Repeated ones are limited like failed sign-ins (`429`) |
| `400` | `password_too_short` / `password_too_long` | The new password breaks the tenant's rule; `min_length` says the least |
| `400` | `invalid_request` | No `new_password` |
| `401` | `insufficient_user_authentication` | Setting a first password needs a recent sign-in — sign in again with `max_age` and retry |
| `401` | `unauthorized` | No token, or one that is no longer valid |

The [account screen](#the-account-screen) offers this.

---

## 5. Signing out everywhere

```http
POST /api/v1/logout
Authorization: Bearer <the user's access token>
```

ends every session of the user at the authorization server — on every device
and for every application — together with their remembered consents and the
tokens issued under them. The access token used for this call stops working,
as does any refresh token. Applications that registered a
`backchannel_logout_uri` are
[told](../FEATURES.md#telling-other-applications-about-a-logout).

On `200` the answer is `{ "success": true, "message" }`; discard the tokens
you hold. The user is always the one the token belongs to — the body cannot
name another.

This is an API call, not a browser redirect: it answers JSON and sends the
browser nowhere. A JSON body is optional: with `post_logout_redirect_uri` (and
optionally `state`) the answer also carries `redirect_url` — that address as
given, with `?state=…` appended when `state` is sent — for the application to
navigate to. To sign
the user out through the browser instead, send it to the authorization
server's `end_session_endpoint` ([logout flow](login-flows.md#logout)).

| Status | Meaning |
|--------|---------|
| `401` | No token, or one that is no longer valid |
| `500` | The sessions could not be ended (`{ "success": false, "error", "guide", "details" }`) — try again |

---

## The account screen

Authway hosts a page where a signed-in user manages their own sign-in —
turning two-step verification on and off, renewing recovery codes, and
changing (or setting a first) password. Link to it from your application:

```
{login screens}/account?client_id=<your application's client_id>
```

The page signs the user in to their tenant's **account screen client**, a
public client Authway registers for the tenant the first time one of its
users arrives (no consent screen; a user with a session signs in without
seeing anything). It offers a way back to your application — the
`client_uri` you registered for it. Behind the page is
`GET /api/v1/account-entry?client_id=…`, which answers with that client and
your application's name and `client_uri`.

The administration API lists the account screen client with `system: true`
and refuses to change or delete it (`409 system_client`); it is removed with
the tenant.
