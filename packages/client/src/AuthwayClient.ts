import {
  AuthwayConfig,
  NormalizedConfig,
  RedirectLoginOptions,
  PopupLoginOptions,
  LogoutOptions,
  AuthResult,
  RedirectLoginResult,
  GetTokenOptions,
  User,
  Claims,
  SessionState,
  PKCEChallenge,
  ConfigurationError,
  AuthenticationError,
  LoginRequiredError,
  MissingRefreshTokenError,
  PopupTimeoutError,
  PopupCancelledError
} from './types'
import {
  generatePKCEChallenge,
  verifyState,
  decodeToken,
  extractUser,
  getTokenExpiration,
  createStorage,
  IStorage,
  buildUrl,
  parseCallbackUrl,
  postForm,
  patch,
  createSessionStorage,
  getDPoPKeyPair,
  createDPoPProof,
  resolveProvider,
  ResolvedProvider,
  ProviderEndpoints
} from './utils'

const DEFAULT_SCOPE = 'openid profile email'

/**
 * Main Authway authentication client
 */
export class AuthwayClient {
  private config: NormalizedConfig
  private storage: IStorage
  private sessionStorage: IStorage
  private tokenRefreshTimer?: number
  private dpopKeyPair: CryptoKeyPair | null = null

  // In-memory cache
  private cache = {
    user: null as User | null,
    accessToken: null as string | null,
    idToken: null as string | null,
    refreshToken: null as string | null,
    expiresAt: null as number | null
  }

  private configReady: Promise<void>
  private provider?: ResolvedProvider

  /**
   * Static method to handle popup callback context.
   * Call this before creating AuthwayClient to check if running in a popup.
   * If in popup context with OAuth params, sends postMessage and closes window.
   *
   * @returns true if popup callback was handled (window will close), false otherwise
   */
  static handlePopupCallback(): boolean {
    if (typeof window === 'undefined') return false

    const params = new URLSearchParams(window.location.search)
    const code = params.get('code')
    const state = params.get('state')
    const error = params.get('error')
    const errorDescription = params.get('error_description')

    // Check if we have OAuth params
    const hasOAuthParams = (code && state) || error

    if (!hasOAuthParams) return false

    // Check if we're in a popup (window.opener exists)
    let isPopup: boolean
    try {
      isPopup = !!(window.opener && !window.opener.closed)
    } catch {
      // COOP policy may block access to window.opener
      // Try alternative detection via window.name or sessionStorage
      isPopup = window.name === 'authway-login' ||
                sessionStorage.getItem('authway_popup_context') === 'true'
    }

    if (!isPopup) return false

    // We're in a popup with OAuth params - send message to parent and close
    try {
      const message = {
        type: 'authway-callback',
        code,
        state,
        error,
        error_description: errorDescription
      }

      // Send to parent window
      window.opener.postMessage(message, window.location.origin)

      console.log('✅ Authway: Popup callback handled automatically')

      // Close the popup
      window.close()

      return true
    } catch (e) {
      console.warn('⚠️ Authway: Failed to send postMessage to parent window:', e)

      // Fallback: Store in localStorage for parent to pick up
      try {
        localStorage.setItem('authway_popup_result', JSON.stringify({
          type: 'authway-callback',
          code,
          state,
          error,
          error_description: errorDescription,
          timestamp: Date.now()
        }))
        window.close()
        return true
      } catch (storageError) {
        console.error('⚠️ Authway: Fallback storage also failed:', storageError)
      }

      return false
    }
  }

  constructor(config: AuthwayConfig) {
    this.config = this.normalizeConfig(config)
    this.storage = createStorage(this.config.cacheLocation)
    this.sessionStorage = createSessionStorage()
    this.loadFromStorage()

    // Initialize DPoP if enabled
    if (this.config.useDPoP) {
      this.initializeDPoP()
    }
    
    // Discover the provider's endpoints. Methods that need them await this;
    // the no-op catch only keeps an unobserved failure from being reported
    // as an unhandled rejection before anyone asks.
    this.configReady = this.discoverProvider()
    this.configReady.catch(() => {})
  }

