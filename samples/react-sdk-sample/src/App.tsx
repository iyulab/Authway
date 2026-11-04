import { useEffect, useState } from 'react'
import { AuthwayProvider, useAuth, detectLocale, LANGUAGE_STORAGE_KEY, type Locale } from '@authway/react'
import type { AuthwayConfig } from '@authway/client'
import { messages, type Messages } from './messages'

// A browser application that signs users in with Authway and calls an API
// with their access token.

const config: AuthwayConfig = {
  // The Authway API; the SDK finds the OpenID Connect issuer, and every
  // endpoint, from there.
  domain: import.meta.env.VITE_AUTHWAY_URL ?? 'http://localhost:8080',
  clientId: import.meta.env.VITE_AUTHWAY_CLIENT_ID ?? '',
  // Must be one of the client's registered redirect URIs, character for character.
  redirectUri: `${window.location.origin}/`,
  // Tokens are kept in the page and nowhere else. A reload loses them, and
  // the provider gives them back: see "checkSessionOnLoad" below.
  // 'sessionstorage' keeps them for the tab, 'localstorage' for the browser.
  cacheLocation: 'memory',
}

const apiUrl = import.meta.env.VITE_API_URL ?? 'http://localhost:5090'

export default function App() {
  // The address bar is this sample's router.
  const [path, setPath] = useState(window.location.pathname)
  const go = (to: string) => {
    window.history.pushState({}, '', to)
    setPath(to)
  }
  useEffect(() => {
    const onPop = () => setPath(window.location.pathname)
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [])

  // English or Korean: the person's choice if they made one (it is kept, and
  // the sign-in screens read the same key), otherwise their browser's.
  const [locale, setLocale] = useState<Locale>(detectLocale)
  const t = messages[locale]
  useEffect(() => {
    document.documentElement.lang = locale
  }, [locale])
  const otherLocale: Locale = locale === 'ko' ? 'en' : 'ko'
  const switchLanguage = () => {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, otherLocale)
    setLocale(otherLocale)
  }

  if (!config.clientId) {
    return <p className="error">{t.setClientId}</p>
  }

  return (
    <AuthwayProvider
      config={config}
      // When a page loads without tokens — a reload, with tokens kept in
      // memory — the provider asks Authway whether the user is still signed
      // in. The browser goes there and back without showing anything. This is
      // the default; it is spelled out here because the sample relies on it.
      checkSessionOnLoad
      // Chosen here, so the components of the SDK speak the same language and
      // the sign-in screen is opened in it too (OpenID Connect ui_locales).
      // Leave it out and both follow the person's browser instead.
      locale={locale}
      // The browser has come back from Authway, after a sign-in or after that
      // question. appState.returnTo is where the user was; an application
      // with a router navigates there.
      onRedirectCallback={(appState) => go(appState?.returnTo ?? '/')}
    >
      <nav>
        <a href="/" onClick={(e) => (e.preventDefault(), go('/'))}>{t.home}</a>
        <a href="/api" onClick={(e) => (e.preventDefault(), go('/api'))}>{t.callTheApi}</a>
        <button onClick={switchLanguage}>{otherLocale === 'ko' ? '한국어' : 'English'}</button>
      </nav>
      <Page path={path} t={t} />
    </AuthwayProvider>
  )
}

function Page({ path, t }: { path: string; t: Messages }) {
  const { isLoading, isAuthenticated, error, user, loginWithRedirect, logout } = useAuth()

  // Also true while the browser is away asking Authway for the session.
  if (isLoading) return <p>{t.loading}</p>

  // The provider refused the sign-in (the user denied consent, has no
  // account, …). "Nobody is signed in" is not an error and does not come here.
  if (error) return <p className="error">{t.signInFailed}: {error.message}</p>

  if (!isAuthenticated) {
    return (
      <>
        <h1>{t.signedOut}</h1>
        <button onClick={() => loginWithRedirect({ appState: { returnTo: path } })}>{t.signIn}</button>
      </>
    )
  }

  return (
    <>
      <h1>{t.signedIn}</h1>
      {/* sub identifies the user at this issuer: key your own records on it, not on the email address. */}
      <p>
        {user?.name} &lt;{user?.email}&gt; — sub <code>{user?.sub}</code>
      </p>
      {path === '/api' && <CallTheApi t={t} />}
      {/* Ends the session at Authway too, for every application signed in on it. */}
      <button onClick={() => logout()}>{t.signOut}</button>
    </>
  )
}

function CallTheApi({ t }: { t: Messages }) {
  const { getAccessToken } = useAuth()
  const [answer, setAnswer] = useState('')

  const call = async (endpoint: string) => {
    try {
      // Ask the SDK each time: it hands out the current token and renews it when it can.
      const token = await getAccessToken()
      const res = await fetch(`${apiUrl}${endpoint}`, { headers: { Authorization: `Bearer ${token}` } })
      setAnswer(`${endpoint} → ${res.status}\n${res.ok ? JSON.stringify(await res.json(), null, 2) : ''}`)
    } catch (e) {
      setAnswer(`${endpoint} → ${(e as Error).message}`)
    }
  }

  return (
    <section>
      <h2>{t.theApi}</h2>
      <button onClick={() => call('/api/me')}>GET /api/me</button>
      <button onClick={() => call('/api/email')}>GET /api/email</button>
      {answer && <pre>{answer}</pre>}
    </section>
  )
}
