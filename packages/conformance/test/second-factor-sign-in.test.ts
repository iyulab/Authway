import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Authenticator } from '../src/authenticator.js'
import { loadConfig } from '../src/config.js'
import { Provider, type LoginOutcome, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

const claimsOf = (jwt: string): Record<string, unknown> => JSON.parse(Buffer.from(jwt.split('.')[1], 'base64url').toString())

// A user who has a second factor owes it however they proved who they are
// first. If only the password asked for it, anyone who can read the user's
// mail could sign in with an emailed link and never meet the second factor.
describe('a user with a second factor is asked for it however they sign in', () => {
  let app: TestClient
  let user: TestUser
  let sub: string
  let authenticator: Authenticator
  let offersLinks = false

  /** Expects a sign-in to have stopped for the second factor, on the flow it started in. */
  function expectSecondFactorOwed(outcome: LoginOutcome, flow: string): Extract<LoginOutcome, { kind: 'second_factor' }> {
    if (outcome.kind !== 'second_factor') throw new Error(`The sign-in did not stop for the second factor: ${JSON.stringify(outcome).slice(0, 300)}`)
    expect(outcome.flow).toBe(flow)
    return outcome
  }

  beforeAll(async () => {
    offersLinks = Boolean((await (await fetch(`${config.api}/api/v1/capabilities`)).json()).magic_link)
    app = await provider.createPublicClient({ signInMethods: offersLinks ? ['email', 'magic_link'] : ['email'] })
    user = await provider.inviteAndAccept()
    const outcome = await provider.login(app, user)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    sub = outcome.sub

    const mfa = (path: string, body?: unknown): Promise<Response> =>
      fetch(`${config.api}/api/v1/users/mfa${path}`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${outcome.tokens.access_token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) },
        body: body ? JSON.stringify(body) : undefined,
      })
    const setup = await mfa('/setup')
    expect(setup.status, await setup.clone().text()).toBe(200)
    authenticator = new Authenticator(((await setup.json()) as { secret: string }).secret)
    const enabled = await mfa('/verify', { code: await authenticator.next() })
    expect(enabled.status, await enabled.clone().text()).toBe(200)
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    if (app) await provider.deleteClient(app)
  })

  it('after a password', async () => {
    const attempt = await provider.startLogin(app)
    const pending = expectSecondFactorOwed(await provider.continueLogin(attempt, user), attempt.flow)

    const wrong = await provider.continueWithSecondFactor(pending, '000000')
    expect(wrong.kind, JSON.stringify(wrong)).toBe('rejected')

    const outcome = await provider.continueWithSecondFactor(pending, await authenticator.next())
    if (outcome.kind !== 'code') throw new Error(`The code was not accepted: ${JSON.stringify(outcome)}`)
    expect(outcome.sub).toBe(sub)
    expect(claimsOf(outcome.tokens.id_token ?? '').amr).toEqual(['pwd', 'otp', 'mfa'])
  })

  // A code is current for a minute or so — long enough for someone who saw it
  // typed to type it too. Accepted once, it is spent.
  it('does not accept the same code a second time', async () => {
    const attempt = await provider.startLogin(app)
    const pending = expectSecondFactorOwed(await provider.continueLogin(attempt, user), attempt.flow)
    const spent = await provider.continueWithSecondFactor(pending, authenticator.last())
    expect(spent.kind, JSON.stringify(spent).slice(0, 200)).toBe('rejected')
  })

  it('after an emailed link', { timeout: 90_000 }, async ({ skip }) => {
    if (!offersLinks) skip()
    const attempt = await provider.startLogin(app)
    const pending = expectSecondFactorOwed(await provider.continueWithMagicLink(attempt, user.email), attempt.flow)

    // The link is used up and has signed no one in: only the code does.
    const wrong = await provider.continueWithSecondFactor(pending, '000000')
    expect(wrong.kind, JSON.stringify(wrong)).toBe('rejected')

    const outcome = await provider.continueWithSecondFactor(pending, await authenticator.next())
    if (outcome.kind !== 'code') throw new Error(`The code was not accepted: ${JSON.stringify(outcome)}`)
    expect(outcome.sub).toBe(sub)
    // The tokens say both factors were used: the emailed link, then the authenticator.
    expect(claimsOf(outcome.tokens.id_token ?? '').amr).toEqual(['email', 'otp', 'mfa'])
  })
})
