import { randomUUID } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

const code = async (res: Response) => ((await res.json()) as { code: string }).code

/** An admin call that names a tenant other than the one the resource belongs to. */
const asOtherTenant = (path: string, init: RequestInit = {}) =>
  fetch(`${provider.config.api}${path}`, {
    ...init,
    headers: { Authorization: `Bearer ${provider.config.adminKey}`, 'X-Tenant-ID': randomUUID() },
  })

describe('admin API: a resource is reached only through its own tenant', () => {
  it('answers not_found for a webhook read, changed, tested or deleted under another tenant', async () => {
    const created = await provider.admin('/api/v1/webhooks', {
      method: 'POST',
      body: JSON.stringify({ name: `conformance-${randomToken(6)}`, url: 'https://conformance.invalid/webhook', events: ['user.created'] }),
    })
    expect(created.status).toBe(201)
    const id = ((await created.json()) as { webhook: { id: string } }).webhook.id

    for (const [method, path, template] of [
      ['GET', `/api/v1/webhooks/${id}`, '/api/v1/webhooks/{id}'],
      ['PATCH', `/api/v1/webhooks/${id}`, '/api/v1/webhooks/{id}'],
      ['POST', `/api/v1/webhooks/${id}/test`, '/api/v1/webhooks/{id}/test'],
      ['DELETE', `/api/v1/webhooks/${id}`, '/api/v1/webhooks/{id}'],
    ] as const) {
      const res = await asOtherTenant(path, { method, ...(method === 'PATCH' ? { body: '{"enabled":false}' } : {}) })
      await conform(method, template, res)
      expect(res.status, `${method} ${path}`).toBe(404)
      expect(await code(res)).toBe('not_found')
    }

    // Still there for its own tenant.
    const own = await provider.admin(`/api/v1/webhooks/${id}`)
    expect(own.status).toBe(200)
    await provider.admin(`/api/v1/webhooks/${id}`, { method: 'DELETE' })
  })

  it('answers not_found for an invitation read or revoked under another tenant', async () => {
    const created = await provider.admin('/api/v1/invitations', {
      method: 'POST',
      body: JSON.stringify({ email: `conformance-${randomToken(6)}@example.test` }),
    })
    expect(created.status).toBe(201)
    const id = ((await created.json()) as { invitation: { id: string } }).invitation.id

    const read = await asOtherTenant(`/api/v1/invitations/${id}`)
    await conform('GET', '/api/v1/invitations/{id}', read)
    expect(read.status).toBe(404)

    const revoke = await asOtherTenant(`/api/v1/invitations/${id}`, { method: 'DELETE' })
    await conform('DELETE', '/api/v1/invitations/{id}', revoke)
    expect(revoke.status).toBe(404)

    const own = await provider.admin(`/api/v1/invitations/${id}`, { method: 'DELETE' })
    expect(own.status).toBe(200)
  })

  it('answers not_found for a client read under another tenant', async () => {
    const client = await provider.createPublicClient()
    const res = await asOtherTenant(`/api/v1/clients/${client.id}`)
    await conform('GET', '/api/v1/clients/{id}', res)
    expect(res.status).toBe(404)
    await provider.deleteClient(client)
  })
})
