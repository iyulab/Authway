import { describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

type Webhook = { id: string; name: string; url: string; events: string[]; enabled: boolean; retry_count: number; timeout_secs: number }
type Delivery = { id: string; success: boolean; status_code: number; attempt: number; error_message?: string }

const code = async (res: Response) => ((await res.json()) as { code: string }).code
const json = (body: unknown) => ({ method: 'POST', body: JSON.stringify(body) })

describe('admin API: managing webhooks', () => {
  const name = `conformance-${randomToken(6)}`
  // Nothing listens here, so deliveries fail fast and nothing leaves the test.
  const url = 'http://127.0.0.1:9/conformance-webhook'
  let webhook: Webhook

  it('creates a webhook with the values it was given, including off and zero', async () => {
    const res = await provider.admin(
      '/api/v1/webhooks',
      json({ name, url, events: ['user.created'], enabled: false, retry_count: 0, timeout_secs: 1 }),
    )
    await conform('POST', '/api/v1/webhooks', res)
    expect(res.status).toBe(201)
    webhook = ((await res.json()) as { webhook: Webhook }).webhook
    expect(webhook).toMatchObject({ name, url, events: ['user.created'], enabled: false, retry_count: 0, timeout_secs: 1 })
  })

  it('refuses values outside their range instead of replacing them', async () => {
    for (const body of [
      { name, url: '/relative', events: ['user.created'] },
      { name, url, events: ['user.exploded'] },
      { name, url, events: [] },
      { name, url, events: ['user.created'], retry_count: 11 },
      { name, url, events: ['user.created'], timeout_secs: 0 },
    ]) {
      const res = await provider.admin('/api/v1/webhooks', json(body))
      await conform('POST', '/api/v1/webhooks', res)
      expect(res.status, JSON.stringify(body)).toBe(400)
      expect(await code(res)).toBe('invalid_request')
    }
  })

  it('refuses a webhook that names no tenant', async () => {
    const res = await fetch(`${provider.config.api}/api/v1/webhooks`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${provider.config.adminKey}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, url, events: ['user.created'] }),
    })
    await conform('POST', '/api/v1/webhooks', res)
    expect(res.status).toBe(400)
    expect(await code(res)).toBe('tenant_required')
  })

  it('lists the events a webhook can subscribe to', async () => {
    const res = await provider.admin('/api/v1/webhooks/events')
    await conform('GET', '/api/v1/webhooks/events', res)
    const events = ((await res.json()) as { events: { type: string }[] }).events.map((e) => e.type)
    expect(events).toContain('user.created')
    expect(events).toContain('*')
  })

  it('lists, reads and changes a webhook', async () => {
    const list = await provider.admin('/api/v1/webhooks')
    await conform('GET', '/api/v1/webhooks', list)
    expect(((await list.json()) as { webhooks: Webhook[] }).webhooks.map((w) => w.id)).toContain(webhook.id)

    const read = await provider.admin(`/api/v1/webhooks/${webhook.id}`)
    await conform('GET', '/api/v1/webhooks/{id}', read)
    expect(((await read.json()) as { webhook: Webhook }).webhook.enabled).toBe(false)

    const patch = await provider.admin(`/api/v1/webhooks/${webhook.id}`, {
      method: 'PATCH',
      body: JSON.stringify({ enabled: true, events: ['user.created', 'user.deleted'] }),
    })
    await conform('PATCH', '/api/v1/webhooks/{id}', patch)
    expect(patch.status).toBe(200)
    const changed = ((await patch.json()) as { webhook: Webhook }).webhook
    expect(changed).toMatchObject({ enabled: true, events: ['user.created', 'user.deleted'], retry_count: 0 })

    const refused = await provider.admin(`/api/v1/webhooks/${webhook.id}`, {
      method: 'PATCH',
      body: JSON.stringify({ timeout_secs: 61 }),
    })
    await conform('PATCH', '/api/v1/webhooks/{id}', refused)
    expect(refused.status).toBe(400)
    expect(await code(refused)).toBe('invalid_request')
  })

  it('tests a webhook once and reports the delivery, then lists it', async () => {
    const res = await provider.admin(`/api/v1/webhooks/${webhook.id}/test`, { method: 'POST' })
    await conform('POST', '/api/v1/webhooks/{id}/test', res)
    expect(res.status).toBe(200)
    const delivery = ((await res.json()) as { delivery: Delivery }).delivery
    expect(delivery).toMatchObject({ success: false, status_code: 0, attempt: 1 })
    expect(delivery.error_message).toBeTruthy()
    expect(delivery.id).not.toBe('00000000-0000-0000-0000-000000000000')

    const deliveries = await provider.admin(`/api/v1/webhooks/${webhook.id}/deliveries?limit=10`)
    await conform('GET', '/api/v1/webhooks/{id}/deliveries', deliveries)
    expect(((await deliveries.json()) as { deliveries: Delivery[] }).deliveries.length).toBeGreaterThanOrEqual(1)
  })

  it('deletes a webhook, then answers not_found for it', async () => {
    const del = await provider.admin(`/api/v1/webhooks/${webhook.id}`, { method: 'DELETE' })
    await conform('DELETE', '/api/v1/webhooks/{id}', del)
    expect(del.status).toBe(200)

    for (const [method, path] of [
      ['GET', `/api/v1/webhooks/${webhook.id}`],
      ['DELETE', `/api/v1/webhooks/${webhook.id}`],
      ['POST', `/api/v1/webhooks/${webhook.id}/test`],
      ['GET', `/api/v1/webhooks/${webhook.id}/deliveries`],
    ] as const) {
      const res = await provider.admin(path, { method })
      expect(res.status, `${method} ${path}`).toBe(404)
      expect(await code(res)).toBe('not_found')
    }
  })
})
