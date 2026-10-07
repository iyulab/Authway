export { AuthwayProvider } from './context/AuthwayProvider'
export type { AuthwayProviderProps } from './context/AuthwayProvider'

export * from './hooks'
export * from './components'

// Re-export types from @authway/client
export type {
  AuthwayConfig,
  User,
  Claims,
  RedirectLoginOptions,
  LogoutOptions
} from '@authway/client'
