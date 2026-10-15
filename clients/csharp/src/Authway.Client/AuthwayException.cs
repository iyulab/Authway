using System.Net;

namespace Authway.Client;

/// <summary>Authway refused a call, or answered with something this client could not read.</summary>
public sealed class AuthwayException : Exception
{
    public AuthwayException()
    {
    }

    public AuthwayException(string message)
        : base(message)
    {
    }

    public AuthwayException(string message, Exception innerException)
        : base(message, innerException)
    {
    }

    public AuthwayException(HttpStatusCode statusCode, string message, AuthwayProblem? problem)
        : base(message)
    {
        StatusCode = statusCode;
        Problem = problem;
    }

    /// <summary>The HTTP status of the answer.</summary>
    public HttpStatusCode StatusCode { get; }

    /// <summary>The refusal as Authway described it; null when the answer carried none.</summary>
    public AuthwayProblem? Problem { get; }

    /// <summary>The refusal's code, such as <c>invitation_already_pending</c>; null when there is none.</summary>
    public string? Code => Problem?.Code;
}
