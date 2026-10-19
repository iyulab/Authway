use std::net::TcpListener;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};

use crate::config::Config;
use crate::discovery::{self, Discovery};
use crate::error::Error;
use crate::store::TokenStore;
use crate::{id_token, loopback, pkce};

/// Who is signed in.
#[derive(Debug, Clone)]
pub struct Session {
    /// The user's identifier at the authorization server (`sub`).
    pub subject: String,
    /// The ID token's claims — `email`, `name` and the others the scopes asked for. For showing who
    /// is signed in; an API trusts only the access token.
    pub claims: Map<String, Value>,
    /// When the current access token expires. [`NativeClient::access_token`] refreshes it before.
    /// The Unix epoch when this process holds none yet — the access token is not stored, so the
    /// first call after a start refreshes.
    pub access_token_expires_at: SystemTime,
}

/// What [`NativeClient::sign_out`] did.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SignedOut {
    /// Whether a session was stored. It is removed either way.
    pub was_signed_in: bool,
    /// Whether the authorization server confirmed the tokens are revoked. `false` when it could
    /// not be reached or names no revocation endpoint: the session is gone from this device, but
    /// its refresh token stays valid at the server until it expires.
    pub revoked: bool,
}

/// What is kept in the [`TokenStore`] between runs: the refresh token and the ID token's claims.
/// The access token stays in the process — it is renewed from the refresh token, and leaving it out
/// keeps the value small enough for one OS credential entry. Only a session without a refresh
/// token keeps its access token, since nothing else could resume it.
#[derive(Debug, Clone, Serialize, Deserialize)]
struct Stored {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    access_token: Option<String>,
    /// Seconds since the Unix epoch.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    expires_at: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    refresh_token: Option<String>,
    #[serde(default)]
    claims: Map<String, Value>,
}

/// An access token in hand, and when it expires (seconds since the Unix epoch).
#[derive(Debug, Clone)]
struct Held {
    token: String,
    expires_at: u64,
}

#[derive(Deserialize)]
struct TokenResponse {
    access_token: String,
    #[serde(default)]
    expires_in: Option<u64>,
    #[serde(default)]
    refresh_token: Option<String>,
    #[serde(default)]
    id_token: Option<String>,
}

/// An access token without `expires_in` is taken to last this long.
const ASSUMED_LIFETIME_SECONDS: u64 = 300;

fn now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

struct Inner {
    config: Config,
    store: Box<dyn TokenStore>,
    agent: ureq::Agent,
    discovery: Mutex<Option<Discovery>>,
    /// Held while the stored session is read and replaced, so one refresh happens at a time.
    session: Mutex<()>,
    /// The access token this process holds. Written only while `session` is held.
    current: Mutex<Option<Held>>,
}

/// Signs the user in with the system browser and keeps their session.
///
/// Cheap to clone: clones share the session. Every call blocks.
#[derive(Clone)]
pub struct NativeClient {
    inner: Arc<Inner>,
}

impl NativeClient {
    /// A client for `config` that keeps its session in `store`.
    pub fn new(config: Config, store: impl TokenStore + 'static) -> Self {
        let agent = ureq::Agent::config_builder()
            .timeout_global(Some(config.http_timeout))
            .http_status_as_error(false)
            .user_agent(concat!("authway-native/", env!("CARGO_PKG_VERSION")))
            .build()
            .into();
        Self {
            inner: Arc::new(Inner {
                config,
                store: Box::new(store),
                agent,
                discovery: Mutex::new(None),
                session: Mutex::new(()),
                current: Mutex::new(None),
            }),
        }
    }

