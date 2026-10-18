# Authway.Native

Sign a desktop or command-line app's user in with the system browser, keep the tokens in a store the app chooses, and
refresh them before they expire.

This is [OAuth 2.0 for Native Apps (RFC 8252)](https://www.rfc-editor.org/rfc/rfc8252): the app listens on a loopback
port the operating system picks, the user signs in in their own browser, and the authorization code comes back to that
port. PKCE (`S256`) and `state` bind it to this sign-in. Endpoints come from OpenID Connect discovery, so it works with
any OpenID provider; it is written for [Authway](https://github.com/iyulab/Authway).

```
dotnet add package Authway.Native
```

It targets .NET 10, depends on nothing outside the framework, and is safe to publish with Native AOT. For a service's
backend that manages Authway, see `Authway.Client`; this package is for the app the user runs.

## Register the app

At the authorization server, register a **public** client:

| | |
|---|---|
| redirect URI | `http://127.0.0.1/callback` — **without a port**; the server accepts it on any port. Use the IP literal, not `localhost`. |
| grants | `authorization_code`, `refresh_token` |
| scopes | `openid profile email offline_access` — `offline_access` brings the refresh token |

With Authway, add `"access_token_strategy": "jwt"` and the `audience` of the APIs the app calls, so their tokens can be
validated as JWTs.

## Use it

```csharp
using Authway.Native;

using var client = new NativeClient(new NativeClientOptions("https://auth.example.com", "my-desktop-app"), myStore);

// Sign in: open the URL the way the app opens links, then wait for the browser to come back.
using var pending = await client.BeginSignInAsync();
Process.Start(new ProcessStartInfo(pending.Url.ToString()) { UseShellExecute = true });
var session = await pending.WaitAsync(TimeSpan.FromMinutes(5), cancelWhenTheDialogCloses);
Console.WriteLine($"signed in as {session.Claims["email"]}");

// Before each API call: refreshed when about to expire; null once the session has ended.
var token = await client.GetAccessTokenAsync();
if (token is null)
{
    // signed out — sign in again
}

// Sign out: revokes the tokens at the server and clears the store.
var signedOut = await client.SignOutAsync();
```

`GetSessionAsync()` tells who is signed in without a network call. `NativeClientOptions` sets the scopes, the redirect
path, the pages the browser shows when it comes back, and how early to refresh.

### Keep the session in the OS credential store

`ITokenStore` is three methods — get, set, delete of one value under one key — the shape OS keychain libraries share,
so an adapter is a few lines (Windows Credential Manager, DPAPI-protected file, …). `MemoryTokenStore` is included for
tests and one-off command-line runs. The stored value holds the refresh token: keep it where only the user can read it.

### One process owns the session

The authorization server issues a new refresh token every time one is used, and treats a second use of the old one as
theft: it ends the session. Two processes refreshing from one stored session — a desktop shell and its .NET background
process — therefore sign the user out. **In one app, let one process sign in, refresh and sign out**; other processes
ask it for the current access token.

## Failures

Every failure is a `NativeSignInException`; branch on `Failure`: `Denied` (the user declined or the server refused —
`Error` holds the OAuth code), `TimedOut`, `Token` (the token endpoint refused — `Error`, `Status`), `Http` (the server
could not be reached). Cancelling the token passed to `WaitAsync` throws `OperationCanceledException`. A refresh token
the server no longer accepts is not a failure: `GetAccessTokenAsync()` returns `null` and the session is removed.

## Try it

```
dotnet run --project examples/SignIn -- https://auth.example.com my-desktop-app
```

prints the URL to open, then who signed in, refreshes, and signs out.

## License

MIT
