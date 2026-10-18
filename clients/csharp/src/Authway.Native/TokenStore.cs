using System.Collections.Concurrent;

namespace Authway.Native;

/// <summary>
/// Where the session is kept between runs of the app — normally the operating system's credential store (Windows
/// Credential Manager, macOS Keychain, the Secret Service).
/// </summary>
/// <remarks>
/// The shape is the one OS keychain libraries share: one secret per key, read, written and removed whole, so an adapter
/// is a few lines. The value holds the refresh token: keep it where only the user can read it.
/// </remarks>
public interface ITokenStore
{
    /// <summary>The value stored under <paramref name="key"/>, or <see langword="null"/> when there is none.</summary>
    ValueTask<string?> GetAsync(string key, CancellationToken cancellationToken = default);

    /// <summary>Stores <paramref name="value"/> under <paramref name="key"/>, replacing what was there.</summary>
    ValueTask SetAsync(string key, string value, CancellationToken cancellationToken = default);

    /// <summary>Removes the value under <paramref name="key"/>. Removing one that is not there is not an error.</summary>
    ValueTask DeleteAsync(string key, CancellationToken cancellationToken = default);
}

/// <summary>
/// A store that forgets everything when the process ends — for tests, and for command-line tools that sign in for one
/// run.
/// </summary>
public sealed class MemoryTokenStore : ITokenStore
{
    private readonly ConcurrentDictionary<string, string> values = new();

    public ValueTask<string?> GetAsync(string key, CancellationToken cancellationToken = default) =>
        ValueTask.FromResult(values.TryGetValue(key, out var value) ? value : null);

    public ValueTask SetAsync(string key, string value, CancellationToken cancellationToken = default)
    {
        values[key] = value;
        return ValueTask.CompletedTask;
    }

    public ValueTask DeleteAsync(string key, CancellationToken cancellationToken = default)
    {
        values.TryRemove(key, out _);
        return ValueTask.CompletedTask;
    }
}
