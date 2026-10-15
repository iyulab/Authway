using System.Globalization;
using System.Net;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization.Metadata;

namespace Authway.Client;

/// <summary>
/// Authway's administration API, for a service's backend. It calls with the deployment's admin key or a tenant service
/// credential's token, for one tenant. Signing users in is not here: that is OpenID Connect, which your framework's
/// handler does against Authway's discovery document.
/// </summary>
public sealed class AuthwayClient
{
    private readonly HttpClient http;
    private readonly Uri root;
    private readonly Func<CancellationToken, ValueTask<string>> credential;

    /// <param name="httpClient">A client whose <see cref="HttpClient.BaseAddress"/> is the Authway API.</param>
    /// <param name="credential">The admin key, or a tenant service credential's access token.</param>
    /// <param name="tenantId">The tenant every call is for; required with the admin key, implied by a service credential.</param>
    public AuthwayClient(HttpClient httpClient, string credential, string? tenantId = null)
        : this(httpClient, Fixed(credential), tenantId)
    {
    }

    /// <param name="httpClient">A client whose <see cref="HttpClient.BaseAddress"/> is the Authway API.</param>
    /// <param name="credential">Called before each call for the credential to send — for a token that is refreshed.</param>
    /// <param name="tenantId">The tenant every call is for; required with the admin key, implied by a service credential.</param>
    public AuthwayClient(HttpClient httpClient, Func<CancellationToken, ValueTask<string>> credential, string? tenantId = null)
    {
        ArgumentNullException.ThrowIfNull(httpClient);
        ArgumentNullException.ThrowIfNull(credential);
        if (httpClient.BaseAddress is null)
        {
            throw new ArgumentException("The client needs the Authway API as its base address.", nameof(httpClient));
        }

        http = httpClient;
        var address = httpClient.BaseAddress.ToString();
        root = new Uri(address.EndsWith('/') ? address : address + "/");
        this.credential = credential;
        TenantId = tenantId;
        Clients = new ClientsApi(this);
        Invitations = new InvitationsApi(this);
        Users = new UsersApi(this);
        Webhooks = new WebhooksApi(this);
        Terms = new TermsApi(this);
    }

    /// <summary>The tenant every call is for, sent as <c>X-Tenant-ID</c>; null to let the credential imply it.</summary>
    public string? TenantId { get; }

    /// <summary>Applications registered with the tenant.</summary>
    public ClientsApi Clients { get; }

    /// <summary>Invitations to join the tenant.</summary>
    public InvitationsApi Invitations { get; }

    /// <summary>The tenant's users, their claims and what they accepted.</summary>
    public UsersApi Users { get; }

    /// <summary>Endpoints that receive the tenant's events. To check a delivery, see <see cref="AuthwayWebhook"/>.</summary>
    public WebhooksApi Webhooks { get; }

    /// <summary>The documents the tenant's users accept at sign-in.</summary>
    public TermsApi Terms { get; }

    internal string RequireTenant() =>
        TenantId ?? throw new InvalidOperationException("This call names the tenant: create the client with a tenant id.");

    internal async Task<T> SendAsync<T>(HttpMethod method, string path, HttpContent? content, JsonTypeInfo<T> type, CancellationToken cancellationToken)
    {
        using var response = await SendAsync(method, path, content, cancellationToken).ConfigureAwait(false);
        var stream = await response.Content.ReadAsStreamAsync(cancellationToken).ConfigureAwait(false);
        await using (stream.ConfigureAwait(false))
        {
            try
            {
                return await JsonSerializer.DeserializeAsync(stream, type, cancellationToken).ConfigureAwait(false)
                    ?? throw new AuthwayException(response.StatusCode, "Authway answered with an empty body.", null);
            }
            catch (JsonException e)
            {
                throw new AuthwayException($"Authway answered {(int)response.StatusCode} with a body this client could not read.", e);
            }
        }
    }

