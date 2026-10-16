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
| `GET /device?device_challenge=…` | `<login UI>/device?flow=<id>` (and `user_code`, when the device's link carried it) |

A flow that needs nothing from the user never opens a screen: when the browser
already has a session the client can use, or there is no consent to ask for,
the API settles the flow where the authorization server sent the browser and
sends it straight back. A screen is opened only to show something — and it
still asks first (`GET …-flows/{flow}`), because the answer may have become
"go on" by the time it loads.

A screen treats the flow id as opaque: it only ever passes it back, URL-encoded
in a path segment. Point the authorization server's login, consent and logout
URLs at the API (`URLS_LOGIN=<API>/login`, `URLS_CONSENT=<API>/consent`,
`URLS_LOGOUT=<API>/logout`, `URLS_DEVICE_VERIFICATION=<API>/device`); its error
URL and the page that ends a device sign-in stay on the login UI
(`<login UI>/error`, `URLS_DEVICE_SUCCESS=<login UI>/device/done`).

## Answers

Every flow endpoint answers JSON with `next`, which tells the screen what to do:

| `next` | Meaning |
|---|---|
| `form` | Show the screen's form (the answer carries what to show) |
| `redirect` | Navigate the browser to `redirect_to` |
| `mfa` | Ask for the second factor; pass `mfa_challenge` back to the named `flow` |
| `terms` | Ask the user to accept the documents in `terms`; pass `terms_challenge` back to the named `flow` ([Terms](#terms)) |
| `email_sent` | A sign-in link was emailed |

Errors are problem details ([Errors](errors.md)): show `detail`, branch on `code`. An unknown flow id is
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
`next: mfa` with `mfa_challenge` and `flow` for a user with a second factor,
or `next: terms` for a user who owes the tenant's documents ([Terms](#terms)).
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
`<login UI>/magic-link?token=…&code=…` and also shows the code. That page
should:

1. `POST /api/v1/magic-links/inspect` with `{"token": "…"}` — reports
   `valid`, `email` and the `flow` the link signs in to, without using the
   link up (mail scanners open links too);
2. on the user's confirmation, `POST /api/v1/magic-links/redeem` with the
   same body → `next: redirect`, or `next: mfa` with `mfa_challenge` and
   `flow` for a user with a second factor. The page has only the token, so it
   takes the flow from the answer.

The link has to be opened in the browser that started the sign-in: the
authorization server ties the login to that browser. The login UI remembers
the flows it sent a link for; a link opened anywhere else — the email read on
a phone — shows its code instead, to type into the screen that started the
sign-in.

### `POST /api/v1/login-flows/{flow}/magic-link/code`

`{"code": "BCDF-GHJK"}` — the code from the email, typed into the screen that
asked for it — does what redeeming the link does: `next: redirect`, `mfa` or
`terms`. Any case, with or without the hyphen. A wrong, used or expired code
is `400 invalid_code`. A code works once and uses its link up; after five
wrong codes for a sign-in its codes stop working, while its links still
work.

### Terms

A tenant can name documents its users accept at sign-in — terms of service, a
privacy policy — one current version of each (`PUT /api/v1/tenants/{id}/terms/{kind}`
with `version`, `url` and an optional `title`; the application publishes the
documents, Authway records who accepted which version). When the user has not
accepted every current version, the sign-in stops **after every factor** —
whichever way they signed in — and also when a browser session is reused,
since a document may have been revised since:

```json
{
  "next": "terms",
  "flow": "…",
  "terms_challenge": "…",
  "terms": [
    { "kind": "privacy_policy", "version": "3", "url": "https://example.com/privacy" },
    { "kind": "terms_of_service", "version": "2026-10", "url": "https://example.com/terms", "title": "Terms of Service" }
  ]
}
```

`terms` lists only what the user has not accepted, by kind. A screen shows
each document as a link, asks for each to be accepted, and then:

- `POST /api/v1/login-flows/{flow}/terms` with
  `{"terms_challenge": "…", "accepted": [{"kind": "…", "version": "…"}, …]}`
  — exactly the documents shown → `next: redirect`. If a document was revised
  while it was on screen the answer is `400 terms_mismatch` with `terms`, what
  to show instead; the same `terms_challenge` stays good. (Do not read the
  login flow again for it: the user has not been signed in yet, and the flow
  would offer the sign-in form.)
- `POST /api/v1/login-flows/{flow}/terms/decline` with `{"terms_challenge": "…"}`
  → `next: redirect` back to the application with `error=access_denied`.
  Nothing is recorded and the account is left as it is; the next sign-in asks
  again.

A sign-in through another identity provider is a page navigation: the browser
arrives at `<login UI>/terms?flow=…&terms_challenge=…`, and the page reads what
to show with `GET /api/v1/login-flows/{flow}/terms?terms_challenge=…`. An
unknown or expired `terms_challenge` is `400 invalid_terms_challenge`.

What a user accepted, when and signing in to which client:
`GET /api/v1/users/{id}/terms-acceptances` (admin key, or a tenant service
credential granted `admin.terms:write` — [service-clients.md](service-clients.md)).
Tokens carry no claim for it: a completed sign-in means the current documents
were accepted.

An application that only refreshes tokens does not see a new version until
its next authorization request.

## Consent

- `GET /api/v1/consent-flows/{flow}` → `next: form` with `client_name`,
  `requested_scope` and `user`, or `next: redirect` when there is nothing to
  ask: the user's earlier answer is remembered, or the client is registered
  with `skip_consent` (the tenant's own application). Being signed in already
  is not a reason — a user who arrives at another client on an existing
  session is asked like anyone else. While a client skips the question, the
  answer is remembered (so a request without a prompt is answered); turning
  `skip_consent` off withdraws those answers and the client's refresh tokens,
  and its users are asked at their next sign-in.
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

## Device sign-in

A device without a usable browser or keyboard — a TV, a console, a
command-line tool — signs in with a code the user enters on another screen
(RFC 8628):

1. The device asks the authorization server's `device_authorization_endpoint`
   (discovery) for a code, with a client registered with the
   `urn:ietf:params:oauth:grant-type:device_code` grant. It shows the user the
   `user_code` and the `verification_uri` (or a QR code of
   `verification_uri_complete`, which carries the code).
2. The user opens the address; the browser reaches the login UI's device page
   with a flow id.
3. `POST /api/v1/device-flows/{flow}` `{"user_code": "…"}` → `next: redirect`.
   The code is read without regard to case, spaces or dashes; a code that is
   not the device's, or whose device request expired, is
   `400 user_code_invalid`.
4. The sign-in goes on like any other: the login screen, then the consent
   screen — which shows the application — and finally the login UI's
   `/device/done` page, which tells the user to go back to the device.
5. Meanwhile the device polls the token endpoint
   (`grant_type=urn:ietf:params:oauth:grant-type:device_code`), answered
   `authorization_pending` until the user approves, then with its tokens
   (with a refresh token when it asked for `offline_access`).

Codes are eight capital letters and live ten minutes. A browser can finish
only the device page it opened last.

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
| a session, but the user owes a document of the tenant ([Terms](#terms)) | `error=interaction_required` |
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
  "device_authorization": true,
  "terms_acceptance": true,
  "token_exchange": true,
  "ciba": false
}
```

`providers` are the social providers this deployment has credentials for (a
client may add its own credentials for any of them).