  /**
   * Wait for client to be ready (config loaded)
   */
  async waitForReady(): Promise<void> {
    await this.configReady
  }

  /**
   * Initialize DPoP key pair
   */
  private async initializeDPoP() {
    try {
      this.dpopKeyPair = await getDPoPKeyPair(this.storage)
    } catch (err) {
      console.error('Failed to initialize DPoP:', err)
    }
  }

  // ==========================================
  // Configuration
  // ==========================================

  private normalizeConfig(config: AuthwayConfig): NormalizedConfig {
    if (!config.domain) {
      throw new ConfigurationError('domain is required — the URL of your Authway deployment')
    }

    if (!config.clientId) {
      throw new ConfigurationError('clientId is required')
    }

    let domain = config.domain
    if (!domain.startsWith('http://') && !domain.startsWith('https://')) {
      domain = `https://${domain}`
    }

    let redirectUri = config.redirectUri
    if (!redirectUri && typeof window !== 'undefined') {
      redirectUri = window.location.origin
    }
    if (!redirectUri) {
      console.warn(
        '⚠️ Authway: No redirectUri configured and window.location.origin not available.\n' +
        'Users may land on auth server after login/logout. Set redirectUri in config.'
      )
      redirectUri = ''
    }

    return {
      domain: domain.replace(/\/+$/, ''),
      issuer: config.issuer,
      clientId: config.clientId,
      redirectUri,
      audience: config.audience,
      scope: config.scope || DEFAULT_SCOPE,
      useRefreshTokens: config.useRefreshTokens ?? true,
      cacheLocation: config.cacheLocation || 'localstorage',
      tenantId: config.tenantId,
      enableDynamicClaims: config.enableDynamicClaims ?? true,
      claimsUpdateInterval: config.claimsUpdateInterval || 0,
      leeway: config.leeway || 60,
      maxAge: config.maxAge || 86400,
      useDPoP: config.useDPoP || false
    }
  }

  private async discoverProvider(): Promise<void> {
    this.provider = await resolveProvider(this.config.domain, this.config.issuer)
  }

  /** The provider's OIDC endpoints, once discovery has finished. */
  private async endpoints(): Promise<ProviderEndpoints> {
    await this.configReady
    return this.provider!.endpoints
  }

  /** Base URL for Authway-specific APIs (claims), once discovery has finished. */
  private async apiUrl(): Promise<string> {
    await this.configReady
    return this.provider!.apiUrl
  }

  // ==========================================
  // OAuth Flow
  // ==========================================

  /**
   * Redirect to Authway hosted login
   */
  async loginWithRedirect(options: RedirectLoginOptions = {}): Promise<void> {
    const pkce = await generatePKCEChallenge()

    // Store PKCE challenge, redirect URI, and app state in sessionStorage
    this.sessionStorage.set('pkce', JSON.stringify(pkce))
    this.sessionStorage.set('redirectUri', options.redirectUri || this.config.redirectUri)
    if (options.appState) {
      this.sessionStorage.set('appState', JSON.stringify(options.appState))
    }

    const authUrl = await this.buildAuthorizationUrl(pkce, options)

    if (typeof window !== 'undefined') {
      window.location.assign(authUrl)
    }
  }

