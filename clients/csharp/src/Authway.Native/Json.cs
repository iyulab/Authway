using System.Text.Json;
using System.Text.Json.Serialization;

namespace Authway.Native;

internal sealed record DiscoveryDocument(
    string Issuer,
    string AuthorizationEndpoint,
    string TokenEndpoint,
    string? RevocationEndpoint);

internal sealed record TokenResponse(
    string AccessToken,
    long? ExpiresIn,
    string? RefreshToken,
    string? IdToken);

/// <summary>
/// What is kept in the <see cref="ITokenStore"/> between runs: the refresh token and the ID token's claims. The access
/// token stays in the process — it is renewed from the refresh token, and leaving it out keeps the value small enough for
/// one OS credential entry. Only a session without a refresh token keeps its access token (and <see cref="ExpiresAt"/>,
/// Unix seconds), since nothing else could resume it.
/// </summary>
internal sealed record StoredSession(
    string? AccessToken,
    long? ExpiresAt,
    string? RefreshToken,
    Dictionary<string, JsonElement> Claims);

[JsonSourceGenerationOptions(
    PropertyNamingPolicy = JsonKnownNamingPolicy.SnakeCaseLower,
    DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull)]
[JsonSerializable(typeof(DiscoveryDocument))]
[JsonSerializable(typeof(TokenResponse))]
[JsonSerializable(typeof(StoredSession))]
[JsonSerializable(typeof(Dictionary<string, JsonElement>))]
internal sealed partial class NativeJsonContext : JsonSerializerContext;
