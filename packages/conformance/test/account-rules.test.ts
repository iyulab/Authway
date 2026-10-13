import { afterAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { waitForLink } from '../src/mail.js'
import { randomToken } from '../src/pkce.js'
import { Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

type User = { id: string; email: string; email_verified: boolean }

/** An admin call on behalf of a tenant other than the suite's own. */
const asTenant = (tenantId: string, path: string, init: RequestInit = {}) =>
  fetch(`${provider.config.api}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${provider.config.adminKey}`,
      'X-Tenant-ID': tenantId,
      ...(init.body ? { 'Content-Type': 'application/json' } : {}),
    },
  })

const userByEmail = async (email: string): Promise<User | undefined> => {
  const res = await provider.admin('/api/v1/users?limit=100')
  await conform('GET', '/api/v1/users', res.clone())
  return ((await res.json()) as { users: User[] }).users.find((u) => u.email === email)
}

/** Invites an address into the suite's tenant and returns the token of the mailed link. */
const invite = async (email: string): Promise<string> => {
  const created = await provider.admin('/api/v1/invitations', { method: 'POST', body: JSON.stringify({ email }) })
  expect(created.status).toBe(201)
  const link = await waitForLink(provider.config.mailApi, email, '/invitation/accept')
  return link.searchParams.get('token')!
}

const accept = (token: string, password: string) =>
  fetch(`${provider.config.api}/api/v1/invitations/accept`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token, password, name: 'Conformance User' }),
  })

