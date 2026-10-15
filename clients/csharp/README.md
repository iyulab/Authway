# Authway.Client

A .NET client for Authway's administration API — what a service's backend calls to manage its tenant:

- **Applications** (OAuth 2.0 clients): register, read, change, delete.
- **Invitations**: invite a person — optionally for one application, which they are sent on to once they accept —
  list, withdraw, send again.
- **Users**: read them, their stored claims (what Authway adds to their tokens) and which terms they accepted.
- **Webhooks**: register endpoints, rotate their secrets, test them, and **verify deliveries** (Standard Webhooks).
- **Terms**: publish and retire the documents a tenant's users accept at sign-in.

Signing users in is not here. That is OpenID Connect: point your framework's handler (in ASP.NET Core,
`AddOpenIdConnect`) at Authway's issuer, and validate access tokens with the standard JWT bearer handler.

```
dotnet add package Authway.Client
```

It targets .NET 10, depends on nothing outside the framework, and is safe to publish with Native AOT.

## Calling the API

```csharp
using Authway.Client;

var http = new HttpClient { BaseAddress = new Uri("https://<your Authway API>/") };

// With the deployment's admin key, name the tenant. A tenant service credential's token implies it.
var authway = new AuthwayClient(http, adminKey, tenantId);

var app = await authway.Clients.CreateAsync(new NewOAuthClient(
    "My app", GrantTypes: ["authorization_code", "refresh_token"], Scopes: ["openid", "profile", "email", "offline_access"],
    RedirectUris: ["https://app.example.com/signin-oidc"]));
// app.Credentials holds the client secret of a confidential client — shown this once.

await authway.Invitations.CreateAsync("person@example.com", clientId: app.Client.ClientId);

var user = await authway.Users.GetAsync(sub);
await authway.Users.PatchClaimsAsync(sub, JsonSerializer.SerializeToElement(new { roles = new[] { "editor" } }));
```

A token that is refreshed goes in as a delegate called before each request:

```csharp
var authway = new AuthwayClient(http, async ct => await tokens.GetAccessTokenAsync(ct));
```

## Refusals

Authway refuses with RFC 9457 problem details. Every refusal becomes an `AuthwayException`: branch on `Code`, show
`Message` (the problem's `detail`).

```csharp
try
{
    await authway.Invitations.CreateAsync(email);
}
catch (AuthwayException e) when (e.Code == "invitation_already_pending")
{
    // already invited
}
```

`e.Problem` carries the rest — `Field` (the request member at fault), `Hint`, `RetryAfter`, and any other member in
`Extensions`.

## Webhook deliveries

Deliveries are signed as [Standard Webhooks](https://www.standardwebhooks.com) specifies. Verify one with the secret
the webhook was created with, the raw body — before any JSON parsing — and the request's headers:

```csharp
app.MapPost("/hooks/authway", async (HttpRequest request) =>
{
    using var buffer = new MemoryStream();
    await request.Body.CopyToAsync(buffer);

    WebhookEvent e;
    try
    {
        e = AuthwayWebhook.Read(webhookSecret, name => request.Headers[name], buffer.ToArray());
    }
    catch (AuthwayException)
    {
        return Results.Unauthorized();
    }

    if (e.Type == WebhookEventTypes.UserDeleted)
    {
        // e.Resource.Id is the user's id — the sub of their tokens. A retry carries the same e.Id.
    }

    return Results.NoContent();
});
```

An event names what happened and to what (`Resource`, `Actor`), nothing more; ask the API for the details.
`AuthwayWebhook.Sign` produces what Authway sends, for testing a receiver.

## License

MIT — see [LICENSE](LICENSE).
