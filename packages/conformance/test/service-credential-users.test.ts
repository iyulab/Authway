import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

interface User {
  id: string
  email: string
  active?: boolean
}

// A product answering its own users — finding one by email, suspending them,
// reading what happened to them — does it from its backend with a service
// credential of its tenant, not with the deployment's admin key. The
// credential reaches its own tenant only, and what its scopes grant.
describe('a service credential administers its tenant’s users', () => {
  let tenant: string
  let user: TestUser
  let sub: string
  const issued: string[] = []
  let support: string
  let readOnly: string

  async function credential(scopes: string[]): Promise<string> {
    const res = await provider.admin(`/api/v1/tenants/${tenant}/service-clients`, {
      method: 'POST',
      body: JSON.stringify({ name: `conformance-support-${randomToken(4)}`, scopes }),
    })
    await conform('POST', '/api/v1/tenants/{id}/service-clients', res.clone())
    expect(res.status, await res.clone().text()).toBe(201)
    const { service_client: svc, credentials } = (await res.json()) as {
      service_client: { id: string }
      credentials: { client_id: string; client_secret: string }
    }
    issued.push(svc.id)
    const { token_endpoint } = await provider.discovery()
    const granted = await fetch(token_endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ grant_type: 'client_credentials', client_id: credentials.client_id, client_secret: credentials.client_secret }),
    })
    expect(granted.status).toBe(200)
    return ((await granted.json()) as { access_token: string }).access_token
  }

  function as(token: string, path: string, init: RequestInit & { tenant?: string } = {}): Promise<Response> {
    const headers = new Headers(init.headers)
    headers.set('Authorization', `Bearer ${token}`)
    if (init.tenant) headers.set('X-Tenant-ID', init.tenant)
    if (init.body) headers.set('Content-Type', 'application/json')
    return fetch(`${config.api}${path}`, { ...init, headers })
  }

  beforeAll(async () => {
    tenant = await provider.tenantId()
    user = await provider.inviteAndAccept()
    support = await credential(['admin.users:read', 'admin.users:write', 'admin.audit:read'])
    readOnly = await credential(['admin.users:read'])
  })

  afterAll(async () => {
    for (const id of issued) await provider.admin(`/api/v1/tenants/${tenant}/service-clients/${id}`, { method: 'DELETE' })
    if (sub) await provider.deleteUser(sub)
  })

  it('finds a user by email in any case, and lists and reads its tenant’s users', async () => {
    const found = await as(support, '/api/v1/users/search', { method: 'POST', body: JSON.stringify({ email: user.email.toUpperCase() }) })
    await conform('POST', '/api/v1/users/search', found.clone())
    expect(found.status, await found.clone().text()).toBe(200)
    const { users } = (await found.json()) as { users: User[] }
    expect(users.map((u) => u.email)).toEqual([user.email])
    sub = users[0]!.id

    const none = await as(support, '/api/v1/users/search', { method: 'POST', body: JSON.stringify({ email: `nobody-${user.email}` }) })
    expect(((await none.json()) as { users: User[] }).users).toEqual([])
    const malformed = await as(support, '/api/v1/users/search', { method: 'POST', body: JSON.stringify({ email: 'not an address' }) })
    await conform('POST', '/api/v1/users/search', malformed.clone())
    expect(malformed.status).toBe(400)

    const listed = await as(readOnly, '/api/v1/users?limit=100')
    await conform('GET', '/api/v1/users', listed.clone())
    expect(listed.status).toBe(200)
    const one = await as(readOnly, `/api/v1/users/${sub}`)
    await conform('GET', '/api/v1/users/{id}', one.clone())
    expect(((await one.json()) as { user: User }).user.email).toBe(user.email)
  })

  it('suspends and reactivates a user, and reads what was recorded', async () => {
    const suspended = await as(support, `/api/v1/users/${sub}`, { method: 'PUT', body: JSON.stringify({ active: false }) })
    await conform('PUT', '/api/v1/users/{id}', suspended.clone())
    expect(suspended.status, await suspended.clone().text()).toBe(200)
    expect(((await suspended.json()) as { user: User }).user.active).toBe(false)
    const back = await as(support, `/api/v1/users/${sub}`, { method: 'PUT', body: JSON.stringify({ active: true }) })
    expect(back.status).toBe(200)

    // An entry is written just after the change it records.
    let actions: string[] = []
    for (let i = 0; i < 20 && !actions.includes('user.reactivated'); i++) {
      const logs = await as(support, `/api/v1/audit/logs?resource_id=${sub}`)
      await conform('GET', '/api/v1/audit/logs', logs.clone())
      expect(logs.status).toBe(200)
      actions = ((await logs.json()) as { logs: { action: string }[] }).logs.map((l) => l.action)
      if (!actions.includes('user.reactivated')) await new Promise((r) => setTimeout(r, 250))
    }
    expect(actions).toEqual(expect.arrayContaining(['user.suspended', 'user.reactivated']))
    const activity = await as(support, `/api/v1/audit/users/${sub}/activity`)
    await conform('GET', '/api/v1/audit/users/{userId}/activity', activity.clone())
    expect(activity.status).toBe(200)
  })

  it('reaches no other tenant, and only what its scopes grant', async () => {
    const elsewhere = await as(support, '/api/v1/users', { tenant: crypto.randomUUID() })
    await conform('GET', '/api/v1/users', elsewhere.clone())
    expect(elsewhere.status).toBe(404)

    const unscoped = await as(readOnly, `/api/v1/users/${sub}`, { method: 'PUT', body: JSON.stringify({ active: false }) })
    await conform('PUT', '/api/v1/users/{id}', unscoped.clone())
    expect(unscoped.status).toBe(403)
    const noAudit = await as(readOnly, '/api/v1/audit/logs')
    expect(noAudit.status).toBe(403)

    // Deleting an account and purging the log stay with the admin API.
    const deleted = await as(support, `/api/v1/users/${sub}`, { method: 'DELETE' })
    await conform('DELETE', '/api/v1/users/{id}', deleted.clone())
    expect(deleted.status).toBe(401)
    const purged = await as(support, '/api/v1/audit/logs/purge', { method: 'DELETE' })
    expect(purged.status).toBe(401)
  })
})
