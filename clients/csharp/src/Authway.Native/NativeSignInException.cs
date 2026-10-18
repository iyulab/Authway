namespace Authway.Native;

/// <summary>Why a sign-in, refresh or sign-out did not complete.</summary>
public enum NativeSignInFailure
{
    /// <summary>The authorization server could not be reached, or answered something that is not OAuth.</summary>
    Http,

    /// <summary>The discovery document is missing an endpoint the client needs, or names another issuer.</summary>
    Discovery,

    /// <summary>The loopback port could not be opened, or failed while waiting.</summary>
    Loopback,

    /// <summary>No redirect arrived from the browser in the time given.</summary>
    TimedOut,

    /// <summary>
    /// The authorization server sent the browser back with an error — the user declined, or the request was refused.
    /// </summary>
    Denied,

    /// <summary>The token endpoint refused a request.</summary>
    Token,

    /// <summary>The ID token in the answer does not belong to this sign-in.</summary>
    IdToken,
}

/// <summary>
/// A sign-in, refresh or sign-out that did not complete. Branch on <see cref="Failure"/>; for
/// <see cref="NativeSignInFailure.Denied"/> and <see cref="NativeSignInFailure.Token"/>, <see cref="Error"/> carries
/// the OAuth error code (<c>access_denied</c>, <c>invalid_grant</c>, …).
/// </summary>
public sealed class NativeSignInException : Exception
{
    public NativeSignInException(NativeSignInFailure failure, string message, Exception? inner = null)
        : base(message, inner) => Failure = failure;

    public NativeSignInException()
        : this(NativeSignInFailure.Http, "sign-in failed")
    {
    }

    public NativeSignInException(string message)
        : this(NativeSignInFailure.Http, message)
    {
    }

    public NativeSignInException(string message, Exception inner)
        : this(NativeSignInFailure.Http, message, inner)
    {
    }

    public NativeSignInFailure Failure { get; }

    /// <summary>The OAuth error code, when the server gave one.</summary>
    public string? Error { get; init; }

    /// <summary>The server's description of the error, when it gave one.</summary>
    public string? ErrorDescription { get; init; }

    /// <summary>The token endpoint's HTTP status, for <see cref="NativeSignInFailure.Token"/>.</summary>
    public int? Status { get; init; }
}
