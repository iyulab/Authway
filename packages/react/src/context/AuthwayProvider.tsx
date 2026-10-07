import { useCallback, useEffect, useState, useRef, ReactNode } from 'react'
import { AuthwayClient, AuthwayConfig, LogoutOptions } from '@authway/client'
import { AuthwayContext, AuthState } from './AuthwayContext'

export interface AuthwayProviderProps {
  config: AuthwayConfig
  children: ReactNode
  onRedirectCallback?: (appState?: any) => void
  skipRedirectCallback?: boolean
}

export function AuthwayProvider({
  config,
  children,
  onRedirectCallback,
  skipRedirectCallback = false
}: AuthwayProviderProps) {
  // Check if we're in a popup callback context BEFORE creating client
  // This handles the Auth0-style auto popup callback
  const [client] = useState(() => {
    // Handle popup callback first - if successful, window will close
    AuthwayClient.handlePopupCallback()
    return new AuthwayClient(config)
  })
  const [state, setState] = useState<AuthState>({
    isAuthenticated: false,
    isLoading: true,
    user: null,
    error: null
  })

  // Prevent duplicate callback processing (React StrictMode, hot reload)
  const isProcessingCallback = useRef(false)

  // Initialize
  useEffect(() => {
    const init = async () => {
      try {
        // Wait for config to be loaded
        await client.waitForReady()
        // OAuth error redirects (e.g. access_denied) carry `error=` without `code=`,
        // so both must route into handleRedirectCallback for it to surface in AuthState.error.
        const callbackParams = new URLSearchParams(window.location.search)
        if (!skipRedirectCallback && (callbackParams.has('code') || callbackParams.has('error'))) {
          // Prevent duplicate processing
          if (isProcessingCallback.current) {
            return
          }
          isProcessingCallback.current = true

          // Capture current URL before clearing
          const currentUrl = window.location.href

          // Clear URL immediately to prevent reprocessing on re-render
          window.history.replaceState({}, document.title, window.location.pathname)

          // Process callback with captured URL
          const result = await client.handleRedirectCallback(currentUrl)

          if (onRedirectCallback) {
            onRedirectCallback(result.appState)
          }
        }

        // Check authentication
        const isAuth = await client.isAuthenticated()
        if (isAuth) {
          const user = await client.getUser()
          setState({
            isAuthenticated: true,
            isLoading: false,
            user,
            error: null
          })
        } else {
          setState({
            isAuthenticated: false,
            isLoading: false,
            user: null,
            error: null
          })
        }
      } catch (error) {
        setState({
          isAuthenticated: false,
          isLoading: false,
          user: null,
          error: error as Error
        })
      } finally {
        // Reset processing flag after completion or error
        isProcessingCallback.current = false
      }
    }

    init()
  }, [client, onRedirectCallback, skipRedirectCallback])

  const loginWithRedirect = useCallback(
    async (options?: any) => {
      await client.loginWithRedirect(options)
    },
    [client]
  )

  const loginWithPopup = useCallback(
    async (options?: any) => {
      try {
        setState(prev => ({ ...prev, isLoading: true, error: null }))
        const result = await client.loginWithPopup(options)
        setState({
          isAuthenticated: true,
          isLoading: false,
          user: result.user,
          error: null
        })
      } catch (error) {
        setState(prev => ({
          ...prev,
          isLoading: false,
          error: error as Error
        }))
        throw error
      }
    },
    [client]
  )

  const logout = useCallback(
    async (options?: LogoutOptions) => {
      setState({
        isAuthenticated: false,
        isLoading: false,
        user: null,
        error: null
      })
      await client.logout(options)
    },
    [client]
  )

  const getAccessToken = useCallback(
    async () => {
      return await client.getAccessToken()
    },
    [client]
  )

  const getAccessTokenWithPopup = useCallback(
    async (options?: any) => {
      return await client.getAccessTokenWithPopup(options)
    },
    [client]
  )

  const getIdTokenClaims = useCallback(
    async () => {
      return await client.getIdTokenClaims()
    },
    [client]
  )

  const updateClaims = useCallback(
    async (claims: any) => {
      await client.updateClaims(claims)
      // Refresh user
      const user = await client.getUser()
      setState(prev => ({ ...prev, user }))
    },
    [client]
  )

  const contextValue = {
    ...state,
    client,
    loginWithRedirect,
    loginWithPopup,
    logout,
    getAccessToken,
    getAccessTokenWithPopup,
    getIdTokenClaims,
    updateClaims
  }

  return (
    <AuthwayContext.Provider value={contextValue}>
      {children}
    </AuthwayContext.Provider>
  )
}
