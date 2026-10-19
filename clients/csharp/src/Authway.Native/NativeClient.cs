using System.Net;
using System.Net.Http.Headers;
using System.Net.Sockets;
using System.Text;
using System.Text.Json;

namespace Authway.Native;

/// <summary>Who is signed in.</summary>
/// <param name="Subject">The user's identifier at the authorization server (<c>sub</c>).</param>
/// <param name="Claims">
/// The ID token's claims — <c>email</c>, <c>name</c> and the others the scopes asked for. For showing who is signed in;
/// an API trusts only the access token.
/// </param>
/// <param name="AccessTokenExpiresAt">
/// When the current access token expires; <see cref="NativeClient.GetAccessTokenAsync"/> refreshes it before. The Unix
/// epoch when this process holds none yet — the access token is not stored, so the first call after a start refreshes.
/// </param>
public sealed record NativeSession(
    string Subject,
    IReadOnlyDictionary<string, JsonElement> Claims,
    DateTimeOffset AccessTokenExpiresAt);

/// <summary>What <see cref="NativeClient.SignOutAsync"/> did.</summary>
/// <param name="WasSignedIn">Whether a session was stored. It is removed either way.</param>
/// <param name="Revoked">
/// Whether the authorization server confirmed the tokens are revoked. <see langword="false"/> when it could not be
/// reached or names no revocation endpoint: the session is gone from this device, but its refresh token stays valid at
/// the server until it expires.
/// </param>
public sealed record SignedOut(bool WasSignedIn, bool Revoked);

/// <summary>
/// Signs the user in with the system browser (OAuth 2.0 for Native Apps, RFC 8252: a loopback port and PKCE) and keeps
/// their session in an <see cref="ITokenStore"/>.
/// </summary>
/// <remarks>
/// <para>
/// One process owns the session. The authorization server issues a new refresh token each time one is used and treats
/// a second use of the old one as theft, ending the whole session; two processes refreshing from the same stored session
/// — a desktop shell and its background process — sign the user out. In one app, let one process sign in, refresh and
/// sign out; other processes ask it for the current access token.
/// </para>
/// <para>Thread-safe: concurrent calls share one refresh.</para>
/// </remarks>
public sealed class NativeClient : IDisposable
{
    /// <summary>An access token without <c>expires_in</c> is taken to last this long.</summary>
    private const long AssumedLifetimeSeconds = 300;

    private readonly NativeClientOptions options;
    private readonly ITokenStore store;
    private readonly HttpClient http;
    private readonly bool ownsHttp;
    private readonly TimeProvider time;
    private readonly SemaphoreSlim session = new(1, 1);
    private DiscoveryDocument? discovery;

    /// <summary>The access token this process holds, and when it expires (Unix seconds). Guarded by <see cref="session"/>.</summary>
    private (string Token, long ExpiresAt)? current;

    /// <param name="http">
    /// The <see cref="HttpClient"/> to reach the authorization server with; one is created and disposed with the client
    /// when omitted.
    /// </param>
    public NativeClient(NativeClientOptions options, ITokenStore store, HttpClient? http = null)
        : this(options, store, http, TimeProvider.System)
    {
    }

    internal NativeClient(NativeClientOptions options, ITokenStore store, HttpClient? http, TimeProvider time)
    {
        ArgumentNullException.ThrowIfNull(options);
        ArgumentNullException.ThrowIfNull(store);
        this.options = options;
        this.store = store;
        this.time = time;
        ownsHttp = http is null;
        this.http = http ?? new HttpClient { Timeout = TimeSpan.FromSeconds(30) };
    }

    private long Now => time.GetUtcNow().ToUnixTimeSeconds();

    /// <summary>Starts a sign-in: opens the loopback port and builds the URL to open in the browser.</summary>
    public async Task<PendingSignIn> BeginSignInAsync(CancellationToken cancellationToken = default)
    {
        var d = await DiscoverAsync(cancellationToken).ConfigureAwait(false);
        var listener = new TcpListener(IPAddress.Loopback, 0);
        try
        {
            listener.Start();
        }
        catch (SocketException e)
        {
            throw new NativeSignInException(NativeSignInFailure.Loopback, $"could not listen for the sign-in redirect: {e.Message}", e);
        }
        var port = ((IPEndPoint)listener.LocalEndpoint).Port;
        var redirectUri = $"http://127.0.0.1:{port}{options.RedirectPath}";
        var verifier = Protocol.RandomToken();
        var state = Protocol.RandomToken();
        var nonce = Protocol.RandomToken();

        var query = new List<KeyValuePair<string, string>>
        {
            new("response_type", "code"),
            new("client_id", options.ClientId),
            new("redirect_uri", redirectUri),
            new("scope", string.Join(' ', options.Scopes)),
            new("state", state),
            new("nonce", nonce),
            new("code_challenge", Protocol.ChallengeFor(verifier)),
            new("code_challenge_method", "S256"),
        };
        if (options.Audience.Count > 0)
        {
            query.Add(new("audience", string.Join(' ', options.Audience)));
        }
        var separator = d.AuthorizationEndpoint.Contains('?', StringComparison.Ordinal) ? "&" : "?";
        var url = new Uri(d.AuthorizationEndpoint + separator +
            string.Join('&', query.Select(p => $"{Uri.EscapeDataString(p.Key)}={Uri.EscapeDataString(p.Value)}")));

        return new PendingSignIn(this, listener, url, redirectUri, verifier, state, nonce);
    }

