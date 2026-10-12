import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Authenticator } from '../src/authenticator.js'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { Provider, type LoginOutcome, type TestClient, type TestUser } from '../src/provider.js'
import { randomToken } from '../src/pkce.js'
import { StandInProvider, type StandInIdentity } from '../src/standin.js'

const config = loadConfig()
const provider = new Provider(config)

const claimsOf = (jwt: string): Record<string, unknown> => JSON.parse(Buffer.from(jwt.split('.')[1], 'base64url').toString())

// Signing in through another identity provider hands part of the decision to
// someone else. What the provider under test does with that provider's word —
// which account it reaches, what it refuses, what it still asks for — is
// driven here against a stand-in the suite plays itself.
describe('sign-in through another identity provider', () => {
  let offered: string[] = []
  let standIn: StandInProvider
  let app: TestClient
  let member: TestUser
  let memberSub: string
  let memberAccessToken: string
  const memberAtProvider = `standin-${randomToken(8)}`
  const subjects = new Set<string>()

  const as = (identity: Partial<StandInIdentity> & { email: string }): StandInIdentity => ({
    subject: `standin-${randomToken(8)}`,
    emailVerified: true,
    ...identity,
  })
  const signIn = async (identity: StandInIdentity): Promise<LoginOutcome> =>
    provider.continueWithProvider(await provider.startLogin(app), standIn, identity)

  beforeAll(async () => {
    offered = ((await (await fetch(`${config.api}/api/v1/capabilities`)).json()) as { providers: string[] }).providers
    if (offered.length === 0) return // nothing to verify: this deployment signs no one in through a provider
    if (!config.idpStandIn || !offered.includes(config.idpStandInAs)) {
      // Fail loudly instead of skipping: the deployment signs users in on a
      // provider's word, and nothing else in the suite checks what it does with it.
      throw new Error(
        `The provider offers sign-in through ${offered.join(', ')}, but the suite has no stand-in for it. ` +
          `Set CONFORMANCE_IDP_STANDIN (and configure the provider to reach "${config.idpStandInAs}" there). See packages/conformance/README.md.`,
      )
    }
    standIn = new StandInProvider(new URL(config.idpStandIn))
    await standIn.start()

    app = await provider.createPublicClient({ signInMethods: ['email', config.idpStandInAs] })
    member = await provider.inviteAndAccept()
    const first = await provider.login(app, member)
    if (first.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(first)}`)
    memberSub = first.sub
    memberAccessToken = first.tokens.access_token
    subjects.add(memberSub)
  })

  afterAll(async () => {
    for (const sub of subjects) await provider.deleteUser(sub)
    if (app) await provider.deleteClient(app)
    if (standIn) await standIn.stop()
  })

  it('reaches an existing account by an address the provider has verified', async ({ skip }) => {
    if (offered.length === 0) skip()
    const outcome = await signIn({ subject: memberAtProvider, email: member.email, emailVerified: true })
    if (outcome.kind !== 'code') throw new Error(`The sign-in was not completed: ${JSON.stringify(outcome)}`)
    expect(outcome.sub).toBe(memberSub)
    const claims = claimsOf(outcome.tokens.id_token ?? '')
    expect(claims.amr).toEqual(['fed'])
    expect(claims.email_verified).toBe(true)
  })

  // Once linked, the provider's account — not the address it reports today —
  // is what names the user: a changed or unverified address signs in to the
  // same account and reaches no other.
  it('then knows that provider account whatever address it reports', async ({ skip }) => {
    if (offered.length === 0) skip()
    const outcome = await signIn({ subject: memberAtProvider, email: `moved-${randomToken(4)}@example.test`, emailVerified: false })
    if (outcome.kind !== 'code') throw new Error(`The sign-in was not completed: ${JSON.stringify(outcome)}`)
    expect(outcome.sub).toBe(memberSub)
  })

  // An address the provider does not vouch for proves nothing about who is
  // signing in: it must not open the account that address belongs to.
  it('does not reach an account by an address the provider has not verified', async ({ skip }) => {
    if (offered.length === 0) skip()
    const other = await provider.inviteAndAccept()
    const otherLogin = await provider.login(app, other)
    if (otherLogin.kind === 'code') subjects.add(otherLogin.sub)

    const outcome = await signIn(as({ email: other.email, emailVerified: false }))
    expect(outcome.kind, JSON.stringify(outcome).slice(0, 300)).toBe('rejected')
    if (outcome.kind === 'rejected') expect(new URL(outcome.body).searchParams.get('error')).toBe('access_denied')
  })

  // The provider's word is one factor. A user who has a second factor of
  // their own still owes it, whatever the provider asked of them.
  it('still asks a user for their own second factor', async ({ skip }) => {
    if (offered.length === 0) skip()
    const mfa = async (path: string, body?: unknown): Promise<Response> => {
      const res = await fetch(`${config.api}/api/v1/users/mfa${path}`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${memberAccessToken}`, ...(body ? { 'Content-Type': 'application/json' } : {}) },
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

    const attempt = await provider.startLogin(app)
    const pending = await provider.continueWithProvider(attempt, standIn, { subject: memberAtProvider, email: member.email, emailVerified: true })
    if (pending.kind !== 'second_factor') throw new Error(`The sign-in did not stop for the second factor: ${JSON.stringify(pending).slice(0, 300)}`)
    expect(pending.flow).toBe(attempt.flow)

    const outcome = await provider.continueWithSecondFactor(pending, await authenticator.next())
    if (outcome.kind !== 'code') throw new Error(`The code was not accepted: ${JSON.stringify(outcome)}`)
    expect(outcome.sub).toBe(memberSub)
    expect(claimsOf(outcome.tokens.id_token ?? '').amr).toEqual(['fed', 'otp', 'mfa'])
  })
})
