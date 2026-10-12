import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AuthwayClient } from './AuthwayClient'
import { AuthenticationError, LoginRequiredError } from './types/errors'
import type { AuthwayConfig } from './types'

const API = 'https://api.example.com'
const ISSUER = 'https://issuer.example.com'
const APP = 'https://app.example.com'

/** A Storage whose keys are its own properties, as `Object.keys(localStorage)` expects. */
function fakeStorage(): Storage {
  const items: Record<string, string> = {}
  Object.defineProperties(items, {
    getItem: { value: (key: string) => (key in items ? items[key] : null) },
    setItem: { value: (key: string, value: string) => void (items[key] = String(value)) },
    removeItem: { value: (key: string) => void delete items[key] },
  })
  return items as unknown as Storage
}

const jwt = (claims: Record<string, unknown>) =>
  `e30.${Buffer.from(JSON.stringify(claims)).toString('base64url')}.signature`

/** The browser the client runs in: storages that outlive a "reload", and where it was last sent. */
function fakeBrowser(path = '/') {
  const location = {
    origin: APP,
    pathname: path,
    search: '',
    hash: '',
    href: `${APP}${path}`,
    sentTo: undefined as URL | undefined,
    assign(url: string) {
      location.sentTo = new URL(url)
    },
  }
  const window = { location, localStorage: fakeStorage(), sessionStorage: fakeStorage() }
  vi.stubGlobal('window', window)
  vi.stubGlobal('sessionStorage', window.sessionStorage)
  vi.stubGlobal('localStorage', window.localStorage)
  return window
}

let tokenRequests: URLSearchParams[]

beforeEach(() => {
  tokenRequests = []
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    if (url === `${API}/.well-known/authway-config`) return json({ issuer: ISSUER, api_url: API })
    if (url === `${ISSUER}/.well-known/openid-configuration`) {
      return json({
        issuer: ISSUER,
        authorization_endpoint: `${ISSUER}/oauth2/auth`,
        token_endpoint: `${ISSUER}/oauth2/token`,
        end_session_endpoint: `${ISSUER}/oauth2/sessions/logout`,
      })
    }
    if (url === `${ISSUER}/oauth2/token`) {
      tokenRequests.push(new URLSearchParams(String(init?.body)))
      return json({
        access_token: 'access-token',
        id_token: jwt({ sub: 'user-1', email: 'user@example.com' }),
        expires_in: 3600,
      })
    }
    return new Response('not found', { status: 404 })
  })
  vi.spyOn(console, 'warn').mockImplementation(() => {})
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  vi.useRealTimers()
})

const config = (cacheLocation: AuthwayConfig['cacheLocation']): AuthwayConfig => ({
  domain: API,
  clientId: 'client-1',
  redirectUri: `${APP}/callback`,
  cacheLocation,
})

/** A page load: a new client over the browser's storages. */
async function load(cacheLocation: AuthwayConfig['cacheLocation']): Promise<AuthwayClient> {
  const client = new AuthwayClient(config(cacheLocation))
  await client.waitForReady()
  return client
}

/** Signs in and leaves the tokens in place. */
async function signedIn(browser: ReturnType<typeof fakeBrowser>, cacheLocation: AuthwayConfig['cacheLocation']): Promise<AuthwayClient> {
  vi.useFakeTimers()
  const before = await load(cacheLocation)
  await before.loginWithRedirect()
  const state = browser.location.sentTo!.searchParams.get('state')!
  const after = await load(cacheLocation)
  await after.handleRedirectCallback(`${APP}/callback?code=the-code&state=${state}`)
  browser.location.sentTo = undefined
  return after
}