    /// <summary>
    /// The current access token — refreshed first when it expires within <see cref="NativeClientOptions.RefreshAhead"/>.
    /// <see langword="null"/> when no one is signed in, or the session ended (the refresh token expired or was revoked):
    /// sign in again.
    /// </summary>
    public async Task<string?> GetAccessTokenAsync(CancellationToken cancellationToken = default)
    {
        await session.WaitAsync(cancellationToken).ConfigureAwait(false);
        try
        {
            var stored = await LoadAsync(cancellationToken).ConfigureAwait(false);
            if (stored is null)
            {
                return null;
            }
            if (AccessTokenOf(stored) is { } access && access.ExpiresAt > Now + (long)options.RefreshAhead.TotalSeconds)
            {
                return access.Token;
            }
            if (stored.RefreshToken is null)
            {
                await RemoveAsync(cancellationToken).ConfigureAwait(false);
                return null;
            }
            return await RefreshAsync(stored, cancellationToken).ConfigureAwait(false);
        }
        finally
        {
            session.Release();
        }
    }

    /// <summary>
    /// Who is signed in, without contacting the server. The access token may have expired; the next
    /// <see cref="GetAccessTokenAsync"/> refreshes it.
    /// </summary>
    public async Task<NativeSession?> GetSessionAsync(CancellationToken cancellationToken = default)
    {
        await session.WaitAsync(cancellationToken).ConfigureAwait(false);
        try
        {
            var stored = await LoadAsync(cancellationToken).ConfigureAwait(false);
            return stored is null ? null : ToSession(stored);
        }
        finally
        {
            session.Release();
        }
    }

    /// <summary>
    /// Ends the session: revokes its tokens at the authorization server and removes it from the store. The store is
    /// cleared even when the server cannot be reached — see <see cref="SignedOut.Revoked"/>.
    /// </summary>
    public async Task<SignedOut> SignOutAsync(CancellationToken cancellationToken = default)
    {
        await session.WaitAsync(cancellationToken).ConfigureAwait(false);
        try
        {
            var stored = await LoadAsync(cancellationToken).ConfigureAwait(false);
            if (stored is null)
            {
                return new SignedOut(false, false);
            }
            string? endpoint = null;
            try
            {
                endpoint = (await DiscoverAsync(cancellationToken).ConfigureAwait(false)).RevocationEndpoint;
            }
            catch (NativeSignInException)
            {
                // Offline: the session still ends on this device.
            }
            var revoked = false;
            if (endpoint is not null)
            {
                var refresh = stored.RefreshToken is null ||
                    await RevokeAsync(endpoint, stored.RefreshToken, "refresh_token", cancellationToken).ConfigureAwait(false);
                var access = AccessTokenOf(stored) is not { } held ||
                    await RevokeAsync(endpoint, held.Token, "access_token", cancellationToken).ConfigureAwait(false);
                revoked = refresh && access;
            }
            await RemoveAsync(cancellationToken).ConfigureAwait(false);
            return new SignedOut(true, revoked);
        }
        finally
        {
            session.Release();
        }
    }

    public void Dispose()
    {
        session.Dispose();
        if (ownsHttp)
        {
            http.Dispose();
        }
    }

    internal NativeClientOptions Options => options;

