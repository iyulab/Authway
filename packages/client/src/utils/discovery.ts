import { ConfigurationError } from '../types/errors'

/** The OIDC endpoints the client uses, as published by the provider. */
export interface ProviderEndpoints {
  issuer: string
  authorizationEndpoint: string
  tokenEndpoint: string
  /** Absent when the provider does not support RP-initiated logout. */
  endSessionEndpoint?: string
  userinfoEndpoint?: string
}

/** Where to talk to: the provider's OIDC endpoints and the Authway API. */
export interface ResolvedProvider {
  endpoints: ProviderEndpoints
  /** Base URL of the Authway API (claims and other Authway-specific calls). */
  apiUrl: string
}

const trimSlash = (url: string) => url.replace(/\/+$/, '')

async function getJson(fetchImpl: typeof fetch, url: string): Promise<Response> {
  return fetchImpl(url, { headers: { Accept: 'application/json' } })
}

/**
 * Resolves the provider for an Authway deployment.
 *
 * 1. The issuer is `issuer` when given. Otherwise the Authway API's
 *    `/.well-known/authway-config` names it; a deployment without that
 *    document is taken to be its own issuer.
 * 2. Every endpoint then comes from the issuer's OpenID Provider metadata
 *    (OpenID Connect Discovery 1.0) — nothing is derived from URL patterns.
 *
 * The published `issuer` must equal the issuer the metadata was fetched for
 * (Discovery §4.3), so a misconfigured deployment fails here instead of
 * producing tokens the client cannot validate.
 */
export async function resolveProvider(
  domain: string,
  issuer?: string,
  fetchImpl: typeof fetch = fetch,
): Promise<ResolvedProvider> {
  let apiUrl = trimSlash(domain)
  let issuerUrl = issuer ? trimSlash(issuer) : undefined

  if (!issuerUrl) {
    let res: Response | undefined
    try {
      res = await getJson(fetchImpl, `${apiUrl}/.well-known/authway-config`)
    } catch (err) {
      throw new ConfigurationError(`Cannot reach the Authway API at ${apiUrl}: ${(err as Error).message}`)
    }
    if (res.ok) {
      const config = (await res.json()) as { issuer?: string; oauth_url?: string; api_url?: string }
      issuerUrl = trimSlash(config.issuer || config.oauth_url || apiUrl)
      if (config.api_url) apiUrl = trimSlash(config.api_url)
    } else if (res.status === 404) {
      issuerUrl = apiUrl
    } else {
      throw new ConfigurationError(`Authway configuration request failed: HTTP ${res.status}`)
    }
  }

  let res: Response
  try {
    res = await getJson(fetchImpl, `${issuerUrl}/.well-known/openid-configuration`)
  } catch (err) {
    throw new ConfigurationError(`Cannot reach the OIDC issuer at ${issuerUrl}: ${(err as Error).message}`)
  }
  if (!res.ok) {
    throw new ConfigurationError(`OIDC discovery failed for ${issuerUrl}: HTTP ${res.status}`)
  }
  const metadata = (await res.json()) as Record<string, unknown>

  if (typeof metadata.issuer !== 'string' || trimSlash(metadata.issuer) !== issuerUrl) {
    throw new ConfigurationError(
      `OIDC discovery for ${issuerUrl} published issuer ${String(metadata.issuer)} — they must match`,
    )
  }
  for (const required of ['authorization_endpoint', 'token_endpoint'] as const) {
    if (typeof metadata[required] !== 'string') {
      throw new ConfigurationError(`OIDC discovery for ${issuerUrl} has no ${required}`)
    }
  }

  return {
    apiUrl,
    endpoints: {
      issuer: metadata.issuer as string,
      authorizationEndpoint: metadata.authorization_endpoint as string,
      tokenEndpoint: metadata.token_endpoint as string,
      endSessionEndpoint: typeof metadata.end_session_endpoint === 'string' ? metadata.end_session_endpoint : undefined,
      userinfoEndpoint: typeof metadata.userinfo_endpoint === 'string' ? metadata.userinfo_endpoint : undefined,
    },
  }
}
