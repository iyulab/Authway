// OAuth2 PKCE Authentication Module
import * as oauth from 'oauth4webapi';
import { CONFIG } from './config.js';

let authorizationServer = null;
let client = null;

// Initialize OAuth2 configuration
export async function initializeAuth() {
  try {
    console.log('🔧 Initializing OAuth2 configuration...');

    // Discover authorization server configuration from Hydra
    const issuer = new URL(CONFIG.issuer);
    const response = await oauth.discoveryRequest(issuer, {
      // Allow HTTP for local development (NEVER use in production!)
      [oauth.allowInsecureRequests]: true
    });
    authorizationServer = await oauth.processDiscoveryResponse(issuer, response, {
      [oauth.allowInsecureRequests]: true
    });

    // Configure OAuth2 client (public client - PKCE)
    client = {
      client_id: CONFIG.clientId,
      token_endpoint_auth_method: 'none', // Public client
    };

    console.log('✅ OAuth2 configured:', {
      issuer: CONFIG.issuer,
      authEndpoint: authorizationServer.authorization_endpoint,
      tokenEndpoint: authorizationServer.token_endpoint
    });

    return true;
  } catch (error) {
    console.error('❌ Failed to initialize OAuth2:', error);
    throw error;
  }
}

// Generate PKCE code verifier
function generateCodeVerifier() {
  const array = new Uint8Array(32);
  crypto.getRandomValues(array);
  return btoa(String.fromCharCode(...array))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '');
}

// Generate PKCE code challenge from verifier
async function generateCodeChallenge(verifier) {
  const encoder = new TextEncoder();
  const data = encoder.encode(verifier);
  const hash = await crypto.subtle.digest('SHA-256', data);
  return btoa(String.fromCharCode(...new Uint8Array(hash)))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '');
}

// Start OAuth2 login flow with PKCE
export async function login() {
  try {
    if (!authorizationServer || !client) {
      await initializeAuth();
    }

    console.log('🔐 Starting OAuth2 login flow...');

    // Generate PKCE parameters
    const codeVerifier = generateCodeVerifier();
    const codeChallenge = await generateCodeChallenge(codeVerifier);
    const state = generateCodeVerifier(); // Random state for CSRF protection

    // Store PKCE parameters in session
    sessionStorage.setItem('code_verifier', codeVerifier);
    sessionStorage.setItem('oauth_state', state);

    // Build authorization URL
    const authUrl = new URL(authorizationServer.authorization_endpoint);
    authUrl.searchParams.set('client_id', CONFIG.clientId);
    authUrl.searchParams.set('redirect_uri', CONFIG.redirectUri);
    authUrl.searchParams.set('response_type', 'code');
    authUrl.searchParams.set('scope', CONFIG.scope);
    authUrl.searchParams.set('code_challenge', codeChallenge);
    authUrl.searchParams.set('code_challenge_method', 'S256');
    authUrl.searchParams.set('state', state);
    authUrl.searchParams.set('audience', 'api'); // Request audience in token

    console.log('↗️  Redirecting to Authway authorization endpoint...');
    console.log('🔍 Authorization URL:', authUrl.toString());
    console.log('📋 URL includes audience:', authUrl.searchParams.has('audience'), authUrl.searchParams.get('audience'));
    window.location.href = authUrl.toString();
  } catch (error) {
    console.error('❌ Login failed:', error);
    throw error;
  }
}

