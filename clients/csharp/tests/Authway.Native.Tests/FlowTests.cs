using System.Net;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;

namespace Authway.Native.Tests;

/// <summary>
/// The whole flow against a stand-in authorization server (an <see cref="HttpMessageHandler"/>): sign in through the
/// loopback port, refresh before expiry, a refused refresh ends the session, sign-out revokes.
/// </summary>
public sealed class FlowTests
{
    private const string Issuer = "https://auth.example";

    private sealed class StandIn(Func<Dictionary<string, string>, (HttpStatusCode, object)> token) : HttpMessageHandler
    {
        public List<(string Path, Dictionary<string, string> Form)> Forms { get; } = [];

        public string? Nonce { get; set; }

        protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
        {
            var path = request.RequestUri!.AbsolutePath;
            var form = request.Content is null
                ? []
                : (await request.Content.ReadAsStringAsync(cancellationToken)).Split('&', StringSplitOptions.RemoveEmptyEntries)
                    .Select(p => p.Split('=', 2))
                    .ToDictionary(p => Uri.UnescapeDataString(p[0]), p => Uri.UnescapeDataString(p[1].Replace('+', ' ')));
            (HttpStatusCode status, object body) = path switch
            {
                _ when path.EndsWith("/.well-known/openid-configuration", StringComparison.Ordinal) => (HttpStatusCode.OK, new
                {
                    issuer = Issuer,
                    authorization_endpoint = $"{Issuer}/oauth2/auth",
                    token_endpoint = $"{Issuer}/oauth2/token",
                    revocation_endpoint = $"{Issuer}/oauth2/revoke",
                }),
                "/oauth2/token" => Record(path, form, token(form)),
                "/oauth2/revoke" => Record(path, form, (HttpStatusCode.OK, new { })),
                _ => (HttpStatusCode.NotFound, new { }),
            };
            return new HttpResponseMessage(status) { Content = new StringContent(JsonSerializer.Serialize(body), Encoding.UTF8, "application/json") };
        }

        private (HttpStatusCode, object) Record(string path, Dictionary<string, string> form, (HttpStatusCode, object) answer)
        {
            lock (Forms)
            {
                Forms.Add((path, form));
            }
            return answer;
        }

        public List<Dictionary<string, string>> FormsTo(string path) => Forms.Where(f => f.Path == path).Select(f => f.Form).ToList();
    }

    private static string IdToken(string? nonce)
    {
        var claims = new Dictionary<string, object>
        {
            ["iss"] = Issuer, ["aud"] = new[] { "desktop-app" }, ["sub"] = "user-1", ["email"] = "user@example.com",
            ["exp"] = DateTimeOffset.UtcNow.ToUnixTimeSeconds() + 3600,
        };
        if (nonce is not null)
        {
            claims["nonce"] = nonce;
        }
        return ProtocolTests.IdToken(claims);
    }

    /// <summary>Plays the browser: follows the authorization URL's redirect to the loopback port.</summary>
    private static async Task<HttpResponseMessage> BrowserReturns(Uri authorizeUrl, string parameters)
    {
        var query = ParseQuery(authorizeUrl.Query);
        using var browser = new HttpClient();
        return await browser.GetAsync($"{query["redirect_uri"]}?{parameters}&state={Uri.EscapeDataString(query["state"])}");
    }

    private static Dictionary<string, string> ParseQuery(string query) =>
        query.TrimStart('?').Split('&').Select(p => p.Split('=', 2))
            .ToDictionary(p => Uri.UnescapeDataString(p[0]), p => Uri.UnescapeDataString(p[1]));

