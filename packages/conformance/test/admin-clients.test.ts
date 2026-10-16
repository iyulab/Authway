import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
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

  // Only grants the provider serves are kept: implicit cannot be used with a
  // client registered for the code response type, the password grant is not
  // offered (RFC 9700), and an unknown name would be configuration that does nothing.
  it('refuses a grant the provider does not offer', async () => {
    for (const grant of ['implicit', 'password', 'not-a-grant']) {
      const res = await provider.admin(`/api/v1/clients/${confidentialClient.id}`, {
        method: 'PUT',
        body: JSON.stringify({ grant_types: ['authorization_code', grant] }),
      })
      await conform('PUT', '/api/v1/clients/{id}', res.clone())
      expect(res.status, grant).toBe(400)
      const refusal = (await res.json()) as { code: string; field: string }
      expect(refusal.code, grant).toBe('unsupported_grant_type')
      expect(refusal.field).toBe('grant_types')
    }
    expect((await read(confidentialClient)).grant_types).not.toContain('implicit')
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

// A client_id the caller chooses is the client's name at the authorization
// server: one that is taken is refused rather than answered with the client
// already under it, and one that was deleted can be registered again.
describe('admin API: a chosen client_id', () => {
  const clientId = `conformance-chosen-${randomToken(6)}`
  const register = async (secret: string) =>
    provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: clientId,
        client_id: clientId,
        client_secret: secret,
        grant_types: ['client_credentials'],
        scopes: ['openid'],
      }),
    })
  const token = async (secret: string) => {
    const { token_endpoint } = await provider.discovery()
    return fetch(token_endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ grant_type: 'client_credentials', client_id: clientId, client_secret: secret }),
    })
  }
  let id: string | undefined

  afterAll(async () => {
    if (id) await provider.admin(`/api/v1/clients/${id}`, { method: 'DELETE' })
  })

  it('is refused while it is registered, and can be registered again once deleted', async () => {
    const first = `first-${randomToken(24)}`
    const created = await register(first)
    await conform('POST', '/api/v1/clients', created)
    expect(created.status).toBe(201)
    id = ((await created.json()) as ClientBody).client.id
    expect((await token(first)).status).toBe(200)

    const taken = await register(`second-${randomToken(24)}`)
    await conform('POST', '/api/v1/clients', taken)
    expect(taken.status).toBe(409)
    expect(await taken.json()).toMatchObject({ code: 'client_id_taken', field: 'client_id' })
    expect((await token(first)).status).toBe(200)

    expect((await provider.admin(`/api/v1/clients/${id}`, { method: 'DELETE' })).status).toBe(200)
    const again = `again-${randomToken(24)}`
    const recreated = await register(again)
    expect(recreated.status).toBe(201)
    id = ((await recreated.json()) as ClientBody).client.id
    expect((await token(again)).status).toBe(200)
    expect((await token(first)).status).toBe(401)
  })
})

// The application's own sign-in provider apps follow one rule for all four
// providers: an enabled provider has every credential, judged with what a
// change leaves as it is; credentials of a provider not enabled are kept.
describe("admin API: a client's own provider credentials", () => {
  let id: string | undefined

  afterAll(async () => {
    if (id) await provider.admin(`/api/v1/clients/${id}`, { method: 'DELETE' })
  })

  const refusedOn = async (res: Response, field: string) => {
    expect(res.status).toBe(400)
    expect(await res.json()).toMatchObject({ code: 'incomplete_provider_credentials', field })
  }

  it('are complete for an enabled provider, when created and when changed', async () => {
    const register = async (extra: Record<string, unknown>) =>
      provider.admin('/api/v1/clients', {
        method: 'POST',
        body: JSON.stringify({
          tenant_id: await provider.tenantId(),
          name: `conformance-providers-${randomToken(6)}`,
          grant_types: ['client_credentials'],
          scopes: ['openid'],
          ...extra,
        }),
      })

    const incomplete = await register({ apple_oauth_enabled: true, apple_client_id: 'app', apple_team_id: 'T', apple_key_id: 'K' })
    await conform('POST', '/api/v1/clients', incomplete)
    await refusedOn(incomplete, 'apple_private_key')

    const kept = await register({ microsoft_client_id: 'app', microsoft_client_secret: 'secret' })
    await conform('POST', '/api/v1/clients', kept)
    expect(kept.status).toBe(201)
    id = ((await kept.json()) as ClientBody).client.id

    const change = (body: Record<string, unknown>) =>
      provider.admin(`/api/v1/clients/${id}`, { method: 'PUT', body: JSON.stringify(body) })

    const enabled = await change({ microsoft_oauth_enabled: true })
    await conform('PUT', '/api/v1/clients/{id}', enabled)
    expect(enabled.status).toBe(200)
    expect(((await enabled.json()) as ClientBody).client).toMatchObject({ microsoft_oauth_enabled: true })

    const cleared = await change({ microsoft_client_secret: '' })
    await conform('PUT', '/api/v1/clients/{id}', cleared)
    await refusedOn(cleared, 'microsoft_client_secret')

    await refusedOn(await change({ github_oauth_enabled: true, github_client_id: 'app' }), 'github_client_secret')
  })
})

// A client is registered in the tenant the request is about: the body names
// it, the X-Tenant-ID header names it, or both name the same one.
describe('admin API: the tenant a client is registered in', () => {
  const body = (extra: Record<string, unknown>) =>
    JSON.stringify({ name: `conformance-${randomToken(4)}`, grant_types: ['client_credentials'], scopes: ['openid'], ...extra })

  it('takes the tenant from the header when the body names none, and refuses two that disagree', async () => {
    const tenantId = await provider.tenantId()
    const fromHeader = await provider.admin('/api/v1/clients', { method: 'POST', body: body({}) })
    await conform('POST', '/api/v1/clients', fromHeader)
    expect(fromHeader.status, await fromHeader.clone().text()).toBe(201)
    const created = ((await fromHeader.json()) as ClientBody).client
    expect(created.tenant_id).toBe(tenantId)
    await provider.admin(`/api/v1/clients/${created.id}`, { method: 'DELETE' })

    const disagree = await provider.admin('/api/v1/clients', {
      method: 'POST',
      body: body({ tenant_id: '00000000-0000-4000-8000-000000000000' }),
    })
    await conform('POST', '/api/v1/clients', disagree)
    expect(disagree.status).toBe(400)
    expect(await disagree.json()).toMatchObject({ code: 'invalid_request', field: 'tenant_id' })
  })
})
