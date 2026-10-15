using System.Text.Json;
using System.Text.Json.Serialization;

namespace Authway.Client;

internal sealed record ClientList(IReadOnlyList<OAuthClient> Clients, int Total, int Limit, int Offset);

internal sealed record ClientBody(OAuthClient Client);

internal sealed record CreatedClientBody(OAuthClient Client, ClientCredentials? Credentials);

internal sealed record UpdatedClientBody(OAuthClient Client, SyncStatus SyncStatus);

internal sealed record InvitationList(IReadOnlyList<Invitation> Invitations, int Total, int Limit, int Offset);

internal sealed record InvitationBody(Invitation Invitation);

internal sealed record NewInvitationBody(string Email, string? Role, string? Message, string? ClientId);

internal sealed record SentInvitationBody(SentInvitation Invitation);

internal sealed record UserList(IReadOnlyList<User> Users, int Total, int Limit, int Offset);

internal sealed record UserBody(User User);

internal sealed record ClaimsBody(JsonElement Claims);

internal sealed record AcceptancesBody(IReadOnlyList<TermsAcceptance> Acceptances);

internal sealed record TermsList(IReadOnlyList<TermsDocument> Terms);

internal sealed record PublishTermsBody(string Version, string Url, string? Title);

internal sealed record WebhookList(IReadOnlyList<Webhook> Webhooks);

internal sealed record WebhookBody(Webhook Webhook);

internal sealed record DeliveryBody(WebhookDelivery Delivery);

internal sealed record DeliveryList(IReadOnlyList<WebhookDelivery> Deliveries);

internal sealed record SecretBody(string Secret);

internal sealed record WebhookPayload(string Id, string Type, DateTimeOffset Timestamp, string TenantId, JsonElement Data);

internal sealed record NewClientBody(
    string TenantId,
    string Name,
    IReadOnlyList<string> GrantTypes,
    IReadOnlyList<string> Scopes,
    bool? Public,
    IReadOnlyList<string>? RedirectUris,
    IReadOnlyList<string>? PostLogoutRedirectUris,
    IReadOnlyList<string>? AllowedOrigins,
    IReadOnlyList<string>? Audience,
    string? TokenEndpointAuthMethod,
    JsonElement? Jwks,
    string? JwksUri,
    string? InitiateLoginUri,
    string? ClientUri,
    string? BackchannelLogoutUri,
    IReadOnlyList<string>? EnabledAuthProviders,
    string? AccessTokenStrategy);

[JsonSourceGenerationOptions(
    PropertyNamingPolicy = JsonKnownNamingPolicy.SnakeCaseLower,
    DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull)]
[JsonSerializable(typeof(ClientList))]
[JsonSerializable(typeof(ClientBody))]
[JsonSerializable(typeof(CreatedClientBody))]
[JsonSerializable(typeof(UpdatedClientBody))]
[JsonSerializable(typeof(NewClientBody))]
[JsonSerializable(typeof(OAuthClientChanges))]
[JsonSerializable(typeof(InvitationList))]
[JsonSerializable(typeof(InvitationBody))]
[JsonSerializable(typeof(NewInvitationBody))]
[JsonSerializable(typeof(SentInvitationBody))]
[JsonSerializable(typeof(UserList))]
[JsonSerializable(typeof(UserBody))]
[JsonSerializable(typeof(ClaimsBody))]
[JsonSerializable(typeof(AcceptancesBody))]
[JsonSerializable(typeof(TermsList))]
[JsonSerializable(typeof(TermsDocument))]
[JsonSerializable(typeof(PublishTermsBody))]
[JsonSerializable(typeof(WebhookList))]
[JsonSerializable(typeof(WebhookBody))]
[JsonSerializable(typeof(CreatedWebhook))]
[JsonSerializable(typeof(NewWebhook))]
[JsonSerializable(typeof(WebhookChanges))]
[JsonSerializable(typeof(DeliveryBody))]
[JsonSerializable(typeof(DeliveryList))]
[JsonSerializable(typeof(SecretBody))]
[JsonSerializable(typeof(WebhookPayload))]
internal sealed partial class ClientJsonContext : JsonSerializerContext;