    [Fact]
    public async Task Signs_in_refreshes_and_signs_out()
    {
        var ct = TestContext.Current.CancellationToken;
        StandIn server = null!;
        server = new StandIn(form => form["grant_type"] switch
        {
            "authorization_code" => (HttpStatusCode.OK, new
            {
                access_token = "access-1", expires_in = 30, refresh_token = "refresh-1", id_token = IdToken(server.Nonce), token_type = "bearer",
            }),
            "refresh_token" when form["refresh_token"] == "refresh-1" => (HttpStatusCode.OK, (object)new
            {
                access_token = "access-2", expires_in = 3600, refresh_token = "refresh-2", token_type = "bearer",
            }),
            _ => (HttpStatusCode.BadRequest, new { error = "invalid_grant" }),
        });
        var store = new MemoryTokenStore();
        using var http = new HttpClient(server);
        using var client = new NativeClient(new NativeClientOptions(Issuer, "desktop-app"), store, http);
        (await client.GetSessionAsync(ct)).Should().BeNull();

        using var pending = await client.BeginSignInAsync(ct);
        var query = ParseQuery(pending.Url.Query);
        pending.Url.GetLeftPart(UriPartial.Path).Should().Be($"{Issuer}/oauth2/auth");
        query["client_id"].Should().Be("desktop-app");
        query["code_challenge_method"].Should().Be("S256");
        query["scope"].Should().Be("openid profile email offline_access");
        query["redirect_uri"].Should().StartWith("http://127.0.0.1:").And.EndWith("/callback");
        server.Nonce = query["nonce"];

        var waiting = pending.WaitAsync(TimeSpan.FromSeconds(10), ct);
        using var page = await BrowserReturns(pending.Url, "code=the-code");
        page.StatusCode.Should().Be(HttpStatusCode.OK);
        var session = await waiting;
        session.Subject.Should().Be("user-1");
        session.Claims["email"].GetString().Should().Be("user@example.com");

        var exchange = server.FormsTo("/oauth2/token")[0];
        exchange["code"].Should().Be("the-code");
        exchange["redirect_uri"].Should().Be(query["redirect_uri"]);
        exchange["code_verifier"].Should().HaveLength(43);

        // The access token stays in the process: the stored value is the refresh token and the claims.
        var signedIn = JsonNode.Parse((await store.GetAsync("authway.desktop-app", ct))!)!.AsObject();
        signedIn.Select(p => p.Key).Should().BeEquivalentTo("refresh_token", "claims");
        signedIn["refresh_token"]!.GetValue<string>().Should().Be("refresh-1");

        // 30 seconds left is inside the 60-second margin: refreshed, and the new refresh token kept.
        (await client.GetAccessTokenAsync(ct)).Should().Be("access-2");
        var refreshed = JsonNode.Parse((await store.GetAsync("authway.desktop-app", ct))!)!.AsObject();
        refreshed["refresh_token"]!.GetValue<string>().Should().Be("refresh-2");
        refreshed.ContainsKey("access_token").Should().BeFalse();
        (await client.GetAccessTokenAsync(ct)).Should().Be("access-2");
        server.FormsTo("/oauth2/token").Should().HaveCount(2);

        var signedOut = await client.SignOutAsync(ct);
        signedOut.Should().Be(new SignedOut(true, true));
        server.FormsTo("/oauth2/revoke").Select(f => f["token"]).Should().Equal("refresh-2", "access-2");
        (await client.GetSessionAsync(ct)).Should().BeNull();
        (await client.GetAccessTokenAsync(ct)).Should().BeNull();
    }

    private static async Task<MemoryTokenStore> Expired(CancellationToken ct)
    {
        var store = new MemoryTokenStore();
        await store.SetAsync("authway.desktop-app", """{"access_token":"old","expires_at":0,"refresh_token":"spent","claims":{"sub":"user-1"}}""", ct);
        return store;
    }

    [Fact]
    public async Task A_restarted_app_refreshes_once_from_the_stored_refresh_token()
    {
        var ct = TestContext.Current.CancellationToken;
        var store = new MemoryTokenStore();
        await store.SetAsync("authway.desktop-app", """{"refresh_token":"r-1","claims":{"sub":"user-1"}}""", ct);
        var server = new StandIn(form => form["refresh_token"] == "r-1"
            ? (HttpStatusCode.OK, new { access_token = "access-2", expires_in = 3600, refresh_token = "r-2", token_type = "bearer" })
            : (HttpStatusCode.BadRequest, new { error = "invalid_grant" }));
        using var http = new HttpClient(server);
        using var client = new NativeClient(new NativeClientOptions(Issuer, "desktop-app"), store, http);

        var before = (await client.GetSessionAsync(ct))!;
        before.Subject.Should().Be("user-1");
        before.AccessTokenExpiresAt.Should().Be(DateTimeOffset.UnixEpoch);
        (await client.GetAccessTokenAsync(ct)).Should().Be("access-2");
        (await client.GetAccessTokenAsync(ct)).Should().Be("access-2");
        server.FormsTo("/oauth2/token").Should().HaveCount(1);
        (await client.GetSessionAsync(ct))!.AccessTokenExpiresAt.Should().BeAfter(DateTimeOffset.UtcNow.AddMinutes(30));
        (await store.GetAsync("authway.desktop-app", ct)).Should().Be("""{"refresh_token":"r-2","claims":{"sub":"user-1"}}""");
    }

