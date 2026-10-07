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
   * Enable refresh tokens
   * @default true
   */
  useRefreshTokens?: boolean

  /**
   * Token cache location
   * - 'memory': Most secure, tokens lost on page refresh
   * - 'localstorage': Persistent, vulnerable to XSS
   * @default 'memory'
   */
  cacheLocation?: 'memory' | 'localstorage'

  /**
   * Tenant ID for multi-tenant mode
   */
  tenantId?: string

  /**
   * Enable dynamic claims support
   * @default true
   */
  enableDynamicClaims?: boolean

  /**
   * Auto-sync claims interval (milliseconds)
   * Set to 0 to disable
   * @default 0
   */
  claimsUpdateInterval?: number

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
