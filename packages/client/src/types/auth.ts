import { User } from './user'

/**
 * OAuth redirect login options
 */
export interface RedirectLoginOptions {
  /**
   * The way of signing in to start with, e.g. 'google': the sign-in screen
   * goes straight on to that provider. Ignored unless the client offers it.
   */
  connection?: string

  /**
   * The address the application expects to sign in; the sign-in screen fills
   * it in (OpenID Connect `login_hint`).
   */
  login_hint?: string

  /**
   * Screen hint ('login' or 'signup')
   */
  screen_hint?: 'login' | 'signup'

  /**
   * The languages to show the sign-in screen in, most preferred first,
   * separated by spaces — e.g. 'ko en' (OpenID Connect `ui_locales`).
   */
  ui_locales?: string

  /**
   * Application state to preserve
   */
  appState?: any

  /**
   * Custom redirect URI for this request
   */
  redirectUri?: string

  /**
   * Additional query parameters
   */
  [key: string]: any
}

/**
 * Options of `checkSession()`
 */
export interface CheckSessionOptions {
  /**
   * Application state to get back from `handleRedirectCallback`.
   * @default { returnTo: <the address the page is at, without its origin> }
   */
  appState?: any

  /**
   * Custom redirect URI for this request
   */
  redirectUri?: string
}

/**
 * Popup login options
 */
export interface PopupLoginOptions {
  connection?: string
  screen_hint?: 'login' | 'signup'
  ui_locales?: string
  /**
   * Custom redirect URI for this request
   */
  redirectUri?: string
  /**
   * Additional query parameters
   */
  [key: string]: any
}

/**
 * Logout options
 */
export interface LogoutOptions {
  /**
   * URL to redirect after logout
   */
  returnTo?: string

  /**
   * Federated logout (logout from identity provider)
   */
  federated?: boolean

  /**
   * Local logout only (don't redirect)
   */
  localOnly?: boolean
}

/**
 * Authentication result
 */
export interface AuthResult {
  accessToken: string
  idToken: string
  refreshToken?: string
  expiresIn: number
  user: User
}

/**
 * Redirect callback result
 */
export interface RedirectLoginResult extends AuthResult {
  appState?: any
}

/**
 * Token claims (JWT payload)
 */
export interface TokenClaims {
  iss: string
  sub: string
  aud: string | string[]
  exp: number
  iat: number
  azp?: string
  scope?: string
  [key: string]: any
}

/**
 * Get token options
 */
export interface GetTokenOptions {
  /**
   * Force token refresh even if not expired
   */
  ignoreCache?: boolean

  /**
   * Custom audience for this token request
   */
  audience?: string

  /**
   * Custom scope for this token request
   */
  scope?: string

  /**
   * Timeout for the request (milliseconds)
   */
  timeoutInSeconds?: number
}

/**
 * Session state
 */
export interface SessionState {
  isAuthenticated: boolean
  user: User | null
  accessToken: string | null
  idToken: string | null
  expiresAt: number | null
}

/**
 * PKCE challenge
 */
export interface PKCEChallenge {
  codeVerifier: string
  codeChallenge: string
  state: string
  nonce: string
}
