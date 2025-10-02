import React from 'react';
import { useAuth } from './useAuth';

/**
 * Options for withAuth HOC
 */
export interface WithAuthOptions {
  /**
   * Custom loading component
   */
  LoadingComponent?: React.ComponentType;

  /**
   * Custom unauthorized component
   */
  UnauthorizedComponent?: React.ComponentType;

  /**
   * Whether to show loading state
   * @default true
   */
  showLoading?: boolean;

  /**
   * Whether to automatically redirect to login
   * @default true
   */
  redirectToLogin?: boolean;
}

/**
 * Higher-order component that protects components requiring authentication
 *
 * @example
 * ```tsx
 * const ProtectedDashboard = withAuth(Dashboard);
 *
 * // With options
 * const ProtectedProfile = withAuth(Profile, {
 *   LoadingComponent: CustomLoader,
 *   redirectToLogin: true,
 * });
 * ```
 */
export function withAuth<P extends object>(
  Component: React.ComponentType<P>,
  options: WithAuthOptions = {}
): React.ComponentType<P> {
  const {
    LoadingComponent,
    UnauthorizedComponent,
    showLoading = true,
    redirectToLogin = true,
  } = options;

  return function WithAuthComponent(props: P) {
    const { isAuthenticated, isLoading, login } = useAuth();

    // Show loading state
    if (isLoading && showLoading) {
      if (LoadingComponent) {
        return <LoadingComponent />;
      }

      return (
        <div style={{ padding: '20px', textAlign: 'center' }}>
          <p>Loading...</p>
        </div>
      );
    }

    // Handle unauthenticated state
    if (!isAuthenticated) {
      if (redirectToLogin) {
        // Automatically trigger login
        React.useEffect(() => {
          login();
        }, []);

        return null;
      }

      if (UnauthorizedComponent) {
        return <UnauthorizedComponent />;
      }

      return (
        <div style={{ padding: '20px', textAlign: 'center' }}>
          <h1>Unauthorized</h1>
          <p>You must be logged in to access this page.</p>
          <button onClick={login}>Login</button>
        </div>
      );
    }

    // Render protected component
    return <Component {...props} />;
  };
}
