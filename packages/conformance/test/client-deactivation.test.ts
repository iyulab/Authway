import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

const WITH_REFRESH = ['openid', 'profile', 'email', 'offline_access']

// An administrator turns a client off to stop it being used without deleting
// it: no one signs in to it and it gets no tokens, by any grant, and the
// tokens it holds stop working. Turned on again, it signs users in as before;
// the sessions it had stay ended.
describe('turning a client off', () => {
  let app: TestClient
  let service: TestClient & { secret: string }
  let resourceServer: TestClient & { secret: string }
  let user: TestUser
  let sub: string | undefined

  const machineClient = async (): Promise<TestClient & { secret: string }> => {
    const res = await provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: `conformance-off-${randomToken(4)}`,
        public: false,
        grant_types: ['client_credentials'],
        scopes: ['openid'],
      }),
    })
    expect(res.status, await res.clone().text()).toBe(201)
    const created = (await res.json()) as { client: { id: string; client_id: string }; credentials: { client_secret: string } }
    return { id: created.client.id, clientId: created.client.client_id, secret: created.credentials.client_secret }
  }

  beforeAll(async () => {
    app = await provider.createPublicClient({ scopes: WITH_REFRESH })
    service = await machineClient()
    resourceServer = await machineClient()
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    if (app) await provider.deleteClient(app)
    if (service) await provider.deleteClient(service)
    if (resourceServer) await provider.deleteClient(resourceServer)
  })

  const setActive = async (client: TestClient, active: boolean) => {
    const res = await provider.admin(`/api/v1/clients/${client.id}`, { method: 'PUT', body: JSON.stringify({ active }) })
    await conform('PUT', '/api/v1/clients/{id}', res)
    expect(res.status, await res.clone().text()).toBe(200)
    expect(((await res.json()) as { client: { active: boolean } }).client.active).toBe(active)
  }

  const clientCredentials = async () => {
    const { token_endpoint } = await provider.discovery()
    return fetch(token_endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ grant_type: 'client_credentials', client_id: service.clientId, client_secret: service.secret }),
    })
  }

  it('signs no one in, issues nothing, and ends the tokens it holds; turned on, it works again', async () => {
    const before = await provider.login(app, user, { scopes: WITH_REFRESH })
    if (before.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(before)}`)
    sub = before.sub
    expect(before.tokens.refresh_token).toBeTruthy()
    const machineToken = ((await (await clientCredentials()).json()) as { access_token: string }).access_token
    // A sign-in already under way when the client is turned off.
    const underway = await provider.startLogin(app, WITH_REFRESH)

    await setActive(app, false)
    await setActive(service, false)

    // A new sign-in is ended at once: the application is handed an error, not a form.
    const stop = await provider.authorize(app, WITH_REFRESH)
    expect(stop.searchParams.has('flow')).toBe(false)
    expect(stop.searchParams.get('error')).toBeTruthy()
    // One already on the login screen does not get through.
    expect((await provider.continueLogin(underway, user)).kind).toBe('rejected')

    // No grant issues a token.
    expect((await provider.refresh(app, before.tokens.refresh_token!)).status).toBeGreaterThanOrEqual(400)
    expect((await clientCredentials()).status).toBeGreaterThanOrEqual(400)

    // The access tokens it held are no longer accepted.
    expect((await provider.userinfo(before.tokens.access_token)).status).toBe(401)
    const introspected = await fetch(`${config.api}/api/v1/oauth2/introspect`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        Authorization: `Basic ${Buffer.from(`${encodeURIComponent(resourceServer.clientId)}:${encodeURIComponent(resourceServer.secret)}`).toString('base64')}`,
      },
      body: new URLSearchParams({ token: machineToken }),
    })
    expect(((await introspected.json()) as { active: boolean }).active).toBe(false)

    await setActive(app, true)
    await setActive(service, true)
    const after = await provider.login(app, user, { scopes: WITH_REFRESH })
    expect(after.kind).toBe('code')
    // Turning it on does not bring back the sessions it had.
    expect((await provider.refresh(app, before.tokens.refresh_token!)).status).toBeGreaterThanOrEqual(400)
    expect((await clientCredentials()).status).toBe(200)
  })
})
