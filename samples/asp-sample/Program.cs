using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.IdentityModel.Protocols.OpenIdConnect;
using AuthwaySample.Services;

var builder = WebApplication.CreateBuilder(args);

// Configure settings: In Release mode, only load appsettings.json
if (builder.Environment.IsProduction() || !builder.Environment.IsDevelopment())
{
    Console.WriteLine($"🔧 Configuration: Environment={builder.Environment.EnvironmentName}");
    Console.WriteLine($"📄 Loading only appsettings.json (Development settings excluded)");

    builder.Configuration.Sources.Clear();
    builder.Configuration
        .AddJsonFile("appsettings.json", optional: false, reloadOnChange: true)
        .AddEnvironmentVariables()
        .AddCommandLine(args);
}
else
{
    Console.WriteLine($"🔧 Configuration: Environment={builder.Environment.EnvironmentName}");
    Console.WriteLine($"📄 Loading appsettings.json + appsettings.Development.json");
}

// Add services to the container.
builder.Services.AddControllersWithViews();

// Add HttpClient for services
builder.Services.AddHttpClient<ClaimsService>();
builder.Services.AddHttpClient<AuthwayConfigService>();

// Discover Authway configuration from server
// This fetches the Hydra URL (OIDC Authority) automatically
var authwayConfigService = new AuthwayConfigService(
    new HttpClient(),
    builder.Configuration,
    LoggerFactory.Create(b => b.AddConsole()).CreateLogger<AuthwayConfigService>());

