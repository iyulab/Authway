using System.Text.Json;

namespace Authway.Native.Tests;

public sealed class ProtocolTests
{
    private const long Now = 1_800_000_000;

    private static readonly string[] OtherAudience = ["other"];

    [Fact]
    public void The_challenge_matches_the_RFC_7636_example() =>
        Protocol.ChallengeFor("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk").Should().Be("E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM");

    [Fact]
    public void Random_tokens_are_43_unreserved_characters_and_differ()
    {
        var a = Protocol.RandomToken();
        a.Should().HaveLength(43).And.MatchRegex("^[A-Za-z0-9_-]+$");
        Protocol.RandomToken().Should().NotBe(a);
    }

    [Fact]
    public void Reads_the_code_and_a_refusal_of_this_sign_in()
    {
        Protocol.ParseRequestLine("GET /callback?code=c%2B1&state=s-1 HTTP/1.1", "/callback", "s-1")
            .Should().Be(new Protocol.Callback.Code("c+1"));
        Protocol.ParseRequestLine("GET /callback?error=access_denied&error_description=The+user+declined&state=s-1 HTTP/1.1", "/callback", "s-1")
            .Should().Be(new Protocol.Callback.Denied("access_denied", "The user declined"));
    }

    [Theory]
    [InlineData("GET /favicon.ico HTTP/1.1")]
    [InlineData("GET /callback?code=c&state=other HTTP/1.1")]
    [InlineData("GET /callback?code=c HTTP/1.1")]
    [InlineData("GET /elsewhere?code=c&state=s-1 HTTP/1.1")]
    [InlineData("POST /callback?code=c&state=s-1 HTTP/1.1")]
    [InlineData("GET /callback?error=access_denied&state=other HTTP/1.1")]
    [InlineData("garbage")]
    public void Ignores_what_is_not_this_sign_ins_redirect(string line) =>
        Protocol.ParseRequestLine(line, "/callback", "s-1").Should().BeNull();

    internal static string IdToken(object claims)
    {
        static string Encode(byte[] b) => Convert.ToBase64String(b).TrimEnd('=').Replace('+', '-').Replace('/', '_');
        return $"{Encode("{\"alg\":\"RS256\"}"u8.ToArray())}.{Encode(JsonSerializer.SerializeToUtf8Bytes(claims))}.sig";
    }

    private static Dictionary<string, object> Claims(Action<Dictionary<string, object>>? change = null)
    {
        var claims = new Dictionary<string, object>
        {
            ["iss"] = "https://auth.example", ["aud"] = new[] { "app" }, ["sub"] = "user-1", ["exp"] = Now + 3600, ["nonce"] = "n-1",
        };
        change?.Invoke(claims);
        return claims;
    }

    private static Func<Dictionary<string, JsonElement>> Check(Dictionary<string, object> claims, string? nonce = "n-1") =>
        () => Protocol.IdTokenClaims(IdToken(claims), "https://auth.example", "app", nonce, Now);

    [Fact]
    public void Accepts_an_ID_token_for_this_sign_in()
    {
        Check(Claims())()["sub"].GetString().Should().Be("user-1");
        Check(Claims(c => c["aud"] = "app"))().Should().ContainKey("sub");
        Check(Claims(c => c.Remove("nonce")), nonce: null)().Should().ContainKey("sub");
    }

    [Fact]
    public void Refuses_another_issuer_client_nonce_or_an_expired_token()
    {
        foreach (var claims in new[]
        {
            Claims(c => c["iss"] = "https://other.example"),
            Claims(c => c["aud"] = OtherAudience),
            Claims(c => c["nonce"] = "n-2"),
            Claims(c => c.Remove("nonce")),
            Claims(c => c["exp"] = Now - 301),
            Claims(c => c.Remove("sub")),
        })
        {
            Check(claims).Should().Throw<NativeSignInException>().Which.Failure.Should().Be(NativeSignInFailure.IdToken);
        }
        FluentActions.Invoking(() => Protocol.IdTokenClaims("not-a-jwt", "https://auth.example", "app", null, Now))
            .Should().Throw<NativeSignInException>();
    }
}
