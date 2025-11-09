import React, { ComponentType, useEffect } from 'react'
import { useAuth } from '../hooks/useAuth'

export interface WithAuthRequiredOptions {
  /**
   * Component to display while checking authentication
   */
  onRedirecting?: () => JSX.Element

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
    const {
      onRedirecting = () => <div>Loading...</div>,
      returnTo,
      loginOptions = {}
    } = options

    useEffect(() => {
      if (!isLoading && !isAuthenticated) {
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
