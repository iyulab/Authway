use std::fmt;

use crate::store::StoreError;

/// Why a call did not complete.
#[derive(Debug)]
#[non_exhaustive]
pub enum Error {
    /// The authorization server could not be reached, or answered something that is not OAuth.
    Http(String),
    /// The discovery document is missing an endpoint the client needs, or names another issuer.
    Discovery(String),
    /// The loopback port could not be opened, or failed while waiting.
    Loopback(std::io::Error),
    /// No redirect arrived before the time given to [`PendingSignIn::wait`](crate::PendingSignIn::wait).
    TimedOut,
    /// The app gave up on the sign-in with [`Cancel::cancel`](crate::Cancel::cancel).
    Cancelled,
    /// The authorization server sent the browser back with an error — the user declined, or the
    /// request was refused (`access_denied`, `invalid_scope`, …).
    Denied {
        error: String,
        description: Option<String>,
    },
    /// The token endpoint refused a request (`invalid_grant`, `invalid_client`, …).
    Token {
        status: u16,
        error: Option<String>,
        description: Option<String>,
    },
    /// The ID token in the answer does not belong to this sign-in.
    IdToken(String),
    /// The token store failed.
    Store(StoreError),
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Error::Http(m) => write!(f, "could not reach the authorization server: {m}"),
            Error::Discovery(m) => write!(f, "unusable discovery document: {m}"),
            Error::Loopback(e) => write!(f, "could not listen for the sign-in redirect: {e}"),
            Error::TimedOut => {
                f.write_str("sign-in timed out: no redirect arrived from the browser")
            }
            Error::Cancelled => f.write_str("sign-in was cancelled"),
            Error::Denied { error, description } => match description {
                Some(d) => write!(f, "sign-in was refused ({error}): {d}"),
                None => write!(f, "sign-in was refused ({error})"),
            },
            Error::Token {
                status,
                error,
                description,
            } => {
                write!(f, "the token endpoint refused the request (HTTP {status}")?;
                if let Some(e) = error {
                    write!(f, ", {e}")?;
                }
                f.write_str(")")?;
                if let Some(d) = description {
                    write!(f, ": {d}")?;
                }
                Ok(())
            }
            Error::IdToken(m) => write!(f, "the ID token does not belong to this sign-in: {m}"),
            Error::Store(e) => write!(f, "the token store failed: {e}"),
        }
    }
}

impl std::error::Error for Error {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Error::Loopback(e) => Some(e),
            Error::Store(e) => Some(e.as_ref()),
            _ => None,
        }
    }
}
