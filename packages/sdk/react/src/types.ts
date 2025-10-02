/**
 * Configuration for Authway client
 */
export interface AuthwayConfig {
  /**
   * The base URL of the Authway server
   * @example "http://localhost:4444"
   */
  authwayUrl: string;

  /**
   * OAuth 2.0 client ID
   */
  clientId: string;

  /**
   * OAuth 2.0 redirect URI after login
   */
  redirectUri: string;

  /**
   * OAuth 2.0 scopes to request
   * @default ["openid", "profile", "email"]
   */
  scope?: string[];

  /**
   * Optional post-logout redirect URI
   */
  postLogoutRedirectUri?: string;

  /**
   * Enable automatic token refresh
   * @default true
   */
  autoRefresh?: boolean;

  /**
   * Token refresh interval in milliseconds
   * @default 60000 (1 minute)
   */
  refreshInterval?: number;
}

/**
 * OAuth 2.0 token response
 */
export interface TokenResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  refresh_token?: string;
  id_token?: string;
  scope: string;
}

/**
 * User information from ID token or UserInfo endpoint
 */
export interface User {
  sub: string;
  name?: string;
  email?: string;
  email_verified?: boolean;
  picture?: string;
  preferred_username?: string;
  [key: string]: any;
}

/**
 * Authentication state
 */
export interface AuthState {
  /**
   * Whether the user is authenticated
   */
  isAuthenticated: boolean;

  /**
   * Whether authentication is loading
   */
  isLoading: boolean;

  /**
   * The authenticated user information
   */
  user: User | null;

  /**
   * The access token
   */
  accessToken: string | null;

  /**
   * The ID token (JWT)
   */
  idToken: string | null;

  /**
   * Error message if authentication failed
   */
  error: string | null;
}

/**
 * Authentication context value
 */
export interface AuthContextValue extends AuthState {
  /**
   * Initiate login flow
   */
  login: () => void;

  /**
   * Logout the user
   */
  logout: () => void;

  /**
   * Get the current access token
   */
  getAccessToken: () => string | null;

  /**
   * Manually refresh the access token
   */
  refreshToken: () => Promise<void>;
}

/**
 * OAuth 2.0 authorization request parameters
 */
export interface AuthorizationParams {
  response_type: 'code';
  client_id: string;
  redirect_uri: string;
  scope: string;
  state: string;
  code_challenge?: string;
  code_challenge_method?: string;
}

/**
 * OAuth 2.0 token request parameters
 */
export interface TokenRequestParams {
  grant_type: 'authorization_code' | 'refresh_token';
  code?: string;
  refresh_token?: string;
  client_id: string;
  redirect_uri?: string;
  code_verifier?: string;
}
