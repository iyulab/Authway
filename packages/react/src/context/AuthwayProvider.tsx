import { useCallback, useEffect, useState, useRef, ReactNode } from 'react'
import { AuthwayClient, AuthwayConfig, LogoutOptions } from '@authway/client'
import { AuthwayContext, AuthState } from './AuthwayContext'
import { initialize } from './initialize'
import { DEFAULT_LOCALE, detectLocale, negotiateLocale, type Locale } from '../i18n'

export interface AuthwayProviderProps {
  config: AuthwayConfig
  children: ReactNode
  /**
   * Called when the browser has come back from the provider — after a login,
   * or after the provider was asked for the session (`checkSessionOnLoad`) —
   * with the `appState` of the request. After a session check that is
   * `{ returnTo }`, the address the user was at.
   *
   * Without it the address bar is put back to `appState.returnTo`. An
   * application with a router passes its own, navigating there.
   */
  onRedirectCallback?: (appState?: any) => void
  skipRedirectCallback?: boolean
  /**
   * When a page loads without tokens on a browser a user has signed in on,
   * ask the provider for their session (`client.checkSession()`): the browser
   * goes there and back without the user being shown anything, and the page
   * loads signed in if the session is still there. `isLoading` stays true in
   * between.
   * @default true
   */
  checkSessionOnLoad?: boolean
  /**
   * The language of the words this package puts on screen (button labels,
   * loading text): a language tag such as `'ko'` or `'en-US'`, or an
   * `Accept-Language` style list. Supported languages are `en` and `ko`.
   *
   * When it is set the sign-in screen is asked for the same language
   * (`ui_locales`), unless a login call names its own. When it is not set the
   * language follows the person: the `lang` query parameter, the language they
   * chose on a sign-in screen, then their browser's — and English otherwise.
   * The sign-in screen then follows the person the same way.
   */
  locale?: string
}

const defaultOnRedirectCallback = (appState?: any) => {
  if (typeof appState?.returnTo === 'string') {
    window.history.replaceState({}, document.title, appState.returnTo)
  }
}

export function AuthwayProvider({
  config,
  children,
  onRedirectCallback = defaultOnRedirectCallback,
  skipRedirectCallback = false,
  checkSessionOnLoad = true,
  locale: localeProp
}: AuthwayProviderProps) {
  // Check if we're in a popup callback context BEFORE creating client
  // This handles the Auth0-style auto popup callback
  const [client] = useState(() => {
    // Handle popup callback first - if successful, window will close
    AuthwayClient.handlePopupCallback()
    return new AuthwayClient(config)
  })
  // The first render is the same wherever it runs (server or browser), so
  // what the browser prefers is read after mounting, not while rendering.
  const chosenLocale = negotiateLocale(localeProp)
  const [detectedLocale, setDetectedLocale] = useState<Locale>(DEFAULT_LOCALE)
  useEffect(() => {
    if (!chosenLocale) setDetectedLocale(detectLocale())
  }, [chosenLocale])
  const locale = chosenLocale ?? detectedLocale
  // An application that chose a language has the sign-in screen opened in it.
  const uiLocales = chosenLocale
  const [state, setState] = useState<AuthState>({
    isAuthenticated: false,
    isLoading: true,
    user: null,
    error: null
  })

  // One initialization at a time (React StrictMode runs the effect twice, hot
  // reload again): an answer in the address is handled once, and the session
  // is looked for once.
  const isInitializing = useRef(false)

  // Initialize
  useEffect(() => {
    const init = async () => {
      if (isInitializing.current) {
        return
      }
      isInitializing.current = true
      try {
        const outcome = await initialize(client, {
          url: window.location.href,
          skipRedirectCallback,
          checkSessionOnLoad,
          // Clear the answer from the address at once, so a re-render does not handle it again
          onAnswer: () => window.history.replaceState({}, document.title, window.location.pathname)
        })
        // On its way to the provider: the page stays loading, and is not
        // initialized again, until it is replaced.
        if (outcome.kind === 'leaving') {
          return
        }
        isInitializing.current = false
        if (outcome.cameBack) {
          onRedirectCallback(outcome.appState)
        }
        setState({
          isAuthenticated: outcome.kind === 'signed-in',
          isLoading: false,
          user: outcome.kind === 'signed-in' ? outcome.user : null,
          error: null
        })
      } catch (error) {
        isInitializing.current = false
        setState({
          isAuthenticated: false,
          isLoading: false,
          user: null,
          error: error as Error
        })
      }
    }

    init()
  }, [client, onRedirectCallback, skipRedirectCallback, checkSessionOnLoad])

  const loginWithRedirect = useCallback(
    async (options?: any) => {
      await client.loginWithRedirect(uiLocales ? { ui_locales: uiLocales, ...options } : options)
    },
    [client, uiLocales]
  )

  const loginWithPopup = useCallback(
    async (options?: any) => {
      try {
        setState(prev => ({ ...prev, isLoading: true, error: null }))
        const result = await client.loginWithPopup(uiLocales ? { ui_locales: uiLocales, ...options } : options)
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
    [client, uiLocales]
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

  const contextValue = {
    ...state,
    client,
    loginWithRedirect,
    loginWithPopup,
    logout,
    getAccessToken,
    getAccessTokenWithPopup,
    getIdTokenClaims,
    locale
  }

  return (
    <AuthwayContext.Provider value={contextValue}>
      {children}
    </AuthwayContext.Provider>
  )
}
