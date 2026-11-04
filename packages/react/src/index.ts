export { AuthwayProvider } from './context/AuthwayProvider'
export type { AuthwayProviderProps } from './context/AuthwayProvider'

export { negotiateLocale, detectLocale, LOCALES, DEFAULT_LOCALE, LANGUAGE_STORAGE_KEY } from './i18n'
export type { Locale } from './i18n'

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
