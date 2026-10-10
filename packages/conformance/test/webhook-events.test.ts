import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

type Delivery = { event_type: string; payload: string }
type EventPayload = {
  id: string
  type: string
  tenant_id: string
  data: { resource?: { type: string; id?: string }; actor?: { type: string; id?: string } }
}

// A webhook is only worth registering if the events it lists are sent. The
// receiver here never resolves (.invalid, RFC 2606), so nothing leaves the
// test; the provider still records each delivery it attempted, and that record
// is what shows an event was raised.
describe('webhook events', () => {
  let webhookId: string

  beforeAll(async () => {
    const res = await provider.admin('/api/v1/webhooks', {
      method: 'POST',
      body: JSON.stringify({
        name: `conformance-events-${randomToken(4)}`,
        url: 'https://conformance.invalid/events',
        events: ['client.created', 'client.deleted', 'user.created', 'user.deleted'],
        retry_count: 0,
        timeout_secs: 1,
      }),
    })
    await conform('POST', '/api/v1/webhooks', res.clone())
    expect(res.status, await res.clone().text()).toBe(201)
    webhookId = ((await res.json()) as { webhook: { id: string } }).webhook.id
  })

  afterAll(async () => {
    if (webhookId) await provider.admin(`/api/v1/webhooks/${webhookId}`, { method: 'DELETE' })
  })

  /** Waits for a recorded delivery of `type` about the resource `id`. */
  const delivered = async (type: string, id: string): Promise<EventPayload> => {
    const deadline = Date.now() + 10_000
    let seen: string[] = []
    while (Date.now() < deadline) {
      const res = await provider.admin(`/api/v1/webhooks/${webhookId}/deliveries?limit=100`)
      const { deliveries } = (await res.json()) as { deliveries: Delivery[] }
      seen = deliveries.map((d) => d.event_type)
      for (const d of deliveries) {
        if (d.event_type !== type) continue
        const payload = JSON.parse(d.payload) as EventPayload
        if (payload.data?.resource?.id === id) return payload
      }
      await new Promise((resolve) => setTimeout(resolve, 200))
    }
    throw new Error(`No ${type} delivery about ${id} was recorded; recorded events: ${seen.join(', ') || 'none'}`)
  }

  it('sends client.created and client.deleted, naming the client and who acted', async () => {
    const client = await provider.createPublicClient()

    const created = await delivered('client.created', client.id)
    expect(created).toMatchObject({
      type: 'client.created',
      tenant_id: await provider.tenantId(),
      data: { resource: { type: 'client', id: client.id } },
    })
    expect(created.id).toBeTruthy()
    expect(created.data.actor?.type).toBeTruthy()

    await provider.deleteClient(client)
    const deleted = await delivered('client.deleted', client.id)
    expect(deleted.data.resource).toEqual({ type: 'client', id: client.id })
    expect(deleted.id).not.toBe(created.id)
  })

  it('sends user.created when an invitation is accepted and user.deleted when the account is removed', async () => {
    const user = await provider.inviteAndAccept()
    const list = await provider.admin(`/api/v1/users?tenant_id=${await provider.tenantId()}&limit=100`)
    const { users } = (await list.json()) as { users: { id: string; email: string }[] }
    const found = users.find((u) => u.email === user.email)
    if (!found) throw new Error(`The invited user ${user.email} is not in the tenant's user list`)

    const created = await delivered('user.created', found.id)
    expect(created.data.resource).toEqual({ type: 'user', id: found.id })
    // No profile data travels in a delivery.
    expect(JSON.stringify(created)).not.toContain(user.email)

    await provider.deleteUser(found.id)
    const deleted = await delivered('user.deleted', found.id)
    expect(deleted.data.resource).toEqual({ type: 'user', id: found.id })
  })
})