    /// <summary>Sends a call and returns its answer when it succeeded; throws <see cref="AuthwayException"/> otherwise.</summary>
    internal async Task<HttpResponseMessage> SendAsync(HttpMethod method, string path, HttpContent? content, CancellationToken cancellationToken)
    {
        using var request = new HttpRequestMessage(method, new Uri(root, path)) { Content = content };
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", await credential(cancellationToken).ConfigureAwait(false));
        if (TenantId is not null)
        {
            request.Headers.Add("X-Tenant-ID", TenantId);
        }

        var response = await http.SendAsync(request, cancellationToken).ConfigureAwait(false);
        if (response.IsSuccessStatusCode)
        {
            return response;
        }

        using (response)
        {
            var body = await response.Content.ReadAsByteArrayAsync(cancellationToken).ConfigureAwait(false);
            if (AuthwayProblem.TryParse(body, out var problem))
            {
                throw new AuthwayException(response.StatusCode, problem.Detail.Length > 0 ? problem.Detail : problem.Code, problem);
            }

            throw new AuthwayException(response.StatusCode, $"Authway answered {(int)response.StatusCode} {response.ReasonPhrase}.", null);
        }
    }

    internal async Task SendAsync(HttpMethod method, string path, CancellationToken cancellationToken)
    {
        using var _ = await SendAsync(method, path, null, cancellationToken).ConfigureAwait(false);
    }

    internal static StringContent Json<T>(T value, JsonTypeInfo<T> type) =>
        new(JsonSerializer.Serialize(value, type), Encoding.UTF8, "application/json");

    internal static string Id(string id)
    {
        ArgumentException.ThrowIfNullOrEmpty(id);
        return Uri.EscapeDataString(id);
    }

    internal static string Query(params (string Name, string? Value)[] parameters)
    {
        var query = string.Join('&', parameters
            .Where(parameter => parameter.Value is not null)
            .Select(parameter => $"{parameter.Name}={Uri.EscapeDataString(parameter.Value!)}"));
        return query.Length == 0 ? "" : "?" + query;
    }

    internal static string? Text(int? value) => value?.ToString(CultureInfo.InvariantCulture);

    private static Func<CancellationToken, ValueTask<string>> Fixed(string credential)
    {
        ArgumentException.ThrowIfNullOrEmpty(credential);
        return _ => ValueTask.FromResult(credential);
    }
}

/// <summary>Applications registered with the tenant (OAuth 2.0 clients).</summary>
public sealed class ClientsApi
{
    private readonly AuthwayClient client;

    internal ClientsApi(AuthwayClient client) => this.client = client;

    /// <summary>A page of the tenant's applications.</summary>
    /// <param name="limit">1 to 100; Authway's default is 20.</param>
    public async Task<Page<OAuthClient>> ListAsync(int? limit = null, int? offset = null, CancellationToken cancellationToken = default)
    {
        var list = await client.SendAsync(HttpMethod.Get,
            "api/v1/clients" + AuthwayClient.Query(("tenant_id", client.TenantId), ("limit", AuthwayClient.Text(limit)), ("offset", AuthwayClient.Text(offset))),
            null, ClientJsonContext.Default.ClientList, cancellationToken).ConfigureAwait(false);
        return new Page<OAuthClient>(list.Clients, list.Total, list.Limit, list.Offset);
    }

    /// <summary>An application, by <see cref="OAuthClient.Id"/>.</summary>
    public async Task<OAuthClient> GetAsync(string id, CancellationToken cancellationToken = default) =>
        (await client.SendAsync(HttpMethod.Get, "api/v1/clients/" + AuthwayClient.Id(id), null, ClientJsonContext.Default.ClientBody, cancellationToken)
            .ConfigureAwait(false)).Client;

    /// <summary>Registers an application. A confidential one comes back with its secret, shown this once.</summary>
    public async Task<RegisteredClient> CreateAsync(NewOAuthClient application, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(application);
        var body = new NewClientBody(
            application.TenantId ?? client.RequireTenant(), application.Name, application.GrantTypes, application.Scopes, application.Public,
            application.RedirectUris, application.PostLogoutRedirectUris, application.AllowedOrigins, application.Audience,
            application.TokenEndpointAuthMethod, application.Jwks, application.JwksUri, application.InitiateLoginUri, application.ClientUri,
            application.BackchannelLogoutUri, application.EnabledAuthProviders, application.AccessTokenStrategy);
        var created = await client.SendAsync(HttpMethod.Post, "api/v1/clients",
            AuthwayClient.Json(body, ClientJsonContext.Default.NewClientBody), ClientJsonContext.Default.CreatedClientBody, cancellationToken)
            .ConfigureAwait(false);
        return new RegisteredClient(created.Client, created.Credentials is { ClientSecret.Length: > 0 } ? created.Credentials : null);
    }