    /// Starts a sign-in: opens the loopback port and builds the URL to open in the browser.
    pub fn begin_sign_in(&self) -> Result<PendingSignIn, Error> {
        let discovery = self.discovery()?;
        let listener = TcpListener::bind("127.0.0.1:0").map_err(Error::Loopback)?;
        let port = listener.local_addr().map_err(Error::Loopback)?.port();
        let config = &self.inner.config;
        let redirect_uri = format!("http://127.0.0.1:{port}{}", config.redirect_path);
        let verifier = pkce::random_token();
        let state = pkce::random_token();
        let nonce = pkce::random_token();

        let mut url = url::Url::parse(&discovery.authorization_endpoint)
            .map_err(|e| Error::Discovery(format!("authorization_endpoint: {e}")))?;
        {
            let mut query = url.query_pairs_mut();
            query
                .append_pair("response_type", "code")
                .append_pair("client_id", &config.client_id)
                .append_pair("redirect_uri", &redirect_uri)
                .append_pair("scope", &config.scopes.join(" "))
                .append_pair("state", &state)
                .append_pair("nonce", &nonce)
                .append_pair("code_challenge", &pkce::challenge_for(&verifier))
                .append_pair("code_challenge_method", "S256");
            if !config.audience.is_empty() {
                query.append_pair("audience", &config.audience.join(" "));
            }
        }

        Ok(PendingSignIn {
            client: self.clone(),
            listener,
            url: url.into(),
            redirect_uri,
            verifier,
            state,
            nonce,
            cancel: Cancel(Arc::new(AtomicBool::new(false))),
        })
    }

    /// The current access token — refreshed first when it expires within the configured margin.
    /// `None` when no one is signed in, or the session ended (the refresh token expired or was
    /// revoked); sign in again.
    pub fn access_token(&self) -> Result<Option<String>, Error> {
        let _one_at_a_time = self.inner.session.lock().unwrap_or_else(|e| e.into_inner());
        let Some(stored) = self.load()? else {
            return Ok(None);
        };
        let ahead = self.inner.config.refresh_ahead.as_secs();
        if let Some(held) = self.held(&stored) {
            if held.expires_at > now() + ahead {
                return Ok(Some(held.token));
            }
        }
        let Some(refresh_token) = stored.refresh_token.clone() else {
            self.remove()?;
            return Ok(None);
        };
        self.refresh(&stored, &refresh_token)
    }

    /// Who is signed in, without contacting the server. The access token may have expired; the
    /// next [`access_token`](Self::access_token) refreshes it.
    pub fn session(&self) -> Result<Option<Session>, Error> {
        let _one_at_a_time = self.inner.session.lock().unwrap_or_else(|e| e.into_inner());
        Ok(self.load()?.map(|s| self.session_of(&s)))
    }

    /// Ends the session: revokes its tokens at the authorization server and removes it from the
    /// store. The store is cleared even when the server cannot be reached — see
    /// [`SignedOut::revoked`].
    pub fn sign_out(&self) -> Result<SignedOut, Error> {
        let _one_at_a_time = self.inner.session.lock().unwrap_or_else(|e| e.into_inner());
        let Some(stored) = self.load()? else {
            return Ok(SignedOut {
                was_signed_in: false,
                revoked: false,
            });
        };
        let endpoint = self.discovery().ok().and_then(|d| d.revocation_endpoint);
        let revoked = match endpoint {
            Some(endpoint) => {
                let refresh = stored
                    .refresh_token
                    .as_deref()
                    .is_none_or(|t| self.revoke(&endpoint, t, "refresh_token"));
                let access = self
                    .held(&stored)
                    .is_none_or(|h| self.revoke(&endpoint, &h.token, "access_token"));
                refresh && access
            }
            None => false,
        };
        self.remove()?;
        Ok(SignedOut {
            was_signed_in: true,
            revoked,
        })
    }

    fn discovery(&self) -> Result<Discovery, Error> {
        let mut cached = self
            .inner
            .discovery
            .lock()
            .unwrap_or_else(|e| e.into_inner());
        if let Some(d) = cached.as_ref() {
            return Ok(d.clone());
        }
        let d = discovery::fetch(&self.inner.agent, &self.inner.config.issuer)?;
        *cached = Some(d.clone());
        Ok(d)
    }