    [Fact]
    public async Task A_session_stored_by_0_1_keeps_its_access_token_until_the_first_refresh()
    {
        var ct = TestContext.Current.CancellationToken;
        var store = new MemoryTokenStore();
        var later = DateTimeOffset.UtcNow.AddHours(1).ToUnixTimeSeconds();
        await store.SetAsync("authway.desktop-app", $$$"""{"access_token":"old","expires_at":{{{later}}},"refresh_token":"r","claims":{"sub":"user-1"}}""", ct);
        var server = new StandIn(_ => (HttpStatusCode.InternalServerError, new { }));
        using var http = new HttpClient(server);
        using var client = new NativeClient(new NativeClientOptions(Issuer, "desktop-app"), store, http);
        (await client.GetAccessTokenAsync(ct)).Should().Be("old");
        server.FormsTo("/oauth2/token").Should().BeEmpty();
    }

    [Fact]
    public async Task Without_a_refresh_token_the_access_token_is_what_is_stored()
    {
        var ct = TestContext.Current.CancellationToken;
        StandIn server = null!;
        server = new StandIn(_ => (HttpStatusCode.OK, new
        {
            access_token = "access-1", expires_in = 3600, id_token = IdToken(server.Nonce), token_type = "bearer",
        }));
        var store = new MemoryTokenStore();
        using var http = new HttpClient(server);
        using var client = new NativeClient(new NativeClientOptions(Issuer, "desktop-app"), store, http);
        using var pending = await client.BeginSignInAsync(ct);
        server.Nonce = ParseQuery(pending.Url.Query)["nonce"];
        var waiting = pending.WaitAsync(TimeSpan.FromSeconds(10), ct);
        using var page = await BrowserReturns(pending.Url, "code=the-code");
        await waiting;

        var stored = JsonNode.Parse((await store.GetAsync("authway.desktop-app", ct))!)!.AsObject();
        stored["access_token"]!.GetValue<string>().Should().Be("access-1");
        stored.ContainsKey("refresh_token").Should().BeFalse();
        using var restarted = new NativeClient(new NativeClientOptions(Issuer, "desktop-app"), store, http);
        (await restarted.GetAccessTokenAsync(ct)).Should().Be("access-1");
    }

    [Fact]
    public async Task A_refused_refresh_ends_the_session()
    {
        var ct = TestContext.Current.CancellationToken;
        var store = await Expired(ct);
        using var http = new HttpClient(new StandIn(_ => (HttpStatusCode.BadRequest, new { error = "invalid_grant", error_description = "revoked" })));
        using var client = new NativeClient(new NativeClientOptions(Issuer, "desktop-app"), store, http);
        (await client.GetSessionAsync(ct))!.Subject.Should().Be("user-1");
        (await client.GetAccessTokenAsync(ct)).Should().BeNull();
        (await store.GetAsync("authway.desktop-app", ct)).Should().BeNull();
    }

    [Fact]
    public async Task Concurrent_calls_share_one_refresh()
    {
        var ct = TestContext.Current.CancellationToken;
        var store = await Expired(ct);
        var used = 0;
        // The server rotates: the refresh token works once, and a second use is refused as theft.
        using var http = new HttpClient(new StandIn(form =>
        {
            Thread.Sleep(50);
            return form["refresh_token"] == "spent" && Interlocked.Increment(ref used) == 1
                ? (HttpStatusCode.OK, new { access_token = "access-2", expires_in = 3600, refresh_token = "refresh-2", token_type = "bearer" })
                : (HttpStatusCode.BadRequest, new { error = "invalid_grant", error_description = "reused" });
        }));
        using var client = new NativeClient(new NativeClientOptions(Issuer, "desktop-app"), store, http);
        var tokens = await Task.WhenAll(Enumerable.Range(0, 8).Select(_ => Task.Run(() => client.GetAccessTokenAsync(ct), ct)));
        tokens.Should().AllBe("access-2");
        used.Should().Be(1);
    }

