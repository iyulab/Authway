import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { Provider, type LoginOutcome, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

type SignedIn = Extract<LoginOutcome, { kind: 'code' }>

// A signed-in user changes their own password: the current one proves it, the
// new one signs in from then on, and the sessions opened with the old one end
// — a password changed because it leaked must not leave the leak signed in.
describe('a user changes their own password', () => {
  let app: TestClient
  let user: TestUser
  let here: SignedIn
  let elsewhere: SignedIn

  const signIn = async (who: TestUser): Promise<SignedIn> => {
    const outcome = await provider.login(app, who)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    return outcome
  }
  const change = async (token: string, body: unknown): Promise<Response> => {
    const res = await fetch(`${config.api}/api/v1/users/password`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    await conform('POST', '/api/v1/users/password', res.clone())
    return res
  }

  beforeAll(async () => {
    app = await provider.createPublicClient()
    user = await provider.inviteAndAccept()
    here = await signIn(user)
    elsewhere = await signIn(user)
  })

  afterAll(async () => {
    if (here) await provider.deleteUser(here.sub)
    if (app) await provider.deleteClient(app)
  })

  it('the profile says the account has a password', async () => {
    const res = await fetch(`${config.api}/api/v1/profile/me`, { headers: { Authorization: `Bearer ${here.tokens.access_token}` } })
    await conform('GET', '/api/v1/profile/me', res.clone())
    expect((await res.json()).has_password).toBe(true)
  })

  it('refuses a wrong current password and a new one the rule refuses', async () => {
    const wrong = await change(here.tokens.access_token, { current_password: 'not-the-password', new_password: 'Cf-another-password-1' })
    expect(wrong.status).toBe(400)
    expect((await wrong.json()).code).toBe('invalid_current_password')

    const short = await change(here.tokens.access_token, { current_password: user.password, new_password: 'short' })
    expect(short.status).toBe(400)
    const refusal = await short.json()
    expect(refusal.code).toBe('password_too_short')
    expect(refusal.min_length).toBeGreaterThanOrEqual(8)
  })

  it('changes it, ends the other sessions, and only the new one signs in', async () => {
    expect((await provider.userinfo(elsewhere.tokens.access_token)).status).toBe(200)

    const changed = await change(here.tokens.access_token, { current_password: user.password, new_password: 'Cf-changed-password-1' })
    expect(changed.status, await changed.clone().text()).toBe(200)
    expect(['others', 'all']).toContain((await changed.json()).sessions_ended)

    expect((await provider.userinfo(elsewhere.tokens.access_token)).status).toBe(401)

    const withOld = await provider.login(app, user)
    expect(withOld.kind).not.toBe('code')
    const withNew = await provider.login(app, { ...user, password: 'Cf-changed-password-1' })
    expect(withNew.kind, JSON.stringify(withNew)).toBe('code')
  })
})
