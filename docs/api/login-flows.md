# Login, Consent and Logout Flows

How a login screen talks to Authway. The bundled login UI uses exactly this
contract; a custom screen can replace it without knowing anything about the
authorization server behind it.

The machine-readable description — including the email verification,
password reset and invitation endpoints the account screens use — is
[`packages/contract/openapi.yaml`](../../packages/contract/openapi.yaml)
(OpenAPI 3.1). The conformance suite checks every answer against it.

## Flow ids

The authorization server sends the browser to the API, which opens a screen
of the login UI with an opaque **flow id**:

| Authorization server sends the browser to | API opens |
|---|---|
| `GET /login?login_challenge=…` | `<login UI>/login?flow=<id>` |
| `GET /consent?consent_challenge=…` | `<login UI>/consent?flow=<id>` |
| `GET /logout?logout_challenge=…` | `<login UI>/logout?flow=<id>` |

A flow that needs nothing from the user never opens a screen: when the browser
already has a session the client can use, or there is no consent to ask for,
the API settles the flow where the authorization server sent the browser and
sends it straight back. A screen is opened only to show something — and it
still asks first (`GET …-flows/{flow}`), because the answer may have become
"go on" by the time it loads.

A screen treats the flow id as opaque: it only ever passes it back, URL-encoded
in a path segment. Point the authorization server's login, consent and logout
URLs at the API (`URLS_LOGIN=<API>/login`, `URLS_CONSENT=<API>/consent`,
`URLS_LOGOUT=<API>/logout`); its error URL stays on the login UI
(`<login UI>/error`).

## Answers

Every flow endpoint answers JSON with `next`, which tells the screen what to do:

| `next` | Meaning |
|---|---|
| `form` | Show the screen's form (the answer carries what to show) |
| `redirect` | Navigate the browser to `redirect_to` |
| `mfa` | Ask for the second factor; pass `mfa_challenge` back to the named `flow` |
| `email_sent` | A sign-in link was emailed |

Errors carry `error` (a message to show) and `code`. An unknown flow id is
`400 invalid_flow`; one that expired or was already completed is
`410 flow_expired`.

## Login

### `GET /api/v1/login-flows/{flow}`

```json
{
  "next": "form",
  "flow": "…",
  "client_name": "My App",
  "requested_scope": ["openid", "email"],
  "client": {
    "client_id": "…",
    "sign_in_methods": ["email", "magic_link", "google"]
  }
}
```

`hints` is what the application's authorization request asked of the screen,
which otherwise only has the flow id: `sign_in_method` (the request's
`connection` — the way of signing in to start with, present only when it is
one of `sign_in_methods`), `login_hint` (the address to expect) and
`ui_locales` (the languages the application prefers, most preferred first).
All are optional and decide nothing; a screen applies them once and lets the
user change the language or the address.

`sign_in_methods` lists what to offer, in order: `email` (the password form),
`magic_link`, then social providers. It already takes into account what the
client enables and what this deployment can run — show exactly this list.

