use std::io::{BufRead, BufReader, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, Instant};

use crate::error::Error;

/// What the browser brought back to the loopback redirect.
#[derive(Debug, PartialEq, Eq)]
pub(crate) enum Callback {
    Code(String),
    Denied {
        error: String,
        description: Option<String>,
    },
}

/// Reads one request line — `GET /callback?code=…&state=… HTTP/1.1` — and returns what it carries
/// if it is this sign-in's redirect: the registered path and this sign-in's `state`. Anything else
/// is not ours.
pub(crate) fn parse_request_line(line: &str, path: &str, state: &str) -> Option<Callback> {
    let mut parts = line.split_whitespace();
    if parts.next() != Some("GET") {
        return None;
    }
    let target = url::Url::parse(&format!("http://127.0.0.1{}", parts.next()?)).ok()?;
    if target.path() != path {
        return None;
    }
    let (mut code, mut got_state, mut error, mut description) = (None, None, None, None);
    for (k, v) in target.query_pairs() {
        match k.as_ref() {
            "code" => code = Some(v.into_owned()),
            "state" => got_state = Some(v.into_owned()),
            "error" => error = Some(v.into_owned()),
            "error_description" => description = Some(v.into_owned()),
            _ => {}
        }
    }
    if got_state.as_deref() != Some(state) {
        return None;
    }
    match (code, error) {
        (_, Some(error)) => Some(Callback::Denied { error, description }),
        (Some(code), None) if !code.is_empty() => Some(Callback::Code(code)),
        _ => None,
    }
}

fn respond(stream: &mut TcpStream, status: &str, body: &str) {
    let response = format!(
        "HTTP/1.1 {status}\r\nContent-Type: text/html; charset=utf-8\r\nCache-Control: no-store\r\n\
         Connection: close\r\nContent-Length: {}\r\n\r\n{body}",
        body.len()
    );
    let _ = stream.write_all(response.as_bytes());
    let _ = stream.flush();
}