describe('accounts: what an account is created with', () => {
  const created: string[] = []
  const tenants: string[] = []

  afterAll(async () => {
    for (const id of created) await provider.deleteUser(id)
    for (const id of tenants) await provider.admin(`/api/v1/tenants/${id}`, { method: 'DELETE' })
  })

  it('counts an accepted invitation as proof of the address it was sent to', async () => {
    const user = await provider.inviteAndAccept()
    const account = await userByEmail(user.email)
    expect(account).toBeDefined()
    created.push(account!.id)
    expect(account!.email_verified).toBe(true)
  })

  it('holds the password set on accepting an invitation to the least length, and says what that is', async () => {
    const email = `conformance-${randomToken(6)}@example.test`
    const token = await invite(email)

    const refused = await accept(token, 'short')
    await conform('POST', '/api/v1/invitations/accept', refused.clone())
    expect(refused.status).toBe(400)
    const body = (await refused.json()) as { code: string; min_length: number }
    expect(body.code).toBe('password_too_short')
    expect(body.min_length).toBe(8)
    expect(await userByEmail(email)).toBeUndefined()

    // The refusal leaves the invitation as it was.
    const accepted = await accept(token, `Cf-${randomToken(12)}`)
    expect(accepted.status).toBe(200)
    created.push(((await accepted.json()) as { user: { id: string } }).user.id)
  })

  it('refuses tenant settings that could not be ones', async () => {
    for (const settings of [
      { password_min_length: 5 },
      { password_min_length: 100 },
      { session_timeout: -1 },
      { allowed_domains: ['not a domain'] },
      { allowed_domains: ['someone@example.test'] },
    ]) {
      const res = await provider.admin('/api/v1/tenants', {
        method: 'POST',
        body: JSON.stringify({ name: 'Conformance tenant', slug: `conformance-${randomToken(4).toLowerCase()}`, settings }),
      })
      await conform('POST', '/api/v1/tenants', res.clone())
      if (res.status === 201) tenants.push(((await res.json()) as { id: string }).id)
      expect(res.status, JSON.stringify(settings)).toBe(400)
    }
  })

  it('tells the screen that is about to ask for a password how long it has to be', async () => {
    const shownLength = async (token: string) => {
      const shown = await fetch(`${provider.config.api}/api/v1/invitations/token/${encodeURIComponent(token)}`)
      await conform('GET', '/api/v1/invitations/token/{token}', shown.clone())
      return ((await shown.json()) as { password_min_length: number }).password_min_length
    }
    const resetLength = async (email: string) => {
      const asked = await fetch(`${provider.config.api}/api/v1/email/forgot-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email }),
      })
      expect(asked.status).toBe(200)
      const link = await waitForLink(provider.config.mailApi, email, '/reset-password')
      const checked = await fetch(`${provider.config.api}/api/v1/email/verify-reset-token?token=${encodeURIComponent(link.searchParams.get('token')!)}`)
      await conform('GET', '/api/v1/email/verify-reset-token', checked.clone())
      expect(checked.status).toBe(200)
      return ((await checked.json()) as { password_min_length: number }).password_min_length
    }

    // A tenant that asks for nothing gets what every tenant gets.
    const plainEmail = `conformance-${randomToken(6)}@example.test`
    const plainToken = await invite(plainEmail)
    expect(await shownLength(plainToken)).toBe(8)
    const plain = await accept(plainToken, `Cf-${randomToken(12)}`)
    expect(plain.status).toBe(200)
    created.push(((await plain.json()) as { user: { id: string } }).user.id)
    expect(await resetLength(plainEmail)).toBe(8)

    // A tenant that asks for more is answered with what it asks.
    const res = await provider.admin('/api/v1/tenants', {
      method: 'POST',
      body: JSON.stringify({
        name: 'Conformance tenant',
        slug: `conformance-${randomToken(4).toLowerCase()}`,
        settings: { password_min_length: 12 },
      }),
    })
    expect(res.status).toBe(201)
    const tenant = (await res.json()) as { id: string }
    tenants.push(tenant.id)

    const email = `conformance-${randomToken(6)}@example.test`
    const invited = await asTenant(tenant.id, '/api/v1/invitations', { method: 'POST', body: JSON.stringify({ email }) })
    expect(invited.status).toBe(201)
    const token = (await waitForLink(provider.config.mailApi, email, '/invitation/accept')).searchParams.get('token')!
    expect(await shownLength(token)).toBe(12)

    const accepted = await accept(token, 'twelve-chars')
    expect(accepted.status).toBe(200)
    const id = ((await accepted.json()) as { user: { id: string } }).user.id
    expect(await resetLength(email)).toBe(12)
    await asTenant(tenant.id, `/api/v1/users/${id}`, { method: 'DELETE' })
  })

  it('takes invitations only for addresses in the domains a tenant names', async () => {
    const res = await provider.admin('/api/v1/tenants', {
      method: 'POST',
      body: JSON.stringify({
        name: 'Conformance tenant',
        slug: `conformance-${randomToken(4).toLowerCase()}`,
        settings: { allowed_domains: ['example.test'], password_min_length: 12 },
      }),
    })
    await conform('POST', '/api/v1/tenants', res.clone())
    expect(res.status).toBe(201)
    const tenant = (await res.json()) as { id: string; settings: { allowed_domains: string[]; password_min_length: number } }
    tenants.push(tenant.id)
    expect(tenant.settings.allowed_domains).toEqual(['example.test'])
    expect(tenant.settings.password_min_length).toBe(12)

    const inviteInto = (email: string) => asTenant(tenant.id, '/api/v1/invitations', { method: 'POST', body: JSON.stringify({ email }) })

    const outside = await inviteInto(`conformance-${randomToken(6)}@elsewhere.test`)
    await conform('POST', '/api/v1/invitations', outside.clone())
    expect(outside.status).toBe(400)
    expect(((await outside.json()) as { code: string }).code).toBe('email_domain_not_allowed')

    const email = `conformance-${randomToken(6)}@example.test`
    const inside = await inviteInto(email)
    await conform('POST', '/api/v1/invitations', inside.clone())
    expect(inside.status).toBe(201)

    // The tenant asks for longer passwords than every tenant gets.
    const link = await waitForLink(provider.config.mailApi, email, '/invitation/accept')
    const token = link.searchParams.get('token')!
    const tooShort = await accept(token, 'elevenchars')
    expect(tooShort.status).toBe(400)
    expect(((await tooShort.json()) as { code: string; min_length: number }).min_length).toBe(12)

    const accepted = await accept(token, 'twelve-chars')
    expect(accepted.status).toBe(200)
    const id = ((await accepted.json()) as { user: { id: string } }).user.id
    await asTenant(tenant.id, `/api/v1/users/${id}`, { method: 'DELETE' })
  })
})
