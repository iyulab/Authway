## [Unreleased]

### Security

- **TOTP-based MFA now actually protects login — it did not, in two
  independent ways.** First, every management endpoint for setting up,
  verifying, or checking MFA status (`/api/v1/users/mfa/*`) panicked into a
  500 on every call: the handlers read the authenticated user ID out of
  request context as a string, but the JWT middleware had already switched
  to storing it as a typed UUID in an earlier hardening pass, and the two
  never got reconciled. There was no way to turn MFA on through the API at
  all. Second, even a user enabled directly in the database was never
  challenged for a code — the login handler verified the password and
  accepted the session with Hydra without ever calling the (fully
  implemented, fully untested-in-production) TOTP verification. Password
  alone was sufficient for every account, MFA enabled or not. Both are fixed
  together, since fixing only one leaves the other half still fully bypassed:
  the management handlers now read the UUID directly, and a TOTP-enabled
  account's login now stops after the password check, hands the client a
  short-lived challenge instead of a session, and only accepts the login once
  a correct code (or recovery code) comes back — capped at five attempts
  before the challenge is discarded. Verified against a real Hydra instance,
  not just unit tests: a TOTP-enabled test account's login now visibly stalls
  at the password step, a wrong code is rejected without touching Hydra, and
  the correct code completes the same OAuth flow a non-MFA login would.

- **Updated axios in both frontend apps (admin console, auth UI) past a run
  of known vulnerabilities** — SSRF via proxy handling, several prototype-
  pollution paths that could allow response tampering or credential theft,
  and a couple of denial-of-service issues, the most recent fixed only in
  1.18.0. Both apps had drifted onto different, both-vulnerable versions;
  both now run 1.19.0. Neither app sends attacker-controlled input into
  axios configuration, so exploitability here was limited, but there was no
  reason to leave the exposure in place once found.

- **Bound parameters no longer reach the logs.** The GORM logger was pinned to
  `Info` in every environment, which prints each statement with its values
  inlined. Live secrets went to the container console that way — an invitation
  token (which grants account creation) and a magic-link token (which is a login
  factor on its own) were both read out of staging logs. Outside development the
  logger now runs at `Warn` with `ParameterizedQueries`, because the level alone
  is not enough: GORM still prints full SQL for slow queries and for errors, so
  redaction is what actually closes it. Development keeps the values, since
  that is the whole point of them there.

- **Magic-link tokens are hashed at rest** (migration 019). The 010/013/014 pass
  hashed admin sessions, password resets and email verifications but missed
  `magic_link_tokens.token` — the one table where a plain read of the column is
  a working login for any pending link. Stored as a SHA-256 digest now, via the
  shared `pkg/tokenhash`. Existing links are invalidated by the migration; they
  live fifteen minutes, so the cost is one retry.

- **Social sign-in no longer creates accounts for uninvited people.** Google,
  GitHub, Microsoft and Apple all provisioned a user on first sign-in, so
  anyone holding an account with one of those providers could join a tenant
  nobody had invited them to — the last path that ignored invitation-only
  onboarding. First-time sign-in now requires a pending invitation for that
  address in that tenant, and the check fails closed on a lookup error or a
  missing gate. Signing in to an account that already exists never consults it,
  so current members are unaffected. Verified against production before the
  change: all nine accounts are password accounts, none is linked to any
  provider, and no account has been created in the last thirty days — so no
  live onboarding path depended on this.

- **Staging now gets the same fail-closed configuration checks as production.**
  Every safety check that rejects a weak admin password, a missing admin/TOTP
  key, or a frontend URL pointing at the container itself was gated on the
  literal environment name `production` — any other deployed environment,
  staging included, silently took the relaxed local-development path instead:
  missing keys were auto-generated on the fly rather than failing to boot. The
  gate is now "development or not", so any real deployment gets the strict
  checks. Verified against the current values a staging deploy actually
  injects, so this does not change staging's boot behavior today — it closes
  the gap for the next time one of those values is accidentally left unset.

- **Logout redirect wildcard matching now checks the actual host.** With
  wildcard post-logout redirects enabled for a client, the match ran against
  the raw request URI as a string, so a `*.example.com` whitelist entry was
  satisfied by any URI whose query string or fragment happened to end in that
  suffix — the host itself was never parsed out and checked. A URI like
  `https://attacker.example/?x=y.example.com` passed the same whitelist entry
  a real `https://app.example.com` request would. Matching now parses the URI
  and compares against its host. The feature defaults off, so only clients
  that had explicitly opted into wildcard redirects were exposed.

### Fixed

- **Proxy requests to Hydra and the Central API could hang indefinitely.**
  Several of the auth backend's own HTTP clients (login, consent, claims,
  profile) were constructed with no timeout, so a slow or unresponsive
  upstream would tie up the handling goroutine and connection with it. They
  now share the same 30-second timeout the service-layer clients already
  used.

### Removed

- **Unused JWT secret configuration is gone.** `jwt.access_token_secret` and
  `jwt.refresh_token_secret` were never read anywhere outside their own
  validation check — token signing has always been Hydra's job — so the check
  guarded a value nothing used, while defaulting to a hardcoded placeholder.
  The admin session store's own hand-rolled SHA-256 hashing and token
  generation are also gone in favor of the shared `pkg/tokenhash`; the digest
  format is unchanged, so existing sessions are unaffected.

- **Account linking (`pkg/accountlink`) is gone.** It mapped to a
  `linked_accounts` table that no migration has ever created, and its routes
  were registered regardless, so `/account/linked` and `/account/providers`
  failed at runtime. Nothing ever called the code that would have written a
  link row either — social sign-in records the provider on the user itself
  (`users.google_id`, `github_id`, …), which is the de-facto link record.
  Reviving the feature would have meant maintaining a second answer to "which
  providers is this account attached to", so the dead half is removed instead.
  The schema contract test now checks that every model maps to a table that
  exists, which is what surfaced this.

- **Magic links no longer let anyone register themselves.** Onboarding has been
  invitation-only since 0.4.0, but the policy lived in a comment: the public,
  unauthenticated `POST /api/v1/auth/magic-link/send` provisioned a user for any
  address on verify — and the tenant came from the request, so an attacker who
  knew a tenant id could plant an account in someone else's organisation.
  Provisioning now requires a pending invitation for that exact
  (tenant, address); because invitations are keyed on both, the arbitrary-tenant
  hole closes with it. Eligibility is re-checked at verify, so revoking an
  invitation invalidates links already sent, and the gate fails closed if it is
  ever left unwired. The response to an uninvited address is deliberately
  identical to the invited one — differing replies would turn the endpoint into
  a membership oracle. Social login still auto-provisions and is tracked
  separately.

