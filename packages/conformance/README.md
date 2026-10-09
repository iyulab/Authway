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

The provider must send mail to the capture service, and the links in that mail
must point at its login UI (`/invitation/accept?token=…`). With the repository's
`docker-compose.yml`, MailHog listens on SMTP `1025` and HTTP `8025`.

The suite refuses to run when the variables are missing rather than skipping —
a skipped conformance run would report success without having checked anything.

One run asks for four sign-in links from the same IP address. A provider that
limits those requests (Authway allows five per IP address in 15 minutes) answers
`429` to a second run started within that window — wait it out, or clear the
limiter's state, before running again.
