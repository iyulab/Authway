import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'

/** The languages the console is written in. */
export const SUPPORTED_LANGUAGES = [
  { code: 'ko', name: '한국어' },
  { code: 'en', name: 'English' },
] as const

export type LanguageCode = (typeof SUPPORTED_LANGUAGES)[number]['code']

export const LANGUAGE_STORAGE_KEY = 'authway-language'

// locales/<language>/<namespace>.json — every file is a namespace, so a new
// area is a new file with nothing to register.
const files = import.meta.glob<Record<string, unknown>>('./locales/*/*.json', {
  eager: true,
  import: 'default',
})

const resources: Record<string, Record<string, Record<string, unknown>>> = {}
for (const [path, strings] of Object.entries(files)) {
  const [, language, namespace] = /\.\/locales\/([^/]+)\/([^/]+)\.json$/.exec(path) ?? []
  if (!language || !namespace) continue
  ;(resources[language] ??= {})[namespace] = strings
}

export const NAMESPACES = Object.keys(resources.en ?? {})

const baseLanguage = (tag: string) => tag.toLowerCase().split('-')[0]

/** The language a rendered string is in (`ko` or `en`), whatever tag the browser reported. */
export const currentLanguage = (): LanguageCode =>
  baseLanguage(i18n.resolvedLanguage ?? i18n.language ?? 'en') === 'ko' ? 'ko' : 'en'

/** The locale tag for Intl formatting in the current language. */
export const intlLocale = () => (currentLanguage() === 'ko' ? 'ko-KR' : 'en-US')

// The page's own language and title follow the console's language.
const applyDocumentLanguage = (language: string) => {
  if (typeof document === 'undefined') return
  document.documentElement.lang = baseLanguage(language)
  document.title = i18n.t('common:documentTitle')
}

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: 'en',
    supportedLngs: SUPPORTED_LANGUAGES.map((l) => l.code),
    nonExplicitSupportedLngs: true,
    load: 'languageOnly',
    defaultNS: 'common',
    ns: NAMESPACES,
    detection: {
      order: ['querystring', 'localStorage', 'navigator', 'htmlTag'],
      lookupQuerystring: 'lang',
      lookupLocalStorage: LANGUAGE_STORAGE_KEY,
      caches: ['localStorage'],
      // 'ko-KR' is Korean; the console has one variant of each language.
      convertDetectedLanguage: baseLanguage,
    },
    interpolation: { escapeValue: false }, // React escapes
    react: { useSuspense: false },
  })

applyDocumentLanguage(i18n.language)
i18n.on('languageChanged', applyDocumentLanguage)

export default i18n
