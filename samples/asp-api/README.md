# ASP.NET Core sample — an API (resource server)

A small ASP.NET Core API that accepts Authway access tokens. It is meant to be
read and copied: the whole of it is [`Program.cs`](Program.cs).

## What it shows

| Choice | Where | Why |
|---|---|---|
| The API knows the issuer and its own name, nothing else | `Authway:Issuer`, `Authway:Audience` | Signing keys and algorithms come from the issuer's discovery document. |
| A token must have been issued **for this API** | `options.Audience` | Without the audience check an API accepts any token of the issuer — one issued to another application, for another API, included. |
| No validation is turned off | the handler's defaults, left on | Signature, issuer, audience and lifetime each close a different hole. |
| What a token may do is its `scope` | the `email` policy | The right audience gets a caller in; it does not allow everything. |
| The browser application's origin is allowed, and only that | `Cors:AllowedOrigins` | The token travels in a header, so no cookies and no credentials are involved. |

## Run it

You need the .NET 10 SDK and a running Authway (see the repository's
`docs/SETUP.md`). The settings in `appsettings.json` assume the OpenID Connect
issuer at `http://localhost:4444`.

```bash
dotnet run --launch-profile http    # http://localhost:5090
```

The API needs a caller with a token. [`../react-sdk-sample`](../react-sdk-sample)
is one: its README registers a client whose tokens are for this API —

- `"audience": ["sample-api"]`: the name in `Authway:Audience`. A client
  registered without it gets tokens this API refuses.
- `"access_token_strategy": "jwt"`: tokens the API can verify by itself. An
  opaque token can only be checked by asking Authway (introspection).

## What it answers

| Call | Answer |
|---|---|
| `GET /api/public` | `200`, with or without a token |
| `GET /api/me` without a token, with a token for another API or for none, or with one that has been altered | `401` |
| `GET /api/me` with a token for this API | `200`: `sub`, `tenant_id`, `scope`, `client_id` |
| `GET /api/email` with a token that lacks the `email` scope | `403` |
| `GET /api/email` with one that has it | `200` |
