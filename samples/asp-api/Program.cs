using Microsoft.AspNetCore.Authentication.JwtBearer;

// An API that accepts Authway access tokens — a resource server.
//
// It knows two things about the provider: its issuer, and the name the API
// goes by there (its audience). Everything else — the signing keys, the
// algorithms — comes from the issuer's discovery document.

var builder = WebApplication.CreateBuilder(args);

var issuer = builder.Configuration["Authway:Issuer"]
    ?? throw new InvalidOperationException("Authway:Issuer is required: the OpenID Connect issuer of the Authway deployment.");
var audience = builder.Configuration["Authway:Audience"]
    ?? throw new InvalidOperationException("Authway:Audience is required: the name this API is registered under as a client's audience.");

builder.Services.AddAuthentication(JwtBearerDefaults.AuthenticationScheme)
    .AddJwtBearer(options =>
    {
        options.Authority = issuer;

        // The token must have been issued for this API. Without this check the
        // API accepts any token of the issuer — one issued to another
        // application, for another API, included.
        options.Audience = audience;

        // Signature, issuer, audience and lifetime are all validated by
        // default. None of them is turned off here.

        // Keep the claim names the token has ("sub", "scope") instead of the
        // long names older .NET code expects.
        options.MapInboundClaims = false;

        // Plain HTTP is for a provider on this machine only.
        options.RequireHttpsMetadata = !builder.Environment.IsDevelopment();
    });

builder.Services.AddAuthorization(options =>
{
    // What a token may do here is what it was granted: the space-separated
    // "scope" claim. A token of the right audience is not thereby allowed
    // everything.
    options.AddPolicy("email", policy => policy.RequireAssertion(context =>
        (context.User.FindFirst("scope")?.Value ?? "").Split(' ').Contains("email")));
});

// A browser application on another origin calls this API with the token in a
// header, so its origin has to be allowed. No cookies are involved.
var origins = builder.Configuration.GetSection("Cors:AllowedOrigins").Get<string[]>() ?? [];
builder.Services.AddCors(options => options.AddDefaultPolicy(policy =>
    policy.WithOrigins(origins).WithHeaders("Authorization", "Content-Type").WithMethods("GET")));

var app = builder.Build();

app.UseCors();
app.UseAuthentication();
app.UseAuthorization();

app.MapGet("/api/public", () => new { message = "Anyone may read this." });

// Who the token is about. "sub" identifies the user at this issuer; it is the
// value to key your own records on.
app.MapGet("/api/me", (HttpContext http) => new
{
    sub = http.User.FindFirst("sub")?.Value,
    tenant_id = http.User.FindFirst("tenant_id")?.Value,
    scope = http.User.FindFirst("scope")?.Value,
    client_id = http.User.FindFirst("client_id")?.Value,
}).RequireAuthorization();

// Needs the "email" scope on top of a valid token.
app.MapGet("/api/email", (HttpContext http) => new
{
    email = http.User.FindFirst("email")?.Value,
}).RequireAuthorization("email");

app.Run();
