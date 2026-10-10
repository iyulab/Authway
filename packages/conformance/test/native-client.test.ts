import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Browser } from '../src/browser.js'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { createPkce, randomToken } from '../src/pkce.js'
import { Provider, type TestClient } from '../src/provider.js'

const provider = new Provider(loadConfig())

// A desktop or command-line app signs the user in with the system browser and
// listens on a loopback port the operating system picks at that moment
// (RFC 8252 §7.3), or is called back on a URI scheme of its own (§7.1). It has
// no browser origin and cannot know its port when it is registered.
describe('native application clients', () => {
  const REGISTERED = 'http://127.0.0.1/conformance-native'
  let client: TestClient

  const register = async (redirectUris: string[]): Promise<Response> =>
    provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: `conformance-native-${randomToken(4)}`,
        public: true,
        redirect_uris: redirectUris,
        grant_types: ['authorization_code', 'refresh_token'],
        scopes: ['openid'],
      }),
    })

  const authorizeUrl = async (redirectUri: string): Promise<string> => {
    const authorize = new URL((await provider.discovery()).authorization_endpoint)
    authorize.search = new URLSearchParams({
      client_id: client.clientId,
      response_type: 'code',
      redirect_uri: redirectUri,
      scope: 'openid',
      state: randomToken(),
      code_challenge: createPkce().challenge,
      code_challenge_method: 'S256',
    }).toString()
    return authorize.toString()
  }

  beforeAll(async () => {
    const res = await register([REGISTERED])
    await conform('POST', '/api/v1/clients', res.clone())
    expect(res.status, await res.clone().text()).toBe(201)
    const { client: created } = (await res.json()) as { client: { id: string; client_id: string } }
    client = { id: created.id, clientId: created.client_id }
  })

  afterAll(async () => {
    if (client) await provider.deleteClient(client)
  })

  it('registers a loopback redirect without any allowed origin', () => {
    expect(client.clientId).toBeTruthy()
  })

  it('registers a private-use scheme redirect', async () => {
    const res = await register(['com.example.conformance:/oauth2redirect'])
    await conform('POST', '/api/v1/clients', res.clone())
    expect(res.status, await res.clone().text()).toBe(201)
    const { client: created } = (await res.json()) as { client: { id: string; client_id: string } }
    await provider.deleteClient({ id: created.id, clientId: created.client_id })
  })

  it('opens the login screen for the registered loopback redirect on any port', async () => {
    const port = 49152 + Math.floor(Math.random() * 16000)
    const screen = await provider.openLoginScreen(
      new Browser(),
      await authorizeUrl(`http://127.0.0.1:${port}/conformance-native`),
    )
    expect(screen.searchParams.get('flow')).toBeTruthy()
  })

  it('does not open the login screen for another path on the loopback address', async () => {
    const reached = await provider
      .openLoginScreen(new Browser(), await authorizeUrl('http://127.0.0.1:50000/somewhere-else'))
      .then((u) => u.searchParams.has('flow'))
      .catch(() => false)
    expect(reached).toBe(false)
  })
})
