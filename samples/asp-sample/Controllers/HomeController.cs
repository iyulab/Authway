using Microsoft.AspNetCore.Authentication;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;
using System.Diagnostics;
using AuthwaySample.Services;

namespace AuthwaySample.Controllers;

public class HomeController : Controller
{
    private readonly ILogger<HomeController> _logger;
    private readonly ClaimsService _claimsService;

    public HomeController(ILogger<HomeController> logger, ClaimsService claimsService)
    {
        _logger = logger;
        _claimsService = claimsService;
    }

    public IActionResult Index()
    {
        return View();
    }

    [Authorize]
    public async Task<IActionResult> Profile()
    {
        // Start with ID Token claims
        var claims = User.Claims.OrderBy(c => c.Type)
            .ToDictionary(c => c.Type, c => c.Value);

        // Fetch latest claims from DB and merge (DB claims take priority)
        try
        {
            var accessToken = await HttpContext.GetTokenAsync("access_token");
            if (!string.IsNullOrEmpty(accessToken))
            {
                var dbClaimsResult = await _claimsService.GetClaimsAsync(accessToken);
                if (dbClaimsResult?.Claims != null)
                {
                    // Merge DB claims into the claims dictionary (DB values override ID Token values)
                    foreach (var dbClaim in dbClaimsResult.Claims)
                    {
                        claims[dbClaim.Key] = dbClaim.Value?.ToString() ?? string.Empty;
                    }
                }
            }
        }
        catch (Exception ex)
        {
            _logger.LogWarning(ex, "Failed to fetch claims from DB, using ID Token claims only");
        }

        var viewModel = new ProfileViewModel
        {
            UserName = User.Identity?.Name,
            IsAuthenticated = User.Identity?.IsAuthenticated ?? false,
            AuthenticationType = User.Identity?.AuthenticationType,
            Claims = claims,
            AccessToken = await HttpContext.GetTokenAsync("access_token"),
            IdToken = await HttpContext.GetTokenAsync("id_token"),
            RefreshToken = await HttpContext.GetTokenAsync("refresh_token")
        };

        return View(viewModel);
    }

    public IActionResult Login(string returnUrl = "/", bool popup = false, string? origin = null)
    {
        var properties = new AuthenticationProperties();

        // Store popup mode in authentication properties for callback handling
        if (popup)
        {
            properties.Items["popup_mode"] = "true";

            // ⚠️ CRITICAL: Store origin for postMessage targetOrigin validation
            if (!string.IsNullOrEmpty(origin))
            {
                properties.Items["popup_origin"] = origin;
                properties.RedirectUri = $"/Home/PopupCallback?origin={Uri.EscapeDataString(origin)}";
                _logger.LogInformation("Setting popup redirect with origin: {Origin}", origin);
            }
            else
            {
                properties.RedirectUri = "/Home/PopupCallback";
                _logger.LogInformation("Setting popup redirect without origin");
            }
        }
        else
        {
            properties.RedirectUri = returnUrl;
        }

        return Challenge(properties, OpenIdConnectDefaults.AuthenticationScheme);
    }

