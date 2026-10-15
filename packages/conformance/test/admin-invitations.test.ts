import { describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

type Invitation = { id: string; email: string; status: string }

const code = async (res: Response) => ((await res.json()) as { code: string }).code

describe('admin API: managing invitations', () => {
  const email = `conformance-${randomToken(6)}@example.test`
  let invitation: Invitation

  const invite = () =>
    // A role is not something an invitation gives: one sent is not kept.
    provider.admin('/api/v1/invitations', { method: 'POST', body: JSON.stringify({ email, role: 'admin' }) })

  it('creates an invitation, refusing a second pending one for the same address', async () => {
    const res = await invite()
    await conform('POST', '/api/v1/invitations', res)
    expect(res.status).toBe(201)
    invitation = ((await res.json()) as { invitation: Invitation }).invitation
    expect(invitation).not.toHaveProperty('role')

    const again = await invite()
    await conform('POST', '/api/v1/invitations', again)
    expect(again.status).toBe(409)
    expect(await again.json()).toMatchObject({ code: 'invitation_already_pending', field: 'email' })
  })

  it('refuses what is not an address, or an application the tenant does not have, naming the member', async () => {
    for (const [body, want] of [
      [{ email: 'not an address' }, { code: 'invalid_request', field: 'email' }],
      [{}, { code: 'invalid_request', field: 'email' }],
      [
        { email: `conformance-${randomToken(6)}@example.test`, client_id: `none-${randomToken(6)}` },
        { code: 'unknown_client', field: 'client_id' },
      ],
    ] as const) {
      const res = await provider.admin('/api/v1/invitations', { method: 'POST', body: JSON.stringify(body) })
      await conform('POST', '/api/v1/invitations', res)
      expect(res.status, JSON.stringify(body)).toBe(400)
      expect(await res.json(), JSON.stringify(body)).toMatchObject(want)
    }
  })

  it('refuses an invitation that names no tenant', async () => {
    const res = await fetch(`${provider.config.api}/api/v1/invitations`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${provider.config.adminKey}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: `conformance-${randomToken(6)}@example.test` }),
    })
    await conform('POST', '/api/v1/invitations', res)
    expect(res.status).toBe(400)
    expect(await code(res)).toBe('tenant_required')
  })

  it('lists pending invitations a page at a time', async () => {
    const res = await provider.admin('/api/v1/invitations?status=pending&limit=100')
    await conform('GET', '/api/v1/invitations', res)
    expect(res.status).toBe(200)
    const page = (await res.json()) as { invitations: Invitation[]; total: number; limit: number }
    expect(page.invitations.map((i) => i.id)).toContain(invitation.id)
    expect(page.invitations.every((i) => i.status === 'pending')).toBe(true)
    expect(page.total).toBeGreaterThanOrEqual(page.invitations.length)

    const one = await provider.admin('/api/v1/invitations?status=pending&limit=1')
    await conform('GET', '/api/v1/invitations', one)
    expect(((await one.json()) as { invitations: Invitation[] }).invitations.length).toBeLessThanOrEqual(1)
  })

  it('reads, resends and revokes a pending invitation, then refuses to revoke it again', async () => {
    const read = await provider.admin(`/api/v1/invitations/${invitation.id}`)
    await conform('GET', '/api/v1/invitations/{id}', read)
    expect(((await read.json()) as { invitation: Invitation }).invitation.email).toBe(email)

    const resend = await provider.admin(`/api/v1/invitations/${invitation.id}/resend`, { method: 'POST' })
    await conform('POST', '/api/v1/invitations/{id}/resend', resend)
    expect(resend.status).toBe(200)

    const revoke = await provider.admin(`/api/v1/invitations/${invitation.id}`, { method: 'DELETE' })
    await conform('DELETE', '/api/v1/invitations/{id}', revoke)
    expect(revoke.status).toBe(200)

    const again = await provider.admin(`/api/v1/invitations/${invitation.id}`, { method: 'DELETE' })
    await conform('DELETE', '/api/v1/invitations/{id}', again)
    expect(again.status).toBe(409)
    expect(await code(again)).toBe('invitation_not_pending')
  })

  it('answers 404 with a code for an invitation that does not exist', async () => {
    const res = await provider.admin(`/api/v1/invitations/${crypto.randomUUID()}`)
    await conform('GET', '/api/v1/invitations/{id}', res)
    expect(res.status).toBe(404)
    expect(await code(res)).toBe('not_found')
  })
})
