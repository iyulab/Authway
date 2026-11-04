import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { Provider, type LoginOutcome, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

type SignedIn = Extract<LoginOutcome, { kind: 'code' }>

// The signed-in user's own profile says what the token's scopes name, as
// userinfo does: the address with `email`, the name with `profile`. An
// application that asked for `openid` alone learns who signed in, not who they
// are.
describe('a user\'s profile follows the scopes of the token', () => {
  let app: TestClient
  let user: TestUser

  const profileOf = async (scopes: string[]): Promise<Record<string, unknown>> => {
    const outcome = await provider.login(app, user, { scopes })
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    const signedIn: SignedIn = outcome
    const res = await fetch(`${config.api}/api/v1/profile/me`, { headers: { Authorization: `Bearer ${signedIn.tokens.access_token}` } })
    await conform('GET', '/api/v1/profile/me', res.clone())
    expect(res.status).toBe(200)
    return (await res.json()) as Record<string, unknown>
  }

  beforeAll(async () => {
    app = await provider.createPublicClient({ scopes: ['openid', 'profile', 'email'], skipConsent: true })
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    if (app) await provider.deleteClient(app)
  })

  it('says only who signed in to a token with openid alone', async () => {
    const profile = await profileOf(['openid'])
    expect(profile).toHaveProperty('id')
    expect(profile).not.toHaveProperty('email')
    expect(profile).not.toHaveProperty('email_verified')
    expect(profile).not.toHaveProperty('name')
  })

  it('adds the address with email and the name with profile, each on its own', async () => {
    const withAddress = await profileOf(['openid', 'email'])
    expect(withAddress).toMatchObject({ email: user.email })
    expect(withAddress).toHaveProperty('email_verified')
    expect(withAddress).not.toHaveProperty('name')

    const withName = await profileOf(['openid', 'profile'])
    expect(withName).toHaveProperty('name')
    expect(withName).not.toHaveProperty('email')

    const both = await profileOf(['openid', 'email', 'profile'])
    expect(both).toMatchObject({ email: user.email })
    expect(both).toHaveProperty('name')
  })
})
