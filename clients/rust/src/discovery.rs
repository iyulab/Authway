use serde::Deserialize;

use crate::error::Error;

/// The parts of the OpenID Provider's metadata the client uses.
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct Discovery {
    pub issuer: String,
    pub authorization_endpoint: String,
    pub token_endpoint: String,
    #[serde(default)]
    pub revocation_endpoint: Option<String>,
}

pub(crate) fn fetch(agent: &ureq::Agent, issuer: &str) -> Result<Discovery, Error> {
    let url = format!("{issuer}/.well-known/openid-configuration");
    let mut response = agent
        .get(&url)
        .call()
        .map_err(|e| Error::Http(e.to_string()))?;
    let status = response.status().as_u16();
    let body = response
        .body_mut()
        .read_to_string()
        .map_err(|e| Error::Http(e.to_string()))?;
    if status != 200 {
        return Err(Error::Http(format!("{url} answered HTTP {status}")));
    }
    let discovery: Discovery =
        serde_json::from_str(&body).map_err(|e| Error::Discovery(format!("{url}: {e}")))?;
    // The document must describe the issuer it was asked for (OpenID Connect Discovery §4.3).
    if discovery.issuer.trim_end_matches('/') != issuer {
        return Err(Error::Discovery(format!(
            "it names the issuer {} instead of {issuer}",
            discovery.issuer
        )));
    }
    Ok(discovery)
}
