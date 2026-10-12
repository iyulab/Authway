import { createRemoteJWKSet, jwtVerify } from 'jose'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conformTo } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, REDIRECT_URI, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())

// An API that receives access tokens — a resource server — knows one thing
// about the provider: its issuer. Everything here uses a standard JOSE
// library and what discovery publishes, and nothing of any particular
// authorization server. The same code has to accept the tokens of every
// Authway provider.
describe('resource server', () => {
  let client: TestClient
  let user: TestUser
  let sub: string
  let accessToken: string
  let issuer: string
  let keys: ReturnType<typeof createRemoteJWKSet>

  beforeAll(async () => {
    const d = await provider.discovery()
    issuer = d.issuer
    keys = createRemoteJWKSet(new URL(d.jwks_uri))

    const res = await provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: `conformance-rs-${randomToken(4)}`,
        public: true,
        redirect_uris: [REDIRECT_URI],
        allowed_origins: ['http://localhost:9999'],
        grant_types: ['authorization_code', 'refresh_token'],
        scopes: ['openid', 'profile', 'email'],
        // The client asks for access tokens a resource server can verify.
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
    accessToken = outcome.tokens.access_token
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    if (client) await provider.deleteClient(client)
  })

  it('verifies the access token with the issuer and its published keys alone', async () => {
    const { payload, protectedHeader } = await jwtVerify(accessToken, keys, { issuer })

    // The claims are those the contract's resource server profile describes
    // — and the profile is not met by a token that lacks one of them, such as
    // the scopes as a string.
    conformTo('JwtAccessToken', payload)
    const { scope: _scope, ...withoutScope } = payload
    expect(() => conformTo('JwtAccessToken', withoutScope)).toThrow(/scope/)

    // Signed, not encrypted, with a key the JWKS names.
    expect(protectedHeader.alg).not.toBe('none')
    expect(protectedHeader.kid).toBeTruthy()

    // Who the token is about, who asked for it, and for what.
    expect(payload.sub).toBe(sub)
    expect(payload.client_id).toBe(client.clientId)
    expect(typeof payload.scope).toBe('string')
    expect((payload.scope as string).split(' ')).toContain('openid')
    expect(typeof payload.exp).toBe('number')
    expect(typeof payload.iat).toBe('number')

    // The provider's own claims, read by their bare names.
    expect(payload.tenant_id).toBe(await provider.tenantId())
    expect(payload.email).toBe(user.email)
    expect(typeof payload.auth_time).toBe('number')
  })

  it('does not verify a token whose claims were changed', async () => {
    const [header, body, signature] = accessToken.split('.')
    const claims = JSON.parse(Buffer.from(body, 'base64url').toString('utf8')) as Record<string, unknown>
    claims.tenant_id = '00000000-0000-0000-0000-000000000000'
    const forged = [header, Buffer.from(JSON.stringify(claims)).toString('base64url'), signature].join('.')

    await expect(jwtVerify(forged, keys, { issuer })).rejects.toThrow()
  })

  it('does not verify the token as another issuer\'s', async () => {
    await expect(jwtVerify(accessToken, keys, { issuer: 'https://another-issuer.example' })).rejects.toThrow()
  })
})