    fn current(&self) -> std::sync::MutexGuard<'_, Option<Held>> {
        self.inner.current.lock().unwrap_or_else(|e| e.into_inner())
    }

    /// The access token in hand: the one this process holds, or — for a session without a refresh
    /// token, or one stored by an earlier version — the stored one.
    fn held(&self, stored: &Stored) -> Option<Held> {
        self.current().clone().or_else(|| {
            stored.access_token.clone().map(|token| Held {
                token,
                expires_at: stored.expires_at.unwrap_or(0),
            })
        })
    }

    fn session_of(&self, stored: &Stored) -> Session {
        Session {
            subject: stored
                .claims
                .get("sub")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .to_string(),
            claims: stored.claims.clone(),
            access_token_expires_at: UNIX_EPOCH
                + Duration::from_secs(self.held(stored).map_or(0, |h| h.expires_at)),
        }
    }

    fn load(&self) -> Result<Option<Stored>, Error> {
        let raw = self
            .inner
            .store
            .get(&self.inner.config.store_key)
            .map_err(Error::Store)?;
        // A value this version cannot read is treated as no session rather than an error the app
        // cannot get past: signing in again replaces it.
        let stored: Option<Stored> = raw.and_then(|r| serde_json::from_str(&r).ok());
        if stored.is_none() {
            // The session is gone from the store, so the access token this process held belongs
            // to no session.
            *self.current() = None;
        }
        Ok(stored)
    }

    /// Keeps a token response: the access token in the process, the refresh token and claims in
    /// the store. Without a refresh token the access token is stored too — nothing else could
    /// resume the session.
    fn save(
        &self,
        response: TokenResponse,
        refresh_token: Option<String>,
        claims: Map<String, Value>,
    ) -> Result<(Stored, Held), Error> {
        let held = Held {
            token: response.access_token,
            expires_at: now() + response.expires_in.unwrap_or(ASSUMED_LIFETIME_SECONDS),
        };
        let stored = match refresh_token {
            Some(refresh_token) => Stored {
                access_token: None,
                expires_at: None,
                refresh_token: Some(refresh_token),
                claims,
            },
            None => Stored {
                access_token: Some(held.token.clone()),
                expires_at: Some(held.expires_at),
                refresh_token: None,
                claims,
            },
        };
        let value = serde_json::to_string(&stored).expect("a session always serializes");
        self.inner
            .store
            .set(&self.inner.config.store_key, &value)
            .map_err(Error::Store)?;
        *self.current() = Some(held.clone());
        Ok((stored, held))
    }

    fn remove(&self) -> Result<(), Error> {
        *self.current() = None;
        self.inner
            .store
            .delete(&self.inner.config.store_key)
            .map_err(Error::Store)
    }

    fn post_form(&self, url: &str, form: &[(&str, &str)]) -> Result<(u16, String), Error> {
        let mut response = self
            .inner
            .agent
            .post(url)
            .send_form(form.iter().copied())
            .map_err(|e| Error::Http(e.to_string()))?;
        let status = response.status().as_u16();
        let body = response
            .body_mut()
            .read_to_string()
            .map_err(|e| Error::Http(e.to_string()))?;
        Ok((status, body))
    }

    fn token_request(&self, form: &[(&str, &str)]) -> Result<TokenResponse, Error> {
        let discovery = self.discovery()?;
        let (status, body) = self.post_form(&discovery.token_endpoint, form)?;
        if status == 200 {
            return serde_json::from_str(&body)
                .map_err(|e| Error::Http(format!("unreadable token response: {e}")));
        }
        // RFC 6749 §5.2 `error` / `error_description`, or RFC 9457 `code` / `detail`.
        let refusal: Map<String, Value> = serde_json::from_str(&body).unwrap_or_default();
        let field = |a: &str, b: &str| {
            refusal
                .get(a)
                .or_else(|| refusal.get(b))
                .and_then(Value::as_str)
                .map(String::from)
        };
        Err(Error::Token {
            status,
            error: field("error", "code"),
            description: field("error_description", "detail"),
        })
    }

    /// Redeems a refresh token, stores what comes back and returns the new access token. `None`
    /// when the server no longer accepts it: the session is removed.
    fn refresh(&self, stored: &Stored, refresh_token: &str) -> Result<Option<String>, Error> {
        let config = &self.inner.config;
        let response = match self.token_request(&[
            ("grant_type", "refresh_token"),
            ("refresh_token", refresh_token),
            ("client_id", &config.client_id),
        ]) {
            Ok(r) => r,
            Err(Error::Token {
                error: Some(ref e), ..
            }) if e == "invalid_grant" => {
                self.remove()?;
                return Ok(None);
            }
            Err(e) => return Err(e),
        };
        let claims = match &response.id_token {
            Some(t) => {
                id_token::claims(t, &self.discovery()?.issuer, &config.client_id, None, now())?
            }
            None => stored.claims.clone(),
        };
        // The server issues a new refresh token with each use; the old one is spent.
        let refresh_token = response
            .refresh_token
            .clone()
            .or_else(|| stored.refresh_token.clone());
        let (_, held) = self.save(response, refresh_token, claims)?;
        Ok(Some(held.token))
    }

    fn revoke(&self, endpoint: &str, token: &str, hint: &str) -> bool {
        let client_id = self.inner.config.client_id.as_str();
        matches!(
            self.post_form(
                endpoint,
                &[
                    ("token", token),
                    ("token_type_hint", hint),
                    ("client_id", client_id)
                ]
            ),
            Ok((200, _))
        )
    }
}