/// Waits for this sign-in's redirect and returns its authorization code.
///
/// Only a request on the registered path with this sign-in's `state` ends the wait. A browser also
/// asks for a favicon, may open speculative connections, and anything on the machine can connect:
/// those get a 404 and the wait goes on, until the deadline or a cancel.
pub(crate) fn wait(
    listener: TcpListener,
    path: &str,
    state: &str,
    deadline: Instant,
    cancel: &AtomicBool,
    success_page: &str,
    failure_page: &str,
) -> Result<String, Error> {
    listener.set_nonblocking(true).map_err(Error::Loopback)?;
    loop {
        if cancel.load(Ordering::SeqCst) {
            return Err(Error::Cancelled);
        }
        match listener.accept() {
            Ok((mut stream, _)) => {
                // A connection that sends nothing must not hold up the wait.
                stream.set_nonblocking(false).map_err(Error::Loopback)?;
                stream
                    .set_read_timeout(Some(Duration::from_secs(5)))
                    .map_err(Error::Loopback)?;
                let mut line = String::new();
                let read = BufReader::new(&stream).read_line(&mut line).is_ok();
                match read
                    .then(|| parse_request_line(&line, path, state))
                    .flatten()
                {
                    Some(Callback::Code(code)) => {
                        respond(&mut stream, "200 OK", success_page);
                        return Ok(code);
                    }
                    Some(Callback::Denied { error, description }) => {
                        respond(&mut stream, "200 OK", failure_page);
                        return Err(Error::Denied { error, description });
                    }
                    None => respond(&mut stream, "404 Not Found", "Not found."),
                }
            }
            Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                if Instant::now() >= deadline {
                    return Err(Error::TimedOut);
                }
                std::thread::sleep(Duration::from_millis(50));
            }
            Err(e) => return Err(Error::Loopback(e)),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Read;

    #[test]
    fn reads_the_code_of_this_sign_in() {
        assert_eq!(
            parse_request_line(
                "GET /callback?code=c%2B1&state=s-1 HTTP/1.1",
                "/callback",
                "s-1"
            ),
            Some(Callback::Code("c+1".into()))
        );
    }

    #[test]
    fn reads_a_refusal_of_this_sign_in() {
        assert_eq!(
            parse_request_line(
                "GET /callback?error=access_denied&error_description=The+user+declined&state=s-1 HTTP/1.1",
                "/callback",
                "s-1"
            ),
            Some(Callback::Denied {
                error: "access_denied".into(),
                description: Some("The user declined".into())
            })
        );
    }

    #[test]
    fn ignores_what_is_not_this_sign_ins_redirect() {
        for line in [
            "GET /favicon.ico HTTP/1.1",
            "GET /callback?code=c&state=other HTTP/1.1",
            "GET /callback?code=c HTTP/1.1",
            "GET /elsewhere?code=c&state=s-1 HTTP/1.1",
            "POST /callback?code=c&state=s-1 HTTP/1.1",
            "GET /callback?error=access_denied&state=other HTTP/1.1",
            "garbage",
        ] {
            assert_eq!(parse_request_line(line, "/callback", "s-1"), None, "{line}");
        }
    }

    fn send(addr: std::net::SocketAddr, request: &str) -> String {
        let mut s = TcpStream::connect(addr).unwrap();
        s.write_all(request.as_bytes()).unwrap();
        let mut out = String::new();
        let _ = s.read_to_string(&mut out);
        out
    }

    #[test]
    fn waits_past_stray_requests_for_the_redirect() {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let addr = listener.local_addr().unwrap();
        let browser = std::thread::spawn(move || {
            let stray = send(addr, "GET /favicon.ico HTTP/1.1\r\n\r\n");
            let forged = send(addr, "GET /callback?code=x&state=forged HTTP/1.1\r\n\r\n");
            let real = send(
                addr,
                "GET /callback?code=the-code&state=s-1 HTTP/1.1\r\n\r\n",
            );
            (stray, forged, real)
        });
        let never = AtomicBool::new(false);
        let deadline = Instant::now() + Duration::from_secs(10);
        let code = wait(
            listener,
            "/callback",
            "s-1",
            deadline,
            &never,
            "done",
            "failed",
        )
        .unwrap();
        assert_eq!(code, "the-code");
        let (stray, forged, real) = browser.join().unwrap();
        assert!(stray.starts_with("HTTP/1.1 404"));
        assert!(forged.starts_with("HTTP/1.1 404"));
        assert!(real.starts_with("HTTP/1.1 200") && real.ends_with("done"));
    }

    #[test]
    fn ends_on_a_refusal_with_the_failure_page() {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let addr = listener.local_addr().unwrap();
        let browser = std::thread::spawn(move || {
            send(
                addr,
                "GET /callback?error=access_denied&state=s-1 HTTP/1.1\r\n\r\n",
            )
        });
        let never = AtomicBool::new(false);
        let deadline = Instant::now() + Duration::from_secs(10);
        let err = wait(
            listener,
            "/callback",
            "s-1",
            deadline,
            &never,
            "done",
            "failed",
        )
        .unwrap_err();
        assert!(matches!(err, Error::Denied { ref error, .. } if error == "access_denied"));
        assert!(browser.join().unwrap().ends_with("failed"));
    }

    #[test]
    fn times_out_and_cancels() {
        let never = AtomicBool::new(false);
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let started = Instant::now();
        let err = wait(
            listener,
            "/callback",
            "s",
            Instant::now() + Duration::from_millis(200),
            &never,
            "",
            "",
        )
        .unwrap_err();
        assert!(matches!(err, Error::TimedOut));
        assert!(started.elapsed() < Duration::from_secs(3));

        let cancelled = AtomicBool::new(true);
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let err = wait(
            listener,
            "/callback",
            "s",
            Instant::now() + Duration::from_secs(60),
            &cancelled,
            "",
            "",
        )
        .unwrap_err();
        assert!(matches!(err, Error::Cancelled));
    }
}
