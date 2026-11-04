import { describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

type Entry = { id: string; action: string; success: boolean; severity: string; actor_id: string | null; resource_id: string; resource_type: string; created_at: string }

const code = async (res: Response) => ((await res.json()) as { code: string }).code

/**
 * An entry is written just after the change it records, so a read straight
 * after may not show it yet: ask until it does, for as long as a busy
 * provider might take.
 */
async function eventually(find: () => Promise<Entry[]>, withinMs = 20_000): Promise<Entry[]> {
  const deadline = Date.now() + withinMs
  for (;;) {
    const entries = await find()
    if (entries.length > 0 || Date.now() > deadline) return entries
    await new Promise((r) => setTimeout(r, 250))
  }
}

describe('admin API: reading the audit log', () => {
  // Registering an application is an audited admin write every installation has, which the suite can make and undo.
  it('records an admin change and finds it by action and resource', async () => {
    const app = await provider.createPublicClient()
    try {
      const entries = await eventually(async () => {
        const res = await provider.admin(`/api/v1/audit/logs?action=client.created&resource_id=${app.id}&limit=5`)
        await conform('GET', '/api/v1/audit/logs', res)
        expect(res.status).toBe(200)
        return ((await res.json()) as { logs: Entry[] }).logs
      })
      expect(entries).toHaveLength(1)
      expect(entries[0]).toMatchObject({ action: 'client.created', resource_id: app.id, success: true })

      const one = await provider.admin(`/api/v1/audit/logs/${entries[0].id}`)
      await conform('GET', '/api/v1/audit/logs/{id}', one)
      expect(((await one.json()) as { log: Entry }).log.id).toBe(entries[0].id)
    } finally {
      await provider.deleteClient(app)
    }

    // Removing it is recorded too.
    const removed = await eventually(async () => {
      const res = await provider.admin(`/api/v1/audit/logs?action=client.deleted&resource_id=${app.id}&limit=5`)
      expect(res.status).toBe(200)
      return ((await res.json()) as { logs: Entry[] }).logs
    })
    expect(removed).toHaveLength(1)
  })

  it('narrows by the kind of resource and by when it happened', async () => {
    const app = await provider.createPublicClient()
    const id = app.id
    const find = async (query: string): Promise<Entry[]> => {
      const res = await provider.admin(`/api/v1/audit/logs?resource_id=${id}&limit=5&${query}`)
      await conform('GET', '/api/v1/audit/logs', res)
      expect(res.status, query).toBe(200)
      return ((await res.json()) as { logs: Entry[] }).logs
    }
    try {
      const entries = await eventually(() => find('action=client.created'))
      expect(entries).toHaveLength(1)
      const [entry] = entries
      expect(entry.resource_type).toBeTruthy()

      // The kind of resource: the entry's own finds it, another does not.
      expect(await find(`resource_type=${entry.resource_type}`)).toHaveLength(1)
      expect(await find(`resource_type=${entry.resource_type === 'user' ? 'client' : 'user'}`)).toHaveLength(0)

      // Its severity, whether it succeeded, and who did it: the entry's own
      // value finds it, another does not.
      expect(await find(`severity=${entry.severity}`)).toHaveLength(1)
      expect(await find(`severity=${entry.severity === 'critical' ? 'info' : 'critical'}`)).toHaveLength(0)
      expect(await find(`success=${entry.success}`)).toHaveLength(1)
      expect(await find(`success=${!entry.success}`)).toHaveLength(0)
      if (entry.actor_id) expect(await find(`actor_id=${entry.actor_id}`)).toHaveLength(1)
      expect(await find('actor_id=00000000-0000-0000-0000-000000000001')).toHaveLength(0)

      // When: a window that ends before the entry, or starts after it, is empty.
      const at = new Date(entry.created_at).getTime()
      const iso = (ms: number) => encodeURIComponent(new Date(ms).toISOString().replace(/\.\d{3}Z$/, 'Z'))
      expect(await find(`end_time=${iso(at - 60_000)}`)).toHaveLength(0)
      expect(await find(`start_time=${iso(at + 60_000)}`)).toHaveLength(0)
      expect(await find(`start_time=${iso(at - 60_000)}&end_time=${iso(at + 60_000)}`)).toHaveLength(1)
    } finally {
      await provider.deleteClient(app)
    }
  })

  it('refuses a malformed filter instead of ignoring it', async () => {
    for (const q of ['action=user.exploded', 'severity=loud', 'success=maybe', 'actor_id=not-an-id', 'start_time=yesterday', 'limit=0', 'limit=1001', 'offset=-1']) {
      const res = await provider.admin(`/api/v1/audit/logs?${q}`)
      await conform('GET', '/api/v1/audit/logs', res)
      expect(res.status, q).toBe(400)
      expect(await code(res)).toBe('invalid_request')
    }
  })

  // Only where there is a tenant to name: an installation with one takes a call that names none to be about it.
  it('refuses a read that names no tenant', async (context) => {
    if (!(await provider.offers('multi_tenant'))) context.skip()
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
    // Every action listed is one the installation records: these two every installation does.
    expect(listed).toEqual(expect.arrayContaining(['user.login_failed', 'client.created']))
    if (await provider.offers('multi_tenant')) expect(listed).toContain('tenant.created')
    if (await provider.offers('webhooks')) expect(listed).toContain('webhook.created')

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
