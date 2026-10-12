# ASP.NET Core sample — a server-side relying party

A small ASP.NET Core application that signs users in with Authway and keeps
its own session. It is meant to be read and copied: every choice in
`Program.cs` is one a real application should make too.

It talks to Authway through OpenID Connect discovery only, so the same code
works against any Authway deployment.

## What it shows

| Choice | Where | Why |
|---|---|---|
| The provider's answer lands in a temporary cookie, not the session | `SignInScheme = "External"` | Nothing is authorized from what the provider said until the application has looked at it. |
| The session is built from the application's own user record | `/signin-complete`, `AppUsers.cs` | The provider says *who* signed in (`iss` + `sub`). What that person may do here is the application's decision — no claim of the provider's becomes a session claim or a role. |
| Users are found by issuer and `sub`, never by email | `AppUsers.FindOrCreate` | `sub` is stable and unique per issuer; an email address can change or be reused. |
| State, nonce and PKCE are all checked | the handler's defaults, left on | Each closes a different attack on the redirect; none is optional. |
| Only the ID token is carried into the session | `/signin-complete` | It is needed to sign out at the provider (`id_token_hint`). Access and refresh tokens are not kept, because this sample calls no API for the user. |
| Sign-out ends both sessions | `Account.cshtml.cs` | The application's cookie first, then RP-initiated logout at the provider. |
| The client secret is not in a tracked file | `dotnet user-secrets` | See below. |

## Run it

You need the .NET 10 SDK and a running Authway (see the repository's
`docs/SETUP.md`). The examples assume the Authway API at
`http://localhost:8080` and its OpenID Connect issuer at
`http://localhost:4444`.

**1. Register a client for the sample.** It is a confidential client: it runs
on a server and can keep a secret.

```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Authorization: Bearer $AUTHWAY_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "<your tenant id>",
    "name": "ASP.NET sample",
    "public": false,
    "redirect_uris": ["http://localhost:5080/signin-oidc"],
    "post_logout_redirect_uris": ["http://localhost:5080/signout-callback-oidc"],
    "grant_types": ["authorization_code", "refresh_token"],
    "scopes": ["openid", "profile", "email"]
  }'
```

The answer carries the client's `client_id` and, once, its `client_secret`.

**2. Give the sample its settings.** The secret goes into user secrets, which
live outside the repository:

```bash
cd samples/asp-sample
dotnet user-secrets set "Authway:ClientId" "<client_id>"
dotnet user-secrets set "Authway:ClientSecret" "<client_secret>"
```

`Authway:Authority` — the issuer — defaults to `http://localhost:4444` in
`appsettings.json`. For another deployment, set it to that deployment's
issuer: the `issuer` of its `/.well-known/openid-configuration`.

Outside development, supply the same three settings as environment variables
(`Authway__Authority`, `Authway__ClientId`, `Authway__ClientSecret`) or from
your secret store. Do not put a client secret in `appsettings.json`.

**3. Start it.**

```bash
dotnet run
```

Open <http://localhost:5080> and sign in with a user of the tenant the client
belongs to.

## Adapting it

- **Store users in your database.** `AppUsers` keeps them in memory so the
  sample has no dependencies; the shape — a row keyed by issuer and `sub` —
  is what to keep.
- **Calling an API for the user.** Keep the access and refresh tokens the
  handler received (`external.Properties` in `/signin-complete`) in a
  server-side store keyed by the session, and request the `offline_access`
  scope for a refresh token. Avoid putting them in the session cookie: they
  make it large, and every request carries them.
- **Roles and permissions.** Give them in your own user record, as the sample
  does with `Role`. If you decide to read a claim from the provider instead —
  for example one your backend stores through Authway's claims API — do it
  explicitly in `/signin-complete`, one named claim at a time, and only for
  claims you know who writes.
- **HTTPS.** Outside development the sample redirects to HTTPS and requires
  the issuer to be HTTPS (`RequireHttpsMetadata`).
