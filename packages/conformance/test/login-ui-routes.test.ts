import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Browser } from '../src/browser.js'
import { loadConfig } from '../src/config.js'
import { createPkce, randomToken } from '../src/pkce.js'
import { Provider, REDIRECT_URI, type TestClient } from '../src/provider.js'
import { conform } from '../src/contract.js'

const provider = new Provider(loadConfig())

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
    await conform('GET', '/api/v1/login-flows/{flow}', res)
    const body = await res.json()
    expect(res.status, JSON.stringify(body)).toBe(200)
    expect(body.next).toBe('form')
    expect(body.flow).toBe(flow)
    expect(body.client?.client_id).toBe(client.clientId)
    expect(Array.isArray(body.client?.sign_in_methods)).toBe(true)
  })

  // An application is configured with one address and finds the rest from
  // here; whichever provider it talks to, the issuer named must be the one
  // whose discovery document it then reads.
  it('publishes a bootstrap document naming the issuer, the API and the login UI', async () => {
    const res = await fetch(`${provider.config.api}/.well-known/authway-config`)
    await conform('GET', '/.well-known/authway-config', res)
    expect(res.status).toBe(200)
    const doc = (await res.json()) as { issuer: string; api_url: string; auth_ui: string }
    expect(doc.issuer.replace(/\/$/, '')).toBe((await provider.discovery()).issuer.replace(/\/$/, ''))
    expect(doc.api_url).toBeTruthy()
    expect(() => new URL(doc.auth_ui)).not.toThrow()
  })

  it('publishes what the deployment offers, and offers sign-in only through it', async () => {
    const capsRes = await fetch(`${provider.config.api}/api/v1/capabilities`)
    await conform('GET', '/api/v1/capabilities', capsRes)
    const caps = await capsRes.json()
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
    await conform('POST', '/api/v1/login-flows/{flow}/password', res)
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
      await conform('POST', `/api/v1/login-flows/{flow}${step}`, res)
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
      await conform(method, `/api/v1/${kind}/{flow}${path}`, res)
      const body = await res.json()
      expect(res.status, JSON.stringify(body)).toBe(400)
      expect(body.code).toBe('invalid_flow')
    })
  }

  // Account screens: each answers what the contract says, and the
  // enumeration-safe ones answer an unknown address like a known one.
  const json = (body: unknown): RequestInit => ({
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const nobody = () => `nobody-${randomToken(4)}@example.test`

  it('answers a verification request for an unknown address like any other', async () => {
    const res = await fetch(`${provider.config.api}/api/v1/email/send-verification`, json({ email: nobody() }))
    await conform('POST', '/api/v1/email/send-verification', res)
    expect(res.status).toBe(200)
  })

  it('answers a password reset request for an unknown address like any other', async () => {
    const res = await fetch(`${provider.config.api}/api/v1/email/forgot-password`, json({ email: nobody() }))
    await conform('POST', '/api/v1/email/forgot-password', res)
    expect(res.status).toBe(200)
  })

  it('refuses tokens it did not issue', async () => {
    const bogus = randomToken(8)
    const checks: [string, string, Response][] = [
      ['GET', '/api/v1/email/verify', await fetch(`${provider.config.api}/api/v1/email/verify?token=${bogus}`)],
      ['GET', '/api/v1/email/verify-reset-token', await fetch(`${provider.config.api}/api/v1/email/verify-reset-token?token=${bogus}`)],
      ['POST', '/api/v1/email/reset-password', await fetch(`${provider.config.api}/api/v1/email/reset-password`, json({ token: bogus, new_password: 'long-enough-1' }))],
      ['GET', '/api/v1/invitations/token/{token}', await fetch(`${provider.config.api}/api/v1/invitations/token/${bogus}`)],
      ['POST', '/api/v1/invitations/accept', await fetch(`${provider.config.api}/api/v1/invitations/accept`, json({ token: bogus, password: 'long-enough-1', name: 'x' }))],
    ]
    for (const [method, path, res] of checks) {
      await conform(method, path, res)
      expect(res.status, `${method} ${path}`).toBeGreaterThanOrEqual(400)
      expect(res.status, `${method} ${path}`).toBeLessThan(500)
    }
  })
})