    internal async Task<NativeSession> CompleteSignInAsync(
        string code, string redirectUri, string verifier, string nonce, CancellationToken cancellationToken)
    {
        var response = await TokenRequestAsync(
        [
            new("grant_type", "authorization_code"),
            new("code", code),
            new("redirect_uri", redirectUri),
            new("client_id", options.ClientId),
            new("code_verifier", verifier),
        ], cancellationToken).ConfigureAwait(false);
        if (response.IdToken is null)
        {
            throw new NativeSignInException(NativeSignInFailure.IdToken, "the answer carries no ID token — request the openid scope");
        }
        var d = await DiscoverAsync(cancellationToken).ConfigureAwait(false);
        var claims = Protocol.IdTokenClaims(response.IdToken, d.Issuer, options.ClientId, nonce, Now);

        await session.WaitAsync(cancellationToken).ConfigureAwait(false);
        try
        {
            return ToSession(await SaveAsync(response, response.RefreshToken, claims, cancellationToken).ConfigureAwait(false));
        }
        finally
        {
            session.Release();
        }
    }

    private NativeSession ToSession(StoredSession stored) => new(
        Protocol.StringClaim(stored.Claims, "sub") ?? "",
        stored.Claims,
        DateTimeOffset.FromUnixTimeSeconds(AccessTokenOf(stored)?.ExpiresAt ?? 0));

    /// <summary>
    /// The access token in hand: the one this process holds, or — for a session without a refresh token, or one stored by
    /// an earlier version — the stored one.
    /// </summary>
    private (string Token, long ExpiresAt)? AccessTokenOf(StoredSession stored) =>
        current ?? (stored.AccessToken is { } token ? (token, stored.ExpiresAt ?? 0) : null);

    /// <summary>
    /// Redeems the refresh token, stores what comes back and returns the new access token; <see langword="null"/> when it
    /// is no longer accepted.
    /// </summary>
    private async Task<string?> RefreshAsync(StoredSession stored, CancellationToken cancellationToken)
    {
        TokenResponse response;
        try
        {
            response = await TokenRequestAsync(
            [
                new("grant_type", "refresh_token"),
                new("refresh_token", stored.RefreshToken!),
                new("client_id", options.ClientId),
            ], cancellationToken).ConfigureAwait(false);
        }
        catch (NativeSignInException e) when (e.Failure == NativeSignInFailure.Token && e.Error == "invalid_grant")
        {
            await RemoveAsync(cancellationToken).ConfigureAwait(false);
            return null;
        }
        var claims = stored.Claims;
        if (response.IdToken is not null)
        {
            var d = await DiscoverAsync(cancellationToken).ConfigureAwait(false);
            claims = Protocol.IdTokenClaims(response.IdToken, d.Issuer, options.ClientId, null, Now);
        }
        // The server issues a new refresh token with each use; the old one is spent.
        await SaveAsync(response, response.RefreshToken ?? stored.RefreshToken, claims, cancellationToken).ConfigureAwait(false);
        return current!.Value.Token;
    }

    private async Task<DiscoveryDocument> DiscoverAsync(CancellationToken cancellationToken)
    {
        if (discovery is { } cached)
        {
            return cached;
        }
        var url = $"{options.Issuer}/.well-known/openid-configuration";
        string body;
        try
        {
            using var response = await http.GetAsync(url, cancellationToken).ConfigureAwait(false);
            body = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
            if (response.StatusCode != HttpStatusCode.OK)
            {
                throw new NativeSignInException(NativeSignInFailure.Http, $"{url} answered HTTP {(int)response.StatusCode}");
            }
        }
        catch (HttpRequestException e)
        {
            throw new NativeSignInException(NativeSignInFailure.Http, $"could not reach the authorization server: {e.Message}", e);
        }
        catch (TaskCanceledException e) when (!cancellationToken.IsCancellationRequested)
        {
            throw new NativeSignInException(NativeSignInFailure.Http, "the authorization server did not answer in time", e);
        }
        DiscoveryDocument? d;
        try
        {
            d = JsonSerializer.Deserialize(body, NativeJsonContext.Default.DiscoveryDocument);
        }
        catch (JsonException e)
        {
            throw new NativeSignInException(NativeSignInFailure.Discovery, $"{url}: {e.Message}", e);
        }
        if (d is null || string.IsNullOrEmpty(d.AuthorizationEndpoint) || string.IsNullOrEmpty(d.TokenEndpoint))
        {
            throw new NativeSignInException(NativeSignInFailure.Discovery, $"{url} names no authorization or token endpoint");
        }
        // The document must describe the issuer it was asked for (OpenID Connect Discovery §4.3).
        if (d.Issuer.TrimEnd('/') != options.Issuer)
        {
            throw new NativeSignInException(NativeSignInFailure.Discovery, $"it names the issuer {d.Issuer} instead of {options.Issuer}");
        }
        discovery = d;
        return d;
    }

