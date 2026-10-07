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
| `mfa` | Ask for the second factor; pass `mfa_challenge` back |
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
    "sign_in_methods": ["email", "magic_link", "google"],
    "allow_email_signup": false
  }
}
```

`sign_in_methods` lists what to offer, in order: `email` (the password form),
`magic_link`, then social providers. It already takes into account what the
client enables and what this deployment can run — show exactly this list.

When the browser already has a session in the client's tenant, the answer is
`{"next": "redirect", "redirect_to": "…"}` and no form is needed.

### `POST /api/v1/login-flows/{flow}/password`

`{"email": "…", "password": "…", "remember": false}` → `next: redirect`, or
`next: mfa` with `mfa_challenge` for a user with a second factor. A wrong
email or password is `401 invalid_credentials`; a client that does not allow
password sign-in is `403 sign_in_method_not_allowed`.

### `POST /api/v1/login-flows/{flow}/mfa` and `/mfa/recovery`

`{"mfa_challenge": "…", "code": "123456"}` (a recovery code for `/recovery`)
→ `next: redirect`. The `mfa_challenge` only completes the flow it was issued
for.

### `GET /api/v1/login-flows/{flow}/social/{provider}`

A **page navigation**, not an API call: link to it or assign it to
`window.location`. The browser continues to the provider and comes back
through the API to the application. A provider the client does not offer ends
the authorization request with an OAuth error.

### `POST /api/v1/login-flows/{flow}/magic-link`

`{"email": "…"}` → `next: email_sent` — whether or not the address may sign
in, so the answer reveals nothing about accounts. The email links to
`<login UI>/magic-link?token=…`. That page should:

1. `POST /api/v1/magic-links/inspect` with `{"token": "…"}` — reports
   `valid` and `email` without using the link up (mail scanners open links
   too);
2. on the user's confirmation, `POST /api/v1/magic-links/redeem` with the
   same body → `next: redirect`.

The link has to be opened in the browser that started the sign-in: the
authorization server ties the login to that browser.

## Consent

- `GET /api/v1/consent-flows/{flow}` → `next: form` with `client_name`,
  `requested_scope` and `user`, or `next: redirect` when there is nothing to
  ask (the client skips consent, single sign-on).
- `POST /api/v1/consent-flows/{flow}/accept` with
  `{"grant_scope": [...], "remember": true, "remember_for": 3600}` →
  `next: redirect`.
- `POST /api/v1/consent-flows/{flow}/reject` → `next: redirect` (the
  application receives `access_denied`).

## Logout

`POST /api/v1/logout-flows/{flow}` → `next: redirect`. It ends the session
and revokes the user's tokens. The destination is decided by the
authorization server: the application's `post_logout_redirect_uri`, which must
be one of the client's registered `post_logout_redirect_uris`.

## Capabilities

`GET /api/v1/capabilities` describes the deployment:

```json
{
  "multi_tenant": true,
  "providers": ["google"],
  "magic_link": true,
  "mfa": ["totp"],
  "signup_modes": ["invite_only", "open"],
  "token_exchange": false,
  "ciba": false
}
```

`providers` are the social providers this deployment has credentials for (a
client may add its own credentials for some of them).