// Handle OAuth2 callback and exchange code for tokens
export async function handleCallback() {
  const params = new URLSearchParams(window.location.search);
  const code = params.get('code');
  const state = params.get('state');
  const error = params.get('error');

  // Check for OAuth errors
  if (error) {
    const errorDesc = params.get('error_description') || 'Unknown error';
    console.error('❌ OAuth error:', error, errorDesc);
    throw new Error(`OAuth error: ${error} - ${errorDesc}`);
  }

  // Not a callback if no code
  if (!code) {
    return null;
  }

  console.log('🔄 Processing OAuth callback...');

  // Validate state parameter (CSRF protection)
  const storedState = sessionStorage.getItem('oauth_state');
  if (state !== storedState) {
    console.error('❌ Invalid state parameter - session may have expired');
    // Clean up stale session data
    sessionStorage.removeItem('code_verifier');
    sessionStorage.removeItem('oauth_state');
    // Clear URL and show login screen
    window.history.replaceState({}, document.title, window.location.pathname);
    throw new Error('Invalid state parameter - session expired. Please login again.');
  }

  // Get code verifier
  const codeVerifier = sessionStorage.getItem('code_verifier');
  if (!codeVerifier) {
    throw new Error('Missing code verifier');
  }

  if (!authorizationServer || !client) {
    await initializeAuth();
  }

  try {
    console.log('🔄 Exchanging authorization code for tokens...');

    // Validate the authorization response (throws on error)
    const validatedParams = oauth.validateAuthResponse(
      authorizationServer,
      client,
      new URL(window.location.href),
      storedState
    );

    // Exchange authorization code for tokens using PKCE
    const tokenResponse = await oauth.authorizationCodeGrantRequest(
      authorizationServer,
      client,
      oauth.None(),  // Public client - no authentication
      validatedParams,
      CONFIG.redirectUri,
      codeVerifier,
      {
        [oauth.allowInsecureRequests]: true
      }
    );

    const result = await oauth.processAuthorizationCodeResponse(
      authorizationServer,
      client,
      tokenResponse,
      {
        requireIdToken: true,
        [oauth.allowInsecureRequests]: true
      }
    );

    const tokens = {
      access_token: result.access_token,
      token_type: result.token_type,
      expires_in: result.expires_in,
      refresh_token: result.refresh_token,
      id_token: result.id_token,
      expires_at: Date.now() + (result.expires_in * 1000)
    };

    // Store tokens securely in sessionStorage
    sessionStorage.setItem('tokens', JSON.stringify(tokens));
    sessionStorage.removeItem('code_verifier');
    sessionStorage.removeItem('oauth_state');

    // Parse ID token claims (user info)
    const claims = oauth.getValidatedIdTokenClaims(result);
    sessionStorage.setItem('user', JSON.stringify(claims));

    console.log('✅ Authentication successful!');
    console.log('👤 User:', claims.sub, claims.email || claims.name);

    // Clean up URL (remove query params)
    window.history.replaceState({}, document.title, window.location.pathname);

    return { tokens, user: claims };
  } catch (error) {
    console.error('❌ Token exchange failed:', error);
    // Clean up on failure
    sessionStorage.removeItem('code_verifier');
    sessionStorage.removeItem('oauth_state');
    throw error;
  }
}

// Get current user from session
export function getUser() {
  const userJson = sessionStorage.getItem('user');
  return userJson ? JSON.parse(userJson) : null;
}

// Get access token from session
export function getAccessToken() {
  const tokensJson = sessionStorage.getItem('tokens');
  if (!tokensJson) return null;
  
  const tokens = JSON.parse(tokensJson);
  
  // Check if token is expired
  if (tokens.expires_at && Date.now() >= tokens.expires_at) {
    console.warn('⚠️  Access token expired');
    // Could implement refresh token logic here
    return null;
  }
  
  return tokens.access_token;
}

// Get all tokens
export function getTokens() {
  const tokensJson = sessionStorage.getItem('tokens');
  return tokensJson ? JSON.parse(tokensJson) : null;
}

// Check if user is authenticated
export function isAuthenticated() {
  const token = getAccessToken();
  return token !== null;
}

