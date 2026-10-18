namespace Authway.Native;

/// <summary>
/// What the client needs to know about the authorization server and the app.
/// </summary>
/// <remarks>
/// Register the app at the authorization server as a public client with the redirect URI
/// <c>http://127.0.0.1/callback</c> — without a port; the server accepts it on any port (RFC 8252 §7.3) — and the
/// <c>authorization_code</c> and <c>refresh_token</c> grants.
/// </remarks>
public sealed record NativeClientOptions
{
    /// <param name="issuer">The authorization server's issuer URL.</param>
    /// <param name="clientId">The app's client id.</param>
    public NativeClientOptions(string issuer, string clientId)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(issuer);
        ArgumentException.ThrowIfNullOrWhiteSpace(clientId);
        Issuer = issuer.TrimEnd('/');
        ClientId = clientId;
        StoreKey = $"authway.{clientId}";
    }

    public string Issuer { get; }

    public string ClientId { get; }

    /// <summary>
    /// The scopes requested. <c>offline_access</c> is what brings a refresh token; without it the session ends when the
    /// first access token expires.
    /// </summary>
    public IReadOnlyList<string> Scopes { get; init; } = ["openid", "profile", "email", "offline_access"];

    /// <summary>
    /// The APIs the access token is for, sent as the <c>audience</c> parameter. Usually unnecessary: a client registered
    /// with an audience gets it in its tokens without asking.
    /// </summary>
    public IReadOnlyList<string> Audience { get; init; } = [];

    /// <summary>The path of the registered loopback redirect URI.</summary>
    public string RedirectPath
    {
        get;
        init => field = value.StartsWith('/') ? value : "/" + value;
    } = "/callback";

    /// <summary>The key the session is stored under. Default <c>authway.&lt;client id&gt;</c>.</summary>
    public string StoreKey { get; init; }

    /// <summary>The HTML the browser shows when it comes back from a sign-in.</summary>
    public string SuccessPage { get; init; } =
        "<!doctype html><html><head><meta charset=\"utf-8\"><title>Signed in</title></head>" +
        "<body style=\"font-family:sans-serif;text-align:center;padding:3rem\"><h2>You are signed in</h2>" +
        "<p>You can close this tab and return to the app.</p></body></html>";

    /// <summary>The HTML the browser shows when the sign-in failed or was declined.</summary>
    public string FailurePage { get; init; } =
        "<!doctype html><html><head><meta charset=\"utf-8\"><title>Not signed in</title></head>" +
        "<body style=\"font-family:sans-serif;text-align:center;padding:3rem\"><h2>Sign-in did not complete</h2>" +
        "<p>Return to the app and try again.</p></body></html>";

    /// <summary>How long before the access token expires it is refreshed.</summary>
    public TimeSpan RefreshAhead { get; init; } = TimeSpan.FromSeconds(60);
}
