# @authway/conformance

Black-box scenarios that any Authway identity provider must pass. The suite talks
only to the provider's public surfaces — OIDC discovery, authorize, token and
userinfo; the backend its login screens call; and the admin API — plus an SMTP
capture service that receives the provider's mail. It never touches a database,
so the same scenarios run against every implementation.

## What it checks

- **Discovery** — required endpoints, `code` + S256 PKCE, a non-empty JWKS.
- **Login** — an admin invitation is mailed, accepted from the link, and the new
  user completes authorization code + PKCE through password login and consent;
  `userinfo` identifies them. A wrong password and an unknown flow id are client
  errors. Logging out invalidates the access token.
- **Login UI routes** — every backend path the bundled login screens call is
  routed.

Each scenario provisions its own client and user and deletes them afterwards.

## Running

Start a provider and an SMTP capture service, then:

```bash
CONFORMANCE_ISSUER=http://localhost:4444 \
CONFORMANCE_API=http://localhost:8080 \
CONFORMANCE_ADMIN_KEY=<admin API key> \
CONFORMANCE_MAIL_API=http://localhost:8025 \
pnpm --filter @authway/conformance test
```

| Variable | Meaning |
|---|---|
| `CONFORMANCE_ISSUER` | OIDC issuer URL |
| `CONFORMANCE_API` | Backend the login screens and admin console call |
| `CONFORMANCE_ADMIN_KEY` | Admin API key used to provision test clients and invitations |
| `CONFORMANCE_MAIL_API` | MailHog-compatible HTTP API (`/api/v2/search`) |
| `CONFORMANCE_TENANT_ID` | Optional. Tenant to provision into; defaults to the `default` tenant |
| `CONFORMANCE_IDP_STANDIN` | Where the suite serves a stand-in identity provider, e.g. `http://localhost:9912`. Required when the provider offers sign-in through other identity providers (`capabilities.providers` is not empty) — see below |
| `CONFORMANCE_IDP_STANDIN_AS` | Optional. The sign-in provider the stand-in is put in the place of. Defaults to `google` |
| `CONFORMANCE_CALLBACK_HOST` | Optional. The name under which the provider reaches the machine running the suite, for the scenarios in which the provider calls the suite (back-channel logout). Defaults to `localhost`; with the authorization server in a container on Docker Desktop, `host.docker.internal` |

The provider must be able to call the machine running the suite: the
back-channel logout scenario registers a receiver there, at a private address
and over plain http. A deployment that refuses such addresses by default has
to allow them for the run (Authway: `AUTHWAY_ALLOW_PRIVATE_TARGETS=true`, and
`CLIENTS_HTTP_DISALLOW_PRIVATE_IP_RANGES=false` on the authorization server).

The provider must send mail to the capture service, and the links in that mail
must point at its login UI (`/invitation/accept?token=…`). With the repository's
`docker-compose.yml`, MailHog listens on SMTP `1025` and HTTP `8025`.

### Sign-in through another identity provider

Whom a provider signs in on another identity provider's word — which account
it reaches, what it refuses, what it still asks for — cannot be checked against
the real Google. The suite plays that provider itself: it listens at
`CONFORMANCE_IDP_STANDIN` and answers the code exchange (`POST …/token`) and
the profile request (`GET …/userinfo`). The provider under test has to be
configured to reach those two there, and to send the browser to an address on
the same origin to sign in (the suite never opens it — it plays the user too).

For Authway, with the stand-in at `http://localhost:9912`:

```bash
AUTHWAY_GOOGLE_ENABLED=true
AUTHWAY_GOOGLE_CLIENT_ID=standin        # any value: the stand-in does not check it
AUTHWAY_GOOGLE_CLIENT_SECRET=standin
AUTHWAY_GOOGLE_AUTH_URL=http://localhost:9912/authorize
AUTHWAY_GOOGLE_TOKEN_URL=http://localhost:9912/token
AUTHWAY_GOOGLE_USERINFO_URL=http://localhost:9912/userinfo
```

Authway accepts the three `…_URL` settings only in development: whatever
answers there decides who signs in.

A provider that offers no such sign-in has nothing to verify and the scenarios
are skipped. One that offers it without a stand-in configured fails the suite.

The suite refuses to run when the variables are missing rather than skipping —
a skipped conformance run would report success without having checked anything.

One run asks for four sign-in links from the same IP address. A provider that
limits those requests (Authway allows five per IP address in 15 minutes) answers
`429` to a second run started within that window — wait it out, or clear the
limiter's state, before running again.