// Popup login with postMessage
export async function loginWithPopup() {
  try {
    if (!authorizationServer || !client) {
      await initializeAuth();
    }

    console.log('🪟 Starting popup login flow...');

    // Generate PKCE parameters
    const codeVerifier = generateCodeVerifier();
    const codeChallenge = await generateCodeChallenge(codeVerifier);
    const state = generateCodeVerifier(); // Random state for CSRF protection

    // Store PKCE parameters in session
    sessionStorage.setItem('popup_code_verifier', codeVerifier);
    sessionStorage.setItem('popup_oauth_state', state);

    // Build authorization URL with popup redirect URI
    const authUrl = new URL(authorizationServer.authorization_endpoint);
    authUrl.searchParams.set('client_id', CONFIG.clientId);
    authUrl.searchParams.set('redirect_uri', CONFIG.popupRedirectUri);
    authUrl.searchParams.set('response_type', 'code');
    authUrl.searchParams.set('scope', CONFIG.scope);
    authUrl.searchParams.set('code_challenge', codeChallenge);
    authUrl.searchParams.set('code_challenge_method', 'S256');
    authUrl.searchParams.set('state', state);
    authUrl.searchParams.set('audience', 'api'); // Request audience in token

    console.log('🔍 Popup Authorization URL:', authUrl.toString());
    console.log('📋 Popup URL includes audience:', authUrl.searchParams.has('audience'), authUrl.searchParams.get('audience'));

    // Open popup window
    const popup = window.open(
      authUrl.toString(),
      'oauth-popup',
      'width=500,height=700,left=100,top=100'
    );

    if (!popup) {
      throw new Error('Failed to open popup window. Please allow popups for this site.');
    }

    // Wait for callback via postMessage
    return new Promise((resolve, reject) => {
      // Timeout after 5 minutes
      const timeout = setTimeout(() => {
        window.removeEventListener('message', handleMessage);
        reject(new Error('Authentication timeout - no response from popup'));
      }, 5 * 60 * 1000);

      const handleMessage = async (event) => {
        // Verify origin
        if (event.origin !== window.location.origin) {
          return;
        }

        // Check message type
        if (event.data && event.data.type === 'oauth-callback') {
          clearTimeout(timeout);
          window.removeEventListener('message', handleMessage);

          const { code, state, error, errorDescription } = event.data;

          if (error) {
            reject(new Error(`OAuth error: ${error} - ${errorDescription || 'Unknown error'}`));
            return;
          }

          if (!code) {
            reject(new Error('No authorization code received'));
            return;
          }

          // Validate state
          const storedState = sessionStorage.getItem('popup_oauth_state');
          if (state !== storedState) {
            reject(new Error('Invalid state parameter - possible CSRF attack'));
            return;
          }

          // Get code verifier
          const codeVerifier = sessionStorage.getItem('popup_code_verifier');
          if (!codeVerifier) {
            reject(new Error('Missing code verifier'));
            return;
          }

          try {
            console.log('🔄 Exchanging authorization code for tokens...');

            // Build callback URL with code and state
            const callbackUrl = new URL(CONFIG.popupRedirectUri);
            callbackUrl.searchParams.set('code', code);
            callbackUrl.searchParams.set('state', state);

            // Validate the response (throws on error, returns validated params)
            const validatedParams = oauth.validateAuthResponse(
              authorizationServer,
              client,
              callbackUrl,
              storedState  // Use stored state for validation
            );

            // Exchange authorization code for tokens using PKCE
            const tokenResponse = await oauth.authorizationCodeGrantRequest(
              authorizationServer,
              client,
              oauth.None(),  // Public client - no authentication
              validatedParams,
              CONFIG.popupRedirectUri,
              codeVerifier,
              {
                [oauth.allowInsecureRequests]: true
              }
            );

            const result = await oauth.processAuthorizationCodeResponse(
              authorizationServer,
              client,
              tokenResponse,
              {
                requireIdToken: true,
                [oauth.allowInsecureRequests]: true
              }
            );

            const tokens = {
              access_token: result.access_token,
              token_type: result.token_type,
              expires_in: result.expires_in,
              refresh_token: result.refresh_token,
              id_token: result.id_token,
              expires_at: Date.now() + (result.expires_in * 1000)
            };

            // Store tokens securely in sessionStorage
            sessionStorage.setItem('tokens', JSON.stringify(tokens));
            sessionStorage.removeItem('popup_code_verifier');
            sessionStorage.removeItem('popup_oauth_state');

            // Parse ID token claims (user info)
            const claims = oauth.getValidatedIdTokenClaims(result);
            sessionStorage.setItem('user', JSON.stringify(claims));

            console.log('✅ Popup authentication successful!');
            console.log('👤 User:', claims.sub, claims.email || claims.name);

            resolve({ tokens, user: claims });
          } catch (err) {
            sessionStorage.removeItem('popup_code_verifier');
            sessionStorage.removeItem('popup_oauth_state');
            reject(err);
          }
        }
      };

      window.addEventListener('message', handleMessage);
    });
  } catch (error) {
    console.error('❌ Popup login failed:', error);
    throw error;
  }
}

