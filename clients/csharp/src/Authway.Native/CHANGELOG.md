# Changelog

All notable changes to `Authway.Native` are recorded here. It is released together with `authway-native` (the same client for
another language), under the same version, apart from the Authway service.

## 0.1.0 — 2026-10-18

### Added

- `NativeClient` — signs a desktop or command-line app's user in through the system browser (OAuth 2.0 for Native Apps:
  a loopback port the operating system picks, PKCE `S256`, `state` and `nonce`), with endpoints from OpenID Connect
  discovery. `BeginSignInAsync` → `PendingSignIn.Url` / `WaitAsync`, `GetAccessTokenAsync` (refreshed before it
  expires; `null` once the server no longer accepts the refresh token), `GetSessionAsync`, `SignOutAsync` (revokes at
  the server and clears the store; `SignedOut.Revoked` says whether the server confirmed).
- `ITokenStore` — where the session is kept (get/set/delete of one value); `MemoryTokenStore`.
- `NativeSignInException` with `NativeSignInFailure` and the OAuth `Error`.
