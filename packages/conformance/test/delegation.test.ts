import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { conform } from '../src/contract.js'
import { loadConfig } from '../src/config.js'
import { randomToken } from '../src/pkce.js'
import { Provider, REDIRECT_URI, TOKEN_EXCHANGE_GRANT, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())
const api = provider.config.api

const claimsOf = (jwt: string): Record<string, unknown> => {
  const parts = jwt.split('.')
  if (parts.length !== 3) throw new Error(`Not a JWT (${parts.length} parts): ${jwt.slice(0, 24)}…`)
  return JSON.parse(Buffer.from(parts[1], 'base64url').toString('utf8')) as Record<string, unknown>
}

interface Delegation {
  id: string
  user_id: string
  client_id: string
  client_name: string
  scopes: string[]
  created_at: string
}

const adminDelegations = async (query: Record<string, string>): Promise<Delegation[]> => {
  const res = await provider.admin(`/api/v1/delegations?${new URLSearchParams(query)}`)
  await conform('GET', '/api/v1/delegations', res.clone())
  expect(res.status).toBe(200)
  return ((await res.json()) as { delegations: Delegation[] }).delegations
}

const ownDelegations = async (accessToken: string): Promise<Delegation[]> => {
  const res = await fetch(`${api}/api/v1/users/delegations`, { headers: { Authorization: `Bearer ${accessToken}` } })
  await conform('GET', '/api/v1/users/delegations', res.clone())
  expect(res.status).toBe(200)
  return ((await res.json()) as { delegations: Delegation[] }).delegations
}

