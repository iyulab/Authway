// Signs in from the command line, shows who signed in, refreshes the access token, and signs out.
//
//   dotnet run --project examples/SignIn -- <issuer> <client id> [--refresh-ahead <seconds>]
//
// Register the client as public, with the redirect URI http://127.0.0.1/callback, the authorization_code and
// refresh_token grants, and the scope offline_access. Open the printed URL in a browser. --refresh-ahead larger than
// the access token's lifetime refreshes at once. Each step prints one line, "STEP value", so a script can follow along.

using System.Text.Json;
using Authway.Native;

if (args.Length < 2)
{
    Console.Error.WriteLine("usage: SignIn <issuer> <client id> [--refresh-ahead <seconds>]");
    return 2;
}

var options = new NativeClientOptions(args[0], args[1]);
var ahead = Array.IndexOf(args, "--refresh-ahead");
if (ahead >= 0)
{
    options = options with { RefreshAhead = TimeSpan.FromSeconds(int.Parse(args[ahead + 1], System.Globalization.CultureInfo.InvariantCulture)) };
}

var store = new MemoryTokenStore();
using var client = new NativeClient(options, store);

async Task<string> StoredRefreshToken()
{
    var raw = await store.GetAsync(options.StoreKey) ?? "{}";
    using var doc = JsonDocument.Parse(raw);
    return doc.RootElement.TryGetProperty("refresh_token", out var r) ? r.GetString() ?? "" : "";
}

try
{
    using var pending = await client.BeginSignInAsync();
    Console.WriteLine($"URL {pending.Url}");
    var session = await pending.WaitAsync(TimeSpan.FromMinutes(5));
    Console.WriteLine($"SIGNED_IN {session.Subject}");
    Console.WriteLine($"REFRESH_BEFORE {await StoredRefreshToken()}");

    var token = await client.GetAccessTokenAsync() ?? "";
    Console.WriteLine($"ACCESS {token}");
    Console.WriteLine($"REFRESH_AFTER {await StoredRefreshToken()}");
    // What an OS credential store would hold: its size as UTF-16, and whether the access token is in it.
    var stored = await store.GetAsync(options.StoreKey) ?? "";
    Console.WriteLine($"STORED {stored.Length * 2} access={(token.Length > 0 && stored.Contains(token, StringComparison.Ordinal) ? "true" : "false")}");

    var signedOut = await client.SignOutAsync();
    Console.WriteLine($"SIGNED_OUT revoked={(signedOut.Revoked ? "true" : "false")}");
    return 0;
}
catch (NativeSignInException e)
{
    Console.WriteLine($"ERROR {e.Message}");
    return 1;
}