    [Authorize]
    public IActionResult PopupCallback([FromQuery] string? origin = null)
    {
        _logger.LogInformation("Popup authentication completed successfully");

        // Default to the current request's origin (where the popup was opened from)
        var frontendUrl = origin ?? $"{Request.Scheme}://{Request.Host}";
        var configuration = HttpContext.RequestServices.GetRequiredService<IConfiguration>();

        // ⚠️ SECURITY: Validate origin is in allowed origins list
        var allowedOrigins = configuration.GetSection("Cors:AllowedOrigins").Get<string[]>() ??
            new[] { "http://localhost:5173", "http://localhost:5174", "http://localhost:3000", "http://localhost:5000" };

        if (!string.IsNullOrEmpty(origin) && !allowedOrigins.Contains(origin))
        {
            _logger.LogWarning("Popup origin {Origin} not in allowed origins list, using fallback", origin);
            frontendUrl = allowedOrigins[0];
        }
        else if (!string.IsNullOrEmpty(origin))
        {
            _logger.LogInformation("Using popup origin from parameter: {FrontendUrl}", frontendUrl);
        }
        else
        {
            // origin is null, using auto-detected value from line 77
            _logger.LogInformation("No origin parameter provided, auto-detected frontend URL: {FrontendUrl}", frontendUrl);
        }

        // Return HTML that sends postMessage to opener and closes the popup
        var html = $@"
<!DOCTYPE html>
<html>
<head>
    <title>Login Successful</title>
    <meta charset='utf-8'>
    <style>
        body {{
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', sans-serif;
            display: flex;
            align-items: center;
            justify-content: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
        }}
        .container {{
            text-align: center;
        }}
        .spinner {{
            width: 50px;
            height: 50px;
            border: 4px solid rgba(255, 255, 255, 0.3);
            border-top-color: white;
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin: 0 auto 1rem;
        }}
        @keyframes spin {{
            to {{ transform: rotate(360deg); }}
        }}
        h2 {{ margin: 0.5rem 0; }}
        p {{ margin: 0.5rem 0; opacity: 0.9; }}
    </style>
</head>
<body>
    <div class='container'>
        <div class='spinner'></div>
        <h2>✅ Login Successful!</h2>
        <p>Closing popup...</p>
    </div>
    <script>
        (function() {{
            try {{
                if (window.opener && !window.opener.closed) {{
                    // ⚠️ CRITICAL: Send success message to parent window with validated origin
                    window.opener.postMessage({{
                        type: 'authway-callback',
                        success: true,
                        timestamp: new Date().toISOString()
                    }}, '{frontendUrl}');

                    // Close popup after a short delay
                    setTimeout(() => {{
                        window.close();
                    }}, 1000);
                }} else {{
                    // If no opener, redirect to frontend app
                    console.log('No opener window found, redirecting to frontend');
                    window.location.href = '{frontendUrl}';
                }}
            }} catch (error) {{
                console.error('Popup callback error:', error);
                // Fallback: redirect to frontend
                window.location.href = '{frontendUrl}';
            }}
        }})();
    </script>
</body>
</html>";

        return Content(html, "text/html");
    }

    [Authorize]
    public async Task<IActionResult> Logout(bool useOidcFlow = false)
    {
        if (useOidcFlow)
        {
            // Option 2: Traditional OIDC logout flow
            // Sign out from cookie authentication
            await HttpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);

            // Sign out from OpenID Connect with redirect to app root
            // Use bare origin URL (without path) for post_logout_redirect_uri
            var oidcRedirectUri = $"{Request.Scheme}://{Request.Host}";
            _logger.LogInformation("OIDC logout flow with redirect URI: {RedirectUri}", oidcRedirectUri);

            var properties = new AuthenticationProperties
            {
                RedirectUri = oidcRedirectUri
            };

            return SignOut(properties, OpenIdConnectDefaults.AuthenticationScheme);
        }