    /// <summary>
    /// Changes an application. Saved even when the authorization server did not take the change — see
    /// <see cref="UpdatedClient.SyncStatus"/>; pass <paramref name="strictSync"/> to be refused instead (502 <c>upstream_sync_failed</c>).
    /// </summary>
    public async Task<UpdatedClient> UpdateAsync(string id, OAuthClientChanges changes, bool strictSync = false, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(changes);
        var updated = await client.SendAsync(HttpMethod.Put,
            "api/v1/clients/" + AuthwayClient.Id(id) + (strictSync ? "?strict_sync=true" : ""),
            AuthwayClient.Json(changes, ClientJsonContext.Default.OAuthClientChanges), ClientJsonContext.Default.UpdatedClientBody, cancellationToken)
            .ConfigureAwait(false);
        return new UpdatedClient(updated.Client, updated.SyncStatus);
    }

    /// <summary>Deletes an application.</summary>
    public Task DeleteAsync(string id, CancellationToken cancellationToken = default) =>
        client.SendAsync(HttpMethod.Delete, "api/v1/clients/" + AuthwayClient.Id(id), cancellationToken);
}

/// <summary>Invitations to join the tenant. An invited person creates their account by accepting.</summary>
public sealed class InvitationsApi
{
    private readonly AuthwayClient client;

    internal InvitationsApi(AuthwayClient client) => this.client = client;

    /// <summary>Invites a person by email.</summary>
    /// <param name="clientId">
    /// The OAuth <c>client_id</c> of the application the invitation is for: once accepted, the person is sent on to it.
    /// </param>
    /// <exception cref="AuthwayException">
    /// Code <c>user_already_member</c> or <c>invitation_already_pending</c> (409), <c>email_domain_not_allowed</c> or
    /// <c>unknown_client</c> (400).
    /// </exception>
    public async Task<SentInvitation> CreateAsync(string email, string? clientId = null, string? message = null, string? role = null, CancellationToken cancellationToken = default)
    {
        ArgumentException.ThrowIfNullOrEmpty(email);
        return (await client.SendAsync(HttpMethod.Post, "api/v1/invitations",
            AuthwayClient.Json(new NewInvitationBody(email, role, message, clientId), ClientJsonContext.Default.NewInvitationBody),
            ClientJsonContext.Default.SentInvitationBody, cancellationToken).ConfigureAwait(false)).Invitation;
    }

    /// <summary>A page of the tenant's invitations, newest first.</summary>
    /// <param name="status"><c>pending</c>, <c>accepted</c>, <c>expired</c> or <c>revoked</c>; null for all.</param>
    public async Task<Page<Invitation>> ListAsync(string? status = null, int? limit = null, int? offset = null, CancellationToken cancellationToken = default)
    {
        var list = await client.SendAsync(HttpMethod.Get,
            "api/v1/invitations" + AuthwayClient.Query(("status", status), ("limit", AuthwayClient.Text(limit)), ("offset", AuthwayClient.Text(offset))),
            null, ClientJsonContext.Default.InvitationList, cancellationToken).ConfigureAwait(false);
        return new Page<Invitation>(list.Invitations, list.Total, list.Limit, list.Offset);
    }

    /// <summary>An invitation, by id.</summary>
    public async Task<Invitation> GetAsync(string id, CancellationToken cancellationToken = default) =>
        (await client.SendAsync(HttpMethod.Get, "api/v1/invitations/" + AuthwayClient.Id(id), null, ClientJsonContext.Default.InvitationBody, cancellationToken)
            .ConfigureAwait(false)).Invitation;

    /// <summary>Withdraws a pending invitation; its link stops working.</summary>
    public Task RevokeAsync(string id, CancellationToken cancellationToken = default) =>
        client.SendAsync(HttpMethod.Delete, "api/v1/invitations/" + AuthwayClient.Id(id), cancellationToken);

    /// <summary>Sends a pending invitation's email again.</summary>
    public Task ResendAsync(string id, CancellationToken cancellationToken = default) =>
        client.SendAsync(HttpMethod.Post, "api/v1/invitations/" + AuthwayClient.Id(id) + "/resend", cancellationToken);
}

/// <summary>The tenant's users.</summary>
public sealed class UsersApi
{
    private readonly AuthwayClient client;

    internal UsersApi(AuthwayClient client) => this.client = client;

