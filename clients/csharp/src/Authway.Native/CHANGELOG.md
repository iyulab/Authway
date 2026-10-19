# Changelog

All notable changes to `Authway.Native` are recorded here. It is released together with `authway-native` (the same client for
another language), under the same version, apart from the Authway service.

## Unreleased

### Changed

- The stored session no longer holds the access token — only the refresh token and the ID token's claims; the access
  token stays in the client, and the first call after a start refreshes it. A real sign-in's stored value carried a
  JWT access token and passed one Windows Credential Manager entry (2,560 bytes), so a store adapter over it refused
  the sign-in. A session without a refresh token still stores its access token, since nothing else could resume it.
  Sessions stored by 0.1.0 are read as they are: no new sign-in is needed.

### Documentation

- The README says that calls from several threads of one process need no lock of the app's own: they take turns,
  and when the access token is due the first caller refreshes while the others get the renewed token. A test holds
  this.

## 0.1.0 — 2026-10-18

### Added

- `NativeClient` — signs a desktop or command-line app's user in through the system browser (OAuth 2.0 for Native Apps:
  a loopback port the operating system picks, PKCE `S256`, `state` and `nonce`), with endpoints from OpenID Connect
  discovery. `BeginSignInAsync` → `PendingSignIn.Url` / `WaitAsync`, `GetAccessTokenAsync` (refreshed before it
  expires; `null` once the server no longer accepts the refresh token), `GetSessionAsync`, `SignOutAsync` (revokes at
  the server and clears the store; `SignedOut.Revoked` says whether the server confirmed).
- `ITokenStore` — where the session is kept (get/set/delete of one value); `MemoryTokenStore`.
- `NativeSignInException` with `NativeSignInFailure` and the OAuth `Error`.