/// Lets the app give up on a sign-in that is waiting — the user closed the dialog.
#[derive(Debug, Clone)]
pub struct Cancel(Arc<AtomicBool>);

impl Cancel {
    /// Ends the wait with [`Error::Cancelled`].
    pub fn cancel(&self) {
        self.0.store(true, Ordering::SeqCst);
    }
}

/// A sign-in waiting for the browser. Open [`url`](Self::url), then [`wait`](Self::wait).
pub struct PendingSignIn {
    client: NativeClient,
    listener: TcpListener,
    url: String,
    redirect_uri: String,
    verifier: String,
    state: String,
    nonce: String,
    cancel: Cancel,
}

impl PendingSignIn {
    /// The URL to open in the user's browser.
    pub fn url(&self) -> &str {
        &self.url
    }

    /// A handle that ends [`wait`](Self::wait) from another thread.
    pub fn canceller(&self) -> Cancel {
        self.cancel.clone()
    }

    /// Waits up to `timeout` for the browser to come back, redeems the code, and stores the
    /// session.
    pub fn wait(self, timeout: Duration) -> Result<Session, Error> {
        let client = &self.client;
        let config = &client.inner.config;
        let code = loopback::wait(
            self.listener,
            &config.redirect_path,
            &self.state,
            Instant::now() + timeout,
            &self.cancel.0,
            &config.success_page,
            &config.failure_page,
        )?;
        let response = client.token_request(&[
            ("grant_type", "authorization_code"),
            ("code", &code),
            ("redirect_uri", &self.redirect_uri),
            ("client_id", &config.client_id),
            ("code_verifier", &self.verifier),
        ])?;
        let id_token = response.id_token.as_deref().ok_or_else(|| {
            Error::IdToken("the answer has none — request the openid scope".into())
        })?;
        let claims = id_token::claims(
            id_token,
            &client.discovery()?.issuer,
            &config.client_id,
            Some(&self.nonce),
            now(),
        )?;
        let _one_at_a_time = client
            .inner
            .session
            .lock()
            .unwrap_or_else(|e| e.into_inner());
        let refresh_token = response.refresh_token.clone();
        let (stored, _) = client.save(response, refresh_token, claims)?;
        Ok(client.session_of(&stored))
    }
}
