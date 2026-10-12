import { useEffect, useState } from 'react'
import { AuthwayProvider, useAuth } from '@authway/react'
import type { AuthwayConfig } from '@authway/client'

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

  if (!config.clientId) {
    return <p className="error">Set VITE_AUTHWAY_CLIENT_ID in .env.local — see the README.</p>
  }

  return (
    <AuthwayProvider
      config={config}
      // When a page loads without tokens — a reload, with tokens kept in
      // memory — the provider asks Authway whether the user is still signed
      // in. The browser goes there and back without showing anything. This is
      // the default; it is spelled out here because the sample relies on it.
      checkSessionOnLoad
      // The browser has come back from Authway, after a sign-in or after that
      // question. appState.returnTo is where the user was; an application
      // with a router navigates there.
      onRedirectCallback={(appState) => go(appState?.returnTo ?? '/')}
    >
      <nav>
        <a href="/" onClick={(e) => (e.preventDefault(), go('/'))}>Home</a>
        <a href="/api" onClick={(e) => (e.preventDefault(), go('/api'))}>Call the API</a>
      </nav>
      <Page path={path} />
    </AuthwayProvider>
  )
}

function Page({ path }: { path: string }) {
  const { isLoading, isAuthenticated, error, user, loginWithRedirect, logout } = useAuth()

  // Also true while the browser is away asking Authway for the session.
  if (isLoading) return <p>Loading…</p>

  // The provider refused the sign-in (the user denied consent, has no
  // account, …). "Nobody is signed in" is not an error and does not come here.
  if (error) return <p className="error">Sign-in failed: {error.message}</p>

  if (!isAuthenticated) {
    return (
      <>
        <h1>Signed out</h1>
        <button onClick={() => loginWithRedirect({ appState: { returnTo: path } })}>Sign in</button>
      </>
    )
  }

  return (
    <>
      <h1>Signed in</h1>
      {/* sub identifies the user at this issuer: key your own records on it, not on the email address. */}
      <p>
        {user?.name} &lt;{user?.email}&gt; — sub <code>{user?.sub}</code>
      </p>
      {path === '/api' && <CallTheApi />}
      {/* Ends the session at Authway too, for every application signed in on it. */}
      <button onClick={() => logout()}>Sign out</button>
    </>
  )
}

function CallTheApi() {
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
      <h2>The API</h2>
      <button onClick={() => call('/api/me')}>GET /api/me</button>
      <button onClick={() => call('/api/email')}>GET /api/email</button>
      {answer && <pre>{answer}</pre>}
    </section>
  )
}