- **Checking a magic link no longer spends it.** `GET /auth/magic-link/status`
  called the verification path, so it marked the token used and created the
  user — an email scanner that prefetches links consumed them before the
  recipient clicked. Status is now a genuine read. Redemption is also a single
  conditional update rather than read-then-write, so one link cannot be redeemed
  twice concurrently, and the `GET` twin of `/verify` is gone: consuming a token
  is a state change and does not belong in a URL that proxies, prefetchers and
  `Referer` headers pass around.

### Fixed

- **Accepting an invitation from its email link now works.** Once the links
  pointed at the auth UI, the accept page reported that valid, unexpired
  invitations did not exist. The page validates through
  `GET /api/v1/invitations/token/:token`, and Fiber returns path params exactly
  as they appear in the URL — it does not percent-decode them. Tokens were
  padded base64, so every one ended in `=`, which a browser sends as `%3D`; the
  handler compared that literal against the stored value and found nothing.
  The handler decodes its path param now, so invitations already in flight work
  without being reissued, and new tokens come from `pkg/tokenhash` (unpadded
  base64url), so nothing in them needs escaping in the first place.

  This predates the link fix rather than following from it. The page reads the
  token with `searchParams` and re-encodes it with `encodeURIComponent`, so the
  request was byte-identical whether or not the emailed URL escaped anything.
  Accepting an invitation through its email link had never worked. Earlier
  verification went through `POST /invitations/accept`, which carries the token
  in a JSON body and so never crosses a path segment.

- **Emailed links now point at the auth UI instead of the API.** Invitation
  accept, magic link, email verification and password reset URLs were all built
  from `app.base_url`, which is this API's own address — the value the discovery
  document advertises as `api_server`, and the value the deploy script hardwires
  to `API_URL`. Every one of those links returned 404, in every environment, and
  no configuration could fix it because no frontend URL setting existed. Under
  invitation-only onboarding that left no way for a person to get an account at
  all; only a consumer able to call `POST /api/v1/invitations/accept` directly
  could get through. There is now a separate `app.frontend_url`
  (`AUTHWAY_APP_FRONTEND_URL`), and startup fails if it is missing, equal to
  `base_url`, or a loopback address in production — the last case because Viper
  ignores an empty environment variable and silently falls back to the localhost
  default. Reported by VibeBase.

- **A fresh instance can be given its first user.** Onboarding is
  invitation-only and the sole admin-side surface for creating a user is
  `POST /api/v1/invitations` — but that endpoint required an existing user as
  the inviter. Called with the admin API key it attributed the invitation to a
  hard-coded UUID that no `users` row ever had, so it failed with
  `inviter not found` on any instance or tenant that had no users yet: creating
  a user needed an invitation, and an invitation needed a user.

  `invitations.inviter_id` is now nullable and expresses what was already true —
  the inviter may be the system rather than a person (migration 016). The
  hard-coded UUID is gone. Deleting a user no longer destroys the invitations
  they sent, either: the foreign key was `ON DELETE CASCADE` and is now
  `SET NULL`, so the history survives and only the attribution drops.

- **Invitations could never be created at all.** Behind the inviter check sat a
  second wall: the model declared `tenant_name`, `inviter_name` and
  `accepted_by` columns that no migration creates, so every insert failed on the
  column name. Those two names are copies of data that lives on `tenants` and
  `users`, so they are now derived at read time instead of stored, and
  `accepted_by` maps to the column that actually exists. Verified end to end
  against a real database: a tenant with zero users now goes invitation → accept
  → first user.

- **Impersonation was unusable with the admin API key.** The same defect in a
  second place — `impersonation_sessions.admin_id` was `NOT NULL REFERENCES
  users(id)` while the handler supplied the same phantom UUID. It is nullable
  now (migration 017), with `admin_email` recording `system` so the audit trail
  still names an actor.

- **Webhooks could not be created.** `webhooks.events` is a Postgres `text[]`,
  but the model typed it as a plain `[]string`, which the driver cannot encode —
  every `POST /api/v1/webhooks` failed with `malformed array literal`.

- **Emailed magic links pointed at a page that does not exist.** The link was
  built against `/auth/magic-link/verify` on the auth UI, whose route is
  `/magic-link`.

### Added

- **Deploys check that emailed links are reachable.** `publish-api.core.ps1`
  reads `auth_ui` back from `/api/v1/config` to confirm the running binary got
  the URL that was meant for it, then requests all four link paths and requires
  200. No mail is sent, so it leaves no test accounts or bounces behind.

- **`pkg/maillink`** holds the mail link paths in one place, and a contract test
  checks each one against the routes the auth UI actually declares. The deploy
  check cannot do this: a single-page app served with a 200-rewrite answers
  *every* path with the same shell, so a link to a route that does not exist
  still returns 200. That is not hypothetical — magic-link mail once pointed at
  `/auth/magic-link/verify`, a route only the API has.

- **A schema contract test** (`internal/database/schema_contract_test.go`) writes
  one row per feature model against the real migrated schema. Every defect above
  is the same kind — a Go model and its SQL table drifting apart — and the
  existing service tests are structurally blind to it, because they build their
  schema with AutoMigrate *from the same struct*. The new test found the webhook
  bug and the fact that `pkg/accountlink` maps to a table no migration creates
  while its routes are registered regardless (tracked as an open decision).

- **Admin console reloaded forever once the session expired.** `/dashboard`
  bounced between itself and `/login` in an endless full-page refresh that only
  clearing browser storage by hand could escape. "Signed in" was stored in three
  places — a `localStorage` token for axios, and a persisted `token` plus a
  persisted `isAuthenticated` boolean in the store — and they drifted apart: the
  401 handler cleared the standalone token while the app still believed it was
  signed in, so `/login` redirected back to `/dashboard`, whose first request
  401'd again. The stored expiry was never checked, so a token months past
  expiry kept the loop alive.

  The store is now the token's only home, and authentication is derived from
  token *and* expiry rather than stored as its own flag. Rehydration reads back
  only those two fields, so a stale `isAuthenticated: true` written by an older
  build cannot come back — which is what lets an already-stuck browser heal by
  simply loading the new build. Verified by replaying the exact poisoned storage
  observed in production: it now lands on the login page and stays there.

