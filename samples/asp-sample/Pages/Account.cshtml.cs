using System.Security.Claims;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;
using Microsoft.AspNetCore.Mvc.RazorPages;

namespace AuthwaySample.Pages;

[Authorize]
public class AccountModel : PageModel
{
    public string? UserId => User.FindFirstValue(ClaimTypes.NameIdentifier);
    public string? Role => User.FindFirstValue(ClaimTypes.Role);

    public void OnGet()
    {
    }

    // Ends this application's session, then the provider's (RP-initiated
    // logout): the provider brings the browser back to "/", which has to be
    // one of the client's post_logout_redirect_uris.
    public IActionResult OnPostLogout() =>
        SignOut(
            new Microsoft.AspNetCore.Authentication.AuthenticationProperties { RedirectUri = "/" },
            CookieAuthenticationDefaults.AuthenticationScheme,
            OpenIdConnectDefaults.AuthenticationScheme);
}