  /**
   * Handle OAuth callback
   */
  async handleRedirectCallback(url?: string): Promise<RedirectLoginResult> {
    const params = parseCallbackUrl(url)

    if (params.error) {
      throw new AuthenticationError(
        params.error_description || params.error,
        params.error
      )
    }

    if (!params.code || !params.state) {
      throw new AuthenticationError('Missing code or state parameter')
    }

    // Verify state from sessionStorage
    const storedPkce = this.sessionStorage.get('pkce')
    if (!storedPkce) {
      throw new AuthenticationError('No PKCE challenge found')
    }

    const pkce: PKCEChallenge = JSON.parse(storedPkce)
    if (!verifyState(params.state, pkce.state)) {
      throw new AuthenticationError('State mismatch')
    }

    // Get redirect URI from sessionStorage
    const redirectUri = this.sessionStorage.get('redirectUri') || this.config.redirectUri

    // Exchange code for tokens
    const result = await this.exchangeCodeForTokens(params.code, pkce.codeVerifier, redirectUri)

    // Get app state from sessionStorage
    const appStateStr = this.sessionStorage.get('appState')
    const appState = appStateStr ? JSON.parse(appStateStr) : undefined

    // Clean up sessionStorage
    this.sessionStorage.remove('pkce')
    this.sessionStorage.remove('redirectUri')
    this.sessionStorage.remove('appState')

    // Save tokens
    this.saveTokens(result)

    return {
      ...result,
      appState
    }
  }

  /**
   * Login with popup - opens login UI in popup window
   * User remains on the app, popup closes automatically after auth
   */
  async loginWithPopup(options: PopupLoginOptions = {}): Promise<AuthResult> {
    const pkce = await generatePKCEChallenge()

    // Store PKCE challenge in sessionStorage (for when popup redirects back)
    this.sessionStorage.set('pkce_popup', JSON.stringify(pkce))

    // Build authorization URL
    const authUrl = await this.buildAuthorizationUrl(pkce, options)

    // Open popup window
    const popup = window.open(
      authUrl,
      'authway-login',
      'width=500,height=700,scrollbars=yes,location=no,toolbar=no,menubar=no'
    )

    if (!popup) {
      this.sessionStorage.remove('pkce_popup')
      throw new AuthenticationError('Popup was blocked. Please allow popups for this site.')
    }

    // Wait for popup to complete OAuth flow
    return new Promise((resolve, reject) => {
      // Cleanup function
      const cleanup = () => {
        if (timeoutId) clearTimeout(timeoutId)
        if (intervalId) clearInterval(intervalId)
        window.removeEventListener('message', messageHandler)
      }

      // Timeout handler
      const timeoutId = window.setTimeout(() => {
        cleanup()
        try {
          popup.close()
        } catch {
          // Ignore COOP error on close
        }
        this.sessionStorage.remove('pkce_popup')
        reject(new PopupTimeoutError('Login timeout', popup))
      }, 300000) // 5 minutes

      // Message handler for postMessage communication (COOP-safe)
      const messageHandler = async (event: MessageEvent) => {
        // Security: Verify origin
        // Allow localhost for development
        const isLocalhost = event.origin.includes('localhost') || event.origin.includes('127.0.0.1')
        const isConfiguredOrigin = event.origin === window.location.origin

        if (!isLocalhost && !isConfiguredOrigin) {
          return // Ignore messages from unknown origins
        }

        // Check if this is an Authway callback message
        if (event.data && event.data.type === 'authway-callback') {
          cleanup()

          try {
            popup.close()
          } catch {
            // Ignore COOP error
          }

          this.sessionStorage.remove('pkce_popup')

          const { code, state, error, error_description } = event.data

          // Handle errors
          if (error) {
            return reject(new AuthenticationError(
              error_description || error,
              error
            ))
          }

          if (!code || !state) {
            return reject(new AuthenticationError('Missing code or state parameter'))
          }

          // Verify state
          if (!verifyState(state, pkce.state)) {
            return reject(new AuthenticationError('State mismatch'))
          }

          try {
            // Exchange code for tokens
            const result = await this.exchangeCodeForTokens(
              code,
              pkce.codeVerifier,
              options.redirectUri || this.config.redirectUri
            )

            // Save tokens
            this.saveTokens(result)

            resolve(result)
          } catch (err) {
            reject(err)
          }
        }
      }

      // Listen for postMessage from popup
      window.addEventListener('message', messageHandler)

      // Fallback: Check if popup is closed or localStorage fallback result
      const intervalId = window.setInterval(async () => {
        // Check localStorage fallback (for COOP-blocked scenarios)
        try {
          const fallbackResult = localStorage.getItem('authway_popup_result')
          if (fallbackResult) {
            const data = JSON.parse(fallbackResult)
            // Check if result is recent (within 30 seconds)
            if (Date.now() - data.timestamp < 30000) {
              localStorage.removeItem('authway_popup_result')

              // Create a synthetic MessageEvent-like object
              const syntheticEvent = {
                data,
                origin: window.location.origin
              } as MessageEvent

              messageHandler(syntheticEvent)
              return
            } else {
              // Old result, remove it
              localStorage.removeItem('authway_popup_result')
            }
          }
        } catch {
          // Ignore localStorage errors
        }

        // Check if popup is closed
        try {
          if (popup.closed) {
            cleanup()
            this.sessionStorage.remove('pkce_popup')
            reject(new PopupCancelledError('Popup was closed by user', popup))
          }
        } catch {
          // COOP policy blocks popup.closed access - continue
        }
      }, 500) // Check every 500ms for faster response
    })
  }

