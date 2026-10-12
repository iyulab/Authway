/**
 * Authway client configuration
 */
export interface AuthwayConfig {
  /**
   * URL of your Authway deployment (e.g. 'https://auth.example.com', or
   * 'http://localhost:8080' in local development).
   *
   * The client asks it for `/.well-known/authway-config` to find the OIDC
   * issuer, then takes every endpoint from the issuer's OpenID Connect
   * discovery document. A deployment that serves no such document is treated
   * as its own issuer.
   */
  domain: string

  /**
   * OAuth 2.0 client ID
   */
  clientId: string

  /**
   * OIDC issuer URL. Set it only to skip asking `domain` for it — every
   * endpoint is still read from the issuer's discovery document.
   * @default discovered from domain
   */
  issuer?: string

  /**
   * Redirect URI after authentication
   * @default window.location.origin
   */
  redirectUri?: string

  /**
   * API audience identifier
   */
  audience?: string

  /**
   * OAuth scopes
   * @default 'openid profile email'
   */
  scope?: string

  /**
   * Ask for a refresh token, so that an access token that runs out is
   * renewed without sending the user through authorization again: adds
   * `offline_access` to the requested scope. The client must be registered
   * for it — the `refresh_token` grant and the `offline_access` scope —
   * or the provider refuses the sign-in.
   *
   * A refresh token is kept where the other tokens are (`cacheLocation`).
   * @default false
   */
  useRefreshTokens?: boolean

  /**
   * Where the tokens are kept.
   * - 'memory': only in the page. A reload or a new tab has none; call
   *   `checkSession()` to get them again from the user's session at the
   *   provider (a round trip there and back).
   * - 'sessionstorage': in the tab. They survive a reload and end with the
   *   tab; a new tab starts without them, as with 'memory'.
   * - 'localstorage': in the browser, shared by every tab, until logout.
   *
   * No location keeps tokens from a script running in the page — one that
   * cannot read them can still ask the provider for new ones. The choice is
   * how long, and how widely, tokens lie around.
   * @default 'localstorage'
   */
  cacheLocation?: 'memory' | 'sessionstorage' | 'localstorage'

  /**
   * Tenant ID for multi-tenant mode
   */
  tenantId?: string

  /**
   * Custom token leeway for expiration checks (seconds)
   * @default 60
   */
  leeway?: number

  /**
   * Maximum token age for silent refresh (seconds)
   * @default 86400 (24 hours)
   */
  maxAge?: number

  /**
   * Enable DPoP (Demonstrating Proof-of-Possession) RFC 9449
   * Adds an extra layer of security by binding tokens to a cryptographic key
   * @default false
   */
  useDPoP?: boolean
}

/**
 * Normalized configuration with defaults applied
 */
export interface NormalizedConfig extends Required<Omit<AuthwayConfig, 'audience' | 'tenantId' | 'issuer'>> {
  audience?: string
  tenantId?: string
  issuer?: string
}
