/**
 * @authway/react - React SDK for Authway authentication
 *
 * This package provides React components and hooks for integrating
 * Authway OAuth 2.0 / OIDC authentication into React applications.
 *
 * @example
 * ```tsx
 * import { AuthProvider, useAuth } from '@authway/react';
 *
 * // Wrap your app with AuthProvider
 * function App() {
 *   return (
 *     <AuthProvider
 *       config={{
 *         authwayUrl: 'http://localhost:4444',
 *         clientId: 'your-client-id',
 *         redirectUri: 'http://localhost:3000/callback',
 *       }}
 *     >
 *       <YourApp />
 *     </AuthProvider>
 *   );
 * }
 *
 * // Use authentication in components
 * function YourApp() {
 *   const { isAuthenticated, user, login, logout } = useAuth();
 *
 *   if (!isAuthenticated) {
 *     return <button onClick={login}>Login</button>;
 *   }
 *
 *   return (
 *     <div>
 *       <p>Welcome, {user?.name}</p>
 *       <button onClick={logout}>Logout</button>
 *     </div>
 *   );
 * }
 * ```
 */

// Main exports
export { AuthProvider } from './AuthProvider';
export type { AuthProviderProps } from './AuthProvider';
export { useAuth } from './useAuth';
export { withAuth } from './withAuth';
export type { WithAuthOptions } from './withAuth';

// Client exports
export { AuthwayClient } from './client';

// Type exports
export type {
  AuthwayConfig,
  TokenResponse,
  User,
  AuthState,
  AuthContextValue,
  AuthorizationParams,
  TokenRequestParams,
} from './types';

// Utility exports (for advanced use cases)
export {
  generateCodeVerifier,
  generateCodeChallenge,
  generateState,
  parseJwt,
  isTokenExpired,
  getTokenExpiration,
  STORAGE_KEYS,
} from './utils';
