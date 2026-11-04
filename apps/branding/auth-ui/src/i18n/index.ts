import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'

// Import translations
import koCommon from './locales/ko/common.json'
import koAuth from './locales/ko/auth.json'
import koConsent from './locales/ko/consent.json'
import koPassword from './locales/ko/password.json'
import koErrors from './locales/ko/errors.json'

import enCommon from './locales/en/common.json'
import enAuth from './locales/en/auth.json'
import enConsent from './locales/en/consent.json'
import enPassword from './locales/en/password.json'
import enErrors from './locales/en/errors.json'

const resources = {
  ko: {
    common: koCommon,
    auth: koAuth,
    consent: koConsent,
    password: koPassword,
    errors: koErrors,
  },
  en: {
    common: enCommon,
    auth: enAuth,
    consent: enConsent,
    password: enPassword,
    errors: enErrors,
  },
}

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: 'en',
    // Two languages are supported; a region tag (ko-KR) reads as its language.
    supportedLngs: ['ko', 'en'],
    nonExplicitSupportedLngs: true,
    defaultNS: 'common',
    ns: ['common', 'auth', 'consent', 'password', 'errors'],

    detection: {
      order: ['querystring', 'localStorage', 'navigator', 'htmlTag'],
      lookupQuerystring: 'lang',
      lookupLocalStorage: 'authway-language',
      caches: ['localStorage'],
    },

    interpolation: {
      escapeValue: false, // React already escapes values
    },

    react: {
      useSuspense: false,
    },
  })

// The document says which language it is in: screen readers, hyphenation and
// the browser's own translation offer all read it.
const setDocumentLanguage = (lng: string) => {
  document.documentElement.lang = lng.split('-')[0]
}
i18n.on('languageChanged', setDocumentLanguage)
if (i18n.language) setDocumentLanguage(i18n.language)

export default i18n
