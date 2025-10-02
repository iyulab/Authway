import React, { createContext, useEffect, useState, useCallback, useRef } from 'react';
import type { AuthwayConfig, AuthState, AuthContextValue } from './types';
import { AuthwayClient } from './client';
import { isTokenExpired, getStorageItem, STORAGE_KEYS } from './utils';

/**
 * Auth context
 */
export const AuthContext = createContext<AuthContextValue | null>(null);

/**
 * AuthProvider props
 */
export interface AuthProviderProps {
  children: React.ReactNode;
  config: AuthwayConfig;
  onRedirectCallback?: (appState?: any) => void;
}

/**
 * AuthProvider component
 * Provides authentication context to the application
 */
export function AuthProvider({ children, config, onRedirectCallback }: AuthProviderProps) {
  const clientRef = useRef<AuthwayClient | null>(null);
  const refreshIntervalRef = useRef<NodeJS.Timeout | null>(null);

  const [state, setState] = useState<AuthState>({
    isAuthenticated: false,
    isLoading: true,
    user: null,
    accessToken: null,
    idToken: null,
    error: null,
  });

  // Initialize client
  useEffect(() => {
    if (!clientRef.current) {
      clientRef.current = new AuthwayClient(config);
    }
  }, [config]);

  /**
   * Update auth state from storage
   */
  const updateAuthState = useCallback(() => {
    const client = clientRef.current;
    if (!client) return;

    const accessToken = client.getAccessToken();
    const idToken = getStorageItem(STORAGE_KEYS.ID_TOKEN);
    const user = client.getUser();

    // Check if token is expired
    if (accessToken && isTokenExpired(accessToken)) {
      // Token is expired, try to refresh
      handleRefreshToken();
      return;
    }

    setState({
      isAuthenticated: !!accessToken && !!user,
      isLoading: false,
      user,
      accessToken,
      idToken,
      error: null,
    });
  }, []);

  /**
   * Handle OAuth callback
   */
  const handleCallback = useCallback(async () => {
    const client = clientRef.current;
    if (!client) return;

    try {
      setState((prev) => ({ ...prev, isLoading: true }));

      const tokenResponse = await client.handleCallback();

      if (tokenResponse) {
        updateAuthState();
        onRedirectCallback?.();
      } else {
        setState((prev) => ({ ...prev, isLoading: false }));
      }
    } catch (error) {
      console.error('Callback handling failed:', error);
      setState((prev) => ({
        ...prev,
        isLoading: false,
        error: error instanceof Error ? error.message : 'Authentication failed',
      }));
    }
  }, [updateAuthState, onRedirectCallback]);

  /**
   * Handle token refresh
   */
  const handleRefreshToken = useCallback(async () => {
    const client = clientRef.current;
    if (!client) return;

    try {
      const tokenResponse = await client.refreshAccessToken();

      if (tokenResponse) {
        updateAuthState();
      } else {
        // Refresh failed, clear state
        setState({
          isAuthenticated: false,
          isLoading: false,
          user: null,
          accessToken: null,
          idToken: null,
          error: 'Session expired',
        });
      }
    } catch (error) {
      console.error('Token refresh failed:', error);
      setState({
        isAuthenticated: false,
        isLoading: false,
        user: null,
        accessToken: null,
        idToken: null,
        error: 'Session expired',
      });
    }
  }, [updateAuthState]);

  /**
   * Initialize authentication state
   */
  useEffect(() => {
    const initAuth = async () => {
      // Check if this is a callback from OAuth provider
      const urlParams = new URLSearchParams(window.location.search);
      const hasCode = urlParams.has('code');
      const hasError = urlParams.has('error');

      if (hasCode || hasError) {
        await handleCallback();
      } else {
        updateAuthState();
      }
    };

    initAuth();
  }, [handleCallback, updateAuthState]);

  /**
   * Setup automatic token refresh
   */
  useEffect(() => {
    if (config.autoRefresh && state.isAuthenticated) {
      const interval = config.refreshInterval || 60000;

      refreshIntervalRef.current = setInterval(() => {
        const accessToken = getStorageItem(STORAGE_KEYS.ACCESS_TOKEN);
        if (accessToken && isTokenExpired(accessToken)) {
          handleRefreshToken();
        }
      }, interval);

      return () => {
        if (refreshIntervalRef.current) {
          clearInterval(refreshIntervalRef.current);
        }
      };
    }
  }, [config.autoRefresh, config.refreshInterval, state.isAuthenticated, handleRefreshToken]);

  /**
   * Login function
   */
  const login = useCallback(async () => {
    const client = clientRef.current;
    if (!client) return;

    try {
      await client.login();
    } catch (error) {
      console.error('Login failed:', error);
      setState((prev) => ({
        ...prev,
        error: error instanceof Error ? error.message : 'Login failed',
      }));
    }
  }, []);

  /**
   * Logout function
   */
  const logout = useCallback(async () => {
    const client = clientRef.current;
    if (!client) return;

    try {
      await client.logout();

      setState({
        isAuthenticated: false,
        isLoading: false,
        user: null,
        accessToken: null,
        idToken: null,
        error: null,
      });
    } catch (error) {
      console.error('Logout failed:', error);
      setState((prev) => ({
        ...prev,
        error: error instanceof Error ? error.message : 'Logout failed',
      }));
    }
  }, []);

  /**
   * Get access token
   */
  const getAccessToken = useCallback((): string | null => {
    const client = clientRef.current;
    if (!client) return null;
    return client.getAccessToken();
  }, []);

  /**
   * Context value
   */
  const value: AuthContextValue = {
    ...state,
    login,
    logout,
    getAccessToken,
    refreshToken: handleRefreshToken,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
