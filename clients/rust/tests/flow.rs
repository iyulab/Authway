//! The whole flow against a stand-in authorization server: sign in through the loopback redirect,
//! refresh before expiry, a refused refresh ends the session, sign out revokes.

use std::collections::HashMap;
use std::io::{BufRead, BufReader, Read, Write};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use authway_native::{Config, Error, MemoryStore, NativeClient, TokenStore};
use base64::Engine as _;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use serde_json::{Value, json};

#[derive(Default)]
struct Seen {
    /// Every form posted, by path.
    forms: Vec<(String, HashMap<String, String>)>,
}

/// An authorization server that answers discovery, the token endpoint and revocation, and records
/// what it was sent. `token` decides each token endpoint answer from the posted form.
struct StandIn {
    addr: SocketAddr,
    seen: Arc<Mutex<Seen>>,
}

type TokenAnswer = dyn Fn(&HashMap<String, String>) -> (u16, Value) + Send + Sync;

impl StandIn {
    fn start(token: Box<TokenAnswer>) -> Self {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let addr = listener.local_addr().unwrap();
        let seen = Arc::new(Mutex::new(Seen::default()));
        let recorded = seen.clone();
        std::thread::spawn(move || {
            for stream in listener.incoming().flatten() {
                serve(stream, addr, &recorded, &*token);
            }
        });
        Self { addr, seen }
    }

    fn issuer(&self) -> String {
        format!("http://{}", self.addr)
    }

    fn forms(&self, path: &str) -> Vec<HashMap<String, String>> {
        self.seen.lock().unwrap().forms.iter().filter(|(p, _)| p == path).map(|(_, f)| f.clone()).collect()
    }
}

