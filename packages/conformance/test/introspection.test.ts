import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)
const PATH = '/api/v1/oauth2/introspect'

// A resource server that validates JWT access tokens itself keeps accepting a
// token until it expires. One that must know at once that a user signed out
// asks the provider about each token instead (RFC 7662), as a confidential
// client of the same tenant.
describe('token introspection', () => {
  let app: TestClient
  let resourceServer: TestClient & { secret: string }
  let user: TestUser
  let sub: string
  let accessToken: string

  const introspect = (token: string, id = resourceServer.clientId, secret = resourceServer.secret): Promise<Response> =>
    fetch(`${config.api}${PATH}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        Authorization: `Basic ${Buffer.from(`${encodeURIComponent(id)}:${encodeURIComponent(secret)}`).toString('base64')}`,
      },
      body: new URLSearchParams({ token }),
    })

  beforeAll(async () => {
    const res = await provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: `conformance-rs-${randomToken(4)}`,
        public: false,
        grant_types: ['client_credentials'],
        scopes: ['openid'],
      }),
    })
    expect(res.status, await res.clone().text()).toBe(201)
    const created = (await res.json()) as { client: { id: string; client_id: string }; credentials: { client_secret: string } }
    resourceServer = { id: created.client.id, clientId: created.client.client_id, secret: created.credentials.client_secret }

    app = await provider.createPublicClient()
    user = await provider.inviteAndAccept()
    const outcome = await provider.login(app, user)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    sub = outcome.sub
    accessToken = outcome.tokens.access_token
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    for (const client of [app, resourceServer]) if (client) await provider.deleteClient(client)
  })

  it('says it answers for tokens', async () => {
    const res = await fetch(`${config.api}/api/v1/capabilities`)
    expect(((await res.json()) as { token_introspection: boolean }).token_introspection).toBe(true)
  })

  it('answers the same whatever kind of token the caller says it is', async () => {
    // RFC 7662 lets a caller hint at the kind of token; a server may ignore
    // the hint, but must not answer differently for a wrong one.
    const hinted = (hint: string) =>
      fetch(`${config.api}${PATH}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          Authorization: `Basic ${Buffer.from(`${encodeURIComponent(resourceServer.clientId)}:${encodeURIComponent(resourceServer.secret)}`).toString('base64')}`,
        },
        body: new URLSearchParams({ token: accessToken, token_type_hint: hint }),
      })
    for (const hint of ['access_token', 'refresh_token']) {
      const res = await hinted(hint)
      await conform('POST', PATH, res.clone())
      expect(res.status, hint).toBe(200)
      const body = (await res.json()) as { active: boolean; sub: string }
      expect(body.active, hint).toBe(true)
      expect(body.sub, hint).toBe(sub)
    }
  })

  it("tells a resource server of the tenant who a user's token belongs to", async () => {
    const res = await introspect(accessToken)
    await conform('POST', PATH, res.clone())
    expect(res.status).toBe(200)
    const body = (await res.json()) as Record<string, unknown>
    expect(body.active).toBe(true)
    expect(body.sub).toBe(sub)
    expect(body.client_id).toBe(app.clientId)
    expect(body.email).toBe(user.email)
    expect(body.tenant_id).toBe(await provider.tenantId())
    expect(typeof body.exp).toBe('number')
    expect(String(body.scope).split(' ')).toContain('openid')
  })

  it('answers a token it does not know as inactive, and nothing more', async () => {
    const res = await introspect(`not-a-token-${randomToken(8)}`)
    await conform('POST', PATH, res.clone())
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({ active: false })
  })

  it('answers only a confidential client that proves who it is', async () => {
    for (const [id, secret] of [
      [resourceServer.clientId, `${resourceServer.secret}x`],
      [app.clientId, 'a-public-client-has-no-secret'],
      [`nobody-${randomToken(4)}`, 'x'],
    ]) {
      const res = await introspect(accessToken, id, secret)
      await conform('POST', PATH, res.clone())
      expect(res.status, id).toBe(401)
      expect(((await res.json()) as { error: string }).error, id).toBe('invalid_client')
    }
  })

  // The point of asking: the answer changes the moment the user signs out,
  // while the same token as a JWT would have stayed valid until it expired.
  it('answers inactive as soon as the user has signed out everywhere', async () => {
    expect(((await (await introspect(accessToken)).json()) as { active: boolean }).active).toBe(true)
    const out = await provider.logout(accessToken)
    expect(out.status, await out.clone().text()).toBe(200)
    expect(await (await introspect(accessToken)).json()).toEqual({ active: false })
  })
})
