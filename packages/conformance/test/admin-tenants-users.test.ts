import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

type Tenant = { id: string; name: string; slug: string; active?: boolean; settings?: { signup_mode?: string } }

const code = async (res: Response) => ((await res.json()) as { code: string }).code

describe('admin API: managing tenants', () => {
  const slug = `conformance-${randomToken(4).toLowerCase()}`
  let tenant: Tenant | undefined

  afterAll(async () => {
    // Normally removed by the last scenario; this covers a failure before it.
    if (tenant) await provider.admin(`/api/v1/tenants/${tenant.id}`, { method: 'DELETE' })
  })

  it('creates a tenant and refuses a second one with the same slug', async () => {
    const create = () =>
      provider.admin('/api/v1/tenants', { method: 'POST', body: JSON.stringify({ name: 'Conformance tenant', slug }) })

    const res = await create()
    await conform('POST', '/api/v1/tenants', res)
    expect(res.status).toBe(201)
    tenant = (await res.json()) as Tenant
    expect(tenant.slug).toBe(slug)

    const again = await create()
    await conform('POST', '/api/v1/tenants', again)
    expect(again.status).toBe(409)
    expect(await code(again)).toBe('tenant_slug_taken')
  })

  it('returns what an update stored', async () => {
    const res = await provider.admin(`/api/v1/tenants/${tenant!.id}`, {
      method: 'PUT',
      body: JSON.stringify({ name: 'Conformance tenant renamed', settings: { signup_mode: 'open' } }),
    })
    await conform('PUT', '/api/v1/tenants/{id}', res)
    expect(res.status).toBe(200)

    const read = await provider.admin(`/api/v1/tenants/${tenant!.id}`)
    await conform('GET', '/api/v1/tenants/{id}', read)
    const stored = (await read.json()) as Tenant
    expect(stored.name).toBe('Conformance tenant renamed')
    expect(stored.settings?.signup_mode).toBe('open')
  })

  it('deletes an empty tenant, after which it is not found', async () => {
    const res = await provider.admin(`/api/v1/tenants/${tenant!.id}`, { method: 'DELETE' })
    await conform('DELETE', '/api/v1/tenants/{id}', res)
    expect(res.status).toBe(204)

    const gone = await provider.admin(`/api/v1/tenants/${tenant!.id}`)
    await conform('GET', '/api/v1/tenants/{id}', gone)
    expect(gone.status).toBe(404)
    expect(await code(gone)).toBe('not_found')
    tenant = undefined
  })

  it('protects the default tenant from being deactivated or deleted', async () => {
    const list = await provider.admin('/api/v1/tenants')
    const fallback = ((await list.json()) as Tenant[]).find((t) => t.slug === 'default')
    if (!fallback) return // a provider without a designated default tenant has nothing to protect

    const deactivate = await provider.admin(`/api/v1/tenants/${fallback.id}`, {
      method: 'PUT',
      body: JSON.stringify({ active: false }),
    })
    await conform('PUT', '/api/v1/tenants/{id}', deactivate)
    expect(deactivate.status).toBe(403)
    expect(await code(deactivate)).toBe('default_tenant_protected')

    const remove = await provider.admin(`/api/v1/tenants/${fallback.id}`, { method: 'DELETE' })
    await conform('DELETE', '/api/v1/tenants/{id}', remove)
    expect(remove.status).toBe(403)
    expect(await code(remove)).toBe('default_tenant_protected')
  })
})

describe('admin API: managing users', () => {
  let userId: string

  beforeAll(async () => {
    const user = await provider.inviteAndAccept()
    const res = await provider.admin(`/api/v1/users?tenant_id=${await provider.tenantId()}&limit=100`)
    await conform('GET', '/api/v1/users', res)
    const { users } = (await res.json()) as { users: { id: string; email: string }[] }
    const found = users.find((u) => u.email === user.email)
    if (!found) throw new Error(`The invited user ${user.email} is not in the tenant's user list`)
    userId = found.id
  })

  afterAll(async () => {
    if (userId) await provider.deleteUser(userId)
  })

  it('reads and updates a user', async () => {
    const res = await provider.admin(`/api/v1/users/${userId}`, {
      method: 'PUT',
      body: JSON.stringify({ name: 'Conformance Renamed' }),
    })
    await conform('PUT', '/api/v1/users/{id}', res)
    expect(res.status).toBe(200)

    const read = await provider.admin(`/api/v1/users/${userId}`)
    await conform('GET', '/api/v1/users/{id}', read)
    expect(((await read.json()) as { user: { name: string } }).user.name).toBe('Conformance Renamed')
  })

  it('answers 404 with a code for a user that does not exist', async () => {
    const res = await provider.admin(`/api/v1/users/${crypto.randomUUID()}`)
    await conform('GET', '/api/v1/users/{id}', res)
    expect(res.status).toBe(404)
    expect(await code(res)).toBe('not_found')
  })
})