  /**
   * Logout
   */
  async logout(options: LogoutOptions = {}): Promise<void> {
    // Local only logout
    if (options.localOnly) {
      this.clearTokens()
      if (this.tokenRefreshTimer) {
        clearTimeout(this.tokenRefreshTimer)
        this.tokenRefreshTimer = undefined
      }
      return
    }

    // Get ID token before clearing (needed for logout URL)
    const idToken = this.cache.idToken

    // Clear tokens
    this.clearTokens()

    // Clear refresh timer
    if (this.tokenRefreshTimer) {
      clearTimeout(this.tokenRefreshTimer)
      this.tokenRefreshTimer = undefined
    }

    // Redirect to logout endpoint
    if (typeof window !== 'undefined') {
      const logoutUrl = await this.buildLogoutUrl(options, idToken)
      if (logoutUrl) window.location.assign(logoutUrl)
    }
  }

  // ==========================================
  // Token Management
  // ==========================================

  /**
   * Get valid access token (auto-refresh if needed)
   */
  async getAccessToken(options: GetTokenOptions = {}): Promise<string> {
    // Use cached token if valid
    if (!options.ignoreCache && this.cache.accessToken && this.cache.expiresAt) {
      // Check expiration using cached expiresAt (supports opaque tokens)
      const now = Date.now()
      const leewayMs = (this.config.leeway || 60) * 1000
      if (this.cache.expiresAt > (now + leewayMs)) {
        return this.cache.accessToken
      }
    }

    // Try to refresh
    if (this.cache.refreshToken) {
      await this.refreshTokens()
      if (this.cache.accessToken) {
        return this.cache.accessToken
      }
    }

    throw new LoginRequiredError()
  }

  /**
   * Get access token with popup
   * Opens a popup to get a new access token (useful when refresh token is not available)
   * Similar to Auth0's getAccessTokenWithPopup()
   */
  async getAccessTokenWithPopup(options: PopupLoginOptions = {}): Promise<string> {
    const result = await this.loginWithPopup(options)
    return result.accessToken
  }

  /**
   * Get ID token
   */
  async getIdToken(): Promise<string> {
    if (!this.cache.idToken) {
      throw new LoginRequiredError()
    }

    return this.cache.idToken
  }

  /**
   * Refresh tokens
   */
  async refreshTokens(): Promise<void> {
    if (!this.cache.refreshToken) {
      throw new MissingRefreshTokenError()
    }

    const url = (await this.endpoints()).tokenEndpoint

    // Add DPoP header if enabled
    const headers: Record<string, string> = {}
    if (this.config.useDPoP && this.dpopKeyPair) {
      const dpopProof = await createDPoPProof(
        this.dpopKeyPair,
        'POST',
        url
      )
      headers['DPoP'] = dpopProof
    }

    const response = await postForm<any>(url, {
      grant_type: 'refresh_token',
      refresh_token: this.cache.refreshToken,
      client_id: this.config.clientId
    }, headers)

    const result: AuthResult = {
      accessToken: response.access_token,
      idToken: response.id_token,
      refreshToken: response.refresh_token || this.cache.refreshToken,
      expiresIn: response.expires_in,
      user: extractUser(response.id_token)
    }

    this.saveTokens(result)
  }

