import React, { ComponentType, useEffect } from 'react'
import { useAuth } from '../hooks/useAuth'

export interface WithAuthRequiredOptions {
  /**
   * Component to display while checking authentication
   */
  onRedirecting?: () => React.JSX.Element

  /**
   * URL to return to after authentication
   */
  returnTo?: string | (() => string)

  /**
   * Additional login options
   */
  loginOptions?: any
}

/**
 * Higher-order component that protects a route by requiring authentication
 * Similar to Auth0's withAuthenticationRequired
 *
 * @example
 * ```tsx
 * const ProtectedProfile = withAuthRequired(ProfilePage)
 *
 * // With options
 * const ProtectedProfile = withAuthRequired(ProfilePage, {
 *   onRedirecting: () => <div>Loading...</div>,
 *   returnTo: '/profile'
 * })
 * ```
 */
export function withAuthRequired<P extends object>(
  Component: ComponentType<P>,
  options: WithAuthRequiredOptions = {}
): React.FC<P> {
  return function WithAuthRequiredWrapper(props: P) {
    const { isAuthenticated, isLoading, loginWithRedirect } = useAuth()
    const { onRedirecting = () => <div>Loading...</div> } = options

    useEffect(() => {
      if (!isLoading && !isAuthenticated) {
        // `options` is fixed when the component is wrapped (outer scope, not
        // a dependency); reading it here rather than from per-render
        // defaults keeps the effect from re-running on every render.
        const { returnTo, loginOptions = {} } = options
        const opts = {
          ...loginOptions,
          appState: {
            returnTo: typeof returnTo === 'function'
              ? returnTo()
              : returnTo || window.location.pathname
          }
        }

        loginWithRedirect(opts)
      }
    }, [isAuthenticated, isLoading, loginWithRedirect])

    if (isLoading || !isAuthenticated) {
      return onRedirecting()
    }

    return <Component {...props} />
  }
}
