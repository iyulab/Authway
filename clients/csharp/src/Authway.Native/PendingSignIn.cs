using System.Net.Sockets;
using System.Text;

namespace Authway.Native;

/// <summary>A sign-in waiting for the browser. Open <see cref="Url"/>, then <see cref="WaitAsync"/>.</summary>
public sealed class PendingSignIn : IDisposable
{
    private readonly NativeClient client;
    private readonly TcpListener listener;
    private readonly string redirectUri;
    private readonly string verifier;
    private readonly string state;
    private readonly string nonce;

    internal PendingSignIn(
        NativeClient client, TcpListener listener, Uri url, string redirectUri, string verifier, string state, string nonce)
    {
        this.client = client;
        this.listener = listener;
        Url = url;
        this.redirectUri = redirectUri;
        this.verifier = verifier;
        this.state = state;
        this.nonce = nonce;
    }

    /// <summary>The URL to open in the user's browser.</summary>
    public Uri Url { get; }

    /// <summary>
    /// Waits up to <paramref name="timeout"/> for the browser to come back, redeems the code, and stores the session.
    /// Cancel <paramref name="cancellationToken"/> to give up — the user closed the dialog.
    /// </summary>
    /// <exception cref="NativeSignInException">
    /// <see cref="NativeSignInFailure.TimedOut"/> when no redirect arrived; <see cref="NativeSignInFailure.Denied"/> when
    /// the user declined or the server refused the request.
    /// </exception>
    public async Task<NativeSession> WaitAsync(TimeSpan timeout, CancellationToken cancellationToken = default)
    {
        string code;
        try
        {
            code = await ReceiveCodeAsync(timeout, cancellationToken).ConfigureAwait(false);
        }
        finally
        {
            listener.Stop();
        }
        return await client.CompleteSignInAsync(code, redirectUri, verifier, nonce, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Only a request on the registered path with this sign-in's state ends the wait. A browser also asks for a favicon,
    /// may open speculative connections, and anything on the machine can connect: those get a 404 and the wait goes on.
    /// </summary>
    private async Task<string> ReceiveCodeAsync(TimeSpan timeout, CancellationToken cancellationToken)
    {
        using var deadline = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        deadline.CancelAfter(timeout);
        var options = client.Options;
        while (true)
        {
            TcpClient connection;
            try
            {
                connection = await listener.AcceptTcpClientAsync(deadline.Token).ConfigureAwait(false);
            }
            catch (OperationCanceledException) when (!cancellationToken.IsCancellationRequested)
            {
                throw new NativeSignInException(NativeSignInFailure.TimedOut, "sign-in timed out: no redirect arrived from the browser");
            }
            catch (SocketException e)
            {
                throw new NativeSignInException(NativeSignInFailure.Loopback, $"the sign-in redirect could not be received: {e.Message}", e);
            }

            using (connection)
            {
                var stream = connection.GetStream();
                string? line = null;
                using (var read = CancellationTokenSource.CreateLinkedTokenSource(deadline.Token))
                {
                    // A connection that sends nothing must not hold up the wait.
                    read.CancelAfter(TimeSpan.FromSeconds(5));
                    try
                    {
                        using var reader = new StreamReader(stream, Encoding.ASCII, false, 1024, leaveOpen: true);
                        line = await reader.ReadLineAsync(read.Token).ConfigureAwait(false);
                    }
                    catch (Exception e) when (e is OperationCanceledException or IOException)
                    {
                    }
                }

                switch (line is null ? null : Protocol.ParseRequestLine(line, options.RedirectPath, state))
                {
                    case Protocol.Callback.Code c:
                        await RespondAsync(stream, "200 OK", options.SuccessPage).ConfigureAwait(false);
                        return c.Value;
                    case Protocol.Callback.Denied d:
                        await RespondAsync(stream, "200 OK", options.FailurePage).ConfigureAwait(false);
                        throw new NativeSignInException(NativeSignInFailure.Denied,
                            d.Description is null ? $"sign-in was refused ({d.Error})" : $"sign-in was refused ({d.Error}): {d.Description}")
                        {
                            Error = d.Error,
                            ErrorDescription = d.Description,
                        };
                    default:
                        await RespondAsync(stream, "404 Not Found", "Not found.").ConfigureAwait(false);
                        break;
                }
            }
        }
    }

    private static async Task RespondAsync(NetworkStream stream, string status, string body)
    {
        var bytes = Encoding.UTF8.GetBytes(body);
        var head = Encoding.ASCII.GetBytes(
            $"HTTP/1.1 {status}\r\nContent-Type: text/html; charset=utf-8\r\nCache-Control: no-store\r\n" +
            $"Connection: close\r\nContent-Length: {bytes.Length}\r\n\r\n");
        try
        {
            await stream.WriteAsync(head).ConfigureAwait(false);
            await stream.WriteAsync(bytes).ConfigureAwait(false);
            await stream.FlushAsync().ConfigureAwait(false);
        }
        catch (IOException)
        {
            // The browser went away; nothing to tell it.
        }
    }

    /// <summary>Closes the loopback port if the sign-in is abandoned without <see cref="WaitAsync"/>.</summary>
    public void Dispose() => listener.Stop();
}