  // ==========================================
  // User & Claims
  // ==========================================

  /**
   * Get current user
   */
  async getUser(): Promise<User | null> {
    if (!this.cache.idToken) {
      return null
    }

    if (!this.cache.user) {
      this.cache.user = extractUser(this.cache.idToken)
    }

    return this.cache.user
  }

  /**
   * Get ID token claims
   * Similar to Auth0's getIdTokenClaims()
   *
   * @returns The decoded ID token payload
   */
  async getIdTokenClaims(): Promise<any | null> {
    if (!this.cache.idToken) {
      return null
    }

    return decodeToken(this.cache.idToken)
  }

  /**
   * Get user claims (combines system claims from ID token + dynamic claims from backend)
   */
  async getClaims(): Promise<Claims> {
    const user = await this.getUser()
    if (!user) {
      return {}
    }

    // 1. Get ALL claims from ID token (system claims)
    const idToken = await this.getIdToken()
    const tokenClaims = decodeToken(idToken)

    // Filter out JWT-specific fields, keep only user claims
    const systemClaims: Claims = { ...tokenClaims }
    delete systemClaims.iss
    delete systemClaims.aud
    delete systemClaims.exp
    delete systemClaims.iat
    delete systemClaims.auth_time
    delete systemClaims.nonce
    delete systemClaims.at_hash
    delete systemClaims.c_hash
    delete systemClaims.sid

    // 2. Get dynamic claims from backend API
    try {
      const accessToken = await this.getAccessToken()
      const url = `${await this.apiUrl()}/api/v1/claims`

      const response = await fetch(url, {
        headers: {
          Authorization: `Bearer ${accessToken}`
        }
      })

      if (response.ok) {
        const data = await response.json()
        const dynamicClaims = data.claims || {}

        // 3. Merge: system claims + dynamic claims (dynamic overrides system if same key)
        return {
          ...systemClaims,
          ...dynamicClaims
        }
      }
    } catch (err) {
      // If backend fails, return just system claims
      console.warn('Failed to fetch dynamic claims from backend:', err)
    }

    // Fallback: return only system claims if backend fails
    return systemClaims
  }

  /**
   * Update dynamic claims
   * Note: Claims updates require re-authentication to take effect
   * This method will automatically redirect to re-authenticate
   */
  async updateClaims(claims: Partial<Claims>): Promise<void> {
    if (!this.config.enableDynamicClaims) {
      throw new Error('Dynamic claims not enabled')
    }

    const accessToken = await this.getAccessToken()
    const url = `${await this.apiUrl()}/api/v1/claims`

    // Backend requires client_id and redirect_uri for OAuth flow
    const payload = {
      claims,
      client_id: this.config.clientId,
      redirect_uri: typeof window !== 'undefined' ? window.location.origin : this.config.redirectUri
    }

    const response = await patch<{ success: boolean; auth_url: string; message: string }>(
      url,
      payload,
      { Authorization: `Bearer ${accessToken}` }
    )

    // Claims update requires re-authentication
    // Automatically start new login flow to get updated token
    if (response.auth_url) {
      // Store app state before redirect
      const appState = { returnTo: window.location.pathname }
      this.sessionStorage.set('appState', JSON.stringify(appState))

      // Redirect to re-authenticate and get new token with updated claims
      if (typeof window !== 'undefined') {
        window.location.href = response.auth_url
      }
    }
  }

  /**
   * Update user-level claims (NO re-authentication required)
   * Use this for user preferences, metadata, custom data
   */
  async updateUserClaims(claims: Partial<Claims>): Promise<void> {
    const accessToken = await this.getAccessToken()
    const url = `${await this.apiUrl()}/api/v1/claims/user`

    await patch(
      url,
      { claims },
      { Authorization: `Bearer ${accessToken}` }
    )
  }

