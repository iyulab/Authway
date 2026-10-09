import { describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

type Entry = { id: string; action: string; success: boolean; resource_id: string }

const code = async (res: Response) => ((await res.json()) as { code: string }).code

describe('admin API: reading the audit log', () => {
  const name = `conformance-${randomToken(6)}`
  let webhookId: string

  it('records an admin change and finds it by action and resource', async () => {
    // Creating a webhook is an audited admin write the suite can make and undo.
    const created = await provider.admin('/api/v1/webhooks', {
      method: 'POST',
      body: JSON.stringify({ name, url: 'https://conformance.invalid/webhook', events: ['user.created'], enabled: false }),
    })
    expect(created.status).toBe(201)
    webhookId = ((await created.json()) as { webhook: { id: string } }).webhook.id

    // The entry is written asynchronously; give it a moment.
    let entries: Entry[] = []
    for (let i = 0; i < 20 && entries.length === 0; i++) {
      const res = await provider.admin(`/api/v1/audit/logs?action=webhook.created&resource_id=${webhookId}&limit=5`)
      await conform('GET', '/api/v1/audit/logs', res)
      expect(res.status).toBe(200)
      entries = ((await res.json()) as { logs: Entry[] }).logs
      if (entries.length === 0) await new Promise((r) => setTimeout(r, 250))
    }
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ action: 'webhook.created', resource_id: webhookId, success: true })

    const one = await provider.admin(`/api/v1/audit/logs/${entries[0].id}`)
    await conform('GET', '/api/v1/audit/logs/{id}', one)
    expect(((await one.json()) as { log: Entry }).log.id).toBe(entries[0].id)

    await provider.admin(`/api/v1/webhooks/${webhookId}`, { method: 'DELETE' })
  })

  it('refuses a malformed filter instead of ignoring it', async () => {
    for (const q of ['action=user.exploded', 'severity=loud', 'success=maybe', 'actor_id=not-an-id', 'start_time=yesterday', 'limit=0', 'limit=1001', 'offset=-1']) {
      const res = await provider.admin(`/api/v1/audit/logs?${q}`)
      await conform('GET', '/api/v1/audit/logs', res)
      expect(res.status, q).toBe(400)
      expect(await code(res)).toBe('invalid_request')
    }
  })

  it('refuses a read that names no tenant', async () => {
    const res = await fetch(`${provider.config.api}/api/v1/audit/logs`, {
      headers: { Authorization: `Bearer ${provider.config.adminKey}` },
    })
    await conform('GET', '/api/v1/audit/logs', res)
    expect(res.status).toBe(400)
    expect(await code(res)).toBe('tenant_required')
  })

  it('answers not_found for an unknown entry', async () => {
    const res = await provider.admin('/api/v1/audit/logs/00000000-0000-0000-0000-000000000001')
    await conform('GET', '/api/v1/audit/logs/{id}', res)
    expect(res.status).toBe(404)
    expect(await code(res)).toBe('not_found')
  })

  it('lists actions, summarizes, and reads security events and user activity', async () => {
    const actions = await provider.admin('/api/v1/audit/actions')
    await conform('GET', '/api/v1/audit/actions', actions)
    const listed = ((await actions.json()) as { actions: { action: string }[] }).actions.map((a) => a.action)
    expect(listed).toEqual(expect.arrayContaining(['user.login_failed', 'webhook.created', 'tenant.created']))

    const summary = await provider.admin('/api/v1/audit/summary')
    await conform('GET', '/api/v1/audit/summary', summary)
    expect(summary.status).toBe(200)

    const security = await provider.admin('/api/v1/audit/security?hours=24')
    await conform('GET', '/api/v1/audit/security', security)
    expect(security.status).toBe(200)

    const tooLong = await provider.admin('/api/v1/audit/security?hours=169')
    await conform('GET', '/api/v1/audit/security', tooLong)
    expect(tooLong.status).toBe(400)

    const activity = await provider.admin('/api/v1/audit/users/00000000-0000-0000-0000-000000000001/activity?limit=5')
    await conform('GET', '/api/v1/audit/users/{userId}/activity', activity)
    expect(((await activity.json()) as { logs: Entry[] }).logs).toEqual([])
  })

  it('refuses a purge shorter than the minimum retention', async () => {
    const res = await provider.admin('/api/v1/audit/logs/purge?retention_days=7', { method: 'DELETE' })
    await conform('DELETE', '/api/v1/audit/logs/purge', res)
    expect(res.status).toBe(400)
    expect(await code(res)).toBe('invalid_request')
  })
})
