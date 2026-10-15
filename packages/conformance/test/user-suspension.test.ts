import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())

const WITH_REFRESH = ['openid', 'profile', 'email', 'offline_access']

// An administrator suspends an account to stop it signing in without deleting
// it: its sessions end, and signing in is refused until the suspension is
// lifted. The account is kept as it was.
describe('suspending a user', () => {
  let client: TestClient
  let user: TestUser
  let sub: string | undefined

  beforeAll(async () => {
    client = await provider.createPublicClient({ scopes: WITH_REFRESH })
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    if (client) await provider.deleteClient(client)
  })

  const setActive = async (active: boolean): Promise<{ active: boolean }> => {
    const res = await provider.admin(`/api/v1/users/${sub}`, { method: 'PUT', body: JSON.stringify({ active }) })
    await conform('PUT', '/api/v1/users/{id}', res)
    expect(res.status).toBe(200)
    return ((await res.json()) as { user: { active: boolean } }).user
  }

  it('ends its sessions, refuses its sign-in, and lets it back in when lifted', async () => {
    const before = await provider.login(client, user, { scopes: WITH_REFRESH })
    if (before.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(before)}`)
    sub = before.sub
    expect(before.tokens.refresh_token).toBeTruthy()

    expect((await setActive(false)).active).toBe(false)

    // The session it had is over: its refresh token no longer works.
    const renewed = await provider.refresh(client, before.tokens.refresh_token!)
    expect(renewed.status).toBe(400)

    // A wrong password says nothing about the account; the right one is refused for it.
    const attempt = await provider.startLogin(client, WITH_REFRESH)
    const wrong = await provider.submitPassword(attempt.browser, attempt.flow, { ...user, password: `${user.password}-wrong` })
    expect(wrong.status).toBe(401)
    const refused = await provider.submitPassword(attempt.browser, attempt.flow, user)
    expect(refused.status).toBe(403)
    expect(((await refused.json()) as { code: string }).code).toBe('account_suspended')

    // Recorded, and sent to webhooks, as a suspension.
    let recorded = 0
    for (let i = 0; i < 40 && recorded === 0; i++) {
      const res = await provider.admin(`/api/v1/audit/logs?action=user.suspended&resource_id=${sub}&limit=5`)
      recorded = ((await res.json()) as { logs: unknown[] }).logs.length
      if (recorded === 0) await new Promise((r) => setTimeout(r, 250))
    }
    expect(recorded).toBe(1)

    expect((await setActive(true)).active).toBe(true)
    const after = await provider.login(client, user, { scopes: WITH_REFRESH })
    expect(after.kind).toBe('code')
  })
})