  /**
   * Get user-level claims (claim_type='user')
   */
  async getUserClaims(): Promise<Claims> {
    const accessToken = await this.getAccessToken()
    const url = `${await this.apiUrl()}/api/v1/claims/user`

    const response = await fetch(url, {
      headers: {
        Authorization: `Bearer ${accessToken}`
      }
    })

    if (!response.ok) {
      throw new Error('Failed to get user claims')
    }

    const data = await response.json()
    return data.claims || {}
  }

  // ==========================================
  // Session
  // ==========================================

  /**
   * Check if authenticated
   */
  async isAuthenticated(): Promise<boolean> {
    try {
      await this.getAccessToken()
      return true
    } catch {
      return false
    }
  }

  /**
   * Check session (silent authentication)
   */
  async checkSession(): Promise<AuthResult | null> {
    // TODO: Implement silent authentication with iframe
    return null
  }

  /**
   * Get session state
   */
  getSessionState(): SessionState {
    return {
      isAuthenticated: !!this.cache.accessToken,
      user: this.cache.user,
      accessToken: this.cache.accessToken,
      idToken: this.cache.idToken,
      expiresAt: this.cache.expiresAt
    }
  }

  // ==========================================
  // Private Methods
  // ==========================================

  private async buildAuthorizationUrl(pkce: PKCEChallenge, options: RedirectLoginOptions): Promise<string> {
    // Determine redirect URI with fallback
    let redirectUri = options.redirectUri || this.config.redirectUri
    if (!redirectUri && typeof window !== 'undefined') {
      redirectUri = window.location.origin
      console.warn(
        '⚠️ Authway: No redirectUri configured for login. Using window.location.origin.\n' +
        'To fix: Set redirectUri in AuthwayConfig or pass redirectUri in login options.'
      )
    }

    const params = {
      client_id: this.config.clientId,
      response_type: 'code',
      redirect_uri: redirectUri,
      scope: this.config.scope,
      state: pkce.state,
      nonce: pkce.nonce,
      code_challenge: pkce.codeChallenge,
      code_challenge_method: 'S256',
      ...options
    }

    delete params.appState

    return buildUrl((await this.endpoints()).authorizationEndpoint, params)
  }

  /**
   * The RP-initiated logout URL, or — when the provider publishes no
   * end-session endpoint — the post-logout URI itself (the local session is
   * already cleared). Null when there is nowhere to send the user.
   */
  private async buildLogoutUrl(options: LogoutOptions, idToken: string | null): Promise<string | null> {
    const params: any = {
      client_id: this.config.clientId
    }

    // Add id_token_hint if available (required when using post_logout_redirect_uri)
    if (idToken) {
      params.id_token_hint = idToken
    }

    // Use returnTo if provided, otherwise default to redirectUri (client's origin)
    // This ensures users return to the app home instead of an error page
    const postLogoutUri = options.returnTo || this.config.redirectUri
    if (postLogoutUri) {
      params.post_logout_redirect_uri = postLogoutUri
    } else {
      // Fallback to window.location.origin to prevent user landing on auth server
      const fallbackUri = typeof window !== 'undefined' ? window.location.origin : ''
      if (fallbackUri) {
        params.post_logout_redirect_uri = fallbackUri
        console.warn(
          '⚠️ Authway: No redirectUri configured. Using window.location.origin as logout redirect.\n' +
          'To fix: Set redirectUri in AuthwayConfig or pass returnTo in logout options.'
        )
      } else {
        console.error(
          '❌ Authway: No redirect URI available for logout. User may land on auth server.\n' +
          'To fix: Configure redirectUri in AuthwayConfig or call logout({ returnTo: "your-url" })'
        )
      }
    }

    if (options.federated) {
      params.federated = 'true'
    }

    const { endSessionEndpoint } = await this.endpoints()
    if (!endSessionEndpoint) return params.post_logout_redirect_uri || null
    return buildUrl(endSessionEndpoint, params)
  }

