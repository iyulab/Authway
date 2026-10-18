//! Signs in from the command line, shows who signed in, refreshes the access token, and signs out.
//!
//! ```text
//! cargo run --example sign_in -- <issuer> <client id> [--refresh-ahead <seconds>]
//! ```
//!
//! Register the client as public, with the redirect URI `http://127.0.0.1/callback` and the
//! `authorization_code` and `refresh_token` grants, and the scope `offline_access`. Open the printed
//! URL in a browser. `--refresh-ahead` larger than the access token's lifetime refreshes at once.
//!
//! Each step prints one line, `STEP value`, so a script can follow along.

use std::sync::Arc;
use std::time::Duration;

use authway_native::{Config, MemoryStore, NativeClient, TokenStore};

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let (issuer, client_id) = match (args.first(), args.get(1)) {
        (Some(i), Some(c)) => (i.clone(), c.clone()),
        _ => {
            eprintln!("usage: sign_in <issuer> <client id> [--refresh-ahead <seconds>]");
            std::process::exit(2);
        }
    };
    let mut config = Config::new(issuer, client_id.clone());
    if let Some(i) = args.iter().position(|a| a == "--refresh-ahead") {
        let seconds = args.get(i + 1).and_then(|s| s.parse().ok()).expect("--refresh-ahead takes seconds");
        config = config.refresh_ahead(Duration::from_secs(seconds));
    }
    if let Err(e) = run(config, &client_id) {
        println!("ERROR {e}");
        std::process::exit(1);
    }
}

fn run(config: Config, client_id: &str) -> Result<(), authway_native::Error> {
    let store = Arc::new(MemoryStore::default());
    let client = NativeClient::new(config, store.clone());
    let stored_refresh_token = || -> String {
        let raw = store.get(&format!("authway.{client_id}")).unwrap().unwrap_or_default();
        let value: serde_json::Value = serde_json::from_str(&raw).unwrap_or_default();
        value["refresh_token"].as_str().unwrap_or_default().to_string()
    };

    let pending = client.begin_sign_in()?;
    println!("URL {}", pending.url());
    let session = pending.wait(Duration::from_secs(300))?;
    println!("SIGNED_IN {}", session.subject);
    println!("REFRESH_BEFORE {}", stored_refresh_token());

    let token = client.access_token()?.unwrap_or_default();
    println!("ACCESS {token}");
    println!("REFRESH_AFTER {}", stored_refresh_token());

    let out = client.sign_out()?;
    println!("SIGNED_OUT revoked={}", out.revoked);
    Ok(())
}
