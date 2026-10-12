import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())

const claimsOf = (jwt: string): Record<string, unknown> =>
  JSON.parse(Buffer.from(jwt.split('.')[1], 'base64url').toString('utf8')) as Record<string, unknown>

const WITH_REFRESH = ['openid', 'profile', 'email', 'offline_access']

// An application that should not send the user through authorization every
// time an access token runs out asks for a refresh token: the scope
// offline_access, from a client registered for the refresh_token grant.
describe('refresh tokens', () => {
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

  const signIn = async (scopes: string[]) => {
    const outcome = await provider.login(client, user, { scopes })
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    sub = outcome.sub
    return outcome
  }

  it('are issued when the request asks for offline_access, and not otherwise', async () => {
    expect((await signIn(WITH_REFRESH)).tokens.refresh_token).toBeTruthy()
    expect((await signIn(['openid', 'profile', 'email'])).tokens.refresh_token).toBeFalsy()
  })

  it('get a new access token that works, for the same user and the same sign-in', async () => {
    const first = await signIn(WITH_REFRESH)
    const renewed = await provider.refresh(client, first.tokens.refresh_token!)
    expect(renewed.status, JSON.stringify(renewed)).toBe(200)
    const tokens = renewed.tokens!
    expect(tokens.access_token).not.toBe(first.tokens.access_token)

    const info = await provider.userinfo(tokens.access_token)
    expect(info.status).toBe(200)
    expect((info.body as { sub: string }).sub).toBe(first.sub)

    // Renewing is not signing in again: the ID token still says when, and how, the user did.
    const before = claimsOf(first.tokens.id_token!)
    const after = claimsOf(tokens.id_token!)
    expect(after.sub).toBe(before.sub)
    expect(after.auth_time).toBe(before.auth_time)
    expect(after.amr).toEqual(before.amr)
  })

  it('are used once: the one that was exchanged is no longer accepted', async () => {
    const first = await signIn(WITH_REFRESH)
    const renewed = await provider.refresh(client, first.tokens.refresh_token!)
    expect(renewed.status).toBe(200)
    expect(renewed.tokens!.refresh_token).toBeTruthy()
    expect(renewed.tokens!.refresh_token).not.toBe(first.tokens.refresh_token)

    const again = await provider.refresh(client, first.tokens.refresh_token!)
    expect(again.status).toBeGreaterThanOrEqual(400)
    expect(again.status).toBeLessThan(500)
  })

  it('stop working when the user signs out everywhere', async () => {
    const first = await signIn(WITH_REFRESH)
    const logout = await provider.logout(first.tokens.access_token)
    expect(logout.status).toBeLessThan(300)

    const after = await provider.refresh(client, first.tokens.refresh_token!)
    expect(after.status).toBeGreaterThanOrEqual(400)
    expect(after.status).toBeLessThan(500)
  })
})
