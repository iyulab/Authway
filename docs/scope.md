# What is in and out of Authway

> The line between Authway and the applications that use it. The rules Authway
> keeps are in the [constitution](CONSTITUTION.md).

## In

- **Sign-in** for applications: OAuth 2.0 / OpenID Connect (authorization code
  with PKCE, refresh, client credentials, device authorization, token exchange),
  with password, emailed link, social providers and passkeys, a second factor
  (TOTP and recovery codes), and the tenant's terms accepted at sign-in.
- **Accounts**: invitation onboarding, an account screen where a user manages how
  they sign in (second factor, password, connected provider accounts, passkeys),
  and deletes their own account; suspension by an administrator.
- **Tenants and clients**: tenants, the clients of each, their sign-in methods,
  logout and back-channel logout, per-tenant sign-up and session policy.
- **Operating it**: an administration API and console, audit logs, signed
  webhooks, custom user claims, health and deployment tooling, an installable
  bundle for one tenant.
- **For applications**: the published contract, TypeScript SDKs
  (`@authway/client`, `@authway/react`), a .NET client for the administration
  API, native-app sign-in (a loopback redirect), and a conformance suite.

## Out

- **Authorization of what a user may do in an application.** Authway says who
  the user is and what the tenant asserted about them (claims); roles,
  permissions and business rules belong to the application.
- **A user directory for other systems.** It is not an LDAP/SCIM server, and it
  does not synchronize accounts from or to other directories.
- **Acting as another user.** There is no impersonation. An administrator's
  access to a tenant is named, time-limited and audited.
- **Billing, payments and subscriptions.**
- **Public self-service sign-up as the default.** A tenant may open it; Authway
  does not.
- **A generic identity-provider hub.** Social sign-in covers the providers Authway
  names; a slot for any OpenID provider is added when a need is shown.
- **Decoupled back-channel approval (CIBA).** Not offered; `capabilities` says so.
