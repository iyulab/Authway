/**
 * Authway client configuration
 */
export interface AuthwayConfig {
  /**
   * Auth Backend URL (e.g., 'http://localhost:8081' or 'https://auth.authway.example.com')
   * This is the Auth Backend server that proxies API calls to Central API and handles CORS.
   *
   * For local development:
   * - Use 'http://localhost:8081' (Auth Backend - recommended for SPAs)
   * - Auth Backend proxies API calls to Central API (port 8080) and provides CORS support
   *
   * OAuth server (Hydra) is auto-detected from this URL (port 8080/8081 → 4444)
   */
  domain: string

  /**
   * OAuth 2.0 client ID
   */
  clientId: string

  /**
   * OAuth server URL (advanced usage only)
   * Auto-detected from domain for most cases
   * For local dev: domain with :8080 or :8081 → auto-changed to :4444 for Hydra
   * Only override this if you have a custom OAuth server setup
   * @default auto-detected from domain
   */
  oauthServerUrl?: string

  /**
   * Central API URL (advanced usage only)
   * Explicitly specify the Central API URL if different from domain
   * @deprecated Use 'domain' instead - this will be removed in future versions
   * @default same as domain
   */
  authwayUrl?: string

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
export interface NormalizedConfig extends Required<Omit<AuthwayConfig, 'audience' | 'tenantId' | 'authwayUrl' | 'oauthServerUrl'>> {
  oauthServerUrl: string  // OAuth server (Hydra) URL
  centralApiUrl: string   // Central API URL for user/claims APIs
  audience?: string
  tenantId?: string
  /** @deprecated Use centralApiUrl instead */
  authwayUrl: string
}
