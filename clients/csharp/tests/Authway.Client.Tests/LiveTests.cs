using System.Net;
using System.Net.Sockets;
using System.Text;

namespace Authway.Client.Tests;

/// <summary>
/// The client against a running Authway: the service's conformance suite checks the server against the contract, these
/// check the client against the server. Set AUTHWAY_CLIENT_TEST_API, AUTHWAY_CLIENT_TEST_KEY (the admin key) and
/// AUTHWAY_CLIENT_TEST_TENANT; the deployment must allow private webhook targets. Skipped otherwise.
/// </summary>
public sealed class LiveTests : IDisposable
{
    private readonly HttpClient http = new();
    private readonly AuthwayClient? client;
    private readonly string suffix = Guid.NewGuid().ToString("N")[..8];

    public LiveTests()
    {
        var api = Environment.GetEnvironmentVariable("AUTHWAY_CLIENT_TEST_API");
        var key = Environment.GetEnvironmentVariable("AUTHWAY_CLIENT_TEST_KEY");
        var tenant = Environment.GetEnvironmentVariable("AUTHWAY_CLIENT_TEST_TENANT");
        if (api is { Length: > 0 } && key is { Length: > 0 } && tenant is { Length: > 0 })
        {
            http.BaseAddress = new Uri(api);
            client = new AuthwayClient(http, key, tenant);
        }
    }

    private AuthwayClient Client =>
        client ?? throw new InvalidOperationException("unreachable: tests skip without a deployment");

    private void RequireDeployment()
    {
        if (client is null)
        {
            Assert.Skip("Set AUTHWAY_CLIENT_TEST_API, AUTHWAY_CLIENT_TEST_KEY and AUTHWAY_CLIENT_TEST_TENANT to run against a deployment.");
        }
    }

    public void Dispose() => http.Dispose();

