import { createContext } from 'react'
import { AuthwayClient, LogoutOptions, User } from '@authway/client'
import type { Locale } from '../i18n'

export interface AuthState {
  isAuthenticated: boolean
  isLoading: boolean
  user: User | null
  error: Error | null
}

export interface AuthContextValue extends AuthState {
  client: AuthwayClient
  loginWithRedirect: (options?: any) => Promise<void>
  loginWithPopup: (options?: any) => Promise<void>
  logout: (options?: LogoutOptions) => Promise<void>
  getAccessToken: () => Promise<string>
  getAccessTokenWithPopup: (options?: any) => Promise<string>
  getIdTokenClaims: () => Promise<any | null>
  /** The language of the words this package puts on screen. */
  locale: Locale
}

export const AuthwayContext = createContext<AuthContextValue | null>(null)
