using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.IdentityModel.Protocols.OpenIdConnect;

var builder = WebApplication.CreateBuilder(args);

// Add services to the container.
builder.Services.AddControllersWithViews();

// Configure Authentication
builder.Services.AddAuthentication(options =>
{
    options.DefaultScheme = CookieAuthenticationDefaults.AuthenticationScheme;
    options.DefaultChallengeScheme = OpenIdConnectDefaults.AuthenticationScheme;
})
.AddCookie(options =>
{
    options.Cookie.Name = "authway-sample";
    options.ExpireTimeSpan = TimeSpan.FromHours(1);
    options.SlidingExpiration = true;
})
.AddOpenIdConnect(options =>
{
    // Ory Hydra configuration (Hydra is the OAuth2/OIDC server, Authway is the login provider)
    options.Authority = builder.Configuration["Authway:HydraPublicUrl"];
    options.ClientId = builder.Configuration["Authway:ClientId"];
    options.ClientSecret = builder.Configuration["Authway:ClientSecret"];

    options.ResponseType = OpenIdConnectResponseType.Code;
    options.UsePkce = true;

    // Scopes
    options.Scope.Clear();
    options.Scope.Add("openid");
    options.Scope.Add("profile");
    options.Scope.Add("email");

    // Save tokens
    options.SaveTokens = true;

    // Get user info
    options.GetClaimsFromUserInfoEndpoint = true;

    // Callback paths
    options.CallbackPath = "/signin-oidc";
    options.SignedOutCallbackPath = "/signout-callback-oidc";

    // Configure metadata
    options.RequireHttpsMetadata = true; // Azure Hydra uses HTTPS

    // Events for debugging
    options.Events = new OpenIdConnectEvents
    {
        OnAuthenticationFailed = context =>
        {
            var logger = context.HttpContext.RequestServices.GetRequiredService<ILogger<Program>>();
            logger.LogError(context.Exception, "Authentication failed");
            context.Response.Redirect("/Home/Error");
            context.HandleResponse();
            return Task.CompletedTask;
        },
        OnRemoteFailure = context =>
        {
            var logger = context.HttpContext.RequestServices.GetRequiredService<ILogger<Program>>();
            logger.LogError(context.Failure, "Remote authentication failure");
            context.Response.Redirect("/Home/Error");
            context.HandleResponse();
            return Task.CompletedTask;
        }
    };
});

var app = builder.Build();

// Configure the HTTP request pipeline.
if (!app.Environment.IsDevelopment())
{
    app.UseExceptionHandler("/Home/Error");
    app.UseHsts();
}

app.UseHttpsRedirection();
app.UseStaticFiles();

app.UseRouting();

app.UseAuthentication();
app.UseAuthorization();

app.MapControllerRoute(
    name: "default",
    pattern: "{controller=Home}/{action=Index}/{id?}");

app.Run();