    private async Task<TokenResponse> TokenRequestAsync(
        IEnumerable<KeyValuePair<string, string>> form, CancellationToken cancellationToken)
    {
        var d = await DiscoverAsync(cancellationToken).ConfigureAwait(false);
        var (status, body) = await PostFormAsync(d.TokenEndpoint, form, cancellationToken).ConfigureAwait(false);
        if (status == 200)
        {
            try
            {
                return JsonSerializer.Deserialize(body, NativeJsonContext.Default.TokenResponse)
                    ?? throw new JsonException("empty");
            }
            catch (JsonException e)
            {
                throw new NativeSignInException(NativeSignInFailure.Http, $"unreadable token response: {e.Message}", e);
            }
        }
        // RFC 6749 §5.2 error / error_description, or RFC 9457 code / detail.
        Dictionary<string, JsonElement>? refusal = null;
        try
        {
            refusal = JsonSerializer.Deserialize(body, NativeJsonContext.Default.DictionaryStringJsonElement);
        }
        catch (JsonException)
        {
        }
        refusal ??= [];
        var error = Protocol.StringClaim(refusal, "error") ?? Protocol.StringClaim(refusal, "code");
        var description = Protocol.StringClaim(refusal, "error_description") ?? Protocol.StringClaim(refusal, "detail");
        var message = new StringBuilder($"the token endpoint refused the request (HTTP {status}");
        if (error is not null)
        {
            message.Append(", ").Append(error);
        }
        message.Append(')');
        if (description is not null)
        {
            message.Append(": ").Append(description);
        }
        throw new NativeSignInException(NativeSignInFailure.Token, message.ToString())
        {
            Error = error,
            ErrorDescription = description,
            Status = status,
        };
    }

    private async Task<(int Status, string Body)> PostFormAsync(
        string url, IEnumerable<KeyValuePair<string, string>> form, CancellationToken cancellationToken)
    {
        try
        {
            using var content = new FormUrlEncodedContent(form);
            using var request = new HttpRequestMessage(HttpMethod.Post, url) { Content = content };
            request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));
            using var response = await http.SendAsync(request, cancellationToken).ConfigureAwait(false);
            return ((int)response.StatusCode, await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false));
        }
        catch (HttpRequestException e)
        {
            throw new NativeSignInException(NativeSignInFailure.Http, $"could not reach the authorization server: {e.Message}", e);
        }
        catch (TaskCanceledException e) when (!cancellationToken.IsCancellationRequested)
        {
            throw new NativeSignInException(NativeSignInFailure.Http, "the authorization server did not answer in time", e);
        }
    }

    private async Task<bool> RevokeAsync(string endpoint, string token, string hint, CancellationToken cancellationToken)
    {
        try
        {
            var (status, _) = await PostFormAsync(endpoint,
            [
                new("token", token),
                new("token_type_hint", hint),
                new("client_id", options.ClientId),
            ], cancellationToken).ConfigureAwait(false);
            return status == 200;
        }
        catch (NativeSignInException)
        {
            return false;
        }
    }

    private async Task<StoredSession?> LoadAsync(CancellationToken cancellationToken)
    {
        var raw = await store.GetAsync(options.StoreKey, cancellationToken).ConfigureAwait(false);
        StoredSession? stored = null;
        if (raw is not null)
        {
            try
            {
                // A value this version cannot read is no session rather than an error the app cannot get past: signing in
                // again replaces it.
                stored = JsonSerializer.Deserialize(raw, NativeJsonContext.Default.StoredSession);
            }
            catch (JsonException)
            {
            }
        }
        if (stored is null)
        {
            // The session is gone from the store, so the access token this process held belongs to no session.
            current = null;
        }
        return stored;
    }

    /// <summary>
    /// Keeps a token response: the access token in the process, the refresh token and claims in the store. Without a
    /// refresh token the access token is stored too — nothing else could resume the session.
    /// </summary>
    private async Task<StoredSession> SaveAsync(
        TokenResponse response, string? refreshToken, Dictionary<string, JsonElement> claims, CancellationToken cancellationToken)
    {
        var expiresAt = Now + (response.ExpiresIn ?? AssumedLifetimeSeconds);
        var stored = refreshToken is null
            ? new StoredSession(response.AccessToken, expiresAt, null, claims)
            : new StoredSession(null, null, refreshToken, claims);
        await store.SetAsync(options.StoreKey, JsonSerializer.Serialize(stored, NativeJsonContext.Default.StoredSession), cancellationToken)
            .ConfigureAwait(false);
        current = (response.AccessToken, expiresAt);
        return stored;
    }

    private async Task RemoveAsync(CancellationToken cancellationToken)
    {
        current = null;
        await store.DeleteAsync(options.StoreKey, cancellationToken).ConfigureAwait(false);
    }
}
