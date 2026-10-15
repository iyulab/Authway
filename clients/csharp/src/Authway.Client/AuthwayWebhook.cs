using System.Globalization;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;

namespace Authway.Client;

/// <summary>What an event is about, or who caused it.</summary>
/// <param name="Type">For a resource, <c>user</c> or <c>client</c>; for an actor, <c>user</c>, <c>api_key</c>, <c>admin_session</c>, <c>service_client</c> or <c>system</c>.</param>
/// <param name="Id">The id the administration API uses for it (for a user, also the <c>sub</c> of their tokens); null for an actor without one.</param>
public sealed record WebhookEventRef(string Type, string? Id);

/// <summary>
/// One event Authway delivered to a webhook. It names what happened and to what — a receiver that needs the details asks the
/// administration API for them.
/// </summary>
/// <param name="Id">Unique per event and the same on every retry of it: use it to ignore a delivery already processed.</param>
/// <param name="Type">One of <see cref="WebhookEventTypes"/>.</param>
/// <param name="Timestamp">When the event happened.</param>
/// <param name="Resource">What the event is about; null for a <c>test</c> event.</param>
/// <param name="Actor">Who caused it; null for a <c>test</c> event.</param>
/// <param name="Data">The event's <c>data</c> member as sent.</param>
public sealed record WebhookEvent(
    string Id,
    string Type,
    DateTimeOffset Timestamp,
    string TenantId,
    WebhookEventRef? Resource,
    WebhookEventRef? Actor,
    JsonElement Data);

/// <summary>The event types a webhook can subscribe to.</summary>
public static class WebhookEventTypes
{
    public const string UserCreated = "user.created";
    public const string UserUpdated = "user.updated";
    public const string UserDeleted = "user.deleted";
    public const string UserLogin = "user.login";
    public const string UserLogout = "user.logout";
    public const string UserPasswordChanged = "user.password_changed";
    public const string UserMfaEnabled = "user.mfa_enabled";
    public const string UserMfaDisabled = "user.mfa_disabled";
    public const string ClientCreated = "client.created";
    public const string ClientUpdated = "client.updated";
    public const string ClientDeleted = "client.deleted";
    public const string Test = "test";

    /// <summary>Subscribes to every event.</summary>
    public const string All = "*";
}

/// <summary>
/// Verifies webhook deliveries. Authway signs them as Standard Webhooks specifies (https://www.standardwebhooks.com):
/// <c>webhook-id</c>, <c>webhook-timestamp</c> and <c>webhook-signature</c> over the raw body, with the webhook's
/// <c>whsec_</c> secret.
/// </summary>
public static class AuthwayWebhook
{
    /// <summary>The header carrying the event's id.</summary>
    public const string IdHeader = "webhook-id";

    /// <summary>The header carrying when the delivery was signed, in Unix seconds.</summary>
    public const string TimestampHeader = "webhook-timestamp";

    /// <summary>The header carrying the space-separated <c>v1,&lt;base64&gt;</c> signatures.</summary>
    public const string SignatureHeader = "webhook-signature";

    /// <summary>How far a delivery's timestamp may be from the clock: the reference libraries' five minutes.</summary>
    public static readonly TimeSpan DefaultTolerance = TimeSpan.FromMinutes(5);

    /// <summary>
    /// Verifies a delivery and reads its event. Pass the body exactly as received — before any JSON parsing — and a way to
    /// read a request header by name (in ASP.NET Core, <c>name =&gt; Request.Headers[name]</c>).
    /// </summary>
    /// <exception cref="AuthwayException">The delivery is not signed by this secret, is too old, or is not an event.</exception>
    public static WebhookEvent Read(string secret, Func<string, string?> header, ReadOnlySpan<byte> body, TimeSpan? tolerance = null, DateTimeOffset? now = null)
    {
        ArgumentNullException.ThrowIfNull(header);
        if (!Verify(secret, header(IdHeader), header(TimestampHeader), header(SignatureHeader), body, tolerance, now))
        {
            throw new AuthwayException("The webhook delivery is not signed by this secret, or it is too old.");
        }

        try
        {
            var payload = JsonSerializer.Deserialize(body, ClientJsonContext.Default.WebhookPayload)
                ?? throw new AuthwayException("The webhook delivery has no event.");
            return new WebhookEvent(payload.Id, payload.Type, payload.Timestamp, payload.TenantId, payload.Data.Resource(), payload.Data.Actor(), payload.Data);
        }
        catch (JsonException e)
        {
            throw new AuthwayException("The webhook delivery is not an event.", e);
        }
    }

