import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { POST_LOGOUT_REDIRECT_URI, Provider, type TestClient, type TestUser } from '../src/provider.js'
import { conform } from '../src/contract.js'

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

  it('signs in a user who asks to stay signed in', async () => {
    // How long the session then lasts is the provider's policy; that the
    // choice is accepted, and the session is there, is the contract.
    const outcome = await provider.login(client, user, { remember: true })
    expect(outcome.kind, JSON.stringify(outcome)).toBe('code')
    if (outcome.kind !== 'code') return
    subjects.add(outcome.sub)
    const again = await provider.reauthorize(outcome.browser, client)
    expect(again.access_token).toBeTruthy()
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

  it('tells the login screen what the application asked of it: where to start, whom to expect, which language', async () => {
    const ask = async (extra: Record<string, string>) => {
      const attempt = await provider.startLogin(client, undefined, undefined, extra)
      const res = await fetch(provider.loginFlowUrl(attempt.flow))
      await conform('GET', '/api/v1/login-flows/{flow}', res.clone())
      return (await res.json()) as { client: { sign_in_methods: string[] }; hints?: Record<string, unknown> }
    }

    const plain = await ask({})
    expect(plain.hints ?? {}).toEqual({})

    // A way of signing in is passed on only if the screen offers it anyway.
    const offered = plain.client.sign_in_methods[0]
    const hinted = await ask({ connection: offered, login_hint: user.email, ui_locales: 'ko en' })
    expect(hinted.hints).toEqual({ sign_in_method: offered, login_hint: user.email, ui_locales: ['ko', 'en'] })

    const notOffered = await ask({ connection: 'a-provider-nobody-offers' })
    expect(notOffered.hints ?? {}).not.toHaveProperty('sign_in_method')
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
    const res = await fetch(provider.loginFlowUrl('not-a-real-flow', '/password'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: user.email, password: user.password }),
    })
    await conform('POST', '/api/v1/login-flows/{flow}/password', res)
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

  it('signs a user out through RP-initiated logout and invalidates their tokens', async () => {
    const outcome = await provider.login(client, user)
    if (outcome.kind !== 'code') throw new Error(`login failed: ${JSON.stringify(outcome)}`)
    subjects.add(outcome.sub)
    if (!outcome.tokens.id_token) throw new Error('no id_token to hint the logout with')

    // The same user signed out through the API in the scenario before.
    const signOuts = async (): Promise<number> => {
      const res = await provider.admin(`/api/v1/audit/logs?action=user.logout&resource_id=${outcome.sub}&limit=20`)
      expect(res.status).toBe(200)
      return ((await res.json()) as { logs: unknown[] }).logs.length
    }
    const before = await signOuts()

    const landed = await provider.rpInitiatedLogout(outcome.browser, outcome.tokens.id_token)
    expect(`${landed.origin}${landed.pathname}`).toBe(POST_LOGOUT_REDIRECT_URI)

    const after = await provider.userinfo(outcome.tokens.access_token)
    expect(after.status).toBe(401)

    // Signing out through the screen is the same event as through the API
    // (user.logout — the audit entry the webhook event follows); it was
    // recorded only for the API. The entry is written asynchronously.
    let recorded = 0
    for (let i = 0; i < 40 && recorded === 0; i++) {
      recorded = (await signOuts()) - before
      if (recorded === 0) await new Promise((r) => setTimeout(r, 250))
    }
    expect(recorded).toBe(1)
  })
})