  private async exchangeCodeForTokens(code: string, codeVerifier: string, redirectUri?: string): Promise<AuthResult> {
    const url = (await this.endpoints()).tokenEndpoint

    // Add DPoP header if enabled
    const headers: Record<string, string> = {}
    if (this.config.useDPoP && this.dpopKeyPair) {
      const dpopProof = await createDPoPProof(
        this.dpopKeyPair,
        'POST',
        url
      )
      headers['DPoP'] = dpopProof
    }

    const response = await postForm<any>(url, {
      grant_type: 'authorization_code',
      code,
      code_verifier: codeVerifier,
      client_id: this.config.clientId,
      redirect_uri: redirectUri || this.config.redirectUri
    }, headers)

    return {
      accessToken: response.access_token,
      idToken: response.id_token,
      refreshToken: response.refresh_token,
      expiresIn: response.expires_in,
      user: extractUser(response.id_token)
    }
  }

  private saveTokens(result: AuthResult): void {
    this.cache.accessToken = result.accessToken
    this.cache.idToken = result.idToken
    this.cache.refreshToken = result.refreshToken || null
    this.cache.user = result.user

    // Calculate expiration from expires_in (supports both JWT and opaque tokens)
    if (result.expiresIn) {
      this.cache.expiresAt = Date.now() + (result.expiresIn * 1000)
    } else {
      // Fallback: Try to get expiration from access_token (JWT only)
      try {
        this.cache.expiresAt = getTokenExpiration(result.accessToken)
      } catch {
        // If token is opaque, set default expiration (1 hour)
        console.warn('Cannot decode access_token, using default expiration')
        this.cache.expiresAt = Date.now() + (3600 * 1000)
      }
    }

    // Persist to storage if using localStorage
    if (this.config.cacheLocation === 'localstorage') {
      this.storage.set('accessToken', result.accessToken)
      this.storage.set('idToken', result.idToken)
      this.storage.set('expiresAt', this.cache.expiresAt.toString())
      if (result.refreshToken) {
        this.storage.set('refreshToken', result.refreshToken)
      }
    }

    // Schedule token refresh
    this.scheduleTokenRefresh(result.expiresIn)
  }

  private clearTokens(): void {
    this.cache.accessToken = null
    this.cache.idToken = null
    this.cache.refreshToken = null
    this.cache.user = null
    this.cache.expiresAt = null

    this.storage.clear()
  }

  private loadFromStorage(): void {
    if (this.config.cacheLocation === 'localstorage') {
      const accessToken = this.storage.get('accessToken')
      const idToken = this.storage.get('idToken')
      const refreshToken = this.storage.get('refreshToken')
      const expiresAt = this.storage.get('expiresAt')

      if (accessToken && idToken) {
        this.cache.accessToken = accessToken
        this.cache.idToken = idToken
        this.cache.refreshToken = refreshToken
        this.cache.user = extractUser(idToken)

        // Load expiration time from storage (supports opaque tokens)
        if (expiresAt) {
          this.cache.expiresAt = parseInt(expiresAt, 10)
        } else {
          // Fallback: Try to decode token (JWT only)
          try {
            this.cache.expiresAt = getTokenExpiration(accessToken)
          } catch {
            // If token is opaque and no expiresAt stored, set default (1 hour)
            console.warn('Cannot determine token expiration, using default')
            this.cache.expiresAt = Date.now() + (3600 * 1000)
          }
        }
      }
    }
  }

  private scheduleTokenRefresh(expiresIn: number): void {
    if (this.tokenRefreshTimer) {
      clearTimeout(this.tokenRefreshTimer)
    }

    // Refresh 1 minute before expiration
    const refreshIn = (expiresIn - 60) * 1000

    if (refreshIn > 0) {
      this.tokenRefreshTimer = setTimeout(() => {
        this.refreshTokens().catch(err => {
          console.error('Token refresh failed:', err)
        })
      }, refreshIn) as any
    }
  }
}
