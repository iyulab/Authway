/**
 * The few words this package puts on screen — the default label of a button,
 * the loading text, the alt text of an avatar — in the two languages Authway
 * supports. Anything an application passes in (`children`, `onRedirecting`)
 * is the application's own and is never translated.
 *
 * A table and a function, no i18n library: the package stays free of one.
 */

export type Locale = 'en' | 'ko'

export const LOCALES: readonly Locale[] = ['en', 'ko']

/** The language used when no preference names a supported one. */
export const DEFAULT_LOCALE: Locale = 'en'

/** Where a person's chosen language is kept — the key the sign-in screens use. */
export const LANGUAGE_STORAGE_KEY = 'authway-language'

/** The query parameter that names a language, as on the sign-in screens. */
export const LANGUAGE_QUERY_PARAM = 'lang'

const messages = {
  en: {
    logIn: 'Log In',
    logOut: 'Log Out',
    loading: 'Loading...',
    userAvatar: 'User avatar'
  },
  ko: {
    logIn: '로그인',
    logOut: '로그아웃',
    loading: '불러오는 중...',
    userAvatar: '사용자 프로필 사진'
  }
} as const satisfies Record<Locale, Record<string, string>>

export type MessageKey = keyof (typeof messages)['en']

/** The text of a message in a locale. */
export function message(locale: Locale, key: MessageKey): string {
  return messages[locale][key]
}

/**
 * Picks a supported locale from a language preference: an `Accept-Language`
 * style value (`ko-KR,ko;q=0.9,en;q=0.8`), a list of tags such as
 * `navigator.languages`, or one tag (`ko`, `en-US`, `ko_KR`).
 *
 * Tags are taken by weight (`q`), highest first, and in the order written when
 * the weights are equal; a tag of weight 0 is refused. The first tag whose
 * language is supported wins, so a region tag matches its language. Nothing
 * usable gives `undefined`.
 */
export function negotiateLocale(
  preference: string | readonly string[] | null | undefined
): Locale | undefined {
  if (!preference) return undefined
  const parts = typeof preference === 'string' ? preference.split(',') : preference.flatMap(p => p.split(','))
  const ranked: { lang: string; q: number; order: number }[] = []
  parts.forEach((part, order) => {
    const [tagPart, ...params] = part.split(';')
    const tag = tagPart.trim().toLowerCase()
    if (!tag || tag === '*') return
    let q = 1
    for (const param of params) {
      const [name, value] = param.split('=')
      if (name?.trim().toLowerCase() === 'q') {
        const parsed = Number.parseFloat(value ?? '')
        if (!Number.isNaN(parsed)) q = parsed
      }
    }
    if (q <= 0) return
    ranked.push({ lang: tag.replace(/_/g, '-').split('-')[0], q, order })
  })
  ranked.sort((a, b) => b.q - a.q || a.order - b.order)
  for (const { lang } of ranked) {
    const found = LOCALES.find(l => l === lang)
    if (found) return found
  }
  return undefined
}

/**
 * The language of the person using the page, by the rule the sign-in screens
 * use: the `lang` query parameter, then the language they chose before
 * (`localStorage`), then the browser's languages; otherwise English. Safe where
 * there is no `window` (server rendering): that is English.
 */
export function detectLocale(): Locale {
  if (typeof window === 'undefined') return DEFAULT_LOCALE
  try {
    const fromQuery = negotiateLocale(new URLSearchParams(window.location.search).get(LANGUAGE_QUERY_PARAM))
    if (fromQuery) return fromQuery
  } catch {
    // an address that cannot be read names no language
  }
  try {
    const fromStorage = negotiateLocale(window.localStorage.getItem(LANGUAGE_STORAGE_KEY))
    if (fromStorage) return fromStorage
  } catch {
    // storage may be blocked
  }
  if (typeof navigator !== 'undefined') {
    const fromBrowser = negotiateLocale(navigator.languages?.length ? navigator.languages : navigator.language)
    if (fromBrowser) return fromBrowser
  }
  return DEFAULT_LOCALE
}
