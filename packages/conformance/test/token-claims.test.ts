import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, POST_LOGOUT_REDIRECT_URI, REDIRECT_URI, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())

/** The claims of a JWT, without verifying it — the suite reads shape, not trust. */
const claimsOf = (jwt: string): Record<string, unknown> => {
  const parts = jwt.split('.')
  if (parts.length !== 3) throw new Error(`Not a JWT (${parts.length} parts): ${jwt.slice(0, 24)}…`)
  return JSON.parse(Buffer.from(parts[1], 'base64url').toString('utf8')) as Record<string, unknown>
}

// A resource server validates the access token and reads who the user is from
// its claims. Those claims are part of what an application is written
// against: Authway's own — the tenant, the address, the name, when the user
// last signed in — sit at the top level of the token, where any JWT library
// reads them, whichever provider issued it.
describe('token claims', () => {
  let client: TestClient
  let user: TestUser
  let sub: string
  let accessToken: string
  let idToken: string

  beforeAll(async () => {
    const res = await provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: `conformance-jwt-${randomToken(4)}`,
        public: true,
        redirect_uris: [REDIRECT_URI],
        post_logout_redirect_uris: [POST_LOGOUT_REDIRECT_URI],
        allowed_origins: ['http://localhost:9999'],
        grant_types: ['authorization_code', 'refresh_token'],
        scopes: ['openid', 'profile', 'email'],
        access_token_strategy: 'jwt',
      }),
    })
    await conform('POST', '/api/v1/clients', res.clone())
    expect(res.status, await res.clone().text()).toBe(201)
    const { client: created } = (await res.json()) as { client: { id: string; client_id: string } }
    client = { id: created.id, clientId: created.client_id }

    user = await provider.inviteAndAccept()
    const outcome = await provider.login(client, user)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    sub = outcome.sub
    accessToken = outcome.tokens.access_token
    idToken = outcome.tokens.id_token ?? ''
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    if (client) await provider.deleteClient(client)
  })

  it("puts Authway's claims at the top level of a JWT access token", () => {
    const claims = claimsOf(accessToken)
    expect(claims.sub).toBe(sub)
    expect(claims.email).toBe(user.email)
    expect(typeof claims.name).toBe('string')
    expect(typeof claims.tenant_id).toBe('string')
    expect(claims.tenant_id).toBeTruthy()
    // When the user last signed in: just now.
    const authTime = claims.auth_time as number
    expect(typeof authTime).toBe('number')
    expect(Math.abs(Date.now() / 1000 - authTime)).toBeLessThan(120)
  })

  // RFC 9068: the scopes of a JWT access token are one space-separated
  // string, which is what standard validators read.
  it('names the granted scopes in `scope`, as one string', () => {
    const claims = claimsOf(accessToken)
    expect(typeof claims.scope).toBe('string')
    expect((claims.scope as string).split(' ')).toEqual(expect.arrayContaining(['openid', 'profile', 'email']))
  })

  it('puts the same claims in the ID token', () => {
    const claims = claimsOf(idToken)
    expect(claims.sub).toBe(sub)
    expect(claims.email).toBe(user.email)
    expect(claims.tenant_id).toBe(claimsOf(accessToken).tenant_id)
    expect(typeof claims.auth_time).toBe('number')
    // Whether the provider verified the address: an invited user's was.
    expect(typeof claims.email_verified).toBe('boolean')
    // How the user signed in: with a password, and nothing else.
    expect(claims.amr).toEqual(['pwd'])
    // No assurance level is defined, so none is claimed.
    expect(claims.acr ?? '').toBe('')
  })

  it('keeps how the session was authenticated when the session is reused', async () => {
    // A second authorization in the same browser reuses the session: no
    // password is asked for, and the tokens still say the session was opened
    // with one — at the time it was.
    const first = await provider.login(client, user, { remember: true })
    if (first.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(first)}`)
    const again = await provider.reauthorize(first.browser, client)
    const before = claimsOf(first.tokens.id_token ?? '')
    const after = claimsOf(again.id_token ?? '')
    expect(after.amr).toEqual(['pwd'])
    expect(after.auth_time).toBe(before.auth_time)
  })
})