- **A blank database can be provisioned again.** `schema_migrations` carried a
  bookkeeping row at version `000` — the same version as the initial schema
  file — so the migrator considered the initial schema applied on every
  database and never ran it. On an empty database migration `001` failed with
  `relation "users" does not exist`, making disaster recovery, a new
  environment, and a new contributor's local setup all depend on an undocumented
  manual `psql` step. The sentinel is gone; `version` now belongs to migration
  files alone.

  The initial schema was also a script whose first act was
  `DROP TABLE ... CASCADE` — the collision skipping it is the only reason no
  deployment was ever wiped. It is replaced by `000_initial_schema.sql`, which
  creates without dropping and is guarded so it does nothing at all when a
  schema already exists. The destructive script survives as an explicitly
  invoked development reset at `scripts/bootstrap/dev-clean-slate.sql`.

  Verified against a live Postgres in both directions: a blank database
  provisions end to end, and re-applying the initial schema to a populated,
  fully-migrated database with data in it leaves schema and rows untouched.
  Both are now regression tests. Existing deployments skip `000` regardless —
  confirmed by query on staging and prod.

- **Migrations 004 and 005 broke the all-or-nothing guarantee.** `RunMigrations`
  wraps the whole run in one transaction; these two opened their own `BEGIN` and
  `COMMIT`, and the inner COMMIT committed the outer transaction, so a later
  failure could no longer roll the run back. The migration guide advised this,
  and now says the opposite.

- **Postgres-gated tests were silently skipped in CI.** The migration and
  client-persistence guards need constraints SQLite cannot express
  (`text[] NOT NULL`, CHECK constraints) — the blind spot a NOT NULL violation
  already escaped through. CI now runs a Postgres service, and a follow-up step
  fails the build if any of those tests stops actually running.

- **Nothing in `docker-compose` could run the application.** Three compose files
  disagreed about how to start Authway locally and every one of them was broken:
  the UI services built from `packages/web/*`, renamed to `apps/*` long ago; the
  dev API image copied a `src/` tree that no longer exists; the "production"
  stack had no Hydra service at all, so it could not have served OAuth even if
  it had built; and the root file overrode a production image with `air`, which
  that image does not contain. What did work — and what `start-dev.ps1` has
  always used — is the backing services with the apps run natively.

  There is now one `docker-compose.yml`, providing Postgres, Redis, MailHog and
  Hydra and nothing else; the rest are gone. Hydra shares the application
  database and is configured purely through environment variables, matching how
  every deployment already runs it. Verified end to end: the stack comes up,
  both Hydra health endpoints return 200, and the API applies all 16 migrations
  onto the shared database and serves.

- **The README's Quick Start could not be followed.** It told you to copy a
  `.env.example` that did not exist, and to start the Central API with
  `go run cmd/main.go`, which does not compile — that command excludes
  `services.go` from the same package. It now says `go run ./cmd/`, and each Go
  service ships an accurate `.env.example` next to itself. The Central API's
  probe for a repo-root `.env` is also gone: it looked two directories up from
  `apps/central/api`, which is `apps/`, so it could never have found one.

### Documentation

- **Consolidated three competing documentation entry points into one.** The
  README, a root documentation index, and `docs/README.md` each claimed to be
  the place to start, and the root index had drifted furthest — the vast
  majority of the paths it listed no longer existed. Removed the root index;
  `docs/README.md` is now the single hub, linked from the README. Its feature
  list and "what's new" now reflect what actually shipped (invitation-only
  onboarding, magic links, MFA, admin impersonation, audit logging,
  webhooks) instead of a snapshot from several releases back.
- Replaced the static project-version badge with a pointer to the
  changelog: the apps and packages in this repo version independently, so a
  single hardcoded number was never accurate for long.
- Fixed several links pointing at guides that had moved to an archive
  directory or never existed under the referenced name, including the setup
  guide's quick-start section (which had also drifted from the README's and
  referenced a Hydra dev-mode flag this project doesn't use in any shared
  environment) and a feature doc that incorrectly said wildcard logout
  redirects were unsupported.
- Added `CONTRIBUTING.md` — it was referenced from the README but did not
  exist.

### Removed

- Four unused deploy scripts that converted a long-gone `schema_migrations`
  layout (`init-migration-system.ps1`, `migrate-tracking-table.ps1`,
  `upgrade-tracking-table.ps1`, `force-upgrade-tracking.ps1`). Nothing called
  them, and each reinserted the version-`000` sentinel — three also claimed
  version `001`, which a real migration owns.
- The two Vite apps as `docker-compose` services. They built from
  `packages/web/*`, renamed long ago, so `docker compose up` failed on a missing
  build context. Compose now provides the backing services only; run the UIs
  with `npm run dev`, where Vite HMR works properly anyway.
- Self-hosting the full stack with Docker Compose, which never worked in this
  repository layout: `docker-compose.dev.yml`, `docker-compose.prod.yml`,
  `docker-compose.proxy.yml`, `Dockerfile.dev`, the UI Dockerfiles (both UIs
  deploy as static bundles, not containers), `.air.toml`, the Traefik and Hydra
  config files nothing mounted any more, and `DOCKER-GUIDE.md`, whose opening
  one-line command failed immediately. Real deployments run on Azure Container
  Apps and Cloudflare Pages; see `scripts/deploy/`.

## [0.4.0] - 2026-07-20

> Run-17, triggered by consumer-reported issues from VibeBase. Minor because
> `access_token_strategy` is a new backward-compatible API field; everything
> else is a fix or docs.
>
> Verified on staging (see `scripts/deploy/POST-DEPLOY-VERIFY.md`): the client
> validation rules, migration 015, per-client JWT issuance including offline
> validation against the published JWKS, un-pinning, and the Hydra env hand-off
> all behave as described. No existing client's token format changes — the
> migration opts in nobody.

### Added

- **Per-client access token format** (`access_token_strategy`: `jwt` | `opaque`,
  migration `015`). Maps to Hydra's per-client field of the same name, which
  overrides the deployment-wide `strategies.access_token`. This is what lets a
  resource server validate Authway-issued tokens offline through standard OIDC
  discovery / JWKS. NULL (the default, and the state of every existing client)
  inherits the deployment-wide strategy — the migration opts in nobody, because
  which clients should carry non-revocable tokens is an operational decision.
  Exposed through the create/update API, the client response, and the Admin
  Console ("Access Token Format"), with the revocation trade-off stated at the
  point of choice.
- **`HYDRA_ALLOWED_TOP_LEVEL_CLAIMS` deployment setting**. Hydra nests custom
  session claims under `ext`; names listed here are mirrored to the token's top
  level for resource servers that read claims by their bare name. The mechanism
  lives in Authway, the names live in deployment config — claim vocabulary
  belongs to the consuming service, not to the issuer.
- **CI coverage for the two Vite apps** (`apps/central/admin`,
  `apps/branding/auth-ui`). Both sit outside the pnpm workspace and had never
  been built or tested by CI — auth-ui's 39 tests only ever ran on a developer
  machine. No artifacts are uploaded; CI verifies, the release pipeline publishes.
