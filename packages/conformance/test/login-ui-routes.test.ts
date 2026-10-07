import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Browser } from '../src/browser.js'
import { loadConfig } from '../src/config.js'
import { createPkce, randomToken } from '../src/pkce.js'
import { Provider, REDIRECT_URI, type TestClient } from '../src/provider.js'

const provider = new Provider(loadConfig())

/**
 * Every backend path the bundled login UI posts to, as the UI calls it. A path the
 * UI calls but the backend does not route is a screen that cannot work.
 *
 * Paths marked `knownBroken` currently 404 on the single backend the login UI
 * is meant to talk to. They are asserted with `it.fails`, so the moment one is
 * fixed this suite turns red and the marker has to be removed — the list can
 * only shrink.
 *
 * This hand-kept list goes away once the login UI and the backend share a
 * machine-readable API contract.
 */
const LOGIN_UI_POSTS: { path: string; knownBroken?: boolean }[] = [
  { path: '/consent' },
  { path: '/consent/accept' },
  { path: '/consent/reject' },
  { path: '/api/v1/auth/magic-link/verify' },
  // The magic-link request form posts here, but the backend has no
  // magic-link step inside the sign-in flow yet.
  { path: '/auth/magic-link/request', knownBroken: true },
  { path: '/api/email/forgot-password' },
  { path: '/api/email/send-verification' },
  { path: '/api/email/reset-password' },
  { path: '/api/v1/invitations/accept' },
]

describe('login UI backend routes', () => {
  let client: TestClient
  let flow: string

  beforeAll(async () => {
    client = await provider.createPublicClient()
    const d = await provider.discovery()
    const authorize = new URL(d.authorization_endpoint)
    authorize.search = new URLSearchParams({
      client_id: client.clientId,
      response_type: 'code',
      redirect_uri: REDIRECT_URI,
      scope: 'openid',
      state: randomToken(),
      code_challenge: createPkce().challenge,
      code_challenge_method: 'S256',
    }).toString()
    flow = provider.loginFlowFrom(await provider.openLoginScreen(new Browser(), authorize.toString()))
  })

  afterAll(async () => {
    if (client) await provider.deleteClient(client)
  })

  // Routing alone is not enough here: a backend once answered this path by
  // starting a Google sign-in, and the screen could not show its form at all.
  it('describes the login flow the sign-in screen opens on', async () => {
    const res = await fetch(`${provider.config.api}/api/v1/login-flows/${encodeURIComponent(flow)}`)
    const body = await res.json()
    expect(res.status, JSON.stringify(body)).toBe(200)
    expect(body.next).toBe('form')
    expect(body.flow).toBe(flow)
    expect(body.client?.client_id).toBe(client.clientId)
    expect(Array.isArray(body.client?.enabled_auth_providers)).toBe(true)
  })

  // The login-flow steps are asserted by what they answer, not by being routed:
  // each must reach the check it exists for.
  it('checks the password step against the user directory', async () => {
    const res = await fetch(provider.loginFlowUrl(flow, '/password'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: `nobody-${randomToken(4)}@example.test`, password: 'not-the-password' }),
    })
    const body = await res.json()
    expect(res.status, JSON.stringify(body)).toBe(401)
    expect(body.code).toBe('invalid_credentials')
  })

  for (const step of ['/mfa', '/mfa/recovery']) {
    it(`refuses the ${step} step without the challenge the password step issues`, async () => {
      const res = await fetch(provider.loginFlowUrl(flow, step), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mfa_challenge: 'not-issued', code: '000000' }),
      })
      const body = await res.json()
      expect(res.status, JSON.stringify(body)).toBe(400)
      expect(body.code).toBe('invalid_mfa_challenge')
    })
  }

  it('starts social sign-in as a page navigation that leaves the backend', async () => {
    const res = await new Browser().fetch(provider.loginFlowUrl(flow, '/social/google'))
    expect(res.status).toBe(302)
    const location = new URL(res.headers.get('location') ?? '', provider.config.api)
    expect(location.origin).not.toBe(new URL(provider.config.api).origin)
  })

  for (const { path, knownBroken } of LOGIN_UI_POSTS) {
    const test = knownBroken ? it.fails : it
    test(`routes POST ${path}`, async () => {
      const res = await fetch(`${provider.config.api}${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ challenge: flow, login_challenge: flow }),
      })
      expect(res.status, await res.text()).not.toBe(404)
    })
  }
})
