# React sample — a browser application

A small React application that signs users in with Authway and calls an API
with their access token. It is meant to be read and copied: the whole of it is
[`src/App.tsx`](src/App.tsx).

## What it shows

| Choice | Where | Why |
|---|---|---|
| The SDK is given the Authway API and a client id, nothing else | `config` | Every endpoint comes from discovery; none is written down. |
| Tokens are kept in the page only | `cacheLocation: 'memory'` | Nothing is left in the browser's storage. `'sessionstorage'` and `'localstorage'` keep them for the tab or the browser — no choice protects them from a script running in the page. |
| A reload does not sign the user out | `checkSessionOnLoad` | The page has no tokens after a reload, and asks Authway for the user's session: the browser goes there and back without showing anything. |
| The user comes back to where they were | `onRedirectCallback` | The request carries the address; the application navigates to it on return. |
| "Nobody is signed in" is not an error | `Page` | `error` is a sign-in the provider refused. A visitor who is not signed in just sees the button. |
| The user is identified by `sub` | `Page` | It is stable and unique per issuer; an email address can change. |
| The access token is asked for on each call | `CallTheApi` | The SDK hands out the current one and renews it when it can; the application keeps no copy. |
| Signing out ends the session at Authway too | `logout()` | And with it the session of every application the user was signed in to there. |

## Run it

You need Node 20+, pnpm, and a running Authway (see the repository's
`docs/SETUP.md`). The examples assume the Authway API at
`http://localhost:8080`. To call the API as well, run
[`../asp-api`](../asp-api) next to it.

**1. Register a client for the sample.** It is a public client: it runs in
the browser and cannot keep a secret.

```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Authorization: Bearer $AUTHWAY_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "<your tenant id>",
    "name": "React sample",
    "public": true,
    "redirect_uris": ["http://localhost:5173/"],
    "post_logout_redirect_uris": ["http://localhost:5173/"],
    "allowed_origins": ["http://localhost:5173"],
    "grant_types": ["authorization_code", "refresh_token"],
    "scopes": ["openid", "profile", "email"],
    "audience": ["sample-api"],
    "access_token_strategy": "jwt",
    "skip_consent": true
  }'
```

- `audience` names the API the tokens are for — the name `asp-api` checks.
- `access_token_strategy: "jwt"` gives access tokens an API can verify by
  itself.
- `skip_consent` is for your own application; leave it out for an application
  whose users should be asked what it may see.

The answer contains the `client_id`.

**2. Configure and start.** From the repository root:

```bash
pnpm install
pnpm --filter @authway/client --filter @authway/react build
cp samples/react-sdk-sample/.env.example samples/react-sdk-sample/.env.local
# put the client_id in .env.local
pnpm --dir samples/react-sdk-sample dev
```

Open `http://localhost:5173`. The port matters: it is part of the origin and
of the redirect URI the client is registered with.

**3. Try it.** Sign in, open *Call the API*, reload the page — you are still
signed in, at the same address. Sign out, and reload again: nothing happens,
because nobody is signed in.
