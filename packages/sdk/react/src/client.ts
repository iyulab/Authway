import axios, { AxiosInstance } from 'axios';
import type {
  AuthwayConfig,
  TokenResponse,
  User,
  AuthorizationParams,
  TokenRequestParams,
} from './types';
import {
  generateCodeVerifier,
  generateCodeChallenge,
  generateState,
  parseJwt,
  STORAGE_KEYS,
  setStorageItem,
  getStorageItem,
  removeStorageItem,
  clearAuthStorage,
} from './utils';

/**
 * Authway OAuth 2.0 client
 */
export class AuthwayClient {
  private config: Required<AuthwayConfig>;
  private httpClient: AxiosInstance;

  constructor(config: AuthwayConfig) {
    this.config = {
      scope: ['openid', 'profile', 'email'],
      autoRefresh: true,
      refreshInterval: 60000,
      postLogoutRedirectUri: config.redirectUri,
      ...config,
    };

    this.httpClient = axios.create({
      baseURL: this.config.authwayUrl,
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
    });
  }

  /**
   * Initiate OAuth 2.0 authorization code flow with PKCE
   */
  async login(): Promise<void> {
    try {
      // Generate PKCE parameters
      const codeVerifier = generateCodeVerifier();
      const codeChallenge = await generateCodeChallenge(codeVerifier);
      const state = generateState();

      // Store PKCE parameters for token exchange
      setStorageItem(STORAGE_KEYS.CODE_VERIFIER, codeVerifier);
      setStorageItem(STORAGE_KEYS.STATE, state);

      // Build authorization URL
      const params: AuthorizationParams = {
        response_type: 'code',
        client_id: this.config.clientId,
        redirect_uri: this.config.redirectUri,
        scope: this.config.scope.join(' '),
        state,
        code_challenge: codeChallenge,
        code_challenge_method: 'S256',
      };

      const authUrl = `${this.config.authwayUrl}/oauth2/auth?${new URLSearchParams(
        params as any
      ).toString()}`;

      // Redirect to authorization endpoint
      window.location.href = authUrl;
    } catch (error) {
      console.error('Login failed:', error);
      throw error;
    }
  }

  /**
   * Handle OAuth callback and exchange code for tokens
   */
  async handleCallback(): Promise<TokenResponse | null> {
    try {
      const urlParams = new URLSearchParams(window.location.search);
      const code = urlParams.get('code');
      const state = urlParams.get('state');
      const error = urlParams.get('error');

      // Check for errors
      if (error) {
        const errorDescription = urlParams.get('error_description');
        throw new Error(errorDescription || error);
      }

      // Validate state
      const storedState = getStorageItem(STORAGE_KEYS.STATE);
      if (!state || state !== storedState) {
        throw new Error('Invalid state parameter');
      }

      // Exchange code for tokens
      if (code) {
        const codeVerifier = getStorageItem(STORAGE_KEYS.CODE_VERIFIER);
        if (!codeVerifier) {
          throw new Error('Missing code verifier');
        }

        const tokenResponse = await this.exchangeCodeForToken(code, codeVerifier);

        // Store tokens
        this.storeTokens(tokenResponse);

        // Clean up PKCE parameters
        removeStorageItem(STORAGE_KEYS.CODE_VERIFIER);
        removeStorageItem(STORAGE_KEYS.STATE);

        // Clean URL
        window.history.replaceState({}, document.title, window.location.pathname);

        return tokenResponse;
      }

      return null;
    } catch (error) {
      console.error('Callback handling failed:', error);
      throw error;
    }
  }

  /**
   * Exchange authorization code for tokens
   */
  private async exchangeCodeForToken(
    code: string,
    codeVerifier: string
  ): Promise<TokenResponse> {
    const params: TokenRequestParams = {
      grant_type: 'authorization_code',
      code,
      client_id: this.config.clientId,
      redirect_uri: this.config.redirectUri,
      code_verifier: codeVerifier,
    };

    const response = await this.httpClient.post<TokenResponse>(
      '/oauth2/token',
      new URLSearchParams(params as any).toString()
    );

    return response.data;
  }

  /**
   * Refresh access token using refresh token
   */
  async refreshAccessToken(): Promise<TokenResponse | null> {
    try {
      const refreshToken = getStorageItem(STORAGE_KEYS.REFRESH_TOKEN);
      if (!refreshToken) {
        throw new Error('No refresh token available');
      }

      const params: TokenRequestParams = {
        grant_type: 'refresh_token',
        refresh_token: refreshToken,
        client_id: this.config.clientId,
      };

      const response = await this.httpClient.post<TokenResponse>(
        '/oauth2/token',
        new URLSearchParams(params as any).toString()
      );

      const tokenResponse = response.data;
      this.storeTokens(tokenResponse);

      return tokenResponse;
    } catch (error) {
      console.error('Token refresh failed:', error);
      // Clear tokens if refresh fails
      clearAuthStorage();
      return null;
    }
  }

  /**
   * Get user information from ID token
   */
  getUser(): User | null {
    try {
      const idToken = getStorageItem(STORAGE_KEYS.ID_TOKEN);
      if (!idToken) {
        return null;
      }

      const payload = parseJwt(idToken);
      if (!payload) {
        return null;
      }

      return payload as User;
    } catch (error) {
      console.error('Failed to get user:', error);
      return null;
    }
  }

  /**
   * Get access token from storage
   */
  getAccessToken(): string | null {
    return getStorageItem(STORAGE_KEYS.ACCESS_TOKEN);
  }

  /**
   * Logout user
   */
  async logout(): Promise<void> {
    try {
      const idToken = getStorageItem(STORAGE_KEYS.ID_TOKEN);

      // Clear local storage
      clearAuthStorage();

      // Redirect to logout endpoint
      if (idToken) {
        const logoutUrl = `${this.config.authwayUrl}/oauth2/sessions/logout?id_token_hint=${idToken}&post_logout_redirect_uri=${this.config.postLogoutRedirectUri}`;
        window.location.href = logoutUrl;
      }
    } catch (error) {
      console.error('Logout failed:', error);
      throw error;
    }
  }

  /**
   * Store tokens in local storage
   */
  private storeTokens(tokenResponse: TokenResponse): void {
    setStorageItem(STORAGE_KEYS.ACCESS_TOKEN, tokenResponse.access_token);

    if (tokenResponse.refresh_token) {
      setStorageItem(STORAGE_KEYS.REFRESH_TOKEN, tokenResponse.refresh_token);
    }

    if (tokenResponse.id_token) {
      setStorageItem(STORAGE_KEYS.ID_TOKEN, tokenResponse.id_token);
    }
  }

  /**
   * Check if user is authenticated
   */
  isAuthenticated(): boolean {
    const accessToken = this.getAccessToken();
    return !!accessToken;
  }
}
