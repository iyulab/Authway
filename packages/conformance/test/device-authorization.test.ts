import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Browser } from '../src/browser.js'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)
const DEVICE_GRANT = 'urn:ietf:params:oauth:grant-type:device_code'

interface DeviceAuthorization {
  device_code: string
  user_code: string
  verification_uri: string
  verification_uri_complete?: string
  expires_in: number
  interval?: number
}

interface TokenAnswer {
  status: number
  body: { access_token?: string; id_token?: string; refresh_token?: string; error?: string }
}

const claimsOf = (jwt: string): Record<string, unknown> =>
  JSON.parse(Buffer.from(jwt.split('.')[1], 'base64url').toString('utf8')) as Record<string, unknown>

// A device without a usable browser or keyboard — a TV, a console, a
// command-line tool — shows the user a code and where to enter it, and asks
// for its tokens until the user has approved it on another screen
// (RFC 8628). Once the code is accepted, the sign-in is the ordinary one: the
// login screen, then consent.
describe('device authorization', () => {
  let client: TestClient
  let user: TestUser

  const authorizeDevice = async (): Promise<DeviceAuthorization> => {
    const endpoint = (await provider.discovery()) as unknown as { device_authorization_endpoint: string }
    const res = await fetch(endpoint.device_authorization_endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ client_id: client.clientId, scope: 'openid offline_access' }),
    })
    expect(res.status, await res.clone().text()).toBe(200)
    return (await res.json()) as DeviceAuthorization
  }

  const poll = async (device: DeviceAuthorization): Promise<TokenAnswer> => {
    const res = await fetch((await provider.discovery()).token_endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ grant_type: DEVICE_GRANT, device_code: device.device_code, client_id: client.clientId }),
    })
    return { status: res.status, body: (await res.json()) as TokenAnswer['body'] }
  }

  const submitCode = async (browser: Browser, flow: string, code: string): Promise<Response> => {
    const res = await browser.fetch(`${config.api}/api/v1/device-flows/${encodeURIComponent(flow)}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ user_code: code }),
    })
    await conform('POST', '/api/v1/device-flows/{flow}', res.clone())
    return res
  }

  /** Opens the verification URI and follows to the login UI's device page. */
  const openDevicePage = async (browser: Browser, verificationUri: string): Promise<URL> =>
    provider.openLoginScreen(browser, verificationUri)

  beforeAll(async () => {
    const res = await provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: `conformance-device-${randomToken(4)}`,
        public: true,
        redirect_uris: [],
        grant_types: [DEVICE_GRANT, 'refresh_token'],
        scopes: ['openid', 'offline_access'],
      }),
    })
    await conform('POST', '/api/v1/clients', res.clone())
    expect(res.status, await res.clone().text()).toBe(201)
    const { client: created } = (await res.json()) as { client: { id: string; client_id: string } }
    client = { id: created.id, clientId: created.client_id }
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    if (client) await provider.deleteClient(client)
  })

  it('is offered', async () => {
    const res = await fetch(`${config.api}/api/v1/capabilities`)
    await conform('GET', '/api/v1/capabilities', res.clone())
    expect(((await res.json()) as { device_authorization: boolean }).device_authorization).toBe(true)
    const disco = (await provider.discovery()) as unknown as { device_authorization_endpoint?: string }
    expect(disco.device_authorization_endpoint).toBeTruthy()
  })

  it('signs the device in once the user enters its code and approves it', async () => {
    const device = await authorizeDevice()
    expect(device.user_code).toMatch(/^[A-Z]{8}$/)
    expect(await poll(device)).toMatchObject({ status: 400, body: { error: 'authorization_pending' } })

    const browser = new Browser()
    const page = await openDevicePage(browser, device.verification_uri)
    expect(page.pathname).toBe('/device')
    const flow = page.searchParams.get('flow')!

    const wrong = await submitCode(browser, flow, 'ZZZZZZZZ')
    expect(wrong.status).toBe(400)
    expect(((await wrong.json()) as { code: string }).code).toBe('user_code_invalid')

    // Typed in lower case and grouped, as people copy a code.
    const typed = `${device.user_code.slice(0, 4)}-${device.user_code.slice(4)}`.toLowerCase()
    const accepted = await submitCode(browser, flow, typed)
    expect(accepted.status, await accepted.clone().text()).toBe(200)
    const { redirect_to } = (await accepted.json()) as { redirect_to: string }

    // The ordinary login screen follows, for the device's client.
    const login = await provider.afterLogin(browser, redirect_to)
    const loginFlow = provider.loginFlowFrom(login)
    const signedIn = await provider.submitPassword(browser, loginFlow, user)
    expect(signedIn.status, await signedIn.clone().text()).toBe(200)
    const afterLogin = ((await signedIn.json()) as { redirect_to: string }).redirect_to

    // Then consent, then the page that tells the user to go back to the device.
    const consentScreen = await provider.afterLogin(browser, afterLogin)
    const consented = await provider.acceptConsent(browser, provider.consentFlowFrom(consentScreen), ['openid', 'offline_access'])
    expect(consented.status, await consented.clone().text()).toBe(200)
    let next = await browser.redirectFrom(((await consented.json()) as { redirect_to: string }).redirect_to)
    for (let hops = 0; hops < 5 && !next.pathname.endsWith('/device/done'); hops++) {
      next = await browser.redirectFrom(next.toString())
    }
    expect(next.pathname).toBe('/device/done')

    const tokens = await poll(device)
    expect(tokens.status, JSON.stringify(tokens.body)).toBe(200)
    expect(tokens.body.refresh_token).toBeTruthy()
    const idToken = claimsOf(tokens.body.id_token!)
    expect(idToken.aud).toContain(client.clientId)
    expect(idToken.tenant_id).toBe(await provider.tenantId())

    // The device code is spent.
    expect((await poll(device)).status).toBe(400)
  })

  it('carries the code to the device page when the user opens the complete verification URI', async () => {
    const device = await authorizeDevice()
    expect(device.verification_uri_complete).toBeTruthy()
    const page = await openDevicePage(new Browser(), device.verification_uri_complete!)
    expect(page.pathname).toBe('/device')
    expect(page.searchParams.get('flow')).toBeTruthy()
    expect(page.searchParams.get('user_code')).toBe(device.user_code)
  })

  it('refuses an empty code without asking the authorization server', async () => {
    const device = await authorizeDevice()
    const browser = new Browser()
    const flow = (await openDevicePage(browser, device.verification_uri)).searchParams.get('flow')!
    const res = await submitCode(browser, flow, '  ')
    expect(res.status).toBe(400)
    expect(((await res.json()) as { code: string }).code).toBe('user_code_required')
  })
})
