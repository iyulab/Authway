using System.Text.Json;

namespace Authway.Client;

/// <summary>An application registered with Authway — an OAuth 2.0 client.</summary>
/// <param name="Id">Authway's id for the registration, used in administration paths.</param>
/// <param name="ClientId">The OAuth <c>client_id</c>.</param>
/// <param name="TokenEndpointAuthMethod"><c>client_secret_post</c>, <c>private_key_jwt</c> or <c>none</c>.</param>
/// <param name="Jwks">The client's public key set, when it authenticates with a key registered here.</param>
/// <param name="Audience">The resource servers its access tokens are for (their <c>aud</c>).</param>
/// <param name="AccessTokenStrategy"><c>jwt</c>, <c>opaque</c>, or empty for the deployment's default.</param>
public sealed record OAuthClient(
    string Id,
    string TenantId,
    string ClientId,
    string Name,
    bool Public,
    IReadOnlyList<string> GrantTypes,
    IReadOnlyList<string> Scopes,
    string TokenEndpointAuthMethod,
    IReadOnlyList<string>? RedirectUris = null,
    IReadOnlyList<string>? PostLogoutRedirectUris = null,
    IReadOnlyList<string>? AllowedOrigins = null,
    IReadOnlyList<string>? Audience = null,
    JsonElement? Jwks = null,
    string? JwksUri = null,
    string? InitiateLoginUri = null,
    string? ClientUri = null,
    string? BackchannelLogoutUri = null,
    IReadOnlyList<string>? EnabledAuthProviders = null,
    bool? Active = null,
    bool? SkipConsent = null,
    bool? SkipLogoutConsent = null,
    string? AccessTokenStrategy = null);

/// <summary>An application to register.</summary>
/// <param name="TenantId">The tenant it belongs to; null for the tenant this client calls for.</param>
/// <param name="EnabledAuthProviders">Sign-in methods to offer: <c>email</c>, <c>magic_link</c>, <c>google</c>, <c>github</c>, <c>microsoft</c>, <c>apple</c>.</param>
public sealed record NewOAuthClient(
    string Name,
    IReadOnlyList<string> GrantTypes,
    IReadOnlyList<string> Scopes,
    string? TenantId = null,
    bool? Public = null,
    IReadOnlyList<string>? RedirectUris = null,
    IReadOnlyList<string>? PostLogoutRedirectUris = null,
    IReadOnlyList<string>? AllowedOrigins = null,
    IReadOnlyList<string>? Audience = null,
    string? TokenEndpointAuthMethod = null,
    JsonElement? Jwks = null,
    string? JwksUri = null,
    string? InitiateLoginUri = null,
    string? ClientUri = null,
    string? BackchannelLogoutUri = null,
    IReadOnlyList<string>? EnabledAuthProviders = null,
    string? AccessTokenStrategy = null);

/// <summary>Changes to a registered application; a null member is left as it is.</summary>
public sealed record OAuthClientChanges(
    string? Name = null,
    string? Description = null,
    bool? Public = null,
    bool? Active = null,
    IReadOnlyList<string>? RedirectUris = null,
    IReadOnlyList<string>? PostLogoutRedirectUris = null,
    IReadOnlyList<string>? AllowedOrigins = null,
    IReadOnlyList<string>? Audience = null,
    string? TokenEndpointAuthMethod = null,
    JsonElement? Jwks = null,
    string? JwksUri = null,
    string? InitiateLoginUri = null,
    string? ClientUri = null,
    string? BackchannelLogoutUri = null,
    IReadOnlyList<string>? GrantTypes = null,
    IReadOnlyList<string>? Scopes = null,
    IReadOnlyList<string>? EnabledAuthProviders = null,
    bool? SkipConsent = null,
    bool? SkipLogoutConsent = null,
    string? AccessTokenStrategy = null);

/// <summary>The client secret of a confidential application — shown once.</summary>
public sealed record ClientCredentials(string ClientId, string ClientSecret);

/// <summary>A registered application, with its secret when it has one.</summary>
public sealed record RegisteredClient(OAuthClient Client, ClientCredentials? Credentials);

/// <summary>Whether the authorization server accepted a change: <c>ok</c>, <c>failed</c> or <c>skipped</c>.</summary>
public sealed record SyncStatus(string State, string? Error = null);

