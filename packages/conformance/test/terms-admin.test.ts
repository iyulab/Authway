import { randomUUID } from 'node:crypto'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider } from '../src/provider.js'

const base = new Provider(loadConfig())

type Doc = { kind: string; version: string; url: string; title?: string; published_at?: string }

// A tenant names the documents its users accept at sign-in — terms of
// service, a privacy policy — one current version per kind. These scenarios
// keep them in a tenant of their own: a document in a shared tenant would be
// owed by every other scenario's sign-in.
describe('tenant terms: administration', () => {
  let tenantId: string
  let provider: Provider

  const terms = (path = '', init: RequestInit = {}) => provider.admin(`/api/v1/tenants/${tenantId}/terms${path}`, init)
  const put = (kind: string, body: Record<string, unknown>) => terms(`/${kind}`, { method: 'PUT', body: JSON.stringify(body) })
  const list = async (): Promise<Doc[]> => {
    const res = await terms()
    await conform('GET', '/api/v1/tenants/{id}/terms', res.clone())
    expect(res.status).toBe(200)
    return ((await res.json()) as { terms: Doc[] }).terms
  }

  beforeAll(async () => {
    const caps = await (await fetch(`${base.config.api}/api/v1/capabilities`)).json()
    if (!caps.terms_acceptance) throw new Error('The provider does not offer terms (capabilities.terms_acceptance is false)')
    const slug = `conformance-terms-${randomToken(6).toLowerCase()}`
    const res = await base.admin('/api/v1/tenants', { method: 'POST', body: JSON.stringify({ name: 'Conformance terms', slug }) })
    expect(res.status, await res.clone().text()).toBe(201)
    tenantId = ((await res.json()) as { id: string }).id
    provider = new Provider({ ...base.config, tenantId })
  })

  afterAll(async () => {
    if (tenantId) await base.admin(`/api/v1/tenants/${tenantId}`, { method: 'DELETE' })
  })

  it('asks for nothing until a document is published', async () => {
    expect(await list()).toEqual([])
  })

  it('makes a published version current, one per kind', async () => {
    const first = await put('terms_of_service', { version: '2026-10', url: 'https://example.test/terms/2026-10', title: 'Terms of Service' })
    await conform('PUT', '/api/v1/tenants/{id}/terms/{kind}', first.clone())
    expect(first.status, await first.clone().text()).toBe(201)
    expect(await first.json()).toMatchObject({ kind: 'terms_of_service', version: '2026-10', title: 'Terms of Service' })

    const policy = await put('privacy_policy', { version: '3', url: 'https://example.test/privacy/3' })
    expect(policy.status).toBe(201)

    // The same version again changes only where it is published.
    const moved = await put('terms_of_service', { version: '2026-10', url: 'https://example.test/terms/moved' })
    await conform('PUT', '/api/v1/tenants/{id}/terms/{kind}', moved.clone())
    expect(moved.status).toBe(200)
    expect(((await moved.json()) as Doc).url).toBe('https://example.test/terms/moved')

    // A newer version replaces the current one.
    const next = await put('terms_of_service', { version: '2026-11', url: 'https://example.test/terms/2026-11' })
    expect(next.status).toBe(201)

    const current = await list()
    expect(current.map((d) => `${d.kind}@${d.version}`)).toEqual(['privacy_policy@3', 'terms_of_service@2026-11'])
    expect(current.every((d) => typeof d.published_at === 'string')).toBe(true)
  })

  it('stops asking for a retired kind', async () => {
    const res = await terms('/privacy_policy', { method: 'DELETE' })
    await conform('DELETE', '/api/v1/tenants/{id}/terms/{kind}', res.clone())
    expect(res.status).toBe(204)
    expect((await list()).map((d) => d.kind)).toEqual(['terms_of_service'])

    const again = await terms('/privacy_policy', { method: 'DELETE' })
    await conform('DELETE', '/api/v1/tenants/{id}/terms/{kind}', again.clone())
    expect(again.status).toBe(404)
  })

  it('refuses what is not a document', async () => {
    for (const [kind, body] of [
      ['Terms', { version: '1', url: 'https://example.test/t' }],
      ['terms_of_service', { version: '', url: 'https://example.test/t' }],
      ['terms_of_service', { version: '1', url: 'http://example.test/t' }],
      ['terms_of_service', { version: '1', url: 'terms.html' }],
    ] as const) {
      const res = await put(kind, body)
      await conform('PUT', '/api/v1/tenants/{id}/terms/{kind}', res.clone())
      expect(res.status, `${kind} ${JSON.stringify(body)}`).toBe(400)
    }
  })

  it('answers not_found for a tenant that does not exist, and under another tenant', async () => {
    const missing = await base.admin(`/api/v1/tenants/${randomUUID()}/terms`)
    await conform('GET', '/api/v1/tenants/{id}/terms', missing.clone())
    expect(missing.status).toBe(404)

    const other = await fetch(`${base.config.api}/api/v1/tenants/${tenantId}/terms`, {
      headers: { Authorization: `Bearer ${base.config.adminKey}`, 'X-Tenant-ID': randomUUID() },
    })
    await conform('GET', '/api/v1/tenants/{id}/terms', other.clone())
    expect(other.status).toBe(404)
  })

  it('lists no acceptances for a user who has accepted nothing, and 404 for no user', async () => {
    const user = await provider.inviteAndAccept()
    const users = await provider.admin(`/api/v1/users?limit=100`)
    const sub = ((await users.json()) as { users: { id: string; email: string }[] }).users.find((u) => u.email === user.email)?.id
    expect(sub).toBeTruthy()

    const res = await provider.admin(`/api/v1/users/${sub}/terms-acceptances`)
    await conform('GET', '/api/v1/users/{id}/terms-acceptances', res.clone())
    expect(res.status).toBe(200)
    expect(((await res.json()) as { acceptances: unknown[] }).acceptances).toEqual([])

    const none = await provider.admin(`/api/v1/users/${randomUUID()}/terms-acceptances`)
    await conform('GET', '/api/v1/users/{id}/terms-acceptances', none.clone())
    expect(none.status).toBe(404)
    await provider.deleteUser(sub!)
  })
})