describe('where the tokens are kept', () => {
  it("keeps them across a reload with 'sessionstorage' and 'localstorage', and not with 'memory'", async () => {
    for (const [cacheLocation, kept] of [['memory', false], ['sessionstorage', true], ['localstorage', true]] as const) {
      const browser = fakeBrowser()
      await signedIn(browser, cacheLocation)
      const reloaded = await load(cacheLocation)
      expect(await reloaded.isAuthenticated(), cacheLocation).toBe(kept)
      expect((await reloaded.getUser())?.sub, cacheLocation).toBe(kept ? 'user-1' : undefined)
    }
  })

  it("keeps 'sessionstorage' tokens out of localStorage, and 'memory' tokens out of both", async () => {
    const inTab = fakeBrowser()
    await signedIn(inTab, 'sessionstorage')
    expect(JSON.stringify(inTab.localStorage)).not.toContain('access-token')
    expect(JSON.stringify(inTab.sessionStorage)).toContain('access-token')

    const inPage = fakeBrowser()
    await signedIn(inPage, 'memory')
    expect(JSON.stringify(inPage.localStorage) + JSON.stringify(inPage.sessionStorage)).not.toContain('access-token')
  })

  it('clears the tokens on logout without clearing an authorization request in flight', async () => {
    const browser = fakeBrowser()
    const client = await signedIn(browser, 'sessionstorage')
    await client.loginWithRedirect()
    await client.logout({ localOnly: true })
    expect(JSON.stringify(browser.sessionStorage)).not.toContain('access-token')
    expect(browser.sessionStorage.getItem('authway.pkce')).toBeTruthy()
  })
})

describe('checkSession', () => {
  it('stays on the page for a browser nobody has signed in on', async () => {
    const browser = fakeBrowser()
    const client = await load('memory')
    expect(await client.checkSession()).toBe(false)
    expect(browser.location.sentTo).toBeUndefined()
  })

  it('stays on the page while tokens are at hand', async () => {
    const browser = fakeBrowser()
    const client = await signedIn(browser, 'memory')
    expect(await client.checkSession()).toBe(false)
    expect(browser.location.sentTo).toBeUndefined()
  })

  it('asks the provider without a prompt after a reload that lost the tokens, and comes back signed in', async () => {
    const browser = fakeBrowser()
    await signedIn(browser, 'memory')

    // The reload: a page somewhere in the application, with no tokens.
    browser.location.pathname = '/reports/42'
    browser.location.search = '?tab=costs'
    const reloaded = await load('memory')
    expect(await reloaded.isAuthenticated()).toBe(false)
    expect(await reloaded.checkSession()).toBe(true)

    const request = browser.location.sentTo!
    expect(`${request.origin}${request.pathname}`).toBe(`${ISSUER}/oauth2/auth`)
    expect(request.searchParams.get('prompt')).toBe('none')
    expect(request.searchParams.get('redirect_uri')).toBe(`${APP}/callback`)
    expect(request.searchParams.get('code_challenge_method')).toBe('S256')
    // Options of the SDK are not parameters of the request.
    expect([...request.searchParams.keys()]).not.toContain('redirectUri')
    expect([...request.searchParams.keys()]).not.toContain('appState')

    // The provider answers with a code: the page at the redirect URI has the tokens again.
    const back = await load('memory')
    const result = await back.handleRedirectCallback(`${APP}/callback?code=again&state=${request.searchParams.get('state')}`)
    expect(result.appState).toEqual({ returnTo: '/reports/42?tab=costs' })
    expect(await back.isAuthenticated()).toBe(true)
    expect(tokenRequests.at(-1)?.get('code')).toBe('again')
    await back.logout({ localOnly: true })
  })

  it('takes "not signed in" for an answer, once: no error, and no second round trip', async () => {
    for (const answer of ['login_required', 'consent_required', 'interaction_required']) {
      const browser = fakeBrowser('/inbox')
      await signedIn(browser, 'memory')
      const reloaded = await load('memory')
      expect(await reloaded.checkSession(), answer).toBe(true)
      const state = browser.location.sentTo!.searchParams.get('state')

      const back = await load('memory')
      const outcome = await back.handleRedirectCallback(`${APP}/callback?error=${answer}&state=${state}`).catch((e) => e)
      expect(outcome, answer).toBeInstanceOf(LoginRequiredError)
      expect((outcome as LoginRequiredError).appState, answer).toEqual({ returnTo: '/inbox' })
      expect(await back.isAuthenticated(), answer).toBe(false)

      // The next page load does not go looking again.
      browser.location.sentTo = undefined
      const next = await load('memory')
      expect(await next.checkSession(), answer).toBe(false)
      expect(browser.location.sentTo, answer).toBeUndefined()
    }
  })

  it('does not go round again when the answer never reaches the application', async () => {
    const browser = fakeBrowser()
    await signedIn(browser, 'memory')
    expect(await (await load('memory')).checkSession()).toBe(true)

    // The browser came back, but nothing handled the answer.
    browser.location.sentTo = undefined
    expect(await (await load('memory')).checkSession()).toBe(false)
    expect(browser.location.sentTo).toBeUndefined()
  })

  it('looks again after the user has signed in again', async () => {
    const browser = fakeBrowser()
    await signedIn(browser, 'memory')
    const reloaded = await load('memory')
    await reloaded.checkSession()
    const state = browser.location.sentTo!.searchParams.get('state')
    await (await load('memory')).handleRedirectCallback(`${APP}/callback?error=login_required&state=${state}`).catch(() => {})

    await signedIn(browser, 'memory')
    expect(await (await load('memory')).checkSession()).toBe(true)
  })

  it('does not look after a logout', async () => {
    const browser = fakeBrowser()
    const client = await signedIn(browser, 'memory')
    await client.logout({ localOnly: true })
    browser.location.sentTo = undefined
    expect(await (await load('memory')).checkSession()).toBe(false)
    expect(browser.location.sentTo).toBeUndefined()
  })

  it("looks in a new tab with 'sessionstorage', where the tab's tokens are not", async () => {
    const browser = fakeBrowser()
    await signedIn(browser, 'sessionstorage')
    // A new tab shares localStorage and starts with an empty sessionStorage.
    const newTab = { ...browser, sessionStorage: fakeStorage() }
    vi.stubGlobal('window', newTab)
    vi.stubGlobal('sessionStorage', newTab.sessionStorage)
    expect(await (await load('sessionstorage')).checkSession()).toBe(true)
    expect(browser.location.sentTo?.searchParams.get('prompt')).toBe('none')
  })
})

