import { afterAll, describe, expect, it } from 'vitest'
import { exportJWK, generateKeyPair, SignJWT, type JWK } from 'jose'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider } from '../src/provider.js'

const provider = new Provider(loadConfig())

type Created = { client: { id: string; client_id: string; token_endpoint_auth_method: string; jwks?: { keys: JWK[] }; jwks_uri?: string }; credentials: { client_id: string; client_secret: string } }
type Refusal = { code: string; field?: string }

/** A key pair, and its public half as the registration takes it. */
const newKey = async (kid: string, alg = 'RS256') => {
  const { publicKey, privateKey } = await generateKeyPair(alg, { extractable: true })
  const bareJwk = { ...(await exportJWK(publicKey)), kid } as JWK
  return { privateKey, kid, alg, bareJwk, publicJwk: { ...bareJwk, use: 'sig', alg } as JWK, privateJwk: await exportJWK(privateKey) }
}

// A client that can keep a key proves who it is at the token endpoint with a
// signed assertion (RFC 7523 — `private_key_jwt`), so that no secret is
// shared with the provider or kept where the client runs.
describe('clients: authenticating with a key instead of a secret', () => {
  const created: string[] = []

  afterAll(async () => {
    for (const id of created) await provider.admin(`/api/v1/clients/${id}`, { method: 'DELETE' })
  })

  const register = async (body: Record<string, unknown>) =>
    provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: `conformance-key-${randomToken(4)}`,
        public: false,
        grant_types: ['client_credentials'],
        scopes: ['openid'],
        ...body,
      }),
    })

  const registered = async (body: Record<string, unknown>): Promise<Created> => {
    const res = await register(body)
    await conform('POST', '/api/v1/clients', res.clone())
    expect(res.status, await res.clone().text()).toBe(201)
    const out = (await res.json()) as Created
    created.push(out.client.id)
    return out
  }

  /** Asks the token endpoint for a token, authenticating with an assertion signed by `key`. */
  const tokenWith = async (clientId: string, key: { privateKey: CryptoKey | Uint8Array; kid: string; alg?: string }) => {
    const { token_endpoint } = await provider.discovery()
    const assertion = await new SignJWT({})
      .setProtectedHeader({ alg: key.alg ?? 'RS256', kid: key.kid })
      .setIssuer(clientId)
      .setSubject(clientId)
      .setAudience(token_endpoint)
      .setJti(randomToken(16))
      .setIssuedAt()
      .setExpirationTime('2m')
      .sign(key.privateKey as CryptoKey)
    return fetch(token_endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        grant_type: 'client_credentials',
        client_id: clientId,
        client_assertion_type: 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer',
        client_assertion: assertion,
      }),
    })
  }

  it('is offered by the provider', async () => {
    const d = (await provider.discovery()) as unknown as { token_endpoint_auth_methods_supported: string[] }
    expect(d.token_endpoint_auth_methods_supported).toContain('private_key_jwt')
  })

  it('issues a token for an assertion signed with the registered key, and for no other', async () => {
    const key = await newKey('k1')
    const { client, credentials } = await registered({ token_endpoint_auth_method: 'private_key_jwt', jwks: { keys: [key.publicJwk] } })

    // The registration says how the client authenticates, and holds no secret.
    expect(client.token_endpoint_auth_method).toBe('private_key_jwt')
    expect(client.jwks?.keys.map((k) => k.kid)).toEqual(['k1'])
    expect(credentials.client_secret).toBe('')

    const issued = await tokenWith(client.client_id, key)
    expect(issued.status, await issued.clone().text()).toBe(200)
    expect(((await issued.json()) as { access_token: string }).access_token).toBeTruthy()

    const other = await newKey('k1')
    const refused = await tokenWith(client.client_id, other)
    expect(refused.status).toBe(401)
    expect(((await refused.json()) as { error: string }).error).toBe('invalid_client')
  })

  it('takes a new key in place of the old one', async () => {
    const first = await newKey('2026-a')
    const { client } = await registered({ token_endpoint_auth_method: 'private_key_jwt', jwks: { keys: [first.publicJwk] } })
    expect((await tokenWith(client.client_id, first)).status).toBe(200)

    const second = await newKey('2026-b')
    const updated = await provider.admin(`/api/v1/clients/${client.id}`, { method: 'PUT', body: JSON.stringify({ jwks: { keys: [second.publicJwk] } }) })
    await conform('PUT', '/api/v1/clients/{id}', updated.clone())
    expect(updated.status, await updated.clone().text()).toBe(200)

    expect((await tokenWith(client.client_id, second)).status).toBe(200)
    // The old key is refused: as an unknown key (400) where the engine goes
    // by the key id, as a failed authentication (401) where it tries the
    // keys it has. Either way no token is issued.
    const old = await tokenWith(client.client_id, first)
    expect([400, 401]).toContain(old.status)
    expect(((await old.json()) as { access_token?: string }).access_token).toBeUndefined()
  })

  it('signs with the algorithm its keys sign with: an EC key, a key that names none, a named one', async () => {
    const ec = await newKey('ec', 'ES256')
    const byEc = await registered({ token_endpoint_auth_method: 'private_key_jwt', jwks: { keys: [ec.bareJwk] } })
    expect(byEc.client).toMatchObject({ token_endpoint_auth_signing_alg: 'ES256' })
    const issued = await tokenWith(byEc.client.client_id, ec)
    expect(issued.status, await issued.clone().text()).toBe(200)

    const rsa = await newKey('rsa')
    const byRsa = await registered({ token_endpoint_auth_method: 'private_key_jwt', jwks: { keys: [rsa.bareJwk] } })
    expect(byRsa.client).toMatchObject({ token_endpoint_auth_signing_alg: 'RS256' })
    expect((await tokenWith(byRsa.client.client_id, rsa)).status).toBe(200)

    const pss = await newKey('pss', 'PS256')
    const byPss = await registered({ token_endpoint_auth_method: 'private_key_jwt', token_endpoint_auth_signing_alg: 'PS256', jwks: { keys: [pss.bareJwk] } })
    expect((await tokenWith(byPss.client.client_id, pss)).status).toBe(200)
    // Checked with that one only.
    const rs = await tokenWith(byPss.client.client_id, { ...pss, alg: 'RS256', privateKey: (await newKey('pss')).privateKey })
    expect([400, 401]).toContain(rs.status)
  })

  it('refuses keys that sign with different algorithms, or an algorithm they do not sign with', async () => {
    const ec = await newKey('ec', 'ES256')
    const rsa = await newKey('rsa')
    for (const [what, body, code] of [
      ['keys of two kinds', { jwks: { keys: [ec.bareJwk, rsa.bareJwk] } }, 'invalid_token_endpoint_auth_signing_alg'],
      ['an RSA algorithm for an EC key', { token_endpoint_auth_signing_alg: 'RS256', jwks: { keys: [ec.bareJwk] } }, 'invalid_token_endpoint_auth_signing_alg'],
      ['a key for encryption', { jwks: { keys: [{ ...rsa.bareJwk, use: 'enc' }] } }, 'invalid_jwks'],
    ] as [string, Record<string, unknown>, string][]) {
      const res = await register({ token_endpoint_auth_method: 'private_key_jwt', ...body })
      await conform('POST', '/api/v1/clients', res.clone())
      const answer = (await res.clone().json().catch(() => ({}))) as Refusal & { client?: { id: string } }
      if (answer.client?.id) created.push(answer.client.id)
      expect(res.status, what).toBe(400)
      expect(answer.code, what).toBe(code)
    }
  })

  it('says how every client authenticates', async () => {
    const withSecret = await registered({})
    expect(withSecret.client.token_endpoint_auth_method).toBe('client_secret_post')
    expect(withSecret.credentials.client_secret).toBeTruthy()
  })

  it('refuses a registration that could not work, naming the field', async () => {
    const key = await newKey('k1')
    const cases: [string, Record<string, unknown>, string][] = [
      ['a key method without a key', { token_endpoint_auth_method: 'private_key_jwt' }, 'invalid_jwks'],
      ['a key set and an address at once', { token_endpoint_auth_method: 'private_key_jwt', jwks: { keys: [key.publicJwk] }, jwks_uri: 'https://app.example.com/jwks.json' }, 'invalid_jwks'],
      ['a private key', { token_endpoint_auth_method: 'private_key_jwt', jwks: { keys: [{ ...key.privateJwk, kid: 'k1' }] } }, 'invalid_jwks'],
      ['an empty key set', { token_endpoint_auth_method: 'private_key_jwt', jwks: { keys: [] } }, 'invalid_jwks'],
      ['a key for a client that has a secret', { jwks: { keys: [key.publicJwk] } }, 'invalid_jwks'],
      ['a secret for a client that has a key', { token_endpoint_auth_method: 'private_key_jwt', jwks: { keys: [key.publicJwk] }, client_id: `conformance-${randomToken(6)}`, client_secret: 'a-secret-nobody-should-need' }, 'invalid_token_endpoint_auth_method'],
      ['a key method for a public client', { public: true, grant_types: ['authorization_code'], redirect_uris: ['http://127.0.0.1/callback'], token_endpoint_auth_method: 'private_key_jwt', jwks: { keys: [key.publicJwk] } }, 'invalid_token_endpoint_auth_method'],
      ['a method that is not offered', { token_endpoint_auth_method: 'client_secret_jwt' }, 'invalid_token_endpoint_auth_method'],
      ['a key address that is not a public https address', { token_endpoint_auth_method: 'private_key_jwt', jwks_uri: 'http://169.254.169.254/keys' }, 'invalid_jwks_uri'],
    ]
    for (const [what, body, code] of cases) {
      const res = await register(body)
      await conform('POST', '/api/v1/clients', res.clone())
      const answer = (await res.clone().json().catch(() => ({}))) as Refusal & { client?: { id: string } }
      if (answer.client?.id) created.push(answer.client.id)
      expect(res.status, what).toBe(400)
      expect(answer.code, what).toBe(code)
    }
  })

  it('does not let a client change how it authenticates, nor take a secret it has no use for', async () => {
    const key = await newKey('k1')
    const { client } = await registered({ token_endpoint_auth_method: 'private_key_jwt', jwks: { keys: [key.publicJwk] } })

    const changed = await provider.admin(`/api/v1/clients/${client.id}`, { method: 'PUT', body: JSON.stringify({ token_endpoint_auth_method: 'client_secret_post' }) })
    expect(changed.status).toBe(400)
    expect(((await changed.json()) as Refusal).code).toBe('token_endpoint_auth_method_is_fixed')

    const secret = await provider.admin(`/api/v1/clients/${client.id}/regenerate-secret`, { method: 'POST' })
    expect(secret.status).toBe(400)
    expect(((await secret.json()) as Refusal).code).toBe('client_has_no_secret')

    // Still the client it was.
    expect((await tokenWith(client.client_id, key)).status).toBe(200)
  })
})