- **`ClientForm` regression tests** (admin console, 7 tests) covering the
  conditional `redirect_uris` rule and the three-state access-token-format field.
- **`scripts/deploy/POST-DEPLOY-VERIFY.md`** — checklist of claims that cannot be
  verified locally, with the command and expected output for each.

### Fixed

- **`redirect_uris` is no longer required for grants that never redirect.** It is
  now mandatory only for `authorization_code` / `implicit` (RFC 6749 §3.1.2).
  Machine-to-machine clients previously had to invent a placeholder URI, which
  then propagated into `post_logout_redirect_uris` / `default_logout_uri` and
  became permanent configuration pollution. The Admin Console mirrors the rule.
  Reported by VibeBase.
- **Partial confidential-client credentials now return 400, not 500.** Supplying
  `client_id` without `client_secret` (or vice versa) is a caller mistake; it now
  returns a structured `ConfigError` (`confidential_client_partial_credentials`)
  naming the missing field, like every other client-config violation. The masked
  secret is no longer echoed in the error message. Reported by VibeBase.
- **Hydra client payloads send `[]` rather than `null`** for a client with no
  redirect URIs.
- **Creating a machine-to-machine client still failed with 500 after the rule
  change above.** Dropping the validation requirement was not enough:
  `clients.redirect_uris` is `text[] NOT NULL`, and GORM writes an explicit NULL
  for a nil slice instead of omitting the column, so the column's `DEFAULT '{}'`
  never applied and every redirect-free create hit a not-null violation — the
  exact scenario the change was meant to enable. The model now stores an empty
  array, on both the create and the soft-delete-restore path. The SQLite test
  harness cannot express that constraint, so the guard lives in the new
  Postgres-gated `pkg/client/service_postgres_test.go`.
- **Clearing a client's pinned access token format now works.** A
  `validate:"omitempty,oneof=…"` tag on a `*string` does not short-circuit for a
  non-nil pointer to `""` — the validator dereferences it and `oneof` rejects the
  empty value, so a client pinned to `jwt` could never be returned to inheriting
  the deployment default. Validation moved out of the struct tag into
  `validateAccessTokenStrategy`, which returns the same structured 400 as every
  other client-config violation. Caught before deploy.
- **The deploy script's Hydra client sync never ran.** `deploy-all.core.ps1`
  POSTs to `/api/v1/clients/sync-hydra` without an `Authorization` header, so
  every deployment since admin routes moved behind `ADMIN_API_KEY` answered
  "No authorization token provided" and reported the step as a soft failure —
  and the fallback command it printed for the operator was unauthenticated too,
  so the suggested remedy failed the same way. Verified fixed against staging
  (`synced: 1, failed: 0`). Found by watching an actual deploy.
- **Deployment health checks failed containers that were merely cold.** A single
  10s attempt marked Hydra down immediately after an image swap, while discovery
  answered 200 in ~5.5s moments later. Now 3 attempts at 30s with a 10s pause —
  a false failure in a deploy checklist trains operators to ignore it.
- **`docs/BACKEND_INTEGRATION.md` documented a contract the deployment could not
  keep** — it described JWT validation via OIDC discovery while Authway shipped
  opaque tokens, so anyone following it hit a wall. It now states the opaque
  default, how to opt a client in, where custom claims actually land (`ext`), and
  the revocation trade-off.

### Removed

- `maskSecret()` and its test — the last production caller disappeared with the
  400 fix above.

---

## [0.3.2] - 2026-04-15

> **Note**: This entry was opened in Run-3 (2026-04-15) for the audit P4
> wiring described below, then expanded in Run-7 (2026-04-27) with the
> deployment-infrastructure changes recorded under "Operational (Run-7)"
> at the bottom. Both sets of changes ship together when prod is updated
> to `v0.3.2`. Date reflects original merge of the entry to follow the
> per-merge-date convention used by 0.3.0/0.3.1.

### Added

- **Audit log coverage for central-API auth flows (P4a/P4b)**. `audit_logs`
  action constants that were defined but never emitted are now wired into
  the request path:
  - `user.created` — `auth.Register` (self-registration), `internal_auth.AuthenticateGoogleUser`
    (JIT provision path, tagged `jit_provisioned=true`).
  - `user.login` — password `auth.Login`, all social callbacks (Google/GitHub/Microsoft/Apple),
    `internal_auth.AuthenticateGoogleUser` (returning user refresh). Provider and
    client_id are recorded in `Details`.
  - `user.login_failed` — recorded synchronously (`Log`, not `LogAsync`) so buffer
    overflow cannot swallow security events. Covers `auth.Login` (user_not_found /
    invalid_password), `auth.LoginEmbedded` (user_not_found / invalid_password /
    tenant_mismatch), and all four social callbacks when `HandleCallbackForClient`
    fails.
  - `user.logout` — both `auth.Logout` (direct API) and `auth.LogoutPage`
    (Hydra-initiated OIDC flow, tagged `flow=oidc`).
  - `user.password_reset` — `email.ResetPassword`.
  - `user.email_verified` — `email.VerifyEmail`.
  - `consent.granted` — `auth.Consent` (with `grant_scope`, `audience`, `client_id`).
  - `consent.revoked` — `auth.RejectConsent` (fetches subject via `GetConsentRequest`
    to preserve actor identity).
  - `webhook.created`/`updated`/`deleted` — `pkg/webhook/handler.go` admin write paths.
    Delete captures before-state snapshot (same pattern as user/tenant delete audits).

- **`LogAsync` non-blocking contract test** (`pkg/audit/service_async_test.go`).
  The login path is the highest-QPS audit producer; if `LogAsync` ever blocked
  when the buffer saturated, request threads would pin waiting on audit writes.
  `TestLogAsync_BufferOverflowNonBlocking` verifies the `default` drop branch
  is taken under buffer saturation with a 2-second watchdog.

### Changed

- `NewAuthHandler`, `NewEmailHandler`, `NewInternalAuthHandler`,
  `NewSocialHandler*`, `webhook.NewHandler` all accept `audit.Service` in
  their signatures.
- `apps/branding/auth-api/README.md` — explicit note that the branding layer
  does **not** record audit (central API is the single source of truth).
  Prevents double-recording during future refactors.

### Carry-Forward Issues Filed

- `ISSUE-Authway-20260415-stale-auth-handler-tests.md` — `internal/handler/auth_test.go`
  is `integration`-tagged and fails to compile due to drift (User field rename,
  two added constructor arguments). Option A (real-DB integration rewrite)
  recommended.
