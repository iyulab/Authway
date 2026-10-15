import { createHmac, timingSafeEqual } from 'node:crypto'
import http from 'node:http'
import type { AddressInfo } from 'node:net'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

type Received = { headers: http.IncomingHttpHeaders; body: string }

/**
 * Verifies a delivery the way any Standard Webhooks library does
 * (https://www.standardwebhooks.com): the key is the base64 after "whsec_",
 * the signed content is "<id>.<timestamp>.<raw body>", and one matching
 * "v1,<base64 HMAC-SHA256>" among the space-separated signatures is enough.
 */
const verifies = (secret: string, r: Received): boolean => {
  const id = r.headers['webhook-id']
  const timestamp = r.headers['webhook-timestamp']
  const signatures = r.headers['webhook-signature']
  if (typeof id !== 'string' || typeof timestamp !== 'string' || typeof signatures !== 'string') return false
  if (Math.abs(Date.now() / 1000 - Number(timestamp)) > 300) return false
  const key = Buffer.from(secret.replace(/^whsec_/, ''), 'base64')
  const expected = createHmac('sha256', key).update(`${id}.${timestamp}.${r.body}`).digest()
  return signatures.split(' ').some((s) => {
    const [version, value] = s.split(',')
    const got = Buffer.from(value ?? '', 'base64')
    return version === 'v1' && got.length === expected.length && timingSafeEqual(got, expected)
  })
}

// A receiver checks every delivery before trusting it. These deliveries reach a
// receiver run by the test, so the check is made on what actually arrived —
// headers and raw body — with nothing but the secret the API handed out.
describe('webhook delivery', () => {
  const received: Received[] = []
  let receiver: http.Server
  let webhookId: string
  let secret: string

  const nextDelivery = async (after: number): Promise<Received> => {
    for (let i = 0; i < 50; i++) {
      if (received.length > after) return received[after]
      await new Promise((r) => setTimeout(r, 200))
    }
    throw new Error('No delivery arrived at the receiver')
  }

  const sendTest = async (): Promise<Received> => {
    const before = received.length
    const res = await provider.admin(`/api/v1/webhooks/${webhookId}/test`, { method: 'POST' })
    await conform('POST', '/api/v1/webhooks/{id}/test', res.clone())
    expect(res.status, await res.clone().text()).toBe(200)
    return nextDelivery(before)
  }

  beforeAll(async () => {
    receiver = http.createServer((req, res) => {
      let body = ''
      req.on('data', (chunk) => (body += chunk))
      req.on('end', () => {
        if (req.method === 'POST') received.push({ headers: req.headers, body })
        res.writeHead(204).end()
      })
    })
    await new Promise<void>((resolve) => receiver.listen(0, '0.0.0.0', resolve))
    const url = `http://${config.callbackHost}:${(receiver.address() as AddressInfo).port}/hooks`

    const res = await provider.admin('/api/v1/webhooks', {
      method: 'POST',
      body: JSON.stringify({ name: `conformance-delivery-${randomToken(4)}`, url, events: ['test'], retry_count: 0, timeout_secs: 5 }),
    })
    await conform('POST', '/api/v1/webhooks', res.clone())
    expect(res.status, await res.clone().text()).toBe(201)
    const created = (await res.json()) as { webhook: { id: string }; secret: string }
    webhookId = created.webhook.id
    secret = created.secret
  })

  afterAll(async () => {
    if (webhookId) await provider.admin(`/api/v1/webhooks/${webhookId}`, { method: 'DELETE' })
    await new Promise<void>((resolve) => receiver?.close(() => resolve()))
  })

  it('signs a delivery as Standard Webhooks specifies, under the secret it handed out', async () => {
    const delivery = await sendTest()
    expect(verifies(secret, delivery)).toBe(true)

    const payload = JSON.parse(delivery.body) as { id: string; type: string }
    expect(payload.type).toBe('test')
    // webhook-id is the event's id, the one a receiver de-duplicates on.
    expect(delivery.headers['webhook-id']).toBe(payload.id)
    expect(delivery.headers['content-type']).toMatch(/^application\/json/)
    // The previous signature headers are gone, not sent alongside.
    expect(delivery.headers['x-webhook-signature']).toBeUndefined()
  })

  it('does not verify when the body, the id or the secret differ', async () => {
    const delivery = await sendTest()
    expect(verifies(secret, { ...delivery, body: delivery.body.replace('"test"', '"user.created"') })).toBe(false)
    expect(verifies(secret, { ...delivery, headers: { ...delivery.headers, 'webhook-id': 'another-event' } })).toBe(false)
    expect(verifies(`whsec_${Buffer.alloc(32).toString('base64')}`, delivery)).toBe(false)
  })

  it('signs with the new secret only, once it is rotated', async () => {
    const res = await provider.admin(`/api/v1/webhooks/${webhookId}/rotate-secret`, { method: 'POST' })
    expect(res.status).toBe(200)
    const rotated = ((await res.json()) as { secret: string }).secret

    const delivery = await sendTest()
    expect(verifies(rotated, delivery)).toBe(true)
    expect(verifies(secret, delivery)).toBe(false)
    secret = rotated
  })
})