string authorityUrl;
try
{
    authorityUrl = authwayConfigService.GetAuthorityAsync().GetAwaiter().GetResult();
    Console.WriteLine($"✅ Successfully discovered OIDC Authority: {authorityUrl}");
}
catch (Exception ex)
{
    Console.WriteLine($"⚠️ Could not fetch Authway configuration from server: {ex.Message}");
    Console.WriteLine($"💡 Falling back to manual configuration (Authway:Domain)...");

    // Fallback: use Domain if Server discovery fails (backward compatibility)
    authorityUrl = builder.Configuration["Authway:Domain"]
                   ?? builder.Configuration["Authway:Authway"]
                   ?? throw new InvalidOperationException(
                       "Could not determine OIDC Authority URL. " +
                       "Please set either Authway:Server (recommended) or Authway:Domain in appsettings.json");

    Console.WriteLine($"✅ Using manual configuration: {authorityUrl}");
}

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
    // OIDC Authority - automatically discovered from Authway Server
    // Server -> /api/v1/config -> issuer URL -> /.well-known/openid-configuration
    options.Authority = authorityUrl;
    options.ClientId = builder.Configuration["Authway:ClientId"];
    options.ClientSecret = builder.Configuration["Authway:ClientSecret"];

    options.ResponseType = OpenIdConnectResponseType.Code;
    options.UsePkce = true;

    // Scopes
    options.Scope.Clear();
    options.Scope.Add("openid");
    options.Scope.Add("profile");
    options.Scope.Add("email");
    options.Scope.Add("offline_access"); // Enable refresh token

    // Save tokens
    options.SaveTokens = true;

    // Get user info
    options.GetClaimsFromUserInfoEndpoint = true;

    // Callback paths
    options.CallbackPath = "/signin-oidc";
    options.SignedOutCallbackPath = "/signout-callback-oidc";

    // Configure metadata
    // Use HTTPS in production, allow HTTP in development for local testing
    options.RequireHttpsMetadata = !builder.Environment.IsDevelopment();

    // Disable prompt=none for all authentication requests
    // This prevents "consent_required" errors during re-authentication
    options.ProtocolValidator.RequireNonce = false;

    // Events for debugging and controlling authentication behavior
    options.Events = new OpenIdConnectEvents
    {
        OnRedirectToIdentityProvider = context =>
        {
            // Check if this is a silent re-authentication request
            var isSilentReauth = context.Properties.Items.ContainsKey("silent_reauth");

            if (isSilentReauth)
            {
                // For silent reauth, use prompt=none to avoid user interaction
                context.ProtocolMessage.Prompt = "none";
            }
            else
            {
                // For normal auth, remove prompt to avoid consent_required errors
                // Hydra's skip_consent=true will handle skipping the consent page
                context.ProtocolMessage.Prompt = null;
            }

            context.ProtocolMessage.MaxAge = null;

            return Task.CompletedTask;
        },
        OnRedirectToIdentityProviderForSignOut = context =>
        {
            var logger = context.HttpContext.RequestServices.GetRequiredService<ILogger<Program>>();

            // Set post_logout_redirect_uri from AuthenticationProperties.RedirectUri
            // This follows OIDC RP-Initiated Logout standard
            var redirectUri = context.Properties?.RedirectUri;
            if (!string.IsNullOrEmpty(redirectUri))
            {
                context.ProtocolMessage.PostLogoutRedirectUri = redirectUri;
                logger.LogInformation("Logout with post_logout_redirect_uri: {Uri}", redirectUri);
            }
            else
            {
                // Fallback to app root - use bare origin (no path)
                // This must match exactly what's registered in Hydra client post_logout_redirect_uris
                var request = context.HttpContext.Request;
                var defaultUri = $"{request.Scheme}://{request.Host}";
                context.ProtocolMessage.PostLogoutRedirectUri = defaultUri;
                logger.LogInformation("Logout with default redirect to app root: {Uri}", defaultUri);
            }

            logger.LogInformation("Logout URL: {LogoutUrl}", context.ProtocolMessage.CreateLogoutRequestUrl());

            return Task.CompletedTask;
        },
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

            // Check if this is a consent_required error from prompt=none
            if (context.Failure?.Message?.Contains("consent_required") == true)
            {
                logger.LogWarning("Consent required during silent re-authentication, falling back to interactive flow");

                // Redirect to ReAuthenticate without the reauthentication flag
                // This will trigger full authentication flow with consent
                context.Response.Redirect("/Home/Login?returnUrl=/Home/Profile");
                context.HandleResponse();
                return Task.CompletedTask;
            }

            // Check if this is a login_required error from prompt=none
            // This happens when the Hydra session cookie is missing or expired
            if (context.Failure?.Message?.Contains("login_required") == true)
            {
                // Check if this is a silent reauth attempt in iframe
                var isSilentReauth = context.Properties?.Items.ContainsKey("silent_reauth") == true;

                if (isSilentReauth)
                {
                    logger.LogWarning("Silent re-authentication failed - session expired");

                    // For iframe silent reauth, return HTML that notifies parent of failure
                    // Don't redirect to full login flow in iframe
                    context.Response.ContentType = "text/html";
                    context.Response.WriteAsync(@"
<!DOCTYPE html>
<html>
<head><title>Silent Authentication Failed</title></head>
<body>
    <script>
        if (window.parent && window.parent !== window) {
            window.parent.postMessage({
                type: 'authway.reauth.error',
                error: 'login_required',
                message: 'Session expired, please login again'
            }, '*');
        }
    </script>
</body>
</html>").Wait();
                    context.HandleResponse();
                    return Task.CompletedTask;
                }

                logger.LogWarning("Login required during authentication, redirecting to login");

                // For non-iframe requests, redirect to login page
                context.Response.Redirect("/Home/Login?returnUrl=/Home/Profile");
                context.HandleResponse();
                return Task.CompletedTask;
            }

            // Check if this is a logout-related error
            var isLogoutRequest = context.Request.Path.StartsWithSegments("/signout-oidc") ||
                                  context.Request.Path.StartsWithSegments("/signout-callback-oidc");

            if (isLogoutRequest)
            {
                // For logout errors, just log and redirect to home
                // Don't show error page - logout issues shouldn't block user
                logger.LogWarning(context.Failure, "Logout error occurred - redirecting to home");
                context.Response.Redirect("/");
                context.HandleResponse();
                return Task.CompletedTask;
            }

            // For authentication errors, show error page
            logger.LogError(context.Failure, "Remote authentication failure");
            context.Response.Redirect("/Home/Error");
            context.HandleResponse();
            return Task.CompletedTask;
        },
        OnTicketReceived = context =>
        {
            var logger = context.HttpContext.RequestServices.GetRequiredService<ILogger<Program>>();

            // 🔍 DEBUG: Log all authentication properties
            logger.LogInformation("OnTicketReceived: Checking authentication properties");
            if (context.Properties?.Items != null)
            {
                foreach (var item in context.Properties.Items)
                {
                    logger.LogInformation("Property: {Key} = {Value}", item.Key, item.Value);
                }
            }

            // ⚠️ CRITICAL: Check if this is a popup callback
            var isPopup = context.Properties?.Items.ContainsKey("popup_mode") == true;
            logger.LogInformation("OnTicketReceived: isPopup = {IsPopup}", isPopup);

            if (isPopup && context.Properties != null)
            {
                // Get stored origin from authentication properties
                var popupOrigin = context.Properties.Items.TryGetValue("popup_origin", out var origin) == true
                    ? origin
                    : null;

                // Set RedirectUri in Properties instead of redirecting directly
                // This ensures cookie authentication is completed BEFORE redirect
                var redirectUrl = !string.IsNullOrEmpty(popupOrigin)
                    ? $"/Home/PopupCallback?origin={Uri.EscapeDataString(popupOrigin)}"
                    : "/Home/PopupCallback";

                logger.LogInformation("OnTicketReceived: Setting RedirectUri to {RedirectUrl}", redirectUrl);
                context.Properties.RedirectUri = redirectUrl;
            }
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