/// <summary>A changed application, and whether the authorization server took the change.</summary>
public sealed record UpdatedClient(OAuthClient Client, SyncStatus SyncStatus);

/// <summary>A page of a list.</summary>
public sealed record Page<T>(IReadOnlyList<T> Items, int Total, int Limit, int Offset);

/// <summary>The application an invitation is for.</summary>
public sealed record InvitationApp(string ClientId, string Name, string? InitiateLoginUri = null, string? ClientUri = null);

/// <summary>An invitation to join a tenant.</summary>
/// <param name="Status"><c>pending</c>, <c>accepted</c>, <c>expired</c> or <c>revoked</c>.</param>
public sealed record Invitation(
    string Id,
    string TenantId,
    string Email,
    string Status,
    DateTimeOffset ExpiresAt,
    string? Role = null,
    string? Message = null,
    string? InviterId = null,
    string? InviterName = null,
    string? TenantName = null,
    DateTimeOffset? AcceptedAt = null,
    string? AcceptedBy = null,
    InvitationApp? Client = null,
    DateTimeOffset? CreatedAt = null,
    DateTimeOffset? UpdatedAt = null);

/// <summary>An invitation just sent.</summary>
public sealed record SentInvitation(string Id, string Email, string Status, DateTimeOffset ExpiresAt);

/// <summary>A user of the tenant. <see cref="Id"/> is also the <c>sub</c> of their tokens.</summary>
public sealed record User(
    string Id,
    string TenantId,
    string Email,
    string? Name = null,
    string? AvatarUrl = null,
    bool? EmailVerified = null,
    bool? Active = null,
    string? Provider = null,
    bool? TotpEnabled = null,
    DateTimeOffset? LastLoginAt = null,
    DateTimeOffset? CreatedAt = null,
    DateTimeOffset? UpdatedAt = null);

/// <summary>A version of a document the user accepted at sign-in.</summary>
/// <param name="ClientId">The application the user was signing in to.</param>
public sealed record TermsAcceptance(string Kind, string Version, DateTimeOffset AcceptedAt, string? ClientId = null);

/// <summary>A document a tenant's users accept at sign-in, in its current version.</summary>
/// <param name="Kind">Which document, such as <c>terms_of_service</c> or <c>privacy_policy</c>.</param>
/// <param name="Url">Where the application publishes it.</param>
/// <param name="Title">What to call it on screen; null means the screen names it from the kind.</param>
public sealed record TermsDocument(string Kind, string Version, string Url, string? Title = null, DateTimeOffset? PublishedAt = null);

/// <summary>An endpoint that receives a tenant's events.</summary>
/// <param name="RetryCount">Attempts after the first.</param>
public sealed record Webhook(
    string Id,
    string TenantId,
    string Name,
    string Url,
    IReadOnlyList<string> Events,
    bool Enabled,
    int RetryCount,
    int TimeoutSecs,
    DateTimeOffset CreatedAt,
    DateTimeOffset UpdatedAt);

/// <summary>An endpoint to register. A null member takes its default.</summary>
/// <param name="Events">What to send — <see cref="WebhookEventTypes"/>.</param>
/// <param name="RetryCount">0 to 10 attempts after the first; default 3.</param>
/// <param name="TimeoutSecs">1 to 60 seconds per attempt; default 30.</param>
public sealed record NewWebhook(string Name, string Url, IReadOnlyList<string> Events, bool? Enabled = null, int? RetryCount = null, int? TimeoutSecs = null);

/// <summary>Changes to a webhook; a null member is left as it is.</summary>
public sealed record WebhookChanges(string? Name = null, string? Url = null, IReadOnlyList<string>? Events = null, bool? Enabled = null, int? RetryCount = null, int? TimeoutSecs = null);

/// <summary>A registered webhook and its signing secret — the only time the secret is shown.</summary>
public sealed record CreatedWebhook(Webhook Webhook, string Secret);

/// <summary>One attempt to deliver an event.</summary>
/// <param name="StatusCode">The receiver's answer; 0 when none arrived.</param>
/// <param name="Payload">The JSON body that was sent.</param>
public sealed record WebhookDelivery(
    string Id,
    string WebhookId,
    string EventType,
    string Payload,
    int StatusCode,
    int Attempt,
    DateTimeOffset DeliveredAt,
    bool Success,
    string? ResponseBody = null,
    string? ErrorMessage = null);
