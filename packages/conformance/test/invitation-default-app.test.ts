import { afterAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { waitForLink } from '../src/mail.js'
import { randomToken } from '../src/pkce.js'
import { REDIRECT_URI, Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

const asTenant = (tenantId: string, path: string, init: RequestInit = {}) =>
  fetch(`${provider.config.api}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${provider.config.adminKey}`,
      'X-Tenant-ID': tenantId,
      ...(init.body ? { 'Content-Type': 'application/json' } : {}),
    },
  })

// An invitation that names no application still leads the person somewhere
// when the tenant has exactly one application to lead to — and nowhere
// otherwise: which application is meant is not guessed.
// The scenarios make tenants with exactly the applications they need: an installation with one tenant has no tenant to spare.
const { multi_tenant: multiTenant } = await provider.capabilities()

describe.skipIf(!multiTenant)('an invitation that names no application', () => {
  const tenants: string[] = []

  afterAll(async () => {
    for (const id of tenants) await provider.admin(`/api/v1/tenants/${id}`, { method: 'DELETE' })
  })

  const newTenant = async (): Promise<string> => {
    const res = await provider.admin('/api/v1/tenants', {
      method: 'POST',
      body: JSON.stringify({ name: 'Conformance tenant', slug: `conformance-${randomToken(4).toLowerCase()}` }),
    })
    expect(res.status).toBe(201)
    const id = ((await res.json()) as { id: string }).id
    tenants.push(id)
    return id
  }

  const addApp = async (tenantId: string, fields: Record<string, unknown>): Promise<string> => {
    const res = await asTenant(tenantId, '/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: tenantId,
        name: `conformance-app-${randomToken(4)}`,
        public: true,
        redirect_uris: [REDIRECT_URI],
        allowed_origins: ['http://localhost:9999'],
        grant_types: ['authorization_code'],
        scopes: ['openid', 'email'],
        ...fields,
      }),
    })
    expect(res.status, await res.clone().text()).toBe(201)
    return ((await res.json()) as { client: { client_id: string } }).client.client_id
  }

  const lookUpAnInvitation = async (tenantId: string) => {
    const email = `conformance-${randomToken(6)}@example.test`
    const invited = await asTenant(tenantId, '/api/v1/invitations', { method: 'POST', body: JSON.stringify({ email }) })
    expect(invited.status).toBe(201)
    const token = (await waitForLink(provider.config.mailApi, email, '/invitation/accept')).searchParams.get('token')!
    const res = await fetch(`${provider.config.api}/api/v1/invitations/token/${encodeURIComponent(token)}`)
    await conform('GET', '/api/v1/invitations/token/{token}', res.clone())
    expect(res.status).toBe(200)
    return ((await res.json()) as { invitation: { client?: { client_id: string; client_uri?: string } } }).invitation
  }

  it("leads to the tenant's only application that has an https address", async () => {
    const tenant = await newTenant()
    const clientId = await addApp(tenant, { client_uri: 'https://only.example.test/' })
    const invitation = await lookUpAnInvitation(tenant)
    expect(invitation.client).toMatchObject({ client_id: clientId, client_uri: 'https://only.example.test/' })
  })

  it('leads nowhere when the tenant has no application with an address, or more than one', async () => {
    const none = await newTenant()
    await addApp(none, {})
    expect((await lookUpAnInvitation(none)).client).toBeUndefined()

    const two = await newTenant()
    await addApp(two, { client_uri: 'https://a.example.test/' })
    await addApp(two, { initiate_login_uri: 'https://b.example.test/login' })
    expect((await lookUpAnInvitation(two)).client).toBeUndefined()
  })
})
