import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, REDIRECT_URI, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())

const claimsOf = (jwt: string): Record<string, unknown> =>
  JSON.parse(Buffer.from(jwt.split('.')[1], 'base64url').toString('utf8')) as Record<string, unknown>

const patchClaims = (sub: string, claims: Record<string, unknown>) =>
  provider.admin(`/api/v1/users/${sub}/claims`, { method: 'PATCH', body: JSON.stringify({ claims }) })

// An application's backend stores what it decided about a user — a plan, a
// role — and the provider carries it in the user's tokens. Only a trusted
// party writes these claims: they may decide what the user is allowed to do,
// so the user's own token cannot.
describe('custom claims', () => {
  let client: TestClient
  let user: TestUser
  let sub: string
  let userToken: string

  beforeAll(async () => {
    const caps = await (await fetch(`${provider.config.api}/api/v1/capabilities`)).json()
    if (!caps.custom_claims) throw new Error('The provider does not offer custom claims (capabilities.custom_claims is false)')

    const res = await provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: `conformance-claims-${randomToken(4)}`,
        public: true,
        redirect_uris: [REDIRECT_URI],
        allowed_origins: ['http://localhost:9999'],
        grant_types: ['authorization_code', 'refresh_token'],
        scopes: ['openid', 'profile', 'email'],
        access_token_strategy: 'jwt',
      }),
    })
    expect(res.status, await res.clone().text()).toBe(201)
    const { client: created } = (await res.json()) as { client: { id: string; client_id: string } }
    client = { id: created.id, clientId: created.client_id }

    user = await provider.inviteAndAccept()
    const outcome = await provider.login(client, user)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    sub = outcome.sub
    userToken = outcome.tokens.access_token
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    if (client) await provider.deleteClient(client)
  })

  it('stores claims as a merge patch and answers with what is stored', async () => {
    const set = await patchClaims(sub, { plan: 'pro', org: 'acme' })
    await conform('PATCH', '/api/v1/users/{id}/claims', set.clone())
    expect(set.status, await set.clone().text()).toBe(200)
    expect(((await set.json()) as { claims: unknown }).claims).toEqual({ plan: 'pro', org: 'acme' })

    const mixed = await patchClaims(sub, { org: null, seats: 5 })
    expect(mixed.status).toBe(200)

    const read = await provider.admin(`/api/v1/users/${sub}/claims`)
    await conform('GET', '/api/v1/users/{id}/claims', read.clone())
    expect(read.status).toBe(200)
    expect(((await read.json()) as { claims: unknown }).claims).toEqual({ plan: 'pro', seats: 5 })
  })

  it('puts the stored claims in the ID token of the next authorization', async () => {
    const outcome = await provider.login(client, user)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    const claims = claimsOf(outcome.tokens.id_token ?? '')
    expect(claims.plan).toBe('pro')
    expect(claims.seats).toBe(5)
  })

  it('refuses a claim the provider asserts, and changes nothing', async () => {
    const res = await patchClaims(sub, { tenant_id: '00000000-0000-0000-0000-000000000000', email_verified: true, plan: 'free' })
    await conform('PATCH', '/api/v1/users/{id}/claims', res.clone())
    expect(res.status).toBe(400)
    const body = (await res.json()) as { code: string; keys: string[] }
    expect(body.code).toBe('reserved_claim')
    expect(body.keys).toEqual(['email_verified', 'tenant_id'])

    const read = await provider.admin(`/api/v1/users/${sub}/claims`)
    expect(((await read.json()) as { claims: Record<string, unknown> }).claims.plan).toBe('pro')
  })

  it("does not take the user's own token", async () => {
    const res = await fetch(`${provider.config.api}/api/v1/users/${sub}/claims`, {
      method: 'PATCH',
      headers: { Authorization: `Bearer ${userToken}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ claims: { role: 'admin' } }),
    })
    await conform('PATCH', '/api/v1/users/{id}/claims', res.clone())
    expect(res.status).toBe(401)
  })

  it('answers 404 for a user that does not exist', async () => {
    const res = await patchClaims('00000000-0000-0000-0000-000000000000', { plan: 'pro' })
    await conform('PATCH', '/api/v1/users/{id}/claims', res.clone())
    expect(res.status).toBe(404)
  })
})