    /// <summary>
    /// Whether the three Standard Webhooks headers sign <paramref name="body"/> under <paramref name="secret"/>, with a
    /// timestamp within <paramref name="tolerance"/> of <paramref name="now"/>.
    /// </summary>
    /// <param name="secret">The webhook's secret, <c>whsec_</c> followed by base64 (the prefix is optional).</param>
    public static bool Verify(string secret, string? webhookId, string? webhookTimestamp, string? webhookSignature, ReadOnlySpan<byte> body, TimeSpan? tolerance = null, DateTimeOffset? now = null)
    {
        if (string.IsNullOrEmpty(webhookId) || string.IsNullOrEmpty(webhookSignature)
            || !long.TryParse(webhookTimestamp, NumberStyles.None, CultureInfo.InvariantCulture, out var timestamp)
            || !TryKey(secret, out var key))
        {
            return false;
        }

        var age = (now ?? DateTimeOffset.UtcNow) - DateTimeOffset.FromUnixTimeSeconds(timestamp);
        var allowed = tolerance ?? DefaultTolerance;
        if (age > allowed || age < -allowed)
        {
            return false;
        }

        var expected = Sign(key, webhookId, timestamp, body);
        Span<byte> given = stackalloc byte[64];
        foreach (var signature in webhookSignature.Split(' ', StringSplitOptions.RemoveEmptyEntries))
        {
            var comma = signature.IndexOf(',', StringComparison.Ordinal);
            if (comma <= 0 || signature[..comma] != "v1")
            {
                continue;
            }

            if (Convert.TryFromBase64String(signature[(comma + 1)..], given, out var written)
                && CryptographicOperations.FixedTimeEquals(given[..written], expected))
            {
                return true;
            }
        }

        return false;
    }

    /// <summary>The <c>webhook-signature</c> value for a body: what Authway sends, for a test of your receiver.</summary>
    public static string Sign(string secret, string webhookId, long webhookTimestamp, ReadOnlySpan<byte> body)
    {
        if (!TryKey(secret, out var key))
        {
            throw new ArgumentException("A webhook secret is whsec_ followed by base64.", nameof(secret));
        }

        return "v1," + Convert.ToBase64String(Sign(key, webhookId, webhookTimestamp, body));
    }

    private static byte[] Sign(byte[] key, string webhookId, long timestamp, ReadOnlySpan<byte> body)
    {
        var prefix = Encoding.UTF8.GetBytes(string.Create(CultureInfo.InvariantCulture, $"{webhookId}.{timestamp}."));
        using var hmac = IncrementalHash.CreateHMAC(HashAlgorithmName.SHA256, key);
        hmac.AppendData(prefix);
        hmac.AppendData(body);
        return hmac.GetHashAndReset();
    }

    private static bool TryKey(string secret, out byte[] key)
    {
        key = [];
        if (string.IsNullOrEmpty(secret))
        {
            return false;
        }

        var encoded = secret.StartsWith("whsec_", StringComparison.Ordinal) ? secret[6..] : secret;
        var buffer = new byte[encoded.Length];
        if (!Convert.TryFromBase64String(encoded, buffer, out var written) || written == 0)
        {
            return false;
        }

        key = buffer[..written];
        return true;
    }
}

internal static class WebhookDataExtensions
{
    public static WebhookEventRef? Resource(this JsonElement data) => Ref(data, "resource");

    public static WebhookEventRef? Actor(this JsonElement data) => Ref(data, "actor");

    private static WebhookEventRef? Ref(JsonElement data, string name) =>
        data.ValueKind == JsonValueKind.Object && data.TryGetProperty(name, out var value) && value.ValueKind == JsonValueKind.Object
            && value.TryGetProperty("type", out var type) && type.ValueKind == JsonValueKind.String
            ? new WebhookEventRef(type.GetString()!, value.TryGetProperty("id", out var id) && id.ValueKind == JsonValueKind.String ? id.GetString() : null)
            : null;
}