// Logout with Hydra end session
export async function logout() {
  try {
    const tokens = getTokens();

    if (tokens && tokens.id_token && authorizationServer) {
      // Construct Hydra logout URL
      const logoutUrl = new URL(authorizationServer.end_session_endpoint || `${CONFIG.issuer}/oauth2/sessions/logout`);
      logoutUrl.searchParams.set('id_token_hint', tokens.id_token);
      logoutUrl.searchParams.set('post_logout_redirect_uri', CONFIG.redirectUri);

      // Clear session data
      sessionStorage.removeItem('tokens');
      sessionStorage.removeItem('user');
      sessionStorage.removeItem('code_verifier');
      sessionStorage.removeItem('oauth_state');
      sessionStorage.removeItem('popup_code_verifier');
      sessionStorage.removeItem('popup_oauth_state');

      console.log('👋 Logging out and redirecting to Hydra...');

      // Redirect to Hydra logout endpoint
      window.location.href = logoutUrl.toString();
    } else {
      // Fallback: Just clear session and reload
      sessionStorage.removeItem('tokens');
      sessionStorage.removeItem('user');
      sessionStorage.removeItem('code_verifier');
      sessionStorage.removeItem('oauth_state');
      sessionStorage.removeItem('popup_code_verifier');
      sessionStorage.removeItem('popup_oauth_state');

      console.log('👋 Logged out - session cleared');
      window.location.href = '/';
    }
  } catch (error) {
    console.error('❌ Logout error:', error);
    // Still clear session on error
    sessionStorage.clear();
    window.location.href = '/';
  }
}

// Get token expiry info
export function getTokenExpiry() {
  const tokens = getTokens();
  if (!tokens || !tokens.expires_at) return null;

  const expiresIn = Math.floor((tokens.expires_at - Date.now()) / 1000);
  return {
    expiresAt: new Date(tokens.expires_at),
    expiresIn: expiresIn,
    isExpired: expiresIn <= 0
  };
}

// Dynamic Claims Management
export async function getDynamicClaims() {
  try {
    const accessToken = getAccessToken();
    if (!accessToken) {
      throw new Error('Not authenticated');
    }

    const response = await fetch(`${CONFIG.centralApiUrl}/api/v1/claims`, {
      method: 'GET',
      headers: {
        'Authorization': `Bearer ${accessToken}`,
        'Content-Type': 'application/json'
      }
    });

    if (!response.ok) {
      throw new Error(`Failed to get claims: ${response.statusText}`);
    }

    const claims = await response.json();
    console.log('✅ Dynamic claims fetched:', claims);
    return claims;
  } catch (error) {
    console.error('❌ Failed to get dynamic claims:', error);
    throw error;
  }
}

export async function updateDynamicClaim(key, value) {
  try {
    const accessToken = getAccessToken();
    if (!accessToken) {
      throw new Error('Not authenticated');
    }

    // Parse value as JSON if it looks like JSON
    let parsedValue = value;
    if (typeof value === 'string') {
      const trimmed = value.trim();
      if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
        try {
          parsedValue = JSON.parse(trimmed);
        } catch {
          // Keep as string if JSON parsing fails
        }
      }
    }

    // Update user claims using PATCH /api/v1/claims/user
    const response = await fetch(`${CONFIG.centralApiUrl}/api/v1/claims/user`, {
      method: 'PATCH',
      headers: {
        'Authorization': `Bearer ${accessToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ claims: { [key]: parsedValue } })
    });

    if (!response.ok) {
      throw new Error(`Failed to update claim: ${response.statusText}`);
    }

    const result = await response.json();
    console.log(`✅ Claim '${key}' updated to:`, parsedValue);
    return result;
  } catch (error) {
    console.error('❌ Failed to update claim:', error);
    throw error;
  }
}

export async function deleteDynamicClaim(key) {
  try {
    const accessToken = getAccessToken();
    if (!accessToken) {
      throw new Error('Not authenticated');
    }

    // Delete claim by setting it to null
    const response = await fetch(`${CONFIG.centralApiUrl}/api/v1/claims/user`, {
      method: 'PATCH',
      headers: {
        'Authorization': `Bearer ${accessToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ claims: { [key]: null } })
    });

    if (!response.ok) {
      throw new Error(`Failed to delete claim: ${response.statusText}`);
    }

    console.log(`✅ Claim '${key}' deleted`);
    return true;
  } catch (error) {
    console.error('❌ Failed to delete claim:', error);
    throw error;
  }
}