// A client that acts for a user while they are away — an agent, a background
// job — is let do so by the user: they consent to its `delegation` scope, and
// the client receives a handle it later exchanges for the user's tokens. These
// scenarios cover granting, seeing and revoking that permission; exchanging it
// is offered when capabilities.token_exchange is true.
describe('delegations users grant to clients', () => {
  let agent: TestClient
  let account: TestClient
  let user: TestUser

  // The user sees and revokes their delegations from an application of their
  // own (an account screen), not from the client they delegated to: revoking
  // a delegation also ends that client's tokens for them.
  const accountToken = async (u: TestUser): Promise<string> => {
    const outcome = await provider.login(account, u, { scopes: ['openid'] })
    if (outcome.kind !== 'code') throw new Error(`signing in to the account application failed: ${JSON.stringify(outcome)}`)
    return outcome.tokens.access_token
  }

  beforeAll(async () => {
    agent = await provider.createDelegatingClient()
    account = await provider.createPublicClient()
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    if (agent) await provider.deleteClient(agent)
    if (account) await provider.deleteClient(account)
  })

  it('are asked for only by a confidential client that may exchange them', async () => {
    const register = (body: Record<string, unknown>) =>
      provider.admin('/api/v1/clients', {
        method: 'POST',
        body: JSON.stringify({
          name: `conformance-refused-${randomToken(4)}`,
          redirect_uris: [REDIRECT_URI],
          allowed_origins: ['http://localhost:9999'],
          ...body,
        }),
      })
    const tenant_id = await provider.tenantId()
    const cases: [string, Record<string, unknown>, string][] = [
      ['a public client with the exchange grant', { tenant_id, public: true, grant_types: ['authorization_code', TOKEN_EXCHANGE_GRANT], scopes: ['openid'] }, 'public_client_with_token_exchange'],
      ['the delegation scope without the exchange grant', { tenant_id, public: false, grant_types: ['authorization_code'], scopes: ['openid', 'delegation'] }, 'delegation_without_token_exchange'],
    ]
    for (const [name, body, code] of cases) {
      const res = await register(body)
      await conform('POST', '/api/v1/clients', res.clone())
      expect(res.status, name).toBe(400)
      expect(((await res.json()) as { code: string }).code, name).toBe(code)
    }
  })

  it('are granted by consenting to the delegation scope, and the ID token carries the handle', async () => {
    const outcome = await provider.login(agent, user, { scopes: ['openid', 'profile', 'email', 'delegation'], rememberConsent: true })
    expect(outcome.kind).toBe('code')
    if (outcome.kind !== 'code') return
    const handle = claimsOf(outcome.tokens.id_token ?? '').delegation_grant
    expect(typeof handle).toBe('string')
    expect((handle as string).length).toBeGreaterThanOrEqual(43)

    const [granted] = await adminDelegations({ client_id: agent.clientId })
    expect(granted).toMatchObject({ user_id: outcome.sub, client_id: agent.clientId })
    expect([...granted.scopes].sort()).toEqual(['email', 'openid', 'profile'])
    expect(await ownDelegations(await accountToken(user))).toEqual([granted])
  })

  it('are not granted when the user does not consent to the delegation scope', async () => {
    const other = await provider.inviteAndAccept()
    const outcome = await provider.login(agent, other, { scopes: ['openid', 'profile', 'email'] })
    expect(outcome.kind).toBe('code')
    if (outcome.kind !== 'code') return
    expect(claimsOf(outcome.tokens.id_token ?? '').delegation_grant).toBeUndefined()
    expect(await adminDelegations({ client_id: agent.clientId, user_id: outcome.sub })).toEqual([])
    expect(await ownDelegations(await accountToken(other))).toEqual([])
  })

  it('end when the user revokes them, and the client must ask again before it is granted another', async () => {
    const browserLogin = await provider.login(agent, user, { scopes: ['openid', 'delegation'], rememberConsent: true })
    expect(browserLogin.kind).toBe('code')
    if (browserLogin.kind !== 'code') return
    const token = await accountToken(user)
    const mine = await ownDelegations(token)
    expect(mine.length).toBeGreaterThan(0)

    // Another user cannot revoke it.
    const strangerToken = await accountToken(await provider.inviteAndAccept())
    const foreign = await fetch(`${api}/api/v1/users/delegations/${mine[0].id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${strangerToken}` } })
    await conform('DELETE', '/api/v1/users/delegations/{id}', foreign.clone())
    expect(foreign.status).toBe(404)

    for (const d of mine) {
      const res = await fetch(`${api}/api/v1/users/delegations/${d.id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } })
      await conform('DELETE', '/api/v1/users/delegations/{id}', res.clone())
      expect(res.status).toBe(204)
    }
    expect(await adminDelegations({ client_id: agent.clientId, user_id: browserLogin.sub })).toEqual([])

    // The answer had been remembered; once revoked, the user is asked again.
    const fresh = await provider.login(agent, user, { scopes: ['openid', 'delegation'] })
    expect(fresh.kind).toBe('code')
    if (fresh.kind !== 'code') return
    expect(fresh.consent).toBe('asked')
  })

  it('are revoked by an administrator of the tenant', async () => {
    const outcome = await provider.login(agent, user, { scopes: ['openid', 'delegation'] })
    if (outcome.kind !== 'code') throw new Error('sign-in failed')
    const [latest] = await adminDelegations({ client_id: agent.clientId, user_id: outcome.sub })
    const res = await provider.admin(`/api/v1/delegations/${latest.id}`, { method: 'DELETE' })
    await conform('DELETE', '/api/v1/delegations/{id}', res.clone())
    expect(res.status).toBe(204)
    const again = await provider.admin(`/api/v1/delegations/${latest.id}`, { method: 'DELETE' })
    expect(again.status).toBe(404)
  })

  it('are asked for even by a client registered to skip consent', async () => {
    const trusted = await provider.createDelegatingClient({ skipConsent: true })
    try {
      const plain = await provider.login(trusted, user, { scopes: ['openid'] })
      expect(plain.kind === 'code' && plain.consent).not.toBe('asked')
      const delegating = await provider.login(trusted, user, { scopes: ['openid', 'delegation'] })
      expect(delegating.kind === 'code' && delegating.consent).toBe('asked')
    } finally {
      await provider.deleteClient(trusted)
    }
  })
})

// The consent screen sends the scopes the user approved. What it sends beyond
// the request is not granted.
describe('a consent', () => {
  it('grants only scopes the client asked for', async () => {
    const client = await provider.createPublicClient()
    try {
      const user = await provider.inviteAndAccept()
      const attempt = await provider.startLogin(client, ['openid'])
      const outcome = await provider.continueLogin({ ...attempt, scopes: ['openid', 'email', 'profile'] }, user)
      expect(outcome.kind).toBe('code')
      if (outcome.kind !== 'code') return
      expect((outcome.tokens.scope ?? '').split(' ')).toEqual(['openid'])
    } finally {
      await provider.deleteClient(client)
    }
  })
})