- `ISSUE-Authway-20260415-admin-session-token-hashing.md` — Admin session tokens
  are stored plaintext in the DB. Needs SHA-256 hash + constant-time compare.
  Blocked on staging environment availability.
- `ISSUE-Authway-20260415-module-consolidation.md` — `apps/branding/auth-api`
  separate `go.mod` blocks shared-pkg reuse. Awaiting architecture review.

### Notes

- `user.password_changed` constant remains unemitted. The only current
  password-change path is `ResetPassword`, which records `user.password_reset`.
  A dedicated change-password API would emit this constant at that point.
- `user.locked`/`user.unlocked` constants remain unemitted; brute-force lockout
  is not yet implemented (separate feature request).
- `session.*` constants are not emitted — Hydra owns session lifecycle. Hydra
  session revocation is captured as `user.logout`.
- `token.*` constants are owned by Hydra; central API does not record them.

### Operational (Run-7, 2026-04-27)

- **`scripts/deploy/` is now git-tracked** (commit `3c83b66`). Previously the
  entire directory was `.gitignore`d, so every deploy machine drifted
  independently. New policy: `scripts/deploy/*/.env` is the only ignored
  pattern, so `prod/.env` and `staging/.env` stay local while the shared
  logic, target wrappers, and `.env.example` templates are versioned.
- **Hydra entrypoint args explicit on every deploy.**
  `publish-hydra.core.ps1` passes `--command "/bin/sh" --args "-c" "hydra serve all --dev"`
  on each `az containerapp update`. Run-6 staging hit a regression where the
  Container App had `args=["serve all --dev"]` as a single token; forcing
  prod's working pattern on every deploy prevents the regression returning.
- **Migration 009** (`009_audit_logs_p4_columns.sql`, commit `cd8e35a`)
  aligns `audit_logs` with the GORM model used by the P4 wiring above:
  `actor_type`/`details`/`error_msg` columns added, `metadata → details`
  backfilled, `tenant_id`/`actor_id` FKs dropped (audit is append-only
  history; system events use `uuid.Nil` which violated the FKs).
- **Polish (commit `1e9699f`)**: gated 5x startup `Config Printf` statements
  behind `Environment != "production"`, removed the lingering "DEBUG Hydra
  Client" `Printf`, and marked `_shared/deploy-with-migration.ps1` as
  DEPRECATED (no callers across `scripts/deploy/` or `.github/`).

### Removed (Run-7 pre-flight cleanup)

- 4 dead helper files in `scripts/deploy/_shared/` that contained inline
  secrets: `hydra-admin-config.yaml`, `hydra-container-config.yaml`,
  `update-hydra-admin.ps1`, `test-oauth-client.ps1`. All confirmed never
  committed (git ledger empty for each path) — no rotation needed. The
  underlying `JWT_ACCESS_SECRET == Hydra SECRETS_SYSTEM` reuse pattern
  observed in `prod/.env` is tracked separately in
  `claudedocs/issues/ISSUE-Authway-20260427-hydra-secrets-system-jwt-reuse.md`.

## [0.3.1] - 2026-04-14

### Security

- **Critical: JWT signatures are now verified**. The `JWTAuth` middleware
  protecting `/api/v1/profile/*`, `/api/v1/claims/*`, `/api/v1/users/mfa/*`,
  `/api/v1/account-link/*`, and `/api/v1/logout` previously decoded
  non-`ory_at_*` tokens locally without checking the signature, accepting any
  base64-encoded JWT-shaped payload as authentication. Forging a valid
  Authorization header for arbitrary `sub` (user_id) and `tenant_id` claims
  was trivial. All tokens — opaque and JWT — now go through Hydra's
  `/admin/oauth2/introspect` endpoint, which validates the signature, active
  state, and expiration. The local `extractClaimsFromToken` path was removed
  entirely.
- **Constant-time credential compare**. `AdminAuth`, `GetAdminConsoleAuth`,
  `InternalAPIAuth`, and `admin.Authenticate` (password) used plain `!=`
  comparisons that leak prefix-match length via timing. Replaced with
  `crypto/subtle.ConstantTimeCompare`.
- **`InternalAPIAuth` no longer logs the configured key**. The previous
  implementation logged `expected_key` and the configured `apiKey` length on
  every failed attempt, turning the audit log into a credential-disclosure
  surface for anyone with log access.
- **`InternalAPIAuth` is fail-closed**. Empty configured key now returns 503
  (matching the `AdminAuth`/`GetAdminConsoleAuth` policy from 0.2.1).
  Production validation now requires `AUTHWAY_ADMIN_INTERNAL_API_KEY` to be
  set; dev mode auto-generates and logs a key (same pattern as the admin key).
- **`crypto/rand` failure handling in client credential generation**. The
  `generateClientID`/`generateClientSecret` helpers ignored `rand.Read`
  errors, which on a broken-entropy system would silently mint zero-byte
  (i.e. fully predictable) credentials. They now panic, matching the
  treatment in `pkg/admin/service.go` and `pkg/impersonation/service.go`.

### Fixed

- **URL encoding in Hydra client**. `subject` (in `RevokeUserSessions`) and
  `challenge` (in 8 login/consent/logout request endpoints) were raw-
  interpolated into URLs via `fmt.Sprintf`. Subjects containing spaces
  produced a 400 Bad Request from Hydra; this is what surfaced the bug
  through the `subject_with_spaces` regression test that started failing
  this release. All values now pass through `url.QueryEscape`.
- **TOTP otpauth URL encoding** (`pkg/mfa/service.go`). The QR-code URL was
  built by `fmt.Sprintf("otpauth://totp/%s:%s?…", issuer, email, …)` and
  silently produced an invalid URI for any email containing `+` (the very
  common alias form `user+tag@example.com`) or any issuer with whitespace.
  The library already returns a properly encoded URL via `key.URL()` — we
  now use it.
- **OAuth authorize URL encoding** (`internal/handler/auth.go`). The
  authorize URL was built by `fmt.Sprintf` with raw `redirect_uri`, `scope`
  (which contains spaces by definition), and caller-supplied `state`.
  Replaced with `url.Values.Encode()`.
- **Removed production DEBUG logs** from `pkg/client/service.go` and
  `internal/hydra/client.go`.

### Migration

- All environments now require Hydra to be reachable for any
  JWT-authenticated endpoint. The previous local-decode path that allowed
  unauthenticated requests through is gone — there is no compatibility shim.
- Production deployments must additionally set
  `AUTHWAY_ADMIN_INTERNAL_API_KEY`. Without it the server refuses to start
  (same policy as `AUTHWAY_ADMIN_API_KEY` in 0.2.1).

