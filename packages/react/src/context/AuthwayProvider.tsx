import { useCallback, useEffect, useState, useRef, ReactNode } from 'react'
import { AuthwayClient, AuthwayConfig, PasswordCredentials, LinkAccountOptions } from '@authway/client'
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
  const [client] = useState(() => new AuthwayClient(config))
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
        // Check if we're handling a callback
        if (!skipRedirectCallback && window.location.search.includes('code=')) {
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

  const loginWithPassword = useCallback(
    async (credentials: PasswordCredentials) => {
      try {
        setState(prev => ({ ...prev, isLoading: true, error: null }))
        const result = await client.loginWithPassword(credentials)
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
    (options?: any) => {
      client.logout(options)
      setState({
        isAuthenticated: false,
        isLoading: false,
        user: null,
        error: null
      })
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

  const getLinkedAccounts = useCallback(
    async () => {
      return await client.getLinkedAccounts()
    },
    [client]
  )

  const linkAccount = useCallback(
    async (options: LinkAccountOptions) => {
      return await client.linkAccount(options)
    },
    [client]
  )

  const unlinkAccount = useCallback(
    async (provider: string, userId: string) => {
      await client.unlinkAccount(provider, userId)
    },
    [client]
  )

  const contextValue = {
    ...state,
    client,
    loginWithRedirect,
    loginWithPopup,
    loginWithPassword,
    logout,
    getAccessToken,
    getAccessTokenWithPopup,
    getIdTokenClaims,
    updateClaims,
    getLinkedAccounts,
    linkAccount,
    unlinkAccount
  }

  return (
    <AuthwayContext.Provider value={contextValue}>
      {children}
    </AuthwayContext.Provider>
  )
}
