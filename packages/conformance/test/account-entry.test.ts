import { afterAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { createPkce, randomToken } from '../src/pkce.js'
import { Provider, REDIRECT_URI } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

type Client = { id: string; client_id: string; tenant_id: string; system: boolean; redirect_uris: string[] | null; public: boolean }
type Entry = { client_id: string; redirect_uri: string; scope: string; app: { name: string; client_uri?: string } }

/** An admin call for a tenant other than the suite's own. */
const adminFor = (tenant: string, path: string, init: RequestInit = {}) =>
  fetch(`${config.api}${path}`, {
    ...init,
    headers: { Authorization: `Bearer ${config.adminKey}`, 'X-Tenant-ID': tenant, 'Content-Type': 'application/json', ...(init.headers ?? {}) },
  })

const entry = (clientId: string) => fetch(`${config.api}/api/v1/account-entry?client_id=${encodeURIComponent(clientId)}`)

const registerApp = async (tenant: string, name: string): Promise<Client> => {
  const res = await adminFor(tenant, '/api/v1/clients', {
    method: 'POST',
    body: JSON.stringify({
      tenant_id: tenant, name, public: true, redirect_uris: [REDIRECT_URI], allowed_origins: ['http://localhost:9999'],
      grant_types: ['authorization_code'], scopes: ['openid'], client_uri: 'https://app.example/home',
    }),
  })
  expect(res.status, await res.clone().text()).toBe(201)
  return ((await res.json()) as { client: Client }).client
}

// A user reaches the account screen from an application. The screen signs them
// in with a client of their tenant that Authway owns — registered the first
// time, the same one after that — and the administration API cannot change it.
describe('the account screen entry', () => {
  let tenant: string | undefined

  afterAll(async () => {
    if (tenant) await adminFor(tenant, `/api/v1/tenants/${tenant}`, { method: 'DELETE' })
  })

  it('answers with the tenant account client, the same one every time, and the application to go back to', async () => {
    const res = await provider.admin('/api/v1/tenants', {
      method: 'POST',
      body: JSON.stringify({ name: 'Conformance account entry', slug: `conformance-account-${randomToken(4).toLowerCase()}` }),
    })
    expect(res.status).toBe(201)
    tenant = ((await res.json()) as { id: string }).id
    const app = await registerApp(tenant, `conformance-account-app-${randomToken(4)}`)

    const first = await entry(app.client_id)
    await conform('GET', '/api/v1/account-entry', first.clone())
    expect(first.status).toBe(200)
    const answer = (await first.json()) as Entry
    expect(answer.client_id).not.toBe(app.client_id)
    expect(answer.redirect_uri).toMatch(/\/account\/callback$/)
    expect(answer.app).toEqual({ name: expect.stringMatching(/^conformance-account-app-/), client_uri: 'https://app.example/home' })

    const second = (await (await entry(app.client_id)).json()) as Entry
    expect(second.client_id).toBe(answer.client_id)

    const list = await adminFor(tenant, `/api/v1/clients?tenant_id=${tenant}&limit=100`)
    await conform('GET', '/api/v1/clients', list.clone())
    const clients = ((await list.json()) as { clients: Client[] }).clients
    const system = clients.filter((c) => c.system)
    expect(system.map((c) => c.client_id)).toEqual([answer.client_id])
    expect(system[0]).toMatchObject({ public: true, redirect_uris: [answer.redirect_uri] })
    expect(clients.find((c) => c.id === app.id)?.system).toBe(false)
  })

  it('refuses to change, delete or re-key the account client', async () => {
    const clients = ((await (await adminFor(tenant!, `/api/v1/clients?tenant_id=${tenant}&limit=100`)).json()) as { clients: Client[] }).clients
    const system = clients.find((c) => c.system)!

    for (const [method, path, body] of [
      ['PUT', `/api/v1/clients/${system.id}`, JSON.stringify({ name: 'renamed' })],
      ['DELETE', `/api/v1/clients/${system.id}`, undefined],
      ['POST', `/api/v1/clients/${system.id}/regenerate-secret`, undefined],
    ] as const) {
      const res = await adminFor(tenant!, path, { method, body })
      const template = path.endsWith('regenerate-secret') ? '/api/v1/clients/{id}/regenerate-secret' : '/api/v1/clients/{id}'
      await conform(method, template, res.clone())
      expect(res.status, `${method} ${path}`).toBe(409)
      expect(((await res.json()) as { code: string }).code).toBe('system_client')
    }
  })

  it('signs a user in with the account client without a consent screen', async () => {
    const answer = (await (await entry(((await (await adminFor(tenant!, `/api/v1/clients?tenant_id=${tenant}&limit=100`)).json()) as { clients: Client[] }).clients.find((c) => !c.system)!.client_id)).json()) as Entry
    const { challenge } = createPkce()
    const auth = new URL(`${config.issuer}/oauth2/auth`)
    for (const [k, v] of Object.entries({
      client_id: answer.client_id, response_type: 'code', scope: answer.scope, redirect_uri: answer.redirect_uri,
      state: randomToken(8), code_challenge: challenge, code_challenge_method: 'S256',
    })) auth.searchParams.set(k, v)
    // The authorization server knows the client and its redirect: it starts a sign-in (to the login screens) rather than refusing.
    const res = await fetch(auth, { redirect: 'manual' })
    expect(res.status).toBe(302)
    expect(res.headers.get('location') ?? '').toContain('login_challenge=')
  })

  it('refuses an unknown or missing client_id', async () => {
    const unknown = await entry(`nobody-${randomToken(6)}`)
    await conform('GET', '/api/v1/account-entry', unknown.clone())
    expect(unknown.status).toBe(404)
    expect(((await unknown.json()) as { code: string }).code).toBe('unknown_client')

    const missing = await fetch(`${config.api}/api/v1/account-entry`)
    await conform('GET', '/api/v1/account-entry', missing.clone())
    expect(missing.status).toBe(400)
  })

  it('goes with the tenant: the tenant deletes once its own applications are gone, and the authorization server forgets the account client', async () => {
    const clients = ((await (await adminFor(tenant!, `/api/v1/clients?tenant_id=${tenant}&limit=100`)).json()) as { clients: Client[] }).clients
    const system = clients.find((c) => c.system)!
    for (const c of clients.filter((c) => !c.system)) {
      expect((await adminFor(tenant!, `/api/v1/clients/${c.id}`, { method: 'DELETE' })).status).toBe(200)
    }

    const del = await adminFor(tenant!, `/api/v1/tenants/${tenant}`, { method: 'DELETE' })
    expect(del.status, await del.clone().text()).toBe(204)
    tenant = undefined

    const auth = new URL(`${config.issuer}/oauth2/auth`)
    for (const [k, v] of Object.entries({ client_id: system.client_id, response_type: 'code', scope: 'openid', redirect_uri: system.redirect_uris![0], state: randomToken(8) }))
      auth.searchParams.set(k, v)
    const res = await fetch(auth, { redirect: 'manual' })
    expect(res.headers.get('location') ?? '').not.toContain('login_challenge=')
  })
})
