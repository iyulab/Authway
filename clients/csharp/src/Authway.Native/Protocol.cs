using System.Security.Cryptography;
using System.Text;
using System.Text.Json;

namespace Authway.Native;

/// <summary>The pure parts: random values, PKCE, the loopback request, the ID token's claims.</summary>
internal static class Protocol
{
    /// <summary>How far a clock may be off before an expired ID token is refused.</summary>
    private const long LeewaySeconds = 300;

    /// <summary>32 random bytes, base64url without padding: 43 unreserved characters (a PKCE verifier, state, nonce).</summary>
    public static string RandomToken() => Base64Url(RandomNumberGenerator.GetBytes(32));

    /// <summary>The <c>S256</c> challenge for a verifier (RFC 7636 §4.2).</summary>
    public static string ChallengeFor(string verifier) => Base64Url(SHA256.HashData(Encoding.ASCII.GetBytes(verifier)));

    private static string Base64Url(byte[] bytes) =>
        Convert.ToBase64String(bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_');

    /// <summary>What a request line to the loopback port carries, if it is this sign-in's redirect.</summary>
    public abstract record Callback
    {
        public sealed record Code(string Value) : Callback;

        public sealed record Denied(string Error, string? Description) : Callback;
    }

    /// <summary>
    /// Reads one request line — <c>GET /callback?code=…&amp;state=… HTTP/1.1</c> — and returns what it carries when it is
    /// on the registered path with this sign-in's state; <see langword="null"/> for anything else.
    /// </summary>
    public static Callback? ParseRequestLine(string line, string path, string state)
    {
        var parts = line.Split(' ', StringSplitOptions.RemoveEmptyEntries);
        if (parts.Length < 2 || parts[0] != "GET")
        {
            return null;
        }
        var target = parts[1];
        var question = target.IndexOf('?', StringComparison.Ordinal);
        var targetPath = question < 0 ? target : target[..question];
        if (targetPath != path)
        {
            return null;
        }
        var query = question < 0 ? [] : ParseQuery(target[(question + 1)..]);
        if (!query.TryGetValue("state", out var got) || got != state)
        {
            return null;
        }
        if (query.TryGetValue("error", out var error))
        {
            return new Callback.Denied(error, query.GetValueOrDefault("error_description"));
        }
        return query.TryGetValue("code", out var code) && code.Length > 0 ? new Callback.Code(code) : null;
    }

    private static Dictionary<string, string> ParseQuery(string query)
    {
        var pairs = new Dictionary<string, string>(StringComparer.Ordinal);
        foreach (var pair in query.Split('&', StringSplitOptions.RemoveEmptyEntries))
        {
            var eq = pair.IndexOf('=', StringComparison.Ordinal);
            var key = Decode(eq < 0 ? pair : pair[..eq]);
            pairs.TryAdd(key, eq < 0 ? "" : Decode(pair[(eq + 1)..]));
        }
        return pairs;
    }

    private static string Decode(string value) => Uri.UnescapeDataString(value.Replace('+', ' '));

    /// <summary>
    /// The claims of an ID token received from the token endpoint, after checking it was issued by this issuer, to this
    /// client, for this sign-in, and has not expired.
    /// </summary>
    /// <remarks>
    /// The signature is not checked: the token came straight from the token endpoint over TLS, which OpenID Connect Core
    /// §3.1.3.7 accepts in place of it. The claims are for showing who is signed in; an API trusts only the access token.
    /// </remarks>
    public static Dictionary<string, JsonElement> IdTokenClaims(
        string idToken, string issuer, string clientId, string? nonce, long now)
    {
        var parts = idToken.Split('.');
        if (parts.Length < 2)
        {
            throw IdTokenError("it is not a JWT");
        }
        Dictionary<string, JsonElement>? claims;
        try
        {
            var payload = parts[1].Replace('-', '+').Replace('_', '/');
            payload = payload.PadRight(payload.Length + ((4 - (payload.Length % 4)) % 4), '=');
            claims = JsonSerializer.Deserialize(Convert.FromBase64String(payload), NativeJsonContext.Default.DictionaryStringJsonElement);
        }
        catch (Exception e) when (e is FormatException or JsonException)
        {
            throw IdTokenError($"its payload is unreadable: {e.Message}");
        }
        if (claims is null)
        {
            throw IdTokenError("its payload is empty");
        }

        if (StringClaim(claims, "iss") != issuer)
        {
            throw IdTokenError("it was issued by another issuer");
        }
        var forUs = claims.TryGetValue("aud", out var aud) && aud.ValueKind switch
        {
            JsonValueKind.String => aud.GetString() == clientId,
            JsonValueKind.Array => aud.EnumerateArray().Any(a => a.ValueKind == JsonValueKind.String && a.GetString() == clientId),
            _ => false,
        };
        if (!forUs)
        {
            throw IdTokenError("it was issued to another client");
        }
        if (!claims.TryGetValue("exp", out var exp) || !exp.TryGetInt64(out var expires))
        {
            throw IdTokenError("it has no expiry");
        }
        if (expires + LeewaySeconds < now)
        {
            throw IdTokenError("it has expired");
        }
        if (nonce is not null && StringClaim(claims, "nonce") != nonce)
        {
            throw IdTokenError("its nonce is not this sign-in's");
        }
        if (string.IsNullOrEmpty(StringClaim(claims, "sub")))
        {
            throw IdTokenError("it has no subject");
        }
        return claims;
    }

    public static string? StringClaim(IReadOnlyDictionary<string, JsonElement> claims, string name) =>
        claims.TryGetValue(name, out var value) && value.ValueKind == JsonValueKind.String ? value.GetString() : null;

    private static NativeSignInException IdTokenError(string message) =>
        new(NativeSignInFailure.IdToken, $"the ID token does not belong to this sign-in: {message}");
}
