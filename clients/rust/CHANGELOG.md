# Changelog

All notable changes to `authway-native` are recorded here. It is released together with `Authway.Native` (the same client for
another language), under the same version, apart from the Authway service.

## 0.1.0 — 2026-10-18

### Added

- `NativeClient` — signs a desktop or command-line app's user in through the system browser (OAuth 2.0 for Native Apps:
  a loopback port the operating system picks, PKCE `S256`, `state` and `nonce`), with endpoints from OpenID Connect
  discovery. `begin_sign_in` → `PendingSignIn::url` / `wait` (and a `Cancel` handle), `access_token` (refreshed before
  it expires; `None` once the server no longer accepts the refresh token), `session`, `sign_out` (revokes at the server
  and clears the store; `SignedOut::revoked` says whether the server confirmed).
- `TokenStore` — where the session is kept (get/set/delete of one value); `MemoryStore`.
- `Error`, including the OAuth error code of a refusal.
