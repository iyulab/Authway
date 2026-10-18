use std::time::Duration;

const DEFAULT_SUCCESS_PAGE: &str = "<!doctype html><html><head><meta charset=\"utf-8\"><title>Signed in</title></head>\
<body style=\"font-family:sans-serif;text-align:center;padding:3rem\"><h2>You are signed in</h2>\
<p>You can close this tab and return to the app.</p></body></html>";

const DEFAULT_FAILURE_PAGE: &str = "<!doctype html><html><head><meta charset=\"utf-8\"><title>Not signed in</title></head>\
<body style=\"font-family:sans-serif;text-align:center;padding:3rem\"><h2>Sign-in did not complete</h2>\
<p>Return to the app and try again.</p></body></html>";

/// What the client needs to know about the authorization server and the app.
///
/// The app is registered at the authorization server as a public client with the redirect URI
/// `http://127.0.0.1/<path>` — without a port — and the `authorization_code` and `refresh_token`
/// grants. The client listens on a port the operating system picks and the server accepts it on
/// any port (RFC 8252 §7.3).
#[derive(Debug, Clone)]
pub struct Config {
    pub(crate) issuer: String,
    pub(crate) client_id: String,
    pub(crate) scopes: Vec<String>,
    pub(crate) audience: Vec<String>,
    pub(crate) redirect_path: String,
    pub(crate) store_key: String,
    pub(crate) success_page: String,
    pub(crate) failure_page: String,
    pub(crate) http_timeout: Duration,
    pub(crate) refresh_ahead: Duration,
}

impl Config {
    /// The authorization server's issuer URL and the app's client id. Requests the scopes
    /// `openid profile email offline_access` — `offline_access` is what brings a refresh token —
    /// and listens on the path `/callback`.
    pub fn new(issuer: impl Into<String>, client_id: impl Into<String>) -> Self {
        let client_id = client_id.into();
        Self {
            issuer: issuer.into().trim_end_matches('/').to_string(),
            store_key: format!("authway.{client_id}"),
            client_id,
            scopes: ["openid", "profile", "email", "offline_access"].map(String::from).to_vec(),
            audience: Vec::new(),
            redirect_path: "/callback".to_string(),
            success_page: DEFAULT_SUCCESS_PAGE.to_string(),
            failure_page: DEFAULT_FAILURE_PAGE.to_string(),
            http_timeout: Duration::from_secs(30),
            refresh_ahead: Duration::from_secs(60),
        }
    }

    /// The scopes to request instead of the default. Leave out `offline_access` and there is no
    /// refresh token: the session ends when the first access token expires.
    pub fn scopes<I, S>(mut self, scopes: I) -> Self
    where
        I: IntoIterator<Item = S>,
        S: Into<String>,
    {
        self.scopes = scopes.into_iter().map(Into::into).collect();
        self
    }

    /// The APIs the access token is for, sent as the `audience` parameter. Usually unnecessary: a
    /// client registered with an audience gets it in its tokens without asking.
    pub fn audience<I, S>(mut self, audience: I) -> Self
    where
        I: IntoIterator<Item = S>,
        S: Into<String>,
    {
        self.audience = audience.into_iter().map(Into::into).collect();
        self
    }

    /// The path of the registered loopback redirect URI. Default `/callback`.
    pub fn redirect_path(mut self, path: impl Into<String>) -> Self {
        let path = path.into();
        self.redirect_path = if path.starts_with('/') { path } else { format!("/{path}") };
        self
    }

    /// The key the session is stored under. Default `authway.<client id>`.
    pub fn store_key(mut self, key: impl Into<String>) -> Self {
        self.store_key = key.into();
        self
    }

    /// The pages the browser shows when the redirect arrives — after a sign-in, and after one that
    /// failed or was declined. HTML.
    pub fn pages(mut self, success: impl Into<String>, failure: impl Into<String>) -> Self {
        self.success_page = success.into();
        self.failure_page = failure.into();
        self
    }

    /// How long one request to the authorization server may take. Default 30 seconds.
    pub fn http_timeout(mut self, timeout: Duration) -> Self {
        self.http_timeout = timeout;
        self
    }

    /// How long before the access token expires it is refreshed. Default 60 seconds.
    pub fn refresh_ahead(mut self, ahead: Duration) -> Self {
        self.refresh_ahead = ahead;
        self
    }
}
