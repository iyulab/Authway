use base64::Engine as _;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use serde_json::{Map, Value};

use crate::error::Error;

/// How far a clock may be off before an expired ID token is refused.
const LEEWAY_SECONDS: u64 = 300;

/// The claims of an ID token received from the token endpoint, after checking that it was issued
/// by this issuer, to this client, for this sign-in, and has not expired.
///
/// The signature is not checked: the token came straight from the token endpoint over TLS, which
/// OpenID Connect Core §3.1.3.7 accepts in place of it. The claims are for showing who is signed
/// in; an API never trusts them from the app — it validates the access token itself.
pub(crate) fn claims(
    id_token: &str,
    issuer: &str,
    client_id: &str,
    nonce: Option<&str>,
    now: u64,
) -> Result<Map<String, Value>, Error> {
    let payload = id_token
        .split('.')
        .nth(1)
        .ok_or_else(|| Error::IdToken("it is not a JWT".into()))?;
    let bytes = URL_SAFE_NO_PAD
        .decode(payload.trim_end_matches('='))
        .map_err(|e| Error::IdToken(format!("its payload is not base64url: {e}")))?;
    let claims: Map<String, Value> =
        serde_json::from_slice(&bytes).map_err(|e| Error::IdToken(format!("its payload is not JSON: {e}")))?;

    if claims.get("iss").and_then(Value::as_str) != Some(issuer) {
        return Err(Error::IdToken("it was issued by another issuer".into()));
    }
    let for_us = match claims.get("aud") {
        Some(Value::String(aud)) => aud == client_id,
        Some(Value::Array(auds)) => auds.iter().any(|a| a.as_str() == Some(client_id)),
        _ => false,
    };
    if !for_us {
        return Err(Error::IdToken("it was issued to another client".into()));
    }
    match claims.get("exp").and_then(Value::as_u64) {
        Some(exp) if exp + LEEWAY_SECONDS >= now => {}
        Some(_) => return Err(Error::IdToken("it has expired".into())),
        None => return Err(Error::IdToken("it has no expiry".into())),
    }
    if let Some(nonce) = nonce {
        if claims.get("nonce").and_then(Value::as_str) != Some(nonce) {
            return Err(Error::IdToken("its nonce is not this sign-in's".into()));
        }
    }
    if claims.get("sub").and_then(Value::as_str).is_none_or(str::is_empty) {
        return Err(Error::IdToken("it has no subject".into()));
    }
    Ok(claims)
}

#[cfg(test)]
pub(crate) fn encode_for_test(claims: &Value) -> String {
    let header = URL_SAFE_NO_PAD.encode(br#"{"alg":"RS256","typ":"JWT"}"#);
    let payload = URL_SAFE_NO_PAD.encode(serde_json::to_vec(claims).unwrap());
    format!("{header}.{payload}.signature")
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    const NOW: u64 = 1_800_000_000;

    fn token(overrides: Value) -> String {
        let mut claims = json!({
            "iss": "https://auth.example", "aud": ["app"], "sub": "user-1",
            "exp": NOW + 3600, "nonce": "n-1",
        });
        for (k, v) in overrides.as_object().unwrap() {
            if v.is_null() {
                claims.as_object_mut().unwrap().remove(k);
            } else {
                claims[k] = v.clone();
            }
        }
        encode_for_test(&claims)
    }

    fn check(t: &str) -> Result<Map<String, Value>, Error> {
        claims(t, "https://auth.example", "app", Some("n-1"), NOW)
    }

    #[test]
    fn accepts_a_token_for_this_sign_in() {
        let c = check(&token(json!({}))).unwrap();
        assert_eq!(c["sub"], "user-1");
        assert!(claims(&token(json!({"aud": "app"})), "https://auth.example", "app", Some("n-1"), NOW).is_ok());
    }

    #[test]
    fn refuses_another_issuer_client_nonce_or_an_expired_token() {
        assert!(check(&token(json!({"iss": "https://other.example"}))).is_err());
        assert!(check(&token(json!({"aud": ["other"]}))).is_err());
        assert!(check(&token(json!({"nonce": "n-2"}))).is_err());
        assert!(check(&token(json!({"nonce": null}))).is_err());
        assert!(check(&token(json!({"exp": NOW - LEEWAY_SECONDS - 1}))).is_err());
        assert!(check(&token(json!({"sub": null}))).is_err());
        assert!(check("not-a-jwt").is_err());
    }

    #[test]
    fn a_refreshed_token_carries_no_nonce_to_check() {
        let t = token(json!({"nonce": null}));
        assert!(claims(&t, "https://auth.example", "app", None, NOW).is_ok());
    }
}
