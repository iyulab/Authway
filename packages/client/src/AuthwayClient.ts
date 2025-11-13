import {
  AuthwayConfig,
  NormalizedConfig,
  RedirectLoginOptions,
  PasswordCredentials,
  PopupLoginOptions,
  LogoutOptions,
  AuthResult,
  RedirectLoginResult,
  GetTokenOptions,
  User,
  Claims,
  Identity,
  LinkAccountOptions,
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
  isTokenExpired,
  getTokenExpiration,
  createStorage,
  IStorage,
  buildUrl,
  parseCallbackUrl,
  post,
  postForm,
  patch,
  createSessionStorage,
  getDPoPKeyPair,
  createDPoPProof
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

  constructor(config: AuthwayConfig) {
    this.config = this.normalizeConfig(config)
    this.storage = createStorage(this.config.cacheLocation)
    this.sessionStorage = createSessionStorage()
    this.loadFromStorage()

    // Initialize DPoP if enabled
    if (this.config.useDPoP) {
      this.initializeDPoP()
    }
    
    // Fetch remote config (async)
    this.configReady = this.fetchRemoteConfig()
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
      throw new ConfigurationError('domain is required - this should be your Central API URL (e.g., http://localhost:8080)')
    }

    if (!config.clientId) {
      throw new ConfigurationError('clientId is required')
    }

    // Normalize domain (Central API URL)
    let centralApiUrl = config.domain
    if (!centralApiUrl.startsWith('http://') && !centralApiUrl.startsWith('https://')) {
      centralApiUrl = `https://${centralApiUrl}`
    }

    // Support legacy authwayUrl (deprecated)
    if (config.authwayUrl) {
      console.warn('⚠️ authwayUrl is deprecated. Use "domain" instead to specify Central API URL.')
      centralApiUrl = config.authwayUrl
    }

    // Auto-detect OAuth server URL (Hydra) from Central API URL
    // For local dev: http://localhost:8080 or :8081 -> http://localhost:4444
    // For production: Use oauthServerUrl if provided, otherwise same as Central API
    let oauthServerUrl = config.oauthServerUrl || centralApiUrl
    if (oauthServerUrl.includes(':8080') || oauthServerUrl.includes(':8081')) {
      oauthServerUrl = oauthServerUrl.replace(/:(8080|8081)/, ':4444')
    }

    const redirectUri = config.redirectUri || (typeof window !== 'undefined' ? window.location.origin : '')

    // Configuration info logging
    if (centralApiUrl.includes(':8081')) {
      console.log(
        '✅ Using Auth Backend (port 8081) as API endpoint.\n' +
        'Auth Backend will proxy API calls to Central API (port 8080) and handle CORS.'
      )
    } else if (centralApiUrl.includes(':8080')) {
      console.warn(
        '⚠️ WARNING: domain is set to port 8080 (Central API).\n' +
        'Direct access to Central API may have CORS issues for browser apps.\n' +
        'For SPAs, consider using Auth Backend (port 8081) which provides CORS support.\n' +
        'Recommended config: { domain: "http://localhost:8081", ... }'
      )
    }

    return {
      domain: oauthServerUrl,        // OAuth server URL (Hydra) - for backwards compatibility
      oauthServerUrl,                // OAuth server URL (Hydra) - new field
      centralApiUrl,                 // Central API URL - new field
      authwayUrl: centralApiUrl,     // Deprecated - kept for backwards compatibility
      clientId: config.clientId,
      redirectUri,
      audience: config.audience,
      scope: config.scope || DEFAULT_SCOPE,
      useRefreshTokens: config.useRefreshTokens ?? true,
      cacheLocation: config.cacheLocation || 'memory',
      tenantId: config.tenantId,
      enableDynamicClaims: config.enableDynamicClaims ?? true,
      claimsUpdateInterval: config.claimsUpdateInterval || 0,
      leeway: config.leeway || 60,
      maxAge: config.maxAge || 86400,
      useDPoP: config.useDPoP || false
    }
  }

  /**
   * Fetch remote configuration from Central API
   * Allows dynamic discovery of OAuth server and other endpoints
   */
  private async fetchRemoteConfig(): Promise<void> {
    try {
      const configUrl = `${this.config.centralApiUrl}/.well-known/authway-config`
      const response = await fetch(configUrl)

      if (response.ok) {
        const remoteConfig = await response.json()

        // Update config with remote values
        if (remoteConfig.oauth_url) {
          this.config.oauthServerUrl = remoteConfig.oauth_url
          this.config.domain = remoteConfig.oauth_url  // Backwards compatibility
        }
        if (remoteConfig.api_url) {
          this.config.centralApiUrl = remoteConfig.api_url
          this.config.authwayUrl = remoteConfig.api_url  // Backwards compatibility
        }

        console.log('✅ Authway config loaded:', {
          oauth_url: this.config.oauthServerUrl,
          api_url: this.config.centralApiUrl
        })
      }
    } catch (err) {
      // Silently fail - use default config
      console.warn('Failed to fetch remote config, using defaults:', err)
    }
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

    const authUrl = this.buildAuthorizationUrl(pkce, options)

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
   * Login with password (custom UI)
   */
  async loginWithPassword(credentials: PasswordCredentials): Promise<AuthResult> {
    const url = `${this.config.centralApiUrl}/api/v1/auth/login`

    const response = await post<any>(url, {
      email: credentials.email,
      password: credentials.password,
      tenant_id: credentials.tenantId || this.config.tenantId,
      client_id: this.config.clientId,
      scope: this.config.scope
    })

    const result: AuthResult = {
      accessToken: response.access_token,
      idToken: response.id_token,
      refreshToken: response.refresh_token,
      expiresIn: response.expires_in,
      user: extractUser(response.id_token)
    }

    this.saveTokens(result)
    return result
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
    const authUrl = this.buildAuthorizationUrl(pkce, options)

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
      let timeoutId: number | undefined
      let intervalId: number | undefined

      // Cleanup function
      const cleanup = () => {
        if (timeoutId) clearTimeout(timeoutId)
        if (intervalId) clearInterval(intervalId)
        window.removeEventListener('message', messageHandler)
      }

      // Timeout handler
      timeoutId = window.setTimeout(() => {
        cleanup()
        try {
          popup.close()
        } catch (err) {
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
          } catch (err) {
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

      // Fallback: Check if popup is closed (for browsers that don't block popup.closed)
      intervalId = window.setInterval(() => {
        try {
          if (popup.closed) {
            cleanup()
            this.sessionStorage.remove('pkce_popup')
            reject(new PopupCancelledError('Popup was closed by user', popup))
          }
        } catch (err) {
          // COOP policy blocks popup.closed access - continue
        }
      }, 1000) // Check every second
    })
  }

  /**
   * Logout
   */
  logout(options: LogoutOptions = {}): void {
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
      const logoutUrl = this.buildLogoutUrl(options, idToken)
      window.location.assign(logoutUrl)
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
    if (!options.ignoreCache && this.cache.accessToken) {
      if (!isTokenExpired(this.cache.accessToken, this.config.leeway)) {
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

    const url = `${this.config.domain}/oauth2/token`

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
      const url = `${this.config.centralApiUrl}/api/v1/claims`

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
    const url = `${this.config.centralApiUrl}/api/v1/claims`

    // Backend requires client_id and redirect_uri for OAuth flow
    const payload = {
      claims,
      client_id: this.config.clientId,
      redirect_uri: typeof window !== 'undefined' ? window.location.origin : this.config.centralApiUrl
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
    const url = `${this.config.centralApiUrl}/api/v1/claims/user`

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
    const url = `${this.config.centralApiUrl}/api/v1/claims/user`

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
  // Account Linking
  // ==========================================

  /**
   * Get linked accounts (identities)
   * Similar to Auth0's getUser() with identities field
   */
  async getLinkedAccounts(): Promise<Identity[]> {
    const accessToken = await this.getAccessToken()
    const url = `${this.config.centralApiUrl}/api/v1/user/identities`

    const response = await fetch(url, {
      headers: {
        Authorization: `Bearer ${accessToken}`
      }
    })

    if (!response.ok) {
      throw new AuthenticationError('Failed to get linked accounts')
    }

    const data = await response.json()
    return data.identities || []
  }

  /**
   * Link a new account (identity)
   * Opens a popup to authenticate with another provider and link it
   * Similar to Auth0's linkWithPopup()
   */
  async linkAccount(options: LinkAccountOptions): Promise<Identity> {
    const { provider, connection, redirectUri } = options
    const accessToken = await this.getAccessToken()

    // Generate PKCE challenge for the linking flow
    const pkce = await generatePKCEChallenge()
    this.sessionStorage.set('pkce_link', JSON.stringify(pkce))

    // Build authorization URL for account linking
    const params = new URLSearchParams({
      client_id: this.config.clientId,
      response_type: 'code',
      redirect_uri: redirectUri || this.config.redirectUri,
      scope: this.config.scope || DEFAULT_SCOPE,
      state: pkce.state,
      code_challenge: pkce.codeChallenge,
      code_challenge_method: 'S256',
      prompt: 'login', // Force re-authentication
      // Special parameter to indicate this is a linking flow
      link_account: 'true',
      // Pass current access token to backend for linking
      access_token: accessToken
    })

    if (connection) {
      params.set('connection', connection)
    }

    const authUrl = `${this.config.domain}/oauth2/authorize?${params.toString()}`

    // Open popup
    const popup = window.open(
      authUrl,
      'authway-link-account',
      'width=500,height=700,scrollbars=yes,location=no,toolbar=no,menubar=no'
    )

    if (!popup) {
      this.sessionStorage.remove('pkce_link')
      throw new AuthenticationError('Popup was blocked. Please allow popups for this site.')
    }

    // Wait for popup to complete
    return new Promise((resolve, reject) => {
      const checkInterval = 100
      let timeoutCounter = 0
      const maxTimeout = 300000 // 5 minutes

      const timer = setInterval(async () => {
        timeoutCounter += checkInterval

        if (popup.closed) {
          clearInterval(timer)
          this.sessionStorage.remove('pkce_link')
          return reject(new PopupCancelledError('Popup was closed by user', popup))
        }

        if (timeoutCounter >= maxTimeout) {
          clearInterval(timer)
          popup.close()
          this.sessionStorage.remove('pkce_link')
          return reject(new PopupTimeoutError('Account linking timeout', popup))
        }

        try {
          const popupUrl = popup.location.href
          const redirectUriToCheck = redirectUri || this.config.redirectUri

          if (popupUrl.startsWith(redirectUriToCheck)) {
            clearInterval(timer)
            const params = parseCallbackUrl(popupUrl)
            popup.close()
            this.sessionStorage.remove('pkce_link')

            if (params.error) {
              return reject(new AuthenticationError(
                params.error_description || params.error,
                params.error
              ))
            }

            if (!params.code || !params.state) {
              return reject(new AuthenticationError('Missing code or state parameter'))
            }

            if (!verifyState(params.state, pkce.state)) {
              return reject(new AuthenticationError('State mismatch'))
            }

            try {
              // Exchange code for the linked identity info
              const url = `${this.config.centralApiUrl}/api/v1/user/identities/link`
              const response = await post<any>(url, {
                code: params.code,
                code_verifier: pkce.codeVerifier,
                provider: provider
              }, {
                Authorization: `Bearer ${accessToken}`
              })

              resolve(response.identity)
            } catch (err) {
              reject(err)
            }
          }
        } catch (err) {
          // Can't access popup.location.href (cross-origin)
          // Continue polling
        }
      }, checkInterval)
    })
  }

  /**
   * Unlink an account (identity)
   * Similar to Auth0's unlinkUser()
   */
  async unlinkAccount(provider: string, userId: string): Promise<void> {
    const accessToken = await this.getAccessToken()
    const url = `${this.config.centralApiUrl}/api/v1/user/identities/${provider}/${userId}`

    const response = await fetch(url, {
      method: 'DELETE',
      headers: {
        Authorization: `Bearer ${accessToken}`
      }
    })

    if (!response.ok) {
      const data = await response.json().catch(() => ({}))
      throw new AuthenticationError(
        data.error_description || 'Failed to unlink account',
        data.error || 'unlink_failed'
      )
    }
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

  private buildAuthorizationUrl(pkce: PKCEChallenge, options: RedirectLoginOptions): string {
    const params = {
      client_id: this.config.clientId,
      response_type: 'code',
      redirect_uri: options.redirectUri || this.config.redirectUri,
      scope: this.config.scope,
      state: pkce.state,
      nonce: pkce.nonce,
      code_challenge: pkce.codeChallenge,
      code_challenge_method: 'S256',
      ...options
    }

    delete params.appState

    return buildUrl(`${this.config.domain}/oauth2/auth`, params)
  }

  private buildLogoutUrl(options: LogoutOptions, idToken: string | null): string {
    const params: any = {
      client_id: this.config.clientId
    }

    // Add id_token_hint if available (required when using post_logout_redirect_uri)
    if (idToken) {
      params.id_token_hint = idToken
    }

    if (options.returnTo) {
      params.post_logout_redirect_uri = options.returnTo
    }

    if (options.federated) {
      params.federated = 'true'
    }

    return buildUrl(`${this.config.domain}/oauth2/sessions/logout`, params)
  }

  private async exchangeCodeForTokens(code: string, codeVerifier: string, redirectUri?: string): Promise<AuthResult> {
    const url = `${this.config.domain}/oauth2/token`

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
