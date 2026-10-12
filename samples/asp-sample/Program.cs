using System.Security.Claims;
using AuthwaySample;
using Microsoft.AspNetCore.Authentication;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.IdentityModel.Protocols.OpenIdConnect;

// A server-side relying party: the browser signs in at the identity provider,
// and this application keeps its own session. It needs nothing from the
// provider beyond OpenID Connect discovery, so the same code works against any
// Authway deployment.

const string AppSession = CookieAuthenticationDefaults.AuthenticationScheme;
const string ExternalSignIn = "External";
const string Provider = OpenIdConnectDefaults.AuthenticationScheme;

var builder = WebApplication.CreateBuilder(args);

var authway = builder.Configuration.GetSection("Authway");
var authority = authway["Authority"];
var clientId = authway["ClientId"];
var clientSecret = authway["ClientSecret"];
if (string.IsNullOrEmpty(authority) || string.IsNullOrEmpty(clientId) || string.IsNullOrEmpty(clientSecret))
{
    throw new InvalidOperationException(
        "Set Authway:Authority, Authway:ClientId and Authway:ClientSecret (the secret with `dotnet user-secrets`, " +
        "never in appsettings.json). See README.md.");
}

builder.Services.AddRazorPages();
builder.Services.AddSingleton<AppUsers>();

builder.Services
    .AddAuthentication(options =>
    {
        options.DefaultScheme = AppSession;
        options.DefaultChallengeScheme = Provider;
    })
    // The application's session. Its claims are the application's own (see
    // /signin-complete below), not the provider's.
    .AddCookie(AppSession, options =>
    {
        options.Cookie.Name = "authway-sample";
        options.ExpireTimeSpan = TimeSpan.FromHours(1);
        options.SlidingExpiration = true;
    })
    // Holds what the provider said for the moment between its callback and
    // /signin-complete. Nothing is authorized from this cookie.
    .AddCookie(ExternalSignIn, options =>
    {
        options.Cookie.Name = "authway-sample-external";
        options.ExpireTimeSpan = TimeSpan.FromMinutes(10);
    })
    .AddOpenIdConnect(Provider, options =>
    {
        // The issuer. Every endpoint comes from its discovery document.
        options.Authority = authority;
        options.ClientId = clientId;
        options.ClientSecret = clientSecret;
        options.RequireHttpsMetadata = !builder.Environment.IsDevelopment();

        // Authorization code flow with PKCE; state and nonce are checked (the defaults).
        options.ResponseType = OpenIdConnectResponseType.Code;
        options.ResponseMode = OpenIdConnectResponseMode.Query;

        options.Scope.Clear();
        options.Scope.Add("openid");
        options.Scope.Add("profile");
        options.Scope.Add("email");

        // The provider's answer goes to the temporary cookie, not the session.
        options.SignInScheme = ExternalSignIn;
        // Signing out reads the ID token back from the session (id_token_hint).
        options.SignOutScheme = AppSession;

        // Keep the provider's claim names (sub, email, name) as they are.
        options.MapInboundClaims = false;

        // Tokens are kept only until /signin-complete, which carries the ID
        // token alone into the session. An application that calls an API for
        // the user would keep the access and refresh tokens server-side.
        options.SaveTokens = true;

        options.CallbackPath = "/signin-oidc";
        options.SignedOutCallbackPath = "/signout-callback-oidc";
    });

builder.Services.AddAuthorization();

var app = builder.Build();

if (!app.Environment.IsDevelopment())
{
    app.UseExceptionHandler("/Error");
    app.UseHsts();
    app.UseHttpsRedirection();
}

app.UseStaticFiles();
app.UseRouting();
app.UseAuthentication();
app.UseAuthorization();

// Sends the browser to the provider. The provider's callback (/signin-oidc)
// is handled by the OpenID Connect middleware, which then comes here ↓.
app.MapGet("/login", (string? returnUrl) =>
    Results.Challenge(
        new AuthenticationProperties { RedirectUri = "/signin-complete?returnUrl=" + Uri.EscapeDataString(Local(returnUrl)) },
        [Provider]));

// Turns "the provider signed this person in" into the application's session:
// find the application's user by the provider's identifier for them, and sign
// in a principal built from the application's own record. No claim of the
// provider's becomes a claim of the session, so nothing the provider — or
// whoever administers it — says can grant anything here.
app.MapGet("/signin-complete", async (HttpContext context, AppUsers users, string? returnUrl) =>
{
    var external = await context.AuthenticateAsync(ExternalSignIn);
    var subject = external.Principal?.FindFirst("sub");
    if (!external.Succeeded || subject is null)
    {
        return Results.Redirect("/");
    }

    var user = users.FindOrCreate(
        issuer: subject.Issuer,
        subject: subject.Value,
        email: external.Principal!.FindFirstValue("email"),
        name: external.Principal!.FindFirstValue("name"));

    var identity = new ClaimsIdentity(AppSession, ClaimTypes.Name, ClaimTypes.Role);
    identity.AddClaim(new Claim(ClaimTypes.NameIdentifier, user.Id.ToString()));
    identity.AddClaim(new Claim(ClaimTypes.Name, user.DisplayName));
    identity.AddClaim(new Claim(ClaimTypes.Role, user.Role));

    var session = new AuthenticationProperties();
    if (external.Properties?.GetTokenValue(OpenIdConnectParameterNames.IdToken) is { } idToken)
    {
        session.StoreTokens([new AuthenticationToken { Name = OpenIdConnectParameterNames.IdToken, Value = idToken }]);
    }

    await context.SignInAsync(AppSession, new ClaimsPrincipal(identity), session);
    await context.SignOutAsync(ExternalSignIn);
    return Results.LocalRedirect(Local(returnUrl));
});

app.MapRazorPages();

app.Run();

// Only ever redirect within this application.
static string Local(string? url) =>
    !string.IsNullOrEmpty(url) && url.StartsWith('/') && !url.StartsWith("//") && !url.StartsWith("/\\") ? url : "/";