        // Option 1: Direct logout via Authway API (recommended)
        // This immediately revokes all sessions without redirects
        try
        {
            // Direct logout now requires the caller's own bearer ACCESS token.
            // The server derives the subject to revoke from the validated token,
            // so the request body no longer carries id_token/subject.
            var accessToken = await HttpContext.GetTokenAsync("access_token");
            if (!string.IsNullOrEmpty(accessToken))
            {
                var authwayConfigService = HttpContext.RequestServices.GetRequiredService<AuthwayConfigService>();
                var apiServer = await authwayConfigService.GetApiServerAsync();
                var httpClient = HttpContext.RequestServices.GetRequiredService<IHttpClientFactory>().CreateClient();
                httpClient.DefaultRequestHeaders.Authorization =
                    new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", accessToken);

                // Use bare origin URL (without path) for post_logout_redirect_uri
                var postLogoutUri = $"{Request.Scheme}://{Request.Host}";
                var logoutRequest = new
                {
                    post_logout_redirect_uri = postLogoutUri
                };

                var response = await httpClient.PostAsJsonAsync($"{apiServer}/api/v1/logout", logoutRequest);

                if (response.IsSuccessStatusCode)
                {
                    var result = await response.Content.ReadFromJsonAsync<LogoutApiResponse>();

                    // Sign out from local cookie authentication
                    await HttpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);

                    _logger.LogInformation("Logout successful via Authway API");

                    // Redirect to home or specified redirect URL
                    if (!string.IsNullOrEmpty(result?.RedirectURL))
                    {
                        return Redirect(result.RedirectURL);
                    }
                    return RedirectToAction("Index", "Home");
                }
                else
                {
                    var errorContent = await response.Content.ReadAsStringAsync();
                    _logger.LogWarning("Authway API logout failed: {Error}. Falling back to OIDC flow.", errorContent);
                }
            }
        }
        catch (Exception ex)
        {
            _logger.LogWarning(ex, "Direct logout failed, falling back to OIDC flow");
        }

        // Fallback to OIDC flow if direct logout fails
        await HttpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);

        // Use bare origin URL (without path) for post_logout_redirect_uri
        // This must match exactly what's registered in Hydra client configuration
        var redirectUri = $"{Request.Scheme}://{Request.Host}";
        _logger.LogInformation("Logout fallback with redirect URI: {RedirectUri}", redirectUri);

        var fallbackProperties = new AuthenticationProperties
        {
            RedirectUri = redirectUri
        };
        return SignOut(fallbackProperties, OpenIdConnectDefaults.AuthenticationScheme);
    }

    private class LogoutApiResponse
    {
        public bool Success { get; set; }
        public string? Message { get; set; }
        public string? RedirectURL { get; set; }
    }

    [Authorize]
    [HttpGet]
    public async Task<IActionResult> GetUserClaims()
    {
        try
        {
            var accessToken = await HttpContext.GetTokenAsync("access_token");
            if (string.IsNullOrEmpty(accessToken))
            {
                _logger.LogWarning("No access token found");
                return Json(new { error = "No access token found" });
            }

            var result = await _claimsService.GetClaimsAsync(accessToken);

            return Json(new {
                success = true,
                claims = result.Claims
            });
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to get user claims");
            return Json(new { error = ex.Message });
        }
    }

    [Authorize]
    [HttpPost]
    public async Task<IActionResult> UpdateCustomClaims([FromBody] UpdateCustomClaimsRequest request)
    {
        try
        {
            var accessToken = await HttpContext.GetTokenAsync("access_token");
            if (string.IsNullOrEmpty(accessToken))
            {
                _logger.LogWarning("No access token found");
                return Json(new { error = "No access token found" });
            }

            var result = await _claimsService.UpdateClaimsAsync(accessToken, request.Claims, request.Permanent);

            return Json(new {
                success = true,
                message = result.Message
            });
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to update custom claims");
            return Json(new { error = ex.Message });
        }
    }

    public IActionResult SilentReauthInit()
    {
        // Start silent re-authentication in iframe
        // DON'T clear cookie - we want to reuse existing session
        // This way Hydra will return skip=true for consent

        var properties = new AuthenticationProperties
        {
            RedirectUri = Url.Action("SilentReauthCallback", "Home"),
            Items =
            {
                { "silent_reauth", "true" },
                { "prompt", "none" }  // Request silent authentication
            }
        };

        return Challenge(properties, OpenIdConnectDefaults.AuthenticationScheme);
    }

    [Authorize]
    public IActionResult SilentReauthCallback()
    {
        // This is called after successful re-authentication in iframe
        // Return HTML that notifies parent window
        var html = @"
<!DOCTYPE html>
<html>
<head>
    <title>Re-authentication Complete</title>
</head>
<body>
    <script>
        (function() {
            if (window.parent && window.parent !== window) {
                // Notify parent window that re-authentication is complete
                window.parent.postMessage({
                    type: 'silent-reauth-complete',
                    success: true
                }, window.location.origin);
            }
        })();
    </script>
</body>
</html>";
        return Content(html, "text/html");
    }

    [Authorize]
    [HttpPost]
    public async Task<IActionResult> SilentTokenRefresh()
    {
        try
        {
            var refreshToken = await HttpContext.GetTokenAsync("refresh_token");
            if (string.IsNullOrEmpty(refreshToken))
            {
                _logger.LogWarning("No refresh token found");
                return Json(new { error = "No refresh token found", requireReauth = true });
            }

            var clientId = HttpContext.RequestServices.GetRequiredService<IConfiguration>()["Authway:ClientId"];
            var clientSecret = HttpContext.RequestServices.GetRequiredService<IConfiguration>()["Authway:ClientSecret"];
            var tokenEndpoint = HttpContext.RequestServices.GetRequiredService<IConfiguration>()["Authway:TokenEndpoint"];

            // Get authority for token endpoint
            var authwayConfigService = HttpContext.RequestServices.GetRequiredService<AuthwayConfigService>();
            var authority = await authwayConfigService.GetAuthorityAsync();
            var actualTokenEndpoint = $"{authority}/oauth2/token";

            // Exchange refresh token for new access token
            var httpClient = HttpContext.RequestServices.GetRequiredService<IHttpClientFactory>().CreateClient();
            var tokenRequest = new Dictionary<string, string>
            {
                { "grant_type", "refresh_token" },
                { "refresh_token", refreshToken },
                { "client_id", clientId! },
                { "client_secret", clientSecret! }
            };

            var response = await httpClient.PostAsync(actualTokenEndpoint, new FormUrlEncodedContent(tokenRequest));
            if (!response.IsSuccessStatusCode)
            {
                var errorContent = await response.Content.ReadAsStringAsync();
                _logger.LogError("Token refresh failed: {Error}", errorContent);
                return Json(new { error = "Token refresh failed", requireReauth = true });
            }

            var tokenResponse = await response.Content.ReadFromJsonAsync<TokenResponse>();
            if (tokenResponse == null || string.IsNullOrEmpty(tokenResponse.AccessToken))
            {
                return Json(new { error = "Invalid token response", requireReauth = true });
            }

            // Update stored tokens
            var authResult = await HttpContext.AuthenticateAsync(CookieAuthenticationDefaults.AuthenticationScheme);
            if (authResult.Principal == null)
            {
                return Json(new { error = "Authentication principal not found", requireReauth = true });
            }

            var tokens = new List<AuthenticationToken>
            {
                new AuthenticationToken { Name = "access_token", Value = tokenResponse.AccessToken },
                new AuthenticationToken { Name = "token_type", Value = tokenResponse.TokenType ?? "Bearer" },
                new AuthenticationToken { Name = "expires_at", Value = DateTime.UtcNow.AddSeconds(tokenResponse.ExpiresIn).ToString("o") }
            };

            if (!string.IsNullOrEmpty(tokenResponse.RefreshToken))
            {
                tokens.Add(new AuthenticationToken { Name = "refresh_token", Value = tokenResponse.RefreshToken });
            }
            else
            {
                // Keep existing refresh token if new one not provided
                var existingRefreshToken = await HttpContext.GetTokenAsync("refresh_token");
                if (!string.IsNullOrEmpty(existingRefreshToken))
                {
                    tokens.Add(new AuthenticationToken { Name = "refresh_token", Value = existingRefreshToken });
                }
            }

            if (!string.IsNullOrEmpty(tokenResponse.IdToken))
            {
                tokens.Add(new AuthenticationToken { Name = "id_token", Value = tokenResponse.IdToken });
            }

            authResult.Properties!.StoreTokens(tokens);
            await HttpContext.SignInAsync(CookieAuthenticationDefaults.AuthenticationScheme, authResult.Principal, authResult.Properties);

            _logger.LogInformation("Token refreshed successfully");
            return Json(new { success = true, message = "Token refreshed successfully" });
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to refresh token");
            return Json(new { error = ex.Message, requireReauth = true });
        }
    }

    [Authorize]
    [HttpPost]
    public async Task<IActionResult> RefreshToken()
    {
        try
        {
            // Trigger a silent re-authentication to refresh tokens
            // This will use the refresh token automatically
            var properties = new AuthenticationProperties
            {
                RedirectUri = Url.Action("Profile", "Home")
            };

            // Sign out and sign in again to refresh tokens
            await HttpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);

            return Challenge(properties, OpenIdConnectDefaults.AuthenticationScheme);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to refresh token");
            return Json(new { error = ex.Message });
        }
    }

    [Authorize]
    [HttpPost]
    public async Task<IActionResult> SwitchWorkspace([FromForm] string workspaceId, [FromForm] string workspaceName)
    {
        try
        {
            var accessToken = await HttpContext.GetTokenAsync("access_token");
            if (string.IsNullOrEmpty(accessToken))
            {
                _logger.LogWarning("No access token found");
                return Json(new { error = "No access token found" });
            }

            // Debug: Log token information
            _logger.LogInformation("Access Token (first 50 chars): {TokenPrefix}",
                accessToken.Length > 50 ? accessToken.Substring(0, 50) + "..." : accessToken);
            _logger.LogInformation("Access Token Length: {TokenLength}", accessToken.Length);

            // Decode JWT to see claims (for debugging)
            try
            {
                var parts = accessToken.Split('.');
                if (parts.Length == 3)
                {
                    _logger.LogInformation("Token is JWT format");
                    // Decode payload (base64url)
                    var payload = parts[1];
                    // Add padding if needed
                    switch (payload.Length % 4)
                    {
                        case 2: payload += "=="; break;
                        case 3: payload += "="; break;
                    }
                    var bytes = Convert.FromBase64String(payload.Replace('-', '+').Replace('_', '/'));
                    var json = System.Text.Encoding.UTF8.GetString(bytes);
                    _logger.LogInformation("Token Claims: {Claims}", json);
                }
                else
                {
                    _logger.LogWarning("Token is NOT JWT format (opaque token?) - parts count: {PartsCount}", parts.Length);
                }
            }
            catch (Exception decodeEx)
            {
                _logger.LogWarning(decodeEx, "Failed to decode token");
            }

            var claims = new Dictionary<string, object>
            {
                ["workspace_id"] = workspaceId,
                ["workspace_name"] = workspaceName
            };

            var result = await _claimsService.UpdateClaimsAsync(accessToken, claims, permanent: false);

            // Return success - client will redirect to ReAuthenticate action
            return Json(new { success = true });
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to switch workspace");
            return Json(new { error = ex.Message });
        }
    }

    [Authorize]
    public async Task<IActionResult> GetClaims()
    {
        try
        {
            var accessToken = await HttpContext.GetTokenAsync("access_token");
            if (string.IsNullOrEmpty(accessToken))
            {
                return Json(new { error = "No access token found" });
            }

            var claims = await _claimsService.GetClaimsAsync(accessToken);
            return Json(claims);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to get claims");
            return Json(new { error = ex.Message });
        }
    }

    [Authorize]
    public IActionResult ReAuthenticate()
    {
        // Trigger fresh authentication without signing out
        // This preserves the session so Hydra skips consent
        var properties = new AuthenticationProperties
        {
            RedirectUri = "/Home/Profile"
        };

        // Mark this as a re-authentication request for workspace switching
        // This flag will be checked in OnRedirectToIdentityProvider event
        properties.Items["is_reauthentication"] = "true";

        return Challenge(properties, "OpenIdConnect");
    }

    [ResponseCache(Duration = 0, Location = ResponseCacheLocation.None, NoStore = true)]
    public IActionResult Error()
    {
        return View(new ErrorViewModel { RequestId = Activity.Current?.Id ?? HttpContext.TraceIdentifier });
    }
}

public class UpdateCustomClaimsRequest
{
    public Dictionary<string, object> Claims { get; set; } = new();
    public bool Permanent { get; set; }
}

public class TokenResponse
{
    [System.Text.Json.Serialization.JsonPropertyName("access_token")]
    public string AccessToken { get; set; } = string.Empty;

    [System.Text.Json.Serialization.JsonPropertyName("token_type")]
    public string? TokenType { get; set; }

    [System.Text.Json.Serialization.JsonPropertyName("expires_in")]
    public int ExpiresIn { get; set; }

    [System.Text.Json.Serialization.JsonPropertyName("refresh_token")]
    public string? RefreshToken { get; set; }

    [System.Text.Json.Serialization.JsonPropertyName("id_token")]
    public string? IdToken { get; set; }
}

public class ErrorViewModel
{
    public string? RequestId { get; set; }
    public bool ShowRequestId => !string.IsNullOrEmpty(RequestId);
}
