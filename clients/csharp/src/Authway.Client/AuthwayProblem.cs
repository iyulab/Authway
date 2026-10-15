using System.Text.Json;

namespace Authway.Client;

/// <summary>
/// A refusal from Authway: an RFC 9457 problem details document. Branch on <see cref="Code"/>; show <see cref="Detail"/>.
/// </summary>
/// <param name="Status">The HTTP status.</param>
/// <param name="Code">A stable identifier, such as <c>tenant_slug_taken</c> or <c>invitation_already_pending</c>.</param>
/// <param name="Detail">A message for a person.</param>
/// <param name="Title">The HTTP status's reason phrase.</param>
/// <param name="Field">The request member at fault, when there is one.</param>
/// <param name="Hint">What to do about it, when there is something.</param>
/// <param name="RetryAfter">How long to wait before trying again, on a rate-limited refusal.</param>
/// <param name="Extensions">Every member of the document beyond the standard ones and those above, as sent.</param>
public sealed record AuthwayProblem(
    int Status,
    string Code,
    string Detail,
    string? Title = null,
    string? Field = null,
    string? Hint = null,
    TimeSpan? RetryAfter = null,
    IReadOnlyDictionary<string, JsonElement>? Extensions = null)
{
    private static readonly HashSet<string> Known = ["type", "title", "status", "detail", "code", "field", "hint", "retry_after"];

    /// <summary>Reads a problem details document; false when the body is not one.</summary>
    public static bool TryParse(ReadOnlySpan<byte> utf8Json, out AuthwayProblem problem)
    {
        problem = null!;
        JsonDocument document;
        try
        {
            var reader = new Utf8JsonReader(utf8Json);
            document = JsonDocument.ParseValue(ref reader);
        }
        catch (JsonException)
        {
            return false;
        }

        using (document)
        {
            var root = document.RootElement;
            if (root.ValueKind != JsonValueKind.Object || Text(root, "code") is not { Length: > 0 } code)
            {
                return false;
            }

            var extensions = new Dictionary<string, JsonElement>();
            foreach (var member in root.EnumerateObject())
            {
                if (!Known.Contains(member.Name))
                {
                    extensions[member.Name] = member.Value.Clone();
                }
            }

            problem = new AuthwayProblem(
                root.TryGetProperty("status", out var status) && status.TryGetInt32(out var number) ? number : 0,
                code,
                Text(root, "detail") ?? "",
                Text(root, "title"),
                Text(root, "field"),
                Text(root, "hint"),
                root.TryGetProperty("retry_after", out var retry) && retry.TryGetInt32(out var seconds) ? TimeSpan.FromSeconds(seconds) : null,
                extensions);
            return true;
        }
    }

    private static string? Text(JsonElement root, string name) =>
        root.TryGetProperty(name, out var value) && value.ValueKind == JsonValueKind.String ? value.GetString() : null;
}
