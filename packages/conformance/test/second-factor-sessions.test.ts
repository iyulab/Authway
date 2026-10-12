import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Authenticator } from '../src/authenticator.js'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { Provider, type LoginOutcome, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

type SignedIn = Extract<LoginOutcome, { kind: 'code' }>

// A session opened before a user had a second factor was opened on one
// factor, and so were its tokens. If they outlived the moment the factor is
// turned on, whoever already held one would keep the account past it — the
// very thing the user turned the factor on against.
describe('turning the second factor on ends the sessions opened before it', () => {
  let app: TestClient
  let user: TestUser
  let here: SignedIn
  let elsewhere: SignedIn

  const signIn = async (): Promise<SignedIn> => {
    const outcome = await provider.login(app, user, { remember: true })
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    return outcome
  }
  /** What the login screen is told to do for a new authorization request in `browser`. */
  const nextStepIn = async (signedIn: SignedIn): Promise<string> => {
    const attempt = await provider.startLogin(app, ['openid', 'profile', 'email'], signedIn.browser)
    return ((await (await fetch(provider.loginFlowUrl(attempt.flow))).json()) as { next: string }).next
  }

  beforeAll(async () => {
    app = await provider.createPublicClient()
    user = await provider.inviteAndAccept()
    // Two browsers, each with a session of its own.
    here = await signIn()
    elsewhere = await signIn()
  })

  afterAll(async () => {
    if (here) await provider.deleteUser(here.sub)
    if (app) await provider.deleteClient(app)
  })

  it('keeps the session that turned it on and ends the others, tokens included', async () => {
    // Before: both browsers are signed in.
    expect((await provider.userinfo(elsewhere.tokens.access_token)).status).toBe(200)
    expect(await nextStepIn(elsewhere)).toBe('redirect')

    const mfa = async (path: string, body?: unknown): Promise<Response> => {
      const res = await fetch(`${config.api}/api/v1/users/mfa${path}`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${here.tokens.access_token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) },
        body: body ? JSON.stringify(body) : undefined,
      })
      await conform('POST', `/api/v1/users/mfa${path}`, res.clone())
      return res
    }
    const setup = await mfa('/setup')
    expect(setup.status, await setup.clone().text()).toBe(200)
    const authenticator = new Authenticator(((await setup.json()) as { secret: string }).secret)
    const enabled = await mfa('/verify', { code: await authenticator.next() })
    expect(enabled.status, await enabled.clone().text()).toBe(200)
    const answer = (await enabled.json()) as { recovery_codes: string[]; sessions_ended: string }
    expect(answer.recovery_codes.length).toBeGreaterThan(0)
    expect(answer.sessions_ended).toBe('others')

    // The other browser: its token is dead and it is no longer signed in.
    expect((await provider.userinfo(elsewhere.tokens.access_token)).status).toBe(401)
    expect(await nextStepIn(elsewhere)).toBe('form')

    // This browser: the token that turned the factor on still works — the
    // recovery codes have just been shown on it — and the session is kept.
    expect((await provider.userinfo(here.tokens.access_token)).status).toBe(200)
    expect(await nextStepIn(here)).toBe('redirect')
  })
})
