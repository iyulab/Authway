import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const admin = new Provider(config)

const WITH_REFRESH = ['openid', 'profile', 'email', 'offline_access']

// Turning a tenant off turns off everything in it: no one signs in to its
// applications, they get no tokens, and the tokens they hold end. Turned on
// again, its applications work as before.
describe('turning a tenant off', () => {
  let tenantId: string
  let provider: Provider
  let app: TestClient
  let service: TestClient & { secret: string }
  let user: TestUser
  let sub: string | undefined

  beforeAll(async () => {
    const res = await admin.admin('/api/v1/tenants', {
      method: 'POST',
      body: JSON.stringify({ name: 'Conformance tenant off', slug: `conformance-off-${randomToken(4).toLowerCase()}` }),
    })
    expect(res.status, await res.clone().text()).toBe(201)
    tenantId = ((await res.json()) as { id: string }).id
    provider = new Provider({ ...config, tenantId })
    app = await provider.createPublicClient({ scopes: WITH_REFRESH })
    const created = await provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({ tenant_id: tenantId, name: `conformance-off-${randomToken(4)}`, public: false, grant_types: ['client_credentials'], scopes: ['openid'] }),
    })
    expect(created.status, await created.clone().text()).toBe(201)
    const body = (await created.json()) as { client: { id: string; client_id: string }; credentials: { client_secret: string } }
    service = { id: body.client.id, clientId: body.client.client_id, secret: body.credentials.client_secret }
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    if (!provider) return
    await provider.admin(`/api/v1/tenants/${tenantId}`, { method: 'PUT', body: JSON.stringify({ active: true }) })
    if (sub) await provider.deleteUser(sub)
    if (app) await provider.deleteClient(app)
    if (service) await provider.deleteClient(service)
    await admin.admin(`/api/v1/tenants/${tenantId}`, { method: 'DELETE' })
  })

  const setActive = async (active: boolean) => {
    const res = await admin.admin(`/api/v1/tenants/${tenantId}`, { method: 'PUT', body: JSON.stringify({ active }) })
    await conform('PUT', '/api/v1/tenants/{id}', res)
    expect(res.status, await res.clone().text()).toBe(200)
  }

  const clientCredentials = async () => {
    const { token_endpoint } = await provider.discovery()
    return fetch(token_endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ grant_type: 'client_credentials', client_id: service.clientId, client_secret: service.secret }),
    })
  }

  it("stops its applications' sign-ins and tokens; turned on, they work again", async () => {
    const before = await provider.login(app, user, { scopes: WITH_REFRESH })
    if (before.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(before)}`)
    sub = before.sub
    const underway = await provider.startLogin(app, WITH_REFRESH)

    await setActive(false)

    const stop = await provider.authorize(app, WITH_REFRESH)
    expect(stop.searchParams.has('flow')).toBe(false)
    expect(stop.searchParams.get('error')).toBeTruthy()
    expect((await provider.continueLogin(underway, user)).kind).toBe('rejected')
    expect((await provider.refresh(app, before.tokens.refresh_token!)).status).toBeGreaterThanOrEqual(400)
    expect((await clientCredentials()).status).toBeGreaterThanOrEqual(400)
    expect((await provider.userinfo(before.tokens.access_token)).status).toBe(401)

    await setActive(true)
    expect((await provider.login(app, user, { scopes: WITH_REFRESH })).kind).toBe('code')
    expect((await clientCredentials()).status).toBe(200)
  })
})