    [Fact]
    public async Task Another_token_endpoint_failure_keeps_the_session()
    {
        var ct = TestContext.Current.CancellationToken;
        var store = await Expired(ct);
        using var http = new HttpClient(new StandIn(_ => (HttpStatusCode.ServiceUnavailable, new { code = "unavailable", detail = "try later" })));
        using var client = new NativeClient(new NativeClientOptions(Issuer, "desktop-app"), store, http);
        var failure = await FluentActions.Awaiting(() => client.GetAccessTokenAsync(ct)).Should().ThrowAsync<NativeSignInException>();
        failure.Which.Failure.Should().Be(NativeSignInFailure.Token);
        failure.Which.Status.Should().Be(503);
        failure.Which.Error.Should().Be("unavailable");
        (await store.GetAsync("authway.desktop-app", ct)).Should().NotBeNull();
    }

    [Fact]
    public async Task A_declined_sign_in_stores_nothing()
    {
        var ct = TestContext.Current.CancellationToken;
        var server = new StandIn(_ => (HttpStatusCode.InternalServerError, new { }));
        var store = new MemoryTokenStore();
        using var http = new HttpClient(server);
        using var client = new NativeClient(new NativeClientOptions(Issuer, "desktop-app"), store, http);
        using var pending = await client.BeginSignInAsync(ct);
        var waiting = pending.WaitAsync(TimeSpan.FromSeconds(10), ct);
        using var page = await BrowserReturns(pending.Url, "error=access_denied");
        var failure = await FluentActions.Awaiting(() => waiting).Should().ThrowAsync<NativeSignInException>();
        failure.Which.Failure.Should().Be(NativeSignInFailure.Denied);
        failure.Which.Error.Should().Be("access_denied");
        (await store.GetAsync("authway.desktop-app", ct)).Should().BeNull();
        server.FormsTo("/oauth2/token").Should().BeEmpty();
    }

    [Fact]
    public async Task Stray_requests_do_not_end_the_wait_and_no_redirect_times_out()
    {
        var ct = TestContext.Current.CancellationToken;
        using var http = new HttpClient(new StandIn(_ => (HttpStatusCode.InternalServerError, new { })));
        using var client = new NativeClient(new NativeClientOptions(Issuer, "desktop-app"), new MemoryTokenStore(), http);
        using var pending = await client.BeginSignInAsync(ct);
        var waiting = pending.WaitAsync(TimeSpan.FromSeconds(2), ct);
        var redirect = ParseQuery(pending.Url.Query)["redirect_uri"];
        using var browser = new HttpClient();
        (await browser.GetAsync(new Uri(new Uri(redirect), "/favicon.ico"), ct)).StatusCode.Should().Be(HttpStatusCode.NotFound);
        (await browser.GetAsync($"{redirect}?code=x&state=forged", ct)).StatusCode.Should().Be(HttpStatusCode.NotFound);
        var failure = await FluentActions.Awaiting(() => waiting).Should().ThrowAsync<NativeSignInException>();
        failure.Which.Failure.Should().Be(NativeSignInFailure.TimedOut);
    }

    [Fact]
    public async Task A_discovery_document_for_another_issuer_is_refused()
    {
        var ct = TestContext.Current.CancellationToken;
        using var http = new HttpClient(new StandIn(_ => (HttpStatusCode.InternalServerError, new { })));
        using var client = new NativeClient(new NativeClientOptions("https://auth.example/other", "desktop-app"), new MemoryTokenStore(), http);
        var failure = await FluentActions.Awaiting(() => client.BeginSignInAsync(ct)).Should().ThrowAsync<NativeSignInException>();
        failure.Which.Failure.Should().Be(NativeSignInFailure.Discovery);
    }
}