describe('an error at the redirect URI', () => {
  it('is "not signed in" only as the answer to a request this page made', async () => {
    const browser = fakeBrowser()
    await signedIn(browser, 'memory')
    const page = await load('memory')

    // An address anyone can link to: no request of ours is waiting for it.
    const linked = await page.handleRedirectCallback(`${APP}/callback?error=login_required&state=made-up`).catch((e) => e)
    expect(linked).toBeInstanceOf(AuthenticationError)
    expect(linked).not.toBeInstanceOf(LoginRequiredError)
    // And it does not stop the page from looking for the session.
    expect(await page.checkSession()).toBe(true)
  })

  it('stays an error when the provider refuses for another reason', async () => {
    const browser = fakeBrowser()
    const client = await load('memory')
    await client.loginWithRedirect()
    const state = browser.location.sentTo!.searchParams.get('state')
    const refused = await client.handleRedirectCallback(`${APP}/callback?error=access_denied&error_description=no+account&state=${state}`).catch((e) => e)
    expect(refused).toBeInstanceOf(AuthenticationError)
    expect(refused).not.toBeInstanceOf(LoginRequiredError)
    expect((refused as AuthenticationError).code).toBe('access_denied')
  })
})

describe('useRefreshTokens', () => {
  const requestedScope = async (overrides: Partial<AuthwayConfig>) => {
    const browser = fakeBrowser()
    const client = new AuthwayClient({ ...config('memory'), ...overrides })
    await client.waitForReady()
    await client.loginWithRedirect()
    return browser.location.sentTo!.searchParams.get('scope')
  }

  it('asks for offline_access when it is on, and only then', async () => {
    expect(await requestedScope({})).toBe('openid profile email')
    expect(await requestedScope({ useRefreshTokens: false })).toBe('openid profile email')
    expect(await requestedScope({ useRefreshTokens: true })).toBe('openid profile email offline_access')
    expect(await requestedScope({ useRefreshTokens: true, scope: 'openid offline_access reports' })).toBe('openid offline_access reports')
  })

  it('does not set out to renew tokens it has no refresh token for', async () => {
    const browser = fakeBrowser()
    vi.useFakeTimers()
    const errors = vi.spyOn(console, 'error').mockImplementation(() => {})
    await signedIn(browser, 'memory')
    // Past the point where the access token (an hour, here) would have been renewed.
    await vi.advanceTimersByTimeAsync(3600 * 1000)
    expect(errors).not.toHaveBeenCalled()
    expect(tokenRequests.filter((r) => r.get('grant_type') === 'refresh_token')).toEqual([])
  })
})
