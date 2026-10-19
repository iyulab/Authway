# authway-native

Sign a desktop or command-line app's user in with the system browser, keep the tokens in a store the app chooses, and
refresh them before they expire.

This is [OAuth 2.0 for Native Apps (RFC 8252)](https://www.rfc-editor.org/rfc/rfc8252): the app listens on a loopback
port the operating system picks, the user signs in in their own browser, and the authorization code comes back to that
port. PKCE (`S256`) and `state` bind it to this sign-in. Endpoints come from OpenID Connect discovery, so it works with
any OpenID provider; it is written for [Authway](https://github.com/iyulab/Authway).

```
cargo add authway-native
```

It blocks and needs no async runtime. In an async app, run its calls on a blocking thread (`spawn_blocking`).

## Register the app

At the authorization server, register a **public** client:

| | |
|---|---|
| redirect URI | `http://127.0.0.1/callback` — **without a port**; the server accepts it on any port. Use the IP literal, not `localhost`. |
| grants | `authorization_code`, `refresh_token` |
| scopes | `openid profile email offline_access` — `offline_access` brings the refresh token |

With Authway, add `"access_token_strategy": "jwt"` and the `audience` of the APIs the app calls, so their tokens can be
validated as JWTs (see `docs/api/client-management.md`).

## Use it

```rust
use authway_native::{Config, NativeClient};
use std::time::Duration;

let client = NativeClient::new(Config::new("https://auth.example.com", "my-desktop-app"), my_store);

// Sign in: open the URL the way the app opens links, then wait for the browser to come back.
let pending = client.begin_sign_in()?;
open_in_browser(pending.url());
let cancel = pending.canceller(); // cancel.cancel() from the UI ends the wait
let session = pending.wait(Duration::from_secs(300))?;
println!("signed in as {}", session.claims["email"]);

// Before each API call: refreshed when about to expire; None once the session has ended.
match client.access_token()? {
    Some(token) => { /* Authorization: Bearer {token} */ }
    None => { /* signed out — sign in again */ }
}

// Sign out: revokes the tokens at the server and clears the store.
let out = client.sign_out()?;
```

`session()` tells who is signed in without a network call. `Config` sets the scopes, the redirect path, the pages the
browser shows when it comes back, timeouts and how early to refresh.

### Keep the session in the OS credential store

`TokenStore` is three methods — `get`, `set`, `delete` of one value under one key — the shape OS keychain libraries
share, so an adapter is a few lines. `MemoryStore` is included for tests and one-off command-line runs. The stored
value is the refresh token and the signed-in user's claims; the access token stays in memory, so the first
`access_token()` after the app starts refreshes it. That keeps the value well inside one Windows Credential Manager
entry (2,560 bytes) and keeps bearer tokens off the disk. Keep the value where only the user can read it.

### One process owns the session

The authorization server issues a new refresh token every time one is used, and treats a second use of the old one as
theft: it ends the session. Two processes refreshing from one stored session — a desktop shell and its background
process — therefore sign the user out. **In one app, let one process sign in, refresh and sign out**; other processes
ask it for the current access token.

Inside that process no lock of your own is needed: `NativeClient` is `Clone + Send + Sync`, its clones share the
session, and calls from several threads take turns — when the access token is due, the first caller refreshes and the
others get the renewed token. Clone the one client rather than building a second: the access token lives in it, so a
second client in the same process refreshes again.

## Errors

`Error::Denied` — the user declined or the server refused the request; `Error::TimedOut` / `Error::Cancelled` — no
sign-in; `Error::Token` — the token endpoint refused (with the OAuth `error` code); `Error::Http` — the server could not
be reached. A refresh token the server no longer accepts is not an error: `access_token()` returns `None` and the session
is removed.

## Try it

```
cargo run --example sign_in -- https://auth.example.com my-desktop-app
```

prints the URL to open, then who signed in, refreshes, and signs out.

## License

MIT