    /// <summary>A page of the tenant's users.</summary>
    public async Task<Page<User>> ListAsync(int? limit = null, int? offset = null, CancellationToken cancellationToken = default)
    {
        var list = await client.SendAsync(HttpMethod.Get,
            "api/v1/users" + AuthwayClient.Query(("tenant_id", client.TenantId), ("limit", AuthwayClient.Text(limit)), ("offset", AuthwayClient.Text(offset))),
            null, ClientJsonContext.Default.UserList, cancellationToken).ConfigureAwait(false);
        return new Page<User>(list.Users, list.Total, list.Limit, list.Offset);
    }

    /// <summary>A user, by id — the <c>sub</c> of their tokens.</summary>
    public async Task<User> GetAsync(string id, CancellationToken cancellationToken = default) =>
        (await client.SendAsync(HttpMethod.Get, "api/v1/users/" + AuthwayClient.Id(id), null, ClientJsonContext.Default.UserBody, cancellationToken)
            .ConfigureAwait(false)).User;

    /// <summary>The claims stored for a user — what Authway adds to their tokens beside its own.</summary>
    public async Task<JsonElement> GetClaimsAsync(string id, CancellationToken cancellationToken = default) =>
        (await client.SendAsync(HttpMethod.Get, "api/v1/users/" + AuthwayClient.Id(id) + "/claims", null, ClientJsonContext.Default.ClaimsBody, cancellationToken)
            .ConfigureAwait(false)).Claims;

    /// <summary>
    /// Merges claims into those stored for a user and returns what is stored afterwards. A member set to null removes that
    /// claim. Claims Authway itself issues (<c>sub</c>, <c>tenant_id</c>, <c>email</c>, …) are refused (<c>reserved_claim</c>).
    /// </summary>
    /// <param name="claims">A JSON object, such as <c>JsonSerializer.SerializeToElement(new { roles = new[] { "editor" } })</c>.</param>
    public async Task<JsonElement> PatchClaimsAsync(string id, JsonElement claims, CancellationToken cancellationToken = default)
    {
        if (claims.ValueKind != JsonValueKind.Object)
        {
            throw new ArgumentException("Claims are a JSON object.", nameof(claims));
        }

        return (await client.SendAsync(HttpMethod.Patch, "api/v1/users/" + AuthwayClient.Id(id) + "/claims",
            AuthwayClient.Json(new ClaimsBody(claims), ClientJsonContext.Default.ClaimsBody), ClientJsonContext.Default.ClaimsBody, cancellationToken)
            .ConfigureAwait(false)).Claims;
    }

    /// <summary>The versions of the tenant's documents a user accepted, and when.</summary>
    public async Task<IReadOnlyList<TermsAcceptance>> GetTermsAcceptancesAsync(string id, CancellationToken cancellationToken = default) =>
        (await client.SendAsync(HttpMethod.Get, "api/v1/users/" + AuthwayClient.Id(id) + "/terms-acceptances", null,
            ClientJsonContext.Default.AcceptancesBody, cancellationToken).ConfigureAwait(false)).Acceptances;
}

/// <summary>Endpoints that receive the tenant's events.</summary>
public sealed class WebhooksApi
{
    private readonly AuthwayClient client;

    internal WebhooksApi(AuthwayClient client) => this.client = client;

    /// <summary>The tenant's webhooks.</summary>
    public async Task<IReadOnlyList<Webhook>> ListAsync(CancellationToken cancellationToken = default) =>
        (await client.SendAsync(HttpMethod.Get, "api/v1/webhooks", null, ClientJsonContext.Default.WebhookList, cancellationToken).ConfigureAwait(false)).Webhooks;

    /// <summary>A webhook, by id.</summary>
    public async Task<Webhook> GetAsync(string id, CancellationToken cancellationToken = default) =>
        (await client.SendAsync(HttpMethod.Get, "api/v1/webhooks/" + AuthwayClient.Id(id), null, ClientJsonContext.Default.WebhookBody, cancellationToken)
            .ConfigureAwait(false)).Webhook;

    /// <summary>Registers an endpoint. The answer carries its signing secret — the only time it is shown.</summary>
    public Task<CreatedWebhook> CreateAsync(NewWebhook webhook, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(webhook);
        return client.SendAsync(HttpMethod.Post, "api/v1/webhooks", AuthwayClient.Json(webhook, ClientJsonContext.Default.NewWebhook),
            ClientJsonContext.Default.CreatedWebhook, cancellationToken);
    }