fn serve(stream: TcpStream, addr: SocketAddr, seen: &Mutex<Seen>, token: &TokenAnswer) {
    let mut reader = BufReader::new(&stream);
    let mut line = String::new();
    if reader.read_line(&mut line).is_err() {
        return;
    }
    let path = line.split_whitespace().nth(1).unwrap_or("").to_string();
    let mut length = 0;
    loop {
        let mut header = String::new();
        if reader.read_line(&mut header).unwrap_or(0) == 0 || header == "\r\n" {
            break;
        }
        if let Some((name, value)) = header.split_once(':') {
            if name.eq_ignore_ascii_case("content-length") {
                length = value.trim().parse().unwrap_or(0);
            }
        }
    }
    let mut body = vec![0; length];
    reader.read_exact(&mut body).unwrap();
    let form: HashMap<String, String> = url::form_urlencoded::parse(&body).into_owned().collect();

    let issuer = format!("http://{addr}");
    let (status, answer) = match path.as_str() {
        "/.well-known/openid-configuration" => (
            200,
            json!({
                "issuer": issuer,
                "authorization_endpoint": format!("{issuer}/oauth2/auth"),
                "token_endpoint": format!("{issuer}/oauth2/token"),
                "revocation_endpoint": format!("{issuer}/oauth2/revoke"),
            }),
        ),
        "/oauth2/token" => {
            seen.lock().unwrap().forms.push((path.clone(), form.clone()));
            token(&form)
        }
        "/oauth2/revoke" => {
            seen.lock().unwrap().forms.push((path.clone(), form.clone()));
            (200, json!({}))
        }
        _ => (404, json!({})),
    };
    let body = answer.to_string();
    let response = format!(
        "HTTP/1.1 {status} X\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    );
    let mut stream = stream;
    let _ = stream.write_all(response.as_bytes());
    let _ = stream.shutdown(std::net::Shutdown::Write);
}

fn id_token(issuer: &str, nonce: Option<&str>) -> String {
    let now = std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_secs();
    let mut claims = json!({"iss": issuer, "aud": ["desktop-app"], "sub": "user-1", "email": "user@example.com", "exp": now + 3600});
    if let Some(n) = nonce {
        claims["nonce"] = json!(n);
    }
    let enc = |v: &Value| URL_SAFE_NO_PAD.encode(v.to_string());
    format!("{}.{}.sig", enc(&json!({"alg": "RS256"})), enc(&claims))
}

/// Plays the browser: follows the authorization URL's redirect to the loopback port.
fn browser_returns(authorize_url: &str, params: &str) -> String {
    let url = url::Url::parse(authorize_url).unwrap();
    let q: HashMap<String, String> = url.query_pairs().into_owned().collect();
    let redirect = url::Url::parse(&q["redirect_uri"]).unwrap();
    let mut s = TcpStream::connect((redirect.host_str().unwrap(), redirect.port().unwrap())).unwrap();
    let state = &q["state"];
    write!(s, "GET {}?{params}&state={state} HTTP/1.1\r\nHost: x\r\n\r\n", redirect.path()).unwrap();
    let mut out = String::new();
    let _ = s.read_to_string(&mut out);
    out
}

fn nonce_of(authorize_url: &str) -> String {
    url::Url::parse(authorize_url).unwrap().query_pairs().find(|(k, _)| k == "nonce").unwrap().1.into_owned()
}

#[test]
fn signs_in_refreshes_and_signs_out() {
    // The nonce is only known once the sign-in starts; the stand-in reads it from a shared slot.
    let nonce: Arc<Mutex<Option<String>>> = Arc::default();
    let issuer_slot: Arc<Mutex<String>> = Arc::default();
    let (n, i) = (nonce.clone(), issuer_slot.clone());
    let server = StandIn::start(Box::new(move |form| {
        let issuer = i.lock().unwrap().clone();
        match form["grant_type"].as_str() {
            "authorization_code" => (
                200,
                json!({
                    "access_token": "access-1", "expires_in": 30, "refresh_token": "refresh-1",
                    "id_token": id_token(&issuer, n.lock().unwrap().as_deref()), "token_type": "bearer",
                }),
            ),
            "refresh_token" if form["refresh_token"] == "refresh-1" => (
                200,
                json!({"access_token": "access-2", "expires_in": 3600, "refresh_token": "refresh-2", "token_type": "bearer"}),
            ),
            _ => (400, json!({"error": "invalid_grant"})),
        }
    }));
    *issuer_slot.lock().unwrap() = server.issuer();

    let store = Arc::new(MemoryStore::default());
    let client = NativeClient::new(Config::new(server.issuer(), "desktop-app"), store.clone());
    assert!(client.session().unwrap().is_none());

    let pending = client.begin_sign_in().unwrap();
    let url = pending.url().to_string();
    assert!(url.starts_with(&format!("{}/oauth2/auth?", server.issuer())));
    let q: HashMap<String, String> = url::Url::parse(&url).unwrap().query_pairs().into_owned().collect();
    assert_eq!(q["client_id"], "desktop-app");
    assert_eq!(q["code_challenge_method"], "S256");
    assert_eq!(q["scope"], "openid profile email offline_access");
    assert!(q["redirect_uri"].starts_with("http://127.0.0.1:") && q["redirect_uri"].ends_with("/callback"));
    *nonce.lock().unwrap() = Some(nonce_of(&url));

    let browser = std::thread::spawn(move || browser_returns(&url, "code=the-code"));
    let session = pending.wait(Duration::from_secs(10)).unwrap();
    assert!(browser.join().unwrap().starts_with("HTTP/1.1 200"));
    assert_eq!(session.subject, "user-1");
    assert_eq!(session.claims["email"], "user@example.com");

    let exchange = &server.forms("/oauth2/token")[0];
    assert_eq!(exchange["code"], "the-code");
    assert_eq!(exchange["redirect_uri"], q["redirect_uri"]);
    assert_eq!(exchange["code_verifier"].len(), 43);

    // 30 seconds left is inside the 60-second margin: refreshed, and the new refresh token kept.
    assert_eq!(client.access_token().unwrap().as_deref(), Some("access-2"));
    let stored: Value = serde_json::from_str(&store.get("authway.desktop-app").unwrap().unwrap()).unwrap();
    assert_eq!(stored["refresh_token"], "refresh-2");
    // Fresh now: no second refresh.
    assert_eq!(client.access_token().unwrap().as_deref(), Some("access-2"));
    assert_eq!(server.forms("/oauth2/token").len(), 2);

    let out = client.sign_out().unwrap();
    assert!(out.was_signed_in && out.revoked);
    let revoked: Vec<String> = server.forms("/oauth2/revoke").iter().map(|f| f["token"].clone()).collect();
    assert_eq!(revoked, ["refresh-2", "access-2"]);
    assert!(client.session().unwrap().is_none());
    assert_eq!(client.access_token().unwrap(), None);
}

#[test]
fn a_refused_refresh_ends_the_session() {
    let server = StandIn::start(Box::new(|_| (400, json!({"error": "invalid_grant", "error_description": "revoked"}))));
    let store = Arc::new(MemoryStore::default());
    store
        .set(
            "authway.desktop-app",
            &json!({"access_token": "old", "expires_at": 0, "refresh_token": "spent", "claims": {"sub": "user-1"}}).to_string(),
        )
        .unwrap();
    let client = NativeClient::new(Config::new(server.issuer(), "desktop-app"), store.clone());
    assert_eq!(client.session().unwrap().unwrap().subject, "user-1");
    assert_eq!(client.access_token().unwrap(), None);
    assert!(store.get("authway.desktop-app").unwrap().is_none());
}

#[test]
fn another_token_endpoint_failure_keeps_the_session() {
    let server = StandIn::start(Box::new(|_| (503, json!({"code": "unavailable", "detail": "try later"}))));
    let store = Arc::new(MemoryStore::default());
    store
        .set("authway.desktop-app", &json!({"access_token": "old", "expires_at": 0, "refresh_token": "r"}).to_string())
        .unwrap();
    let client = NativeClient::new(Config::new(server.issuer(), "desktop-app"), store.clone());
    let err = client.access_token().unwrap_err();
    assert!(
        matches!(err, Error::Token { status: 503, ref error, .. } if error.as_deref() == Some("unavailable")),
        "{err}"
    );
    assert!(store.get("authway.desktop-app").unwrap().is_some());
}

#[test]
fn a_declined_sign_in_stores_nothing() {
    let server = StandIn::start(Box::new(|_| (500, json!({}))));
    let store = Arc::new(MemoryStore::default());
    let client = NativeClient::new(Config::new(server.issuer(), "desktop-app"), store.clone());
    let pending = client.begin_sign_in().unwrap();
    let url = pending.url().to_string();
    let browser = std::thread::spawn(move || browser_returns(&url, "error=access_denied"));
    let err = pending.wait(Duration::from_secs(10)).unwrap_err();
    browser.join().unwrap();
    assert!(matches!(err, Error::Denied { ref error, .. } if error == "access_denied"));
    assert!(store.get("authway.desktop-app").unwrap().is_none());
    assert!(server.forms("/oauth2/token").is_empty());
}

#[test]
fn a_discovery_document_for_another_issuer_is_refused() {
    let server = StandIn::start(Box::new(|_| (500, json!({}))));
    // Ask for the issuer under a different host name than the document names.
    let issuer = server.issuer().replace("127.0.0.1", "localhost");
    let client = NativeClient::new(Config::new(issuer, "desktop-app"), MemoryStore::default());
    assert!(matches!(client.begin_sign_in(), Err(Error::Discovery(_))));
}