    [Fact]
    public async Task Registers_reads_changes_and_deletes_an_application()
    {
        RequireDeployment();
        var ct = TestContext.Current.CancellationToken;
        var registered = await Client.Clients.CreateAsync(new NewOAuthClient(
            $"client-test-{suffix}", ["authorization_code"], ["openid"],
            RedirectUris: ["http://127.0.0.1:9999/cb"], InitiateLoginUri: "https://app.example/login"), ct);
        try
        {
            registered.Credentials.Should().NotBeNull("a confidential client gets a secret");
            var read = await Client.Clients.GetAsync(registered.Client.Id, ct);
            read.ClientId.Should().Be(registered.Client.ClientId);
            read.InitiateLoginUri.Should().Be("https://app.example/login");

            var updated = await Client.Clients.UpdateAsync(read.Id, new OAuthClientChanges(Name: $"client-test-{suffix}-renamed"), cancellationToken: ct);
            updated.Client.Name.Should().EndWith("-renamed");
            updated.SyncStatus.State.Should().Be("ok");

            var page = await Client.Clients.ListAsync(limit: 100, cancellationToken: ct);
            page.Items.Should().Contain(c => c.Id == read.Id);
        }
        finally
        {
            await Client.Clients.DeleteAsync(registered.Client.Id, ct);
        }

        var gone = () => Client.Clients.GetAsync(registered.Client.Id, ct);
        (await gone.Should().ThrowAsync<AuthwayException>()).Which.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task Invites_for_an_application_then_withdraws_the_invitation()
    {
        RequireDeployment();
        var ct = TestContext.Current.CancellationToken;
        var app = await Client.Clients.CreateAsync(new NewOAuthClient($"client-test-invite-{suffix}", ["authorization_code"], ["openid"],
            Public: true, RedirectUris: ["http://127.0.0.1:9999/cb"]), ct);
        try
        {
            var email = $"client-test-{suffix}@authway.invalid";
            var sent = await Client.Invitations.CreateAsync(email, clientId: app.Client.ClientId, cancellationToken: ct);
            sent.Status.Should().Be("pending");

            var again = () => Client.Invitations.CreateAsync(email, cancellationToken: ct);
            (await again.Should().ThrowAsync<AuthwayException>()).Which.Code.Should().Be("invitation_already_pending");

            var read = await Client.Invitations.GetAsync(sent.Id, ct);
            read.Client!.ClientId.Should().Be(app.Client.ClientId);
            (await Client.Invitations.ListAsync("pending", 100, cancellationToken: ct)).Items.Should().Contain(i => i.Id == sent.Id);

            await Client.Invitations.ResendAsync(sent.Id, ct);
            await Client.Invitations.RevokeAsync(sent.Id, ct);
            (await Client.Invitations.GetAsync(sent.Id, ct)).Status.Should().Be("revoked");
        }
        finally
        {
            await Client.Clients.DeleteAsync(app.Client.Id, ct);
        }
    }

    [Fact]
    public async Task A_webhook_delivery_verifies_with_the_secret_handed_out_and_only_that()
    {
        RequireDeployment();
        var ct = TestContext.Current.CancellationToken;
        using var receiver = new Receiver();
        var created = await Client.Webhooks.CreateAsync(new NewWebhook($"client-test-{suffix}", receiver.Url, [WebhookEventTypes.Test], RetryCount: 0, TimeoutSecs: 5), ct);
        try
        {
            created.Secret.Should().StartWith("whsec_");
            // The test delivery is answered while the call waits, so the receiver listens first.
            var next = receiver.Next(ct);
            var delivery = await Client.Webhooks.TestAsync(created.Webhook.Id, ct);
            delivery.Success.Should().BeTrue(delivery.ErrorMessage);

            var (headers, body) = await next;
            var e = AuthwayWebhook.Read(created.Secret, name => headers.GetValueOrDefault(name), body);
            e.Type.Should().Be(WebhookEventTypes.Test);
            headers["webhook-id"].Should().Be(e.Id);

            var rotated = await Client.Webhooks.RotateSecretAsync(created.Webhook.Id, ct);
            var next2 = receiver.Next(ct);
            await Client.Webhooks.TestAsync(created.Webhook.Id, ct);
            var (headers2, body2) = await next2;
            AuthwayWebhook.Verify(rotated, headers2["webhook-id"], headers2["webhook-timestamp"], headers2["webhook-signature"], body2).Should().BeTrue();
            AuthwayWebhook.Verify(created.Secret, headers2["webhook-id"], headers2["webhook-timestamp"], headers2["webhook-signature"], body2).Should().BeFalse();

            (await Client.Webhooks.GetDeliveriesAsync(created.Webhook.Id, 10, ct)).Should().HaveCountGreaterThanOrEqualTo(2);
            (await Client.Webhooks.UpdateAsync(created.Webhook.Id, new WebhookChanges(Enabled: false), ct)).Enabled.Should().BeFalse();
        }
        finally
        {
            await Client.Webhooks.DeleteAsync(created.Webhook.Id, ct);
        }
    }

    [Fact]
    public async Task Publishes_and_retires_a_document_of_the_tenant()
    {
        RequireDeployment();
        var ct = TestContext.Current.CancellationToken;
        var kind = $"client_test_{suffix}";
        var published = await Client.Terms.PublishAsync(kind, "1", "https://app.example/terms/1", "Test terms", ct);
        try
        {
            published.Version.Should().Be("1");
            (await Client.Terms.ListAsync(ct)).Should().Contain(d => d.Kind == kind && d.Url == "https://app.example/terms/1");
        }
        finally
        {
            await Client.Terms.RetireAsync(kind, ct);
        }

        (await Client.Terms.ListAsync(ct)).Should().NotContain(d => d.Kind == kind);
    }

    [Fact]
    public async Task Reads_users_and_refuses_with_problem_details()
    {
        RequireDeployment();
        var ct = TestContext.Current.CancellationToken;
        var page = await Client.Users.ListAsync(limit: 5, cancellationToken: ct);
        page.Limit.Should().Be(5);

        var missing = () => Client.Users.GetAsync(Guid.NewGuid().ToString(), ct);
        var e = (await missing.Should().ThrowAsync<AuthwayException>()).Which;
        e.StatusCode.Should().Be(HttpStatusCode.NotFound);
        e.Problem!.Status.Should().Be(404);
        e.Code.Should().NotBeNullOrEmpty();
    }

    /// <summary>A one-endpoint HTTP receiver on a loopback port.</summary>
    private sealed class Receiver : IDisposable
    {
        private readonly HttpListener listener = new();

        public Receiver()
        {
            using var probe = new TcpListener(IPAddress.Loopback, 0);
            probe.Start();
            var port = ((IPEndPoint)probe.LocalEndpoint).Port;
            probe.Stop();
            Url = $"http://localhost:{port}/hooks/";
            listener.Prefixes.Add(Url);
            listener.Start();
        }

        public string Url { get; }

        public async Task<(Dictionary<string, string> Headers, byte[] Body)> Next(CancellationToken ct)
        {
            var context = await listener.GetContextAsync().WaitAsync(TimeSpan.FromSeconds(15), ct);
            using var buffer = new MemoryStream();
            await context.Request.InputStream.CopyToAsync(buffer, ct);
            var headers = context.Request.Headers.AllKeys.Where(k => k is not null)
                .ToDictionary(k => k!.ToLowerInvariant(), k => context.Request.Headers[k]!);
            context.Response.StatusCode = 204;
            context.Response.Close();
            return (headers, buffer.ToArray());
        }

        public void Dispose() => listener.Close();
    }
}
