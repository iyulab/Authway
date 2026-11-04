import { afterEach, describe, expect, it, vi } from 'vitest'
import { createElement } from 'react'
import { renderToString } from 'react-dom/server'
import { AuthwayContext, type AuthContextValue } from './context/AuthwayContext'
import { LoginButton, LogoutButton, UserProfile, withAuthRequired } from './components'
import { detectLocale, negotiateLocale, type Locale } from './i18n'

describe('negotiateLocale', () => {
  const cases: [string | string[] | null | undefined, Locale | undefined][] = [
    [undefined, undefined],
    [null, undefined],
    ['', undefined],
    ['ko', 'ko'],
    ['KO-kr', 'ko'],
    ['ko_KR', 'ko'],
    ['en-US', 'en'],
    ['ko-KR,ko;q=0.9,en-US;q=0.8,en;q=0.7', 'ko'],
    ['en-US,en;q=0.9,ko;q=0.8', 'en'],
    ['en;q=0.5, ko;q=0.9', 'ko'],
    ['ko, en', 'ko'],
    ['en, ko', 'en'],
    ['fr-FR,fr;q=0.9,ko;q=0.8', 'ko'],
    ['ja,zh;q=0.8', undefined],
    ['*', undefined],
    ['ko;q=0, en', 'en'],
    ['ko;q=0', undefined],
    [['fr-FR', 'ko-KR', 'en'], 'ko'],
    [['en-US', 'ko'], 'en'],
    [[], undefined]
  ]
  for (const [preference, want] of cases) {
    it(`reads ${JSON.stringify(preference)} as ${want}`, () => {
      expect(negotiateLocale(preference)).toBe(want)
    })
  }
})

/** Stands in for the browser a page runs in. */
function browser(opts: { search?: string; stored?: string | null; languages?: string[] }) {
  vi.stubGlobal('window', {
    location: { search: opts.search ?? '' },
    localStorage: { getItem: () => opts.stored ?? null }
  })
  vi.stubGlobal('navigator', { languages: opts.languages ?? [], language: opts.languages?.[0] })
}

describe('detectLocale', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('is English where there is no window', () => {
    expect(detectLocale()).toBe('en')
  })

  it('takes the lang query parameter first, then the stored choice, then the browser', () => {
    browser({ search: '?lang=ko', stored: 'en', languages: ['en-US'] })
    expect(detectLocale()).toBe('ko')
    browser({ search: '', stored: 'ko', languages: ['en-US'] })
    expect(detectLocale()).toBe('ko')
    browser({ search: '?lang=xx', stored: null, languages: ['fr-FR', 'ko-KR'] })
    expect(detectLocale()).toBe('ko')
  })

  it('falls back to English', () => {
    browser({ search: '', stored: null, languages: ['ja-JP'] })
    expect(detectLocale()).toBe('en')
    browser({})
    expect(detectLocale()).toBe('en')
  })

  it('survives storage that throws', () => {
    vi.stubGlobal('window', {
      location: { search: '' },
      localStorage: { getItem: () => { throw new Error('blocked') } }
    })
    vi.stubGlobal('navigator', { languages: ['ko-KR'] })
    expect(detectLocale()).toBe('ko')
  })
})

/** The auth context of a page in a given state and language. */
function inContext(locale: Locale, state: Partial<AuthContextValue>, child: ReturnType<typeof createElement>) {
  const value = {
    isAuthenticated: false,
    isLoading: false,
    user: null,
    error: null,
    locale,
    loginWithRedirect: async () => {},
    logout: async () => {},
    ...state
  } as AuthContextValue
  return renderToString(createElement(AuthwayContext.Provider, { value }, child))
}

describe('the words the components put on screen', () => {
  it('are English or Korean as the locale says', () => {
    expect(inContext('en', {}, createElement(LoginButton))).toContain('>Log In<')
    expect(inContext('ko', {}, createElement(LoginButton))).toContain('>로그인<')
    expect(inContext('en', {}, createElement(LogoutButton))).toContain('>Log Out<')
    expect(inContext('ko', {}, createElement(LogoutButton))).toContain('>로그아웃<')
    expect(inContext('en', { isLoading: true }, createElement(UserProfile))).toContain('Loading...')
    expect(inContext('ko', { isLoading: true }, createElement(UserProfile))).toContain('불러오는 중...')
  })

  it('leave an application’s own words alone', () => {
    expect(inContext('ko', {}, createElement(LoginButton, null, 'Sign in here'))).toContain('>Sign in here<')
    expect(inContext('ko', {}, createElement(LogoutButton, null, 'Bye'))).toContain('>Bye<')
  })

  it('name an avatar in the locale when the user has no name', () => {
    const user = { sub: 'u', picture: 'https://example.com/a.png' } as AuthContextValue['user']
    expect(inContext('en', { user }, createElement(UserProfile))).toContain('alt="User avatar"')
    expect(inContext('ko', { user }, createElement(UserProfile))).toContain('alt="사용자 프로필 사진"')
  })

  it('show the loading text while a protected page redirects', () => {
    const Protected = withAuthRequired(() => createElement('div', null, 'secret'))
    expect(inContext('en', { isLoading: true }, createElement(Protected))).toContain('Loading...')
    expect(inContext('ko', { isLoading: true }, createElement(Protected))).toContain('불러오는 중...')
  })
})

describe('AuthwayProvider locale', () => {
  const config = { domain: 'https://auth.example.com', clientId: 'app', redirectUri: 'https://app.example.com/cb' }

  it('hands the chosen language to the components, and to the sign-in screen as ui_locales', async () => {
    const { AuthwayProvider } = await import('./context/AuthwayProvider')
    const { AuthwayClient } = await import('@authway/client')
    const login = vi.spyOn(AuthwayClient.prototype, 'loginWithRedirect').mockResolvedValue(undefined as never)
    let seen: AuthContextValue | undefined
    const { useAuth } = await import('./hooks/useAuth')
    const Reader = () => {
      seen = useAuth()
      return null
    }
    renderToString(createElement(AuthwayProvider, { config: config as never, locale: 'ko-KR', children: createElement(Reader) }))
    expect(seen?.locale).toBe('ko')
    await seen?.loginWithRedirect()
    expect(login).toHaveBeenLastCalledWith({ ui_locales: 'ko' })
    await seen?.loginWithRedirect({ ui_locales: 'en', appState: { returnTo: '/x' } })
    expect(login).toHaveBeenLastCalledWith({ ui_locales: 'en', appState: { returnTo: '/x' } })

    renderToString(createElement(AuthwayProvider, { config: config as never, children: createElement(Reader) }))
    expect(seen?.locale).toBe('en')
    await seen?.loginWithRedirect()
    expect(login).toHaveBeenLastCalledWith(undefined)
    login.mockRestore()
  })
})