---

## [0.3.0] - 2026-04-14

### Added

- **`sync_status` in client mutation responses**. `PUT/DELETE /api/v1/clients/:id`
  and `POST /api/v1/clients/:id/regenerate-secret` now include a
  `sync_status: { state, error }` object describing whether the change was
  replicated to Hydra. Previously a Hydra failure was logged as a warning and
  the response remained 200 OK — silently producing drift between Authway DB
  and Hydra. (Issue: `hydra-sync-silent-failure`.)
- **Strict-sync mode**. Pass `?strict_sync=true` on a mutation to receive
  `502 Bad Gateway` (with `sync_status` and `hint`) when the upstream sync
  fails, instead of the default best-effort 200. Useful for CI/migration
  scripts that must not proceed past silent drift.
- **Client config validation**. Create/Update now reject internally
  inconsistent OAuth client configurations with a structured 400 response
  (`code`, `field`, `message`, `hint`). Catches the ASP.NET-on-PKCE foot-gun
  at registration instead of at first login. (Issue:
  `aspnet-confidential-guidance-gap`.)
  - `public_client_with_secret` — public clients must not have a `client_secret`
  - `public_client_with_client_credentials` / `public_client_with_password_grant` — wrong client type for grant
  - `public_client_missing_allowed_origins` — SPA needs CORS allow-list
  - `confidential_client_unsupported_grants` — confidential clients need credential-bearing grants

---

## [0.2.1] - 2026-04-14

### Security

- **Critical: Authentication added to `/api/v1/clients/*` CRUD endpoints**. Before
  this release, Create/Read/Update/Delete/RegenerateSecret/SyncHydra/GoogleOAuth
  endpoints accepted unauthenticated requests, allowing any network-reachable
  caller to enumerate OAuth client configurations, modify `redirect_uris`
  (enabling authorization-code hijacking), and delete clients across tenants.
  All write endpoints and sensitive reads now require `AUTHWAY_ADMIN_API_KEY`
  via `Authorization: Bearer <key>`. Public-config (`/clients/:client_id/config`)
  and internal lookup (`/clients/by-client-id/:client_id`) remain unchanged.
- **Critical: `AdminAuth` middleware is now fail-closed**. Previously, when the
  configured API key was empty the middleware called `c.Next()` — meaning a
  deployment missing `AUTHWAY_ADMIN_API_KEY` silently exposed every protected
  endpoint. It now returns `503 Service Unavailable` on empty configuration.
  Production config validation enforces the key is set; development deployments
  auto-generate a random key at startup and log it.
- **PII: `/api/v1/profile/:id` now requires JWT**. The endpoint previously
  returned `email`, `name`, and `email_verified` for any UUID without auth,
  enabling user-enumeration. Use `/profile/me` for the authenticated user's
  own profile and `/users/:id` (admin) for administrative access.

### Changed

- **Unified admin auth**. `/api/v1/clients/*`, `/api/v1/users/*`, and
  `/api/v1/tenants/*` now accept either the long-lived `AUTHWAY_ADMIN_API_KEY`
  (programmatic callers) **or** an admin session token issued by `/admin/login`
  (Admin Console UI). Previously only the API key was accepted by the new
  client routes, which would have broken the Admin Console.
- **`GetAdminConsoleAuth` is fail-closed**. The same silent-bypass pattern that
  affected `AdminAuth` was present here for `/api/v1/webhooks/*`,
  `/api/v1/audit/*`, `/api/v1/invitations/*`, and `/api/v1/admin/impersonate/*`.
  Empty `AUTHWAY_ADMIN_API_KEY` now returns 503 instead of granting access.

### Migration

Operators running Authway without `AUTHWAY_ADMIN_API_KEY` set in production
**must** set it before upgrading — otherwise the server will refuse to start
(`configuration validation failed`). Existing clients of the admin API must
send `Authorization: Bearer $AUTHWAY_ADMIN_API_KEY` on all `/clients/*` and
`/profile/:id` requests. The Admin Console UI continues to work via session
tokens issued by `/admin/login`.

---

## [0.2.0] - 2025-12-03

### Added
- **i18n (Internationalization)**: Multi-language support for Auth UI
  - Korean (ko) and English (en) languages
  - Automatic browser language detection
  - URL query parameter support (`?lang=ko`)
  - Language preference persistence via localStorage
- **LanguageSwitcher Component**: User-selectable language switching in Auth UI
- **Popup Callback Module**: `@authway/client/popup-callback` for auto-executing callback handling
  - Detects popup/iframe context automatically
  - Extracts OAuth code and sends to parent via postMessage
  - Closes popup after completion
- **Next.js Sample**: Complete integration example with `@authway/react`
- **OIDC Logout Enhancement**: `post_logout_redirect_uri` support for controlled logout redirects

### Changed
- **Auth UI Pages**: All pages migrated to use i18n (`useTranslation` hook)
  - LoginPage, RegisterPage, ConsentPage, ErrorPage
  - ForgotPasswordPage, ResetPasswordPage, VerifyEmailPage
  - ResendVerificationPage, LogoutPage, PopupCallbackPage
  - GoogleLoginButton
- **Zod Validation**: Dynamic schema factories for i18n validation messages
- **Error Messages**: API error mapping to localized messages

### Technical Details
- **i18n Stack**: i18next + react-i18next + i18next-browser-languagedetector
- **Translation Namespaces**: common, auth, consent, password, errors
- **Language Detection Order**: querystring → localStorage → navigator → fallback (en)

---

## [0.1.4] - 2025-11-10

### Fixed
- **Popup Login Mode with OAuth Providers**: Popup mode now works correctly with Google OAuth and other external providers
- **Cross-Origin-Opener-Policy (COOP) Blocking**: Solved `window.opener` loss after Google OAuth redirect using sessionStorage
- **Popup Window Closure**: Popup now closes automatically after successful authentication
- **Authorization Code Extraction**: Code is extracted AFTER Hydra generates it, not before

### Changed
- **GoogleLoginButton.tsx**: Set sessionStorage flag when popup mode is detected
- **ConsentPage.tsx**: Check both window.opener AND sessionStorage for popup mode detection; added postMessage listener to receive code from iframe
- **LoginPage.tsx**: Check both window.opener AND sessionStorage for popup mode detection; added postMessage listener to receive code from iframe
- **callback.html**: Modified to actively send postMessage when running in iframe (both asp-spa and react-sdk-sample versions)
- **Popup Flow**: SessionStorage persists popup mode flag across cross-origin redirects

