import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())

describe('password login through the authorization-code flow', () => {
  let client: TestClient
  let user: TestUser
  const subjects = new Set<string>()

  beforeAll(async () => {
    client = await provider.createPublicClient()
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    for (const sub of subjects) await provider.deleteUser(sub)
    if (client) await provider.deleteClient(client)
  })

  it('issues tokens for an invited user and identifies them at userinfo', async () => {
    const outcome = await provider.login(client, user)
    expect(outcome.kind).toBe('code')
    if (outcome.kind !== 'code') return
    subjects.add(outcome.sub)
    expect(outcome.tokens.access_token).toBeTruthy()
    expect(outcome.tokens.id_token).toBeTruthy()
    expect(outcome.sub).toBeTruthy()
  })

  it('rejects a wrong password without advancing the flow', async () => {
    const outcome = await provider.login(client, { ...user, password: `${user.password}-wrong` })
    expect(outcome.kind).toBe('rejected')
    if (outcome.kind !== 'rejected') return
    expect(outcome.status).toBeGreaterThanOrEqual(400)
    expect(outcome.status).toBeLessThan(500)
  })

  it('keeps the login screen usable after a wrong password', async () => {
    const attempt = await provider.startLogin(client)
    const wrong = await provider.continueLogin(attempt, { ...user, password: `${user.password}-wrong` })
    expect(wrong.kind).toBe('rejected')

    const retry = await provider.continueLogin(attempt, user)
    expect(retry.kind, JSON.stringify(retry)).toBe('code')
    if (retry.kind === 'code') subjects.add(retry.sub)
  })

  it('rejects an unknown flow id as a client error, not a server error', async () => {
    const res = await fetch(`${provider.config.api}/authenticate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ challenge: 'not-a-real-flow', email: user.email, password: user.password }),
    })
    expect(res.status).toBeGreaterThanOrEqual(400)
    expect(res.status).toBeLessThan(500)
  })

  it('stops honoring an access token once the user logs out', async () => {
    const outcome = await provider.login(client, user)
    if (outcome.kind !== 'code') throw new Error(`login failed: ${JSON.stringify(outcome)}`)
    subjects.add(outcome.sub)

    const logout = await provider.logout(outcome.tokens.access_token)
    expect(logout.status).toBeLessThan(300)

    const after = await provider.userinfo(outcome.tokens.access_token)
    expect(after.status).toBe(401)
  })
})
