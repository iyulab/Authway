//! Sign a desktop or command-line app's user in with the system browser, keep the tokens in a store
//! the app chooses, and refresh them before they expire.
//!
//! This is OAuth 2.0 for Native Apps ([RFC 8252]): the app listens on a loopback port the operating
//! system picks, sends the user's browser to the authorization server, and receives the
//! authorization code on that port. PKCE ([RFC 7636], `S256`) binds the code to this app; `state`
//! binds the redirect to this sign-in. Endpoints come from OpenID Connect discovery, so the client
//! works with any OpenID provider — Authway is the one it is written for.
//!
//! The app's code is three calls: sign in, ask for the current access token, sign out.
//!
//! ```no_run
//! use authway_native::{Config, MemoryStore, NativeClient};
//! use std::time::Duration;
//!
//! # fn main() -> Result<(), authway_native::Error> {
//! let client = NativeClient::new(
//!     Config::new("https://auth.example.com", "my-desktop-app"),
//!     MemoryStore::default(),
//! );
//!
//! let pending = client.begin_sign_in()?;
//! // Open it however the app opens links — a Tauri opener, `open::that`, …
//! println!("Sign in at {}", pending.url());
//! let session = pending.wait(Duration::from_secs(300))?;
//! println!("Signed in as {}", session.subject);
//!
//! // Before each call to an API: refreshed when it is about to expire, `None` once signed out.
//! if let Some(token) = client.access_token()? {
//!     // Authorization: Bearer {token}
//! }
//!
//! client.sign_out()?;
//! # Ok(())
//! # }
//! ```
//!
//! # One process owns the session
//!
//! The authorization server issues a new refresh token each time one is used and treats a second use
//! of the old one as theft, ending the whole session. Two processes refreshing from the same stored
//! session — a desktop shell and its background process, say — therefore sign the user out. In one
//! app, let one process sign in, refresh and sign out; other processes ask it for the current access
//! token.
//!
//! The calls block. In an async app, run them on a blocking thread (`spawn_blocking`).
//!
//! [RFC 8252]: https://www.rfc-editor.org/rfc/rfc8252
//! [RFC 7636]: https://www.rfc-editor.org/rfc/rfc7636

mod client;
mod config;
mod discovery;
mod error;
mod id_token;
mod loopback;
mod pkce;
mod store;

pub use client::{Cancel, NativeClient, PendingSignIn, Session, SignedOut};
pub use config::Config;
pub use error::Error;
pub use store::{MemoryStore, StoreError, TokenStore};