### Technical Details
- **Root Cause**: Google OAuth's Cross-Origin-Opener-Policy (COOP) blocks `window.opener` during cross-origin redirects
- **Secondary Issue**: redirect_to is a Hydra URL that GENERATES the code, not a callback URL that HAS the code
- **Solution**: SessionStorage + Hidden iframe + postMessage communication
  1. GoogleLoginButton detects popup mode and sets `sessionStorage.setItem('authway_popup_mode', 'true')`
  2. After Google OAuth redirect (which blocks window.opener), pages check sessionStorage
  3. Create hidden iframe in popup window (ConsentPage/LoginPage)
  4. Load Hydra redirect URL in iframe (popup stays intact)
  5. callback.html detects it's in iframe and sends postMessage with code/state to parent
  6. ConsentPage/LoginPage receives postMessage from iframe
  7. Forward code/state via postMessage to main window (using window.opener || window.parent)
  8. Clean up sessionStorage, iframe, event listeners, and close popup
  9. Fallback: URL polling every 100ms if postMessage fails (max 5 seconds)
- **Why SessionStorage**: Survives cross-origin redirects (unlike window.opener which COOP blocks)
- **Pattern**: Hybrid approach - sessionStorage for state persistence + hidden iframe for OAuth flow
- **Compatibility**: 100% backward compatible - redirect mode unchanged

### How It Works
```
Before (v0.1.3): ❌
LoginPage → Google OAuth (cross-origin) → ⚠️ COOP blocks window.opener
→ ConsentPage: window.opener === null → ❌ popup mode NOT detected

After (v0.1.4): ✅ SessionStorage + Hidden iframe
GoogleLoginButton (3001) popup:
  ↓ Detects popup mode: window.opener !== null
  ↓ Sets sessionStorage.setItem('authway_popup_mode', 'true')
  ↓ window.location.href = Google OAuth (cross-origin redirect)

Google OAuth redirect → ⚠️ COOP blocks window.opener

ConsentPage (3001) in popup:
  ↓ window.opener === null (BLOCKED by COOP)
  ↓ BUT sessionStorage.getItem('authway_popup_mode') === 'true' ✅
  ↓ Popup mode detected via sessionStorage!
  ↓ Create hidden iframe
  ↓ iframe.src = Hydra URL (4444)
  ↓ Hydra redirects iframe → callback.html (5173) with code
  ↓ Read iframe.contentWindow.location.href
  ↓ Extract code from iframe URL
  ↓ (window.opener || window.parent).postMessage({code, state})
  ↓ sessionStorage.removeItem('authway_popup_mode')
  ↓ Close popup

Main window (5173):
  ↓ Receives postMessage with code
  ✅ Exchange code for tokens
```

---

## [0.1.3] - 2025-11-10

### Fixed (Partial)
- **Popup Mode Detection**: Login UI now detects popup mode and logs flow progression
- **Developer Experience**: Added console logging for debugging popup authentication flow

### Known Issues (Fixed in v0.1.4)
- Popup mode detection worked but popup login still failed due to cross-origin redirect
- `window.opener` was lost when redirecting from localhost:3001 to localhost:5173

### Added
- **Popup Mode Detection**: Added `window.opener` detection logic to 7 redirect points
- **Flow Tracking**: Console logs show popup mode status at every navigation step

### Changed
- **GoogleLoginButton.tsx**: Added popup mode detection logging
- **ConsentPage.tsx**: Added popup mode detection logging (3 locations)
- **LoginPage.tsx**: Added popup mode detection logging (3 locations)

### Technical Details
- Detection only - did not solve the underlying cross-origin redirect problem
- Required v0.1.4 for actual popup mode functionality

---

## [0.1.2] - 2025-11-10

### Fixed
- **OAuth 2.0 Compliance**: Public clients can now register without `client_secret`, following RFC 6749 Section 2.1
- **Client Registration Logic**: Fixed silent failure when custom `client_id` was provided without `client_secret`
- **Error Messages**: Added clear validation errors for confidential clients with partial credentials

### Added
- **Comprehensive Documentation**: New [Client Registration Guide](./docs/CLIENT_REGISTRATION.md) covering Public and Confidential clients
- **Unit Tests**: Added extensive test coverage for public/confidential client creation scenarios
- **Better Logging**: Enhanced error messages with masked secrets for security

### Changed
- **API Behavior**: Public clients now correctly ignore `client_secret` field (backward compatible)
- **Confidential Clients**: Now require both `client_id` and `client_secret`, or neither (for auto-generation)
- **Code Comments**: Updated `CreateClientRequest` documentation to reflect actual behavior

### Security
- **Public Client Security**: Removed requirement to store dummy secrets for public clients
- **Secret Masking**: Added `maskSecret()` helper for safe logging of credentials

---

## [0.1.1] - 2025-10-29

### Fixed

#### 🔒 CORS Configuration for OAuth Authentication
- **Fixed CORS wildcard issue**: Changed `AUTHWAY_CORS_ALLOWED_ORIGINS` from wildcard `*` to specific allowed origins
- **Resolved OAuth failures**: Fixed browser security policy violation that blocked all OAuth authentication flows (Google OAuth, social login providers)
- **Production domains**: `https://auth.iyulab.com`, `https://authway-admin.iyulab.com`
- **Development support**: Included `http://localhost:3000`, `http://localhost:5173` for local development
- **Impact**: Resolves 100% OAuth authentication failures in production
- **Reference**: Issue #001 - CORS Wildcard with Credentials

---

## [0.1.0] - 2025-10-20

### Added

#### 🆕 Dynamic Claims Management
- **Real-time Claims Updates**: Update user claims (workspace, role, permissions) without requiring logout/login
- **Automatic Token Refresh**: Silent re-authentication flow for seamless token updates
- **Claims API**: New REST endpoints for claims management
  - `GET /api/v1/claims` - Retrieve current user claims
  - `POST /api/v1/claims/update` - Update claims and trigger re-authentication
  - `DELETE /api/v1/claims/:claim_key` - Delete specific claim
- **Dual Storage Strategy**:
  - Redis for pending/session claims (5-minute TTL)
  - PostgreSQL for permanent claims (persistent across sessions)
- **Multi-tenant Support**: Claims isolated per tenant for security
- **Comprehensive Documentation**:
  - Complete feature guide with use cases and architecture
  - Implementation examples for JavaScript, Python, Go, and .NET
  - Integration guides with code samples
  - Testing instructions and best practices

#### 🔄 Database Migration System
- **Automatic Schema Management**: Embedded SQL migrations with version tracking
- **Migration Runner**: Internal package for automated database updates
- **Version Control**: Migration history tracked in `schema_migrations` table
- **Startup Automation**: Migrations run automatically on server startup
- **Schema Versioning**: Support for incremental schema updates

