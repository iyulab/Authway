import { createContext } from 'react'
import { AuthwayClient, User, Identity, LinkAccountOptions } from '@authway/client'

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
  loginWithPassword: (credentials: any) => Promise<void>
  logout: (options?: any) => void
  getAccessToken: () => Promise<string>
  getAccessTokenWithPopup: (options?: any) => Promise<string>
  getIdTokenClaims: () => Promise<any | null>
  updateClaims: (claims: any) => Promise<void>
  getLinkedAccounts: () => Promise<Identity[]>
  linkAccount: (options: LinkAccountOptions) => Promise<Identity>
  unlinkAccount: (provider: string, userId: string) => Promise<void>
}

export const AuthwayContext = createContext<AuthContextValue | null>(null)