When the browser already has a session in the client's tenant, the answer is
`{"next": "redirect", "redirect_to": "…"}` and no form is needed. A session
of another tenant is not this client's to use: the form is shown — or, when
the application asked for an answer without a prompt (`prompt=none`), the
answer sends the browser back to it with `login_required`. See
[Sessions](#sessions).

### `POST /api/v1/login-flows/{flow}/password`

`{"email": "…", "password": "…", "remember": false}` → `next: redirect`, or
`next: mfa` with `mfa_challenge` and `flow` for a user with a second factor.
`remember` is the user's "keep me signed in": it makes the session a longer
one, nothing else ([Sessions](#sessions)). A wrong
email or password is `401 invalid_credentials`; a client that does not allow
password sign-in is `403 sign_in_method_not_allowed`.

### `POST /api/v1/login-flows/{flow}/mfa` and `/mfa/recovery`

`{"mfa_challenge": "…", "code": "123456"}` (a recovery code for `/recovery`)
→ `next: redirect`. The `mfa_challenge` only completes the flow it was issued
for. An authenticator code is accepted once: the one just used, or an earlier
one, is `401 invalid_code` — the user waits for the next.

A user who has a second factor owes it **however they proved who they are
first** — a password, an emailed link, or another identity provider. Each of
those ends in the same step: `next: mfa` naming the `flow`, or, for a provider
sign-in (a page navigation), the browser arriving at
`<login UI>/mfa/verify?flow=…&mfa_challenge=…`. A login UI needs one screen
for the code and reaches it from all three. The ID token's `amr` then lists
both factors (`["email", "otp", "mfa"]`, `["fed", "otp", "mfa"]`).

### `GET /api/v1/login-flows/{flow}/social/{provider}`

A **page navigation**, not an API call: link to it or assign it to
`window.location`. The browser continues to the provider and comes back
through the API to the application — or, for a user with a second factor, to
the login UI's second factor step (above): what the provider asked of the user
does not stand in for it. A provider the client does not offer ends the
authorization request with an OAuth error.

### `POST /api/v1/login-flows/{flow}/magic-link`

`{"email": "…"}` → `next: email_sent` — whether or not the address may sign
in, so the answer reveals nothing about accounts. The email links to
`<login UI>/magic-link?token=…`. That page should:

1. `POST /api/v1/magic-links/inspect` with `{"token": "…"}` — reports
   `valid` and `email` without using the link up (mail scanners open links
   too);
2. on the user's confirmation, `POST /api/v1/magic-links/redeem` with the
   same body → `next: redirect`, or `next: mfa` with `mfa_challenge` and
   `flow` for a user with a second factor. The page has only the token, so it
   takes the flow from the answer.

The link has to be opened in the browser that started the sign-in: the
authorization server ties the login to that browser.

## Consent

- `GET /api/v1/consent-flows/{flow}` → `next: form` with `client_name`,
  `requested_scope` and `user`, or `next: redirect` when there is nothing to
  ask: the user's earlier answer is remembered, or the client is registered
  with `skip_consent` (the tenant's own application). Being signed in already
  is not a reason — a user who arrives at another client on an existing
  session is asked like anyone else.
- `POST /api/v1/consent-flows/{flow}/accept` with
  `{"grant_scope": [...], "remember": true}` → `next: redirect`. `remember`
  is the user's "do not ask me again for this application"; the answer is
  then kept until the user's consents are revoked (signing out everywhere
  does that).
- `POST /api/v1/consent-flows/{flow}/reject` → `next: redirect` (the
  application receives `access_denied`).

## Logout

`POST /api/v1/logout-flows/{flow}` → `next: redirect`. It ends the session
and revokes the user's tokens. The destination is decided by the
authorization server: the application's `post_logout_redirect_uri`, which must
be one of the client's registered `post_logout_redirect_uris`.

## Sessions

Every sign-in — by password, an emailed link or another identity provider —
opens a **session** at the authorization server: until it ends, that browser
is known there. The session is what makes the rest work:

- the user's next application in the same tenant does not ask them to sign in
  again;
- a logout has something to end, and the applications signed in on the session
  are told ([back-channel logout](../FEATURES.md));
- an application can ask whether the user is still signed in without showing
  them anything.

How long it lasts is decided when the user signs in, and reusing it does not
make it longer:

| | Session lasts |
|---|---|
| A sign-in | the tenant's `session_timeout` — 60 minutes unless the tenant sets another |
| A sign-in with "keep me signed in" (`remember: true`) | 30 days |

A `session_timeout` longer than 30 days is treated as 30 days. An application
that needs the user to have signed in recently says so with `max_age` on its
authorization request.

### Asking without a prompt

An authorization request with `prompt=none` is answered without showing the
user anything, and without loading a page of the login UI — with a code, or
with an error at the application's redirect URI:

| The browser has | Answer |
|---|---|
| a session, and a remembered consent for this client (or the client is registered with `skip_consent`) | `code` |
| a session, but the user would have to be asked for consent | `error=consent_required` |
| no session, one older than the request's `max_age`, or one of another tenant | `error=login_required` |

On either error the application sends the user through an ordinary
authorization request.

## Capabilities

`GET /api/v1/capabilities` describes the deployment:

```json
{
  "multi_tenant": true,
  "providers": ["google"],
  "magic_link": true,
  "mfa": ["totp"],
  "signup_modes": ["invite_only", "open"],
  "account_deletion": true,
  "custom_claims": true,
  "backchannel_logout": true,
  "token_introspection": true,
  "token_exchange": false,
  "ciba": false
}
```

`providers` are the social providers this deployment has credentials for (a
client may add its own credentials for some of them).
