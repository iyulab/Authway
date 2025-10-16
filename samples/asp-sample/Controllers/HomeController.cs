using Microsoft.AspNetCore.Authentication;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;
using System.Diagnostics;

namespace AuthwaySample.Controllers;

public class HomeController : Controller
{
    private readonly ILogger<HomeController> _logger;

    public HomeController(ILogger<HomeController> logger)
    {
        _logger = logger;
    }

    public IActionResult Index()
    {
        return View();
    }

    [Authorize]
    public async Task<IActionResult> Profile()
    {
        var viewModel = new ProfileViewModel
        {
            UserName = User.Identity?.Name,
            IsAuthenticated = User.Identity?.IsAuthenticated ?? false,
            AuthenticationType = User.Identity?.AuthenticationType,
            Claims = User.Claims.OrderBy(c => c.Type)
                .ToDictionary(c => c.Type, c => c.Value),
            AccessToken = await HttpContext.GetTokenAsync("access_token"),
            IdToken = await HttpContext.GetTokenAsync("id_token"),
            RefreshToken = await HttpContext.GetTokenAsync("refresh_token")
        };

        return View(viewModel);
    }

    public IActionResult Login(string returnUrl = "/")
    {
        var properties = new AuthenticationProperties
        {
            RedirectUri = returnUrl
        };

        return Challenge(properties, OpenIdConnectDefaults.AuthenticationScheme);
    }

    [Authorize]
    public async Task<IActionResult> Logout()
    {
        // Sign out from cookie authentication
        await HttpContext.SignOutAsync(CookieAuthenticationDefaults.AuthenticationScheme);

        // Sign out from OpenID Connect
        var properties = new AuthenticationProperties
        {
            RedirectUri = Url.Action("Index", "Home")
        };

        return SignOut(properties, OpenIdConnectDefaults.AuthenticationScheme);
    }

    [ResponseCache(Duration = 0, Location = ResponseCacheLocation.None, NoStore = true)]
    public IActionResult Error()
    {
        return View(new ErrorViewModel { RequestId = Activity.Current?.Id ?? HttpContext.TraceIdentifier });
    }
}

public class ErrorViewModel
{
    public string? RequestId { get; set; }
    public bool ShowRequestId => !string.IsNullOrEmpty(RequestId);
}
