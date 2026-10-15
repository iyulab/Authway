import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { Provider, type TestClient } from '../src/provider.js'

const provider = new Provider(loadConfig())

type ClientBody = { client: Record<string, unknown> & { id: string } }

describe('admin API: managing clients', () => {
  let publicClient: TestClient
  let confidentialClient: TestClient

  beforeAll(async () => {
    publicClient = await provider.createPublicClient()
    confidentialClient = await provider.createConfidentialClient()
  })

  afterAll(async () => {
    if (publicClient) await provider.deleteClient(publicClient)
    if (confidentialClient) await provider.deleteClient(confidentialClient)
  })

  const read = async (client: TestClient): Promise<ClientBody['client']> => {
    const res = await provider.admin(`/api/v1/clients/${client.id}`)
    await conform('GET', '/api/v1/clients/{id}', res)
    expect(res.status).toBe(200)
    return ((await res.json()) as ClientBody).client
  }

  it('refuses a missing or wrong credential with 401 and a code', async () => {
    const url = `${provider.config.api}/api/v1/clients`
    const attempts: Record<string, string>[] = [{}, { Authorization: 'Bearer not-the-admin-key' }]
    for (const headers of attempts) {
      const res = await fetch(url, { headers })
      await conform('GET', '/api/v1/clients', res)
      expect(res.status).toBe(401)
      expect(((await res.json()) as { code: string }).code).toBe('unauthorized')
    }
  })

  it("lists a tenant's clients", async () => {
    const tenant = await provider.tenantId()
    const res = await provider.admin(`/api/v1/clients?tenant_id=${tenant}&limit=100`)
    await conform('GET', '/api/v1/clients', res)
    expect(res.status).toBe(200)
    const { clients, total } = (await res.json()) as { clients: { id: string }[]; total: number }
    expect(total).toBeGreaterThanOrEqual(2)
    expect(clients.map((c) => c.id)).toContain(publicClient.id)
  })

  // An editor sends back what it read. Anything an update accepts must come
  // back from a read, or saving an untouched form resets it.
  it('returns what an update stored, and keeps what the update did not send', async () => {
    const before = await read(publicClient)

    const res = await provider.admin(`/api/v1/clients/${publicClient.id}`, {
      method: 'PUT',
      body: JSON.stringify({ name: `${before.name}-renamed`, skip_consent: true, skip_logout_consent: true }),
    })
    await conform('PUT', '/api/v1/clients/{id}', res)
    expect(res.status).toBe(200)

    const after = await read(publicClient)
    expect(after.name).toBe(`${before.name}-renamed`)
    expect(after.skip_consent).toBe(true)
    expect(after.skip_logout_consent).toBe(true)
    expect(after.redirect_uris).toEqual(before.redirect_uris)
    expect(after.allowed_origins).toEqual(before.allowed_origins)
  })

  it('refuses an update it cannot apply with 400 and a code', async () => {
    const res = await provider.admin(`/api/v1/clients/${publicClient.id}`, {
      method: 'PUT',
      body: JSON.stringify({ redirect_uris: ['not a url'] }),
    })
    await conform('PUT', '/api/v1/clients/{id}', res)
    expect(res.status).toBe(400)
  })

  // A client nobody can sign in to is not a configuration; an empty list of
  // sign-in methods is refused rather than read as some default.
  it('refuses a client with no way to sign in', async () => {
    const res = await provider.admin(`/api/v1/clients/${publicClient.id}`, {
      method: 'PUT',
      body: JSON.stringify({ enabled_auth_providers: [] }),
    })
    await conform('PUT', '/api/v1/clients/{id}', res.clone())
    expect(res.status).toBe(400)
    const refusal = (await res.json()) as { code: string; field: string }
    expect(refusal.code).toBe('invalid_enabled_auth_providers')
    expect(refusal.field).toBe('enabled_auth_providers')
  })

  // A page holds at most 100 clients: asking for more gets 100, not a silent default.
  it('answers a page of at most 100, newest first', async () => {
    const res = await provider.admin(`/api/v1/clients?limit=500`)
    await conform('GET', '/api/v1/clients', res.clone())
    const page = (await res.json()) as { limit: number; clients: { created_at: string }[] }
    expect(page.limit).toBe(100)
    const times = page.clients.map((c) => Date.parse(c.created_at))
    expect(times).toEqual([...times].sort((a, b) => b - a))
  })

  it('answers 404 with a code for a client that does not exist', async () => {
    const res = await provider.admin(`/api/v1/clients/${crypto.randomUUID()}`)
    await conform('GET', '/api/v1/clients/{id}', res)
    expect(res.status).toBe(404)
    expect(((await res.json()) as { code: string }).code).toBe('not_found')
  })

  it("regenerates a confidential client's secret", async () => {
    const res = await provider.admin(`/api/v1/clients/${confidentialClient.id}/regenerate-secret`, { method: 'POST' })
    await conform('POST', '/api/v1/clients/{id}/regenerate-secret', res)
    expect(res.status).toBe(200)
    const { credentials } = (await res.json()) as { credentials: { client_id: string } }
    expect(credentials.client_id).toBe(confidentialClient.clientId)
  })

  it('refuses to regenerate a secret for a public client, which has none', async () => {
    const res = await provider.admin(`/api/v1/clients/${publicClient.id}/regenerate-secret`, { method: 'POST' })
    await conform('POST', '/api/v1/clients/{id}/regenerate-secret', res)
    expect(res.status).toBe(400)
    expect(((await res.json()) as { code: string }).code).toBe('public_client_has_no_secret')
  })
})
