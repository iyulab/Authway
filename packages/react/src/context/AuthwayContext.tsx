import { createContext } from 'react'
import { AuthwayClient, LogoutOptions, User } from '@authway/client'

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
  updateClaims: (claims: any) => Promise<void>
}

export const AuthwayContext = createContext<AuthContextValue | null>(null)
