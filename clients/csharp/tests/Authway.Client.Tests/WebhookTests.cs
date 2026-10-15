using System.Text;

namespace Authway.Client.Tests;

public class WebhookTests
{
    // The reference vector of the Standard Webhooks libraries.
    private const string Secret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw";
    private const string MessageId = "msg_p5jXN8AQM9LWM0D4loKWxJek";
    private const long Timestamp = 1614265330;
    private const string Signature = "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=";
    private static readonly byte[] Body = Encoding.UTF8.GetBytes("""{"test": 2432232314}""");
    private static readonly DateTimeOffset At = DateTimeOffset.FromUnixTimeSeconds(Timestamp);

    [Fact]
    public void Signs_as_the_reference_libraries_do() =>
        AuthwayWebhook.Sign(Secret, MessageId, Timestamp, Body).Should().Be(Signature);

    [Fact]
    public void Verifies_the_reference_delivery()
    {
        AuthwayWebhook.Verify(Secret, MessageId, "1614265330", Signature, Body, now: At.AddMinutes(1)).Should().BeTrue();
        AuthwayWebhook.Verify(Secret["whsec_".Length..], MessageId, "1614265330", Signature, Body, now: At).Should().BeTrue("the prefix is optional");
        AuthwayWebhook.Verify(Secret, MessageId, "1614265330", "v1,bm90LXRoaXMtb25l " + Signature, Body, now: At).Should().BeTrue("one match among several is enough");
    }

    [Theory]
    [InlineData("tampered body")]
    [InlineData("wrong secret")]
    [InlineData("stale")]
    [InlineData("from the future")]
    [InlineData("other event id")]
    [InlineData("time changed")]
    [InlineData("unknown version")]
    [InlineData("previous t=,v1= form")]
    [InlineData("no signature")]
    [InlineData("secret not base64")]
    public void Refuses_what_was_not_signed_so(string change)
    {
        var (secret, id, timestamp, signature, body, now) = (Secret, MessageId, "1614265330", Signature, Body, At);
        switch (change)
        {
            case "tampered body": body = Encoding.UTF8.GetBytes("""{"test": 1}"""); break;
            case "wrong secret": secret = "whsec_" + Convert.ToBase64String(new byte[24]); break;
            case "stale": now = At.AddMinutes(6); break;
            case "from the future": now = At.AddMinutes(-6); break;
            case "other event id": id = "msg_other"; break;
            case "time changed": timestamp = "1614265331"; break;
            case "unknown version": signature = signature.Replace("v1,", "v2,", StringComparison.Ordinal); break;
            case "previous t=,v1= form": signature = "t=1614265330,v1=deadbeef"; break;
            case "no signature": signature = ""; break;
            case "secret not base64": secret = "whsec_not base64!"; break;
        }

        AuthwayWebhook.Verify(secret, id, timestamp, signature, body, now: now).Should().BeFalse();
    }

    [Fact]
    public void Reads_the_event_of_a_delivery_it_verified()
    {
        var body = Encoding.UTF8.GetBytes("""
            {"id":"b2c6","type":"user.deleted","timestamp":"2026-10-15T02:40:46.123456789Z","tenant_id":"t1",
             "data":{"resource":{"type":"user","id":"u1"},"actor":{"type":"api_key"}}}
            """);
        var now = DateTimeOffset.UtcNow;
        var timestamp = now.ToUnixTimeSeconds().ToString(System.Globalization.CultureInfo.InvariantCulture);
        var headers = new Dictionary<string, string>
        {
            ["webhook-id"] = "b2c6",
            ["webhook-timestamp"] = timestamp,
            ["webhook-signature"] = AuthwayWebhook.Sign(Secret, "b2c6", now.ToUnixTimeSeconds(), body),
        };

        var e = AuthwayWebhook.Read(Secret, name => headers.GetValueOrDefault(name), body);

        e.Type.Should().Be(WebhookEventTypes.UserDeleted);
        e.TenantId.Should().Be("t1");
        e.Resource.Should().Be(new WebhookEventRef("user", "u1"));
        e.Actor.Should().Be(new WebhookEventRef("api_key", null));
        e.Timestamp.Should().Be(new DateTimeOffset(2026, 10, 15, 2, 40, 46, TimeSpan.Zero).AddTicks(1234567));

        headers["webhook-signature"] = "v1,AAAA";
        var refused = () => AuthwayWebhook.Read(Secret, name => headers.GetValueOrDefault(name), body);
        refused.Should().Throw<AuthwayException>();
    }
}