#### ✏️ OAuth Client Management Enhancements
- **Edit Functionality**: Update existing OAuth clients in Admin Dashboard
  - Client name, description, and website
  - Redirect URIs configuration
  - Grant types and scopes management
  - Client secret remains view-once only (security best practice)
- **Improved UX**: Inline edit forms with validation
- **React Query Integration**: Optimistic updates and error handling

#### 📚 Documentation Improvements
- **Comprehensive API Documentation**: New `API_INTRODUCTION.md` with complete endpoint reference
- **Dynamic Claims Guide**: Detailed feature documentation at `docs/features/DYNAMIC_CLAIMS.md`
- **Language-Specific Quickstarts**:
  - JavaScript/Node.js integration guide
  - Python/Flask integration guide
  - .NET/ASP.NET Core integration guide with ClaimsService example
  - Go integration guide
- **Integration Guide Updates**: Added Dynamic Claims Integration section
- **Quick Start Guide**: Updated with links to new features and documentation
- **README Updates**:
  - Added Dynamic Claims and Database Migration features
  - Updated project structure to reflect new packages
  - Added comprehensive API & Integration section
- **Documentation Index**: Central navigation hub for all project documentation

### Changed

#### 🧹 Code Quality Improvements
- **Production-Ready Logging**: Cleaned up verbose debug logging in claims service
  - Removed temporary diagnostic markers (🔍 DEBUG)
  - Retained essential production-level logging
  - Improved observability for production debugging
- **Error Handling**: Enhanced error messages and logging context

#### 📁 Project Structure
- **New Packages**:
  - `src/server/pkg/claims/` - Dynamic claims management service
  - `src/server/internal/database/` - Migration runner and database utilities
  - `src/server/internal/middleware/` - JWT authentication middleware
- **Migration Organization**: SQL migrations in `src/server/internal/database/migrations/`

### Fixed

- **Claims Integration Issues**: Resolved missing claims after workspace switching
  - Implemented proper UserInfo parameter passing
  - Fixed consent flow to include base user claims
  - Added fallback mechanisms for Redis and database claims
- **API Endpoint Consistency**: Corrected endpoint paths in API documentation
  - POST endpoint is `/api/v1/claims/update` (not `/api/v1/claims`)
  - DELETE endpoint parameter is `:claim_key` (standardized)

### Security

- **Claims Validation**: Server-side validation of all claim updates
- **Token Lifecycle Management**:
  - Access tokens: 15-minute lifetime (recommended)
  - Pending claims: 5-minute Redis TTL
  - Login claims: 10-minute Redis TTL
- **CSRF Protection**: State parameter validation in OAuth flows
- **Multi-tenant Isolation**: Claims scoped to authenticated user and tenant

### Documentation

All documentation files updated to version 0.1.0 with last updated date of 2025-10-18 or 2025-10-20.

### Infrastructure

- **Azure Application Insights**: Telemetry and distributed tracing
- **Database Schema**: New `user_claims` table with JSONB support
- **Redis Integration**: Claims caching and session management

---

## [0.0.1] - 2025-10-01 (Initial Release)

### Added

#### Core Features
- **Multi-tenant Architecture**: Complete tenant isolation with central management
- **OAuth 2.0 / OpenID Connect**: Standard-compliant authentication flows
- **JWT Token Management**: Secure access and refresh tokens
- **Email Authentication**: User registration with email verification
- **Password Reset**: Secure password recovery workflow
- **Social Login**: Google OAuth integration
- **Admin Console**: React-based dashboard for managing:
  - OAuth clients
  - Users and tenants
  - System configuration
- **Login UI**: Modern authentication interface with Vite and React
- **Session Management**: Redis-based session storage

#### Backend (Go)
- **Fiber Framework**: High-performance web server
- **PostgreSQL Integration**: Primary data storage with GORM
- **Ory Hydra Integration**: OAuth 2.0 server backend
- **Structured Logging**: zap logger with context
- **CORS Configuration**: Configurable cross-origin request handling

#### Frontend
- **React 18**: Modern UI library
- **TypeScript**: Type-safe development
- **TailwindCSS**: Utility-first styling
- **TanStack Query**: Efficient data fetching and caching
- **Vite**: Fast build tooling

#### Security
- **SSL/TLS Support**: Production PostgreSQL encryption
- **Azure Key Vault**: Secret management integration
- **Password Hashing**: Secure credential storage
- **Token Validation**: JWT signature verification

#### Deployment
- **Azure Container Apps**: Backend hosting
- **Azure Static Web Apps**: Frontend hosting
- **Azure Database for PostgreSQL**: Managed database
- **Docker Support**: Containerized deployment
- **PowerShell Scripts**: Automated deployment workflows

#### Documentation
- **Quick Start Guide**: 5-minute local setup
- **Docker Guide**: Complete containerization instructions
- **Configuration Guide**: Comprehensive environment variable documentation
- **Architecture Documentation**: Multi-tenancy design and system architecture
- **Deployment Guides**: Azure-specific production deployment
- **Testing Guide**: Test suite documentation

---

## Release Notes

### Version 0.1.0 Highlights

This release brings **Dynamic Claims Management**, a powerful feature that enables real-time user context switching without requiring logout/login cycles. This is particularly valuable for:

- **SaaS Applications**: Users can switch between workspaces or organizations seamlessly
- **Enterprise Systems**: Administrators can update permissions instantly
- **Feature Flags**: Enable/disable features dynamically per user
- **Support Systems**: Grant temporary elevated access for troubleshooting

Additionally, the **Database Migration System** ensures smooth schema updates across deployments, and the **OAuth Client Edit** functionality provides better administrative control.

### Breaking Changes

None. This is a minor version update that is fully backward compatible with 0.0.1.

### Upgrade Instructions

1. **Database Migration**: Migrations run automatically on server startup
2. **Environment Variables**: No new required variables (optional Application Insights connection string)
3. **API Changes**: New endpoints are additive only, existing endpoints unchanged
4. **Dependencies**: Update Go modules with `go mod download`
5. **Frontend**: Update npm packages with `npm install` in admin-dashboard and login-ui

### Known Issues

- Rate limiting not yet implemented
- Postman collection and OpenAPI spec coming in future release
- Language SDKs (JavaScript, Python, Go, .NET) planned for future releases

---

## Links

- **Repository**: https://github.com/iyulab/authway
- **Documentation**: [docs/](docs/)
- **Issues**: https://github.com/iyulab/authway/issues
- **Discussions**: https://github.com/iyulab/authway/discussions

---

**Maintained by**: Authway Team
**License**: MIT