    /// <summary>Changes a webhook.</summary>
    public async Task<Webhook> UpdateAsync(string id, WebhookChanges changes, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(changes);
        return (await client.SendAsync(HttpMethod.Patch, "api/v1/webhooks/" + AuthwayClient.Id(id),
            AuthwayClient.Json(changes, ClientJsonContext.Default.WebhookChanges), ClientJsonContext.Default.WebhookBody, cancellationToken)
            .ConfigureAwait(false)).Webhook;
    }

    /// <summary>Deletes a webhook.</summary>
    public Task DeleteAsync(string id, CancellationToken cancellationToken = default) =>
        client.SendAsync(HttpMethod.Delete, "api/v1/webhooks/" + AuthwayClient.Id(id), cancellationToken);

    /// <summary>Issues a new signing secret; the previous one stops signing at once.</summary>
    public async Task<string> RotateSecretAsync(string id, CancellationToken cancellationToken = default) =>
        (await client.SendAsync(HttpMethod.Post, "api/v1/webhooks/" + AuthwayClient.Id(id) + "/rotate-secret", null, ClientJsonContext.Default.SecretBody,
            cancellationToken).ConfigureAwait(false)).Secret;

    /// <summary>Sends one <c>test</c> event to the webhook and returns how the attempt went.</summary>
    public async Task<WebhookDelivery> TestAsync(string id, CancellationToken cancellationToken = default) =>
        (await client.SendAsync(HttpMethod.Post, "api/v1/webhooks/" + AuthwayClient.Id(id) + "/test", null, ClientJsonContext.Default.DeliveryBody,
            cancellationToken).ConfigureAwait(false)).Delivery;

    /// <summary>The webhook's recent delivery attempts, newest first.</summary>
    public async Task<IReadOnlyList<WebhookDelivery>> GetDeliveriesAsync(string id, int? limit = null, CancellationToken cancellationToken = default) =>
        (await client.SendAsync(HttpMethod.Get, "api/v1/webhooks/" + AuthwayClient.Id(id) + "/deliveries" + AuthwayClient.Query(("limit", AuthwayClient.Text(limit))),
            null, ClientJsonContext.Default.DeliveryList, cancellationToken).ConfigureAwait(false)).Deliveries;
}

/// <summary>The documents — terms of service, a privacy policy — the tenant's users accept at sign-in.</summary>
public sealed class TermsApi
{
    private readonly AuthwayClient client;

    internal TermsApi(AuthwayClient client) => this.client = client;

    /// <summary>The current version of each of the tenant's documents.</summary>
    public async Task<IReadOnlyList<TermsDocument>> ListAsync(CancellationToken cancellationToken = default) =>
        (await client.SendAsync(HttpMethod.Get, $"api/v1/tenants/{AuthwayClient.Id(client.RequireTenant())}/terms", null, ClientJsonContext.Default.TermsList,
            cancellationToken).ConfigureAwait(false)).Terms;

    /// <summary>
    /// Makes <paramref name="version"/> the current version of a document. From the next sign-in, a user who has not
    /// accepted it is asked to. Publishing the current version again only changes its address or title.
    /// </summary>
    /// <param name="kind">Which document, such as <c>terms_of_service</c>.</param>
    /// <param name="url">Where the application publishes this version.</param>
    public Task<TermsDocument> PublishAsync(string kind, string version, string url, string? title = null, CancellationToken cancellationToken = default)
    {
        ArgumentException.ThrowIfNullOrEmpty(version);
        ArgumentException.ThrowIfNullOrEmpty(url);
        return client.SendAsync(HttpMethod.Put, $"api/v1/tenants/{AuthwayClient.Id(client.RequireTenant())}/terms/{AuthwayClient.Id(kind)}",
            AuthwayClient.Json(new PublishTermsBody(version, url, title), ClientJsonContext.Default.PublishTermsBody), ClientJsonContext.Default.TermsDocument,
            cancellationToken);
    }

    /// <summary>Stops asking users to accept a document. What they accepted stays recorded.</summary>
    public Task RetireAsync(string kind, CancellationToken cancellationToken = default) =>
        client.SendAsync(HttpMethod.Delete, $"api/v1/tenants/{AuthwayClient.Id(client.RequireTenant())}/terms/{AuthwayClient.Id(kind)}", cancellationToken);
}
