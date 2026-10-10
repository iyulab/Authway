import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())

// A user of a self-service application must be able to leave: delete their own
// account with the token the application already holds, provided they signed
// in just now. Everything issued to them ends with the account, and services
// that keep data about them hear of it.
describe('a user deleting their own account', () => {
  let client: TestClient
  let user: TestUser
  let webhookId: string
  let accessToken: string
  let sub: string

  const me = (init: RequestInit = {}, token = accessToken) =>
    fetch(`${provider.config.api}/api/v1/profile/me`, {
      ...init,
      headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    })

  beforeAll(async () => {
    const caps = await (await fetch(`${provider.config.api}/api/v1/capabilities`)).json()
    if (!caps.account_deletion) throw new Error('The provider does not offer account deletion (capabilities.account_deletion is false)')

    const hook = await provider.admin('/api/v1/webhooks', {
      method: 'POST',
      body: JSON.stringify({
        name: `conformance-deletion-${randomToken(4)}`,
        url: 'https://conformance.invalid/deletions',
        events: ['user.deleted'],
        retry_count: 0,
        timeout_secs: 1,
      }),
    })
    webhookId = ((await hook.json()) as { webhook: { id: string } }).webhook.id

    client = await provider.createPublicClient()
    user = await provider.inviteAndAccept()
    const outcome = await provider.login(client, user)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    accessToken = outcome.tokens.access_token
    sub = outcome.sub
  })

  afterAll(async () => {
    if (webhookId) await provider.admin(`/api/v1/webhooks/${webhookId}`, { method: 'DELETE' })
    if (client) await provider.deleteClient(client)
  })

  it('shows the signed-in user their own profile', async () => {
    const res = await me()
    await conform('GET', '/api/v1/profile/me', res.clone())
    expect(res.status).toBe(200)
    expect(await res.json()).toMatchObject({ id: sub, email: user.email })
  })

  it('refuses to delete an account without a token', async () => {
    const res = await me({ method: 'DELETE' }, '')
    expect(res.status).toBe(401)
    expect(((await provider.admin(`/api/v1/users/${sub}`)) as Response).status).toBe(200)
  })

  it('deletes the account of a user who has just signed in, and ends what was issued to them', async () => {
    const res = await me({ method: 'DELETE' })
    await conform('DELETE', '/api/v1/profile/me', res.clone())
    expect(res.status, await res.clone().text()).toBe(200)

    // The account is gone …
    expect((await provider.admin(`/api/v1/users/${sub}`)).status).toBe(404)
    // … the token no longer works anywhere …
    expect((await provider.userinfo(accessToken)).status).toBe(401)
    expect((await me()).status).toBe(401)
    // … and the user cannot sign in again.
    const again = await provider.login(client, user)
    expect(again.kind).toBe('rejected')
  })

  it('ends the tokens of a user an administrator deletes', async () => {
    const other = await provider.inviteAndAccept()
    const outcome = await provider.login(client, other)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    expect((await provider.userinfo(outcome.tokens.access_token)).status).toBe(200)

    await provider.deleteUser(outcome.sub)
    expect((await provider.userinfo(outcome.tokens.access_token)).status).toBe(401)
  })

  it('sends user.deleted naming the user as the one who did it', async () => {
    const deadline = Date.now() + 10_000
    for (;;) {
      const list = await provider.admin(`/api/v1/webhooks/${webhookId}/deliveries?limit=50`)
      const { deliveries } = (await list.json()) as { deliveries: { event_type: string; payload: string }[] }
      const found = deliveries
        .filter((d) => d.event_type === 'user.deleted')
        .map((d) => JSON.parse(d.payload) as { data: { resource: { type: string; id: string }; actor: { type: string; id?: string } } })
        .find((p) => p.data.resource.id === sub)
      if (found) {
        expect(found.data).toEqual({ resource: { type: 'user', id: sub }, actor: { type: 'user', id: sub } })
        return
      }
      if (Date.now() > deadline) throw new Error(`No user.deleted delivery about ${sub} was recorded`)
      await new Promise((resolve) => setTimeout(resolve, 200))
    }
  })
})
