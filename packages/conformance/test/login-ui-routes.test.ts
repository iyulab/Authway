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
 * This hand-kept list goes away once the login UI and the backend share a
 * machine-readable API contract.
 */
const LOGIN_UI_POSTS: { path: string }[] = [
  { path: '/api/email/forgot-password' },
  { path: '/api/email/send-verification' },
  { path: '/api/email/reset-password' },
  { path: '/api/v1/invitations/accept' },
]

describe('login UI backend routes', () => {
  let client: TestClient
  let flow: string

  /** Opens a fresh login flow for the test client. */
  const startFlow = async (): Promise<string> => {
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
    return provider.loginFlowFrom(await provider.openLoginScreen(new Browser(), authorize.toString()))
  }

  beforeAll(async () => {
    client = await provider.createPublicClient()
    flow = await startFlow()
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
    expect(Array.isArray(body.client?.sign_in_methods)).toBe(true)
  })

  it('publishes what the deployment offers, and offers sign-in only through it', async () => {
    const caps = await (await fetch(`${provider.config.api}/api/v1/capabilities`)).json()
    expect(Array.isArray(caps.providers), JSON.stringify(caps)).toBe(true)
    expect(typeof caps.multi_tenant).toBe('boolean')
    expect(typeof caps.magic_link).toBe('boolean')

    const res = await fetch(provider.loginFlowUrl(flow))
    const methods: string[] = (await res.json()).client.sign_in_methods
    for (const m of methods.filter((m) => m !== 'email')) {
      expect(caps.providers, `sign-in offers ${m}, which the deployment cannot run`).toContain(m)
    }
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

  it('starts social sign-in as a page navigation: to the provider, or back with an error', async () => {
    const caps = await (await fetch(`${provider.config.api}/api/v1/capabilities`)).json()
    // Its own flow: an unusable provider ends the flow it was started on.
    const browser = new Browser()
    let location = await browser.redirectFrom(provider.loginFlowUrl(await startFlow(), '/social/google'))
    if (caps.providers.includes('google')) {
      expect(location.origin).not.toBe(new URL(provider.config.api).origin)
      return
    }
    // Not configured here: the authorization request ends, and the client gets an OAuth error.
    const client = new URL(REDIRECT_URI).origin
    for (let hops = 0; hops < 5 && location.origin !== client; hops++) {
      location = await browser.redirectFrom(location.toString())
    }
    expect(location.searchParams.get('error'), location.toString()).toBe('invalid_request')
  })

  // Consent and logout steps must reach the flow lookup: an unknown flow id is
  // answered as such, not as a missing route or a server fault.
  const UNKNOWN_FLOW_STEPS: { kind: 'consent-flows' | 'logout-flows'; path: string; method: string }[] = [
    { kind: 'consent-flows', path: '', method: 'GET' },
    { kind: 'consent-flows', path: '/accept', method: 'POST' },
    { kind: 'consent-flows', path: '/reject', method: 'POST' },
    { kind: 'logout-flows', path: '', method: 'POST' },
  ]
  for (const { kind, path, method } of UNKNOWN_FLOW_STEPS) {
    it(`answers ${method} ${kind}/{flow}${path} for an unknown flow as invalid_flow`, async () => {
      const res = await fetch(provider.flowUrl(kind, `unknown-${randomToken(4)}`, path), {
        method,
        headers: method === 'POST' ? { 'Content-Type': 'application/json' } : undefined,
        body: method === 'POST' ? '{}' : undefined,
      })
      const body = await res.json()
      expect(res.status, JSON.stringify(body)).toBe(400)
      expect(body.code).toBe('invalid_flow')
    })
  }

  for (const { path } of LOGIN_UI_POSTS) {
    it(`routes POST ${path}`, async () => {
      const res = await fetch(`${provider.config.api}${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: "{}",
      })
      expect(res.status, await res.text()).not.toBe(404)
    })
  }
})
