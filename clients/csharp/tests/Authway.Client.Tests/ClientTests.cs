using System.Net;
using System.Text;
using System.Text.Json;

namespace Authway.Client.Tests;

/// <summary>Answers every request with one canned response and remembers what was asked.</summary>
internal sealed class Canned(HttpStatusCode status, string body, string contentType = "application/json") : HttpMessageHandler
{
    public List<(HttpRequestMessage Request, string? Body)> Seen { get; } = [];

    protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
    {
        Seen.Add((request, request.Content is null ? null : await request.Content.ReadAsStringAsync(cancellationToken)));
        return new HttpResponseMessage(status) { Content = new StringContent(body, Encoding.UTF8, contentType) };
    }
}

public class ClientTests
{
    private static (AuthwayClient Client, Canned Handler) Answering(HttpStatusCode status, string body, string contentType = "application/json", string? tenant = "t1")
    {
        var handler = new Canned(status, body, contentType);
#pragma warning disable CA2000 // the HttpClient lives as long as the test
        var http = new HttpClient(handler) { BaseAddress = new Uri("https://authway.example/") };
#pragma warning restore CA2000
        return (new AuthwayClient(http, "admin-key", tenant), handler);
    }

    [Fact]
    public async Task A_refusal_becomes_an_exception_carrying_its_problem_details()
    {
        var (client, _) = Answering(HttpStatusCode.Conflict,
            """{"type":"about:blank","title":"Conflict","status":409,"detail":"An invitation is already pending.","code":"invitation_already_pending"}""",
            "application/problem+json");

        var call = () => client.Invitations.CreateAsync("a@example.com", cancellationToken: TestContext.Current.CancellationToken);

        var e = (await call.Should().ThrowAsync<AuthwayException>()).Which;
        e.StatusCode.Should().Be(HttpStatusCode.Conflict);
        e.Code.Should().Be("invitation_already_pending");
        e.Message.Should().Be("An invitation is already pending.");
        e.Problem!.Title.Should().Be("Conflict");
    }

    [Fact]
    public async Task A_refusal_keeps_its_extension_members()
    {
        var (client, _) = Answering(HttpStatusCode.TooManyRequests,
            """{"type":"about:blank","title":"Too Many Requests","status":429,"detail":"Later.","code":"too_many_requests","retry_after":30,"field":"email","min_length":12}""",
            "application/problem+json");

        var e = (await ((Func<Task>)(() => client.Users.ListAsync(cancellationToken: TestContext.Current.CancellationToken))).Should().ThrowAsync<AuthwayException>()).Which;

        e.Problem!.RetryAfter.Should().Be(TimeSpan.FromSeconds(30));
        e.Problem.Field.Should().Be("email");
        e.Problem.Extensions!["min_length"].GetInt32().Should().Be(12);
    }

    [Fact]
    public async Task An_answer_that_is_not_problem_details_still_says_its_status()
    {
        var (client, _) = Answering(HttpStatusCode.BadGateway, "<html>bad gateway</html>", "text/html");

        var e = (await ((Func<Task>)(() => client.Webhooks.ListAsync(TestContext.Current.CancellationToken))).Should().ThrowAsync<AuthwayException>()).Which;

        e.StatusCode.Should().Be(HttpStatusCode.BadGateway);
        e.Problem.Should().BeNull();
    }

    [Fact]
    public async Task Calls_with_the_credential_and_the_tenant()
    {
        var (client, handler) = Answering(HttpStatusCode.OK, """{"webhooks":[]}""");

        await client.Webhooks.ListAsync(TestContext.Current.CancellationToken);

        var request = handler.Seen.Single().Request;
        request.RequestUri.Should().Be(new Uri("https://authway.example/api/v1/webhooks"));
        request.Headers.Authorization!.ToString().Should().Be("Bearer admin-key");
        request.Headers.GetValues("X-Tenant-ID").Should().Equal("t1");
    }

    [Fact]
    public async Task Registers_an_application_in_the_clients_tenant_and_keeps_its_secret()
    {
        var (client, handler) = Answering(HttpStatusCode.Created, """
            {"client":{"id":"c1","tenant_id":"t1","client_id":"authway_x","name":"App","public":false,"grant_types":["authorization_code"],
                       "scopes":["openid"],"token_endpoint_auth_method":"client_secret_post","redirect_uris":["https://app.example/cb"]},
             "credentials":{"client_id":"authway_x","client_secret":"s3cret"}}
            """);

        var registered = await client.Clients.CreateAsync(
            new NewOAuthClient("App", ["authorization_code"], ["openid"], RedirectUris: ["https://app.example/cb"]),
            TestContext.Current.CancellationToken);

        registered.Client.ClientId.Should().Be("authway_x");
        registered.Credentials!.ClientSecret.Should().Be("s3cret");
        using var sent = JsonDocument.Parse(handler.Seen.Single().Body!);
        sent.RootElement.GetProperty("tenant_id").GetString().Should().Be("t1");
        sent.RootElement.GetProperty("redirect_uris")[0].GetString().Should().Be("https://app.example/cb");
        sent.RootElement.TryGetProperty("jwks", out _).Should().BeFalse("unset members are not sent");
    }

    [Fact]
    public async Task A_public_application_comes_back_without_credentials()
    {
        var (client, _) = Answering(HttpStatusCode.Created, """
            {"client":{"id":"c1","tenant_id":"t1","client_id":"authway_x","name":"SPA","public":true,"grant_types":["authorization_code"],
                       "scopes":["openid"],"token_endpoint_auth_method":"none"},"credentials":{"client_id":"authway_x","client_secret":""}}
            """);

        var registered = await client.Clients.CreateAsync(new NewOAuthClient("SPA", ["authorization_code"], ["openid"], Public: true), TestContext.Current.CancellationToken);

        registered.Credentials.Should().BeNull();
    }

    [Fact]
    public async Task Invites_for_an_application_and_sends_only_what_was_given()
    {
        var (client, handler) = Answering(HttpStatusCode.Created,
            """{"invitation":{"id":"i1","email":"a@example.com","status":"pending","expires_at":"2026-10-22T02:40:46Z"}}""");

        var sent = await client.Invitations.CreateAsync("a@example.com", clientId: "authway_x", cancellationToken: TestContext.Current.CancellationToken);

        sent.Status.Should().Be("pending");
        handler.Seen.Single().Body.Should().Be("""{"email":"a@example.com","client_id":"authway_x"}""");
    }

    [Fact]
    public async Task Patches_claims_as_a_json_object()
    {
        var (client, handler) = Answering(HttpStatusCode.OK, """{"claims":{"roles":["editor"]}}""");

        var stored = await client.Users.PatchClaimsAsync("u 1", JsonSerializer.SerializeToElement(new Dictionary<string, string[]> { ["roles"] = ["editor"] }),
            TestContext.Current.CancellationToken);

        stored.GetProperty("roles")[0].GetString().Should().Be("editor");
        var (request, body) = handler.Seen.Single();
        request.Method.Should().Be(HttpMethod.Patch);
        request.RequestUri!.AbsolutePath.Should().Be("/api/v1/users/u%201/claims");
        body.Should().Be("""{"claims":{"roles":["editor"]}}""");
    }

    [Fact]
    public async Task Calls_that_name_the_tenant_need_one()
    {
        var (client, _) = Answering(HttpStatusCode.OK, """{"terms":[]}""", tenant: null);

        var call = () => client.Terms.ListAsync(TestContext.Current.CancellationToken);

        await call.Should().ThrowAsync<InvalidOperationException>();
    }
}
