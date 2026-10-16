import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestClient } from '../src/provider.js'

const provider = new Provider(loadConfig())
const api = provider.config.api

// A browser application runs on its own origin. Registering that origin on its
// client (`allowed_origins`) is what lets its pages call the provider — no
// change to the deployment. These scenarios ask the questions a browser asks.
describe('browser applications and their origins', () => {
  const registered = `https://app-${randomToken(6).toLowerCase()}.example.org`
  const stranger = `https://stranger-${randomToken(6).toLowerCase()}.example.net`
  let client: TestClient

  const preflight = (path: string, origin: string, method = 'GET'): Promise<Response> =>
    fetch(`${api}${path}`, {
      method: 'OPTIONS',
      headers: {
        Origin: origin,
        'Access-Control-Request-Method': method,
        'Access-Control-Request-Headers': 'authorization',
      },
    })

  beforeAll(async () => {
    const res = await provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: `conformance-origins-${randomToken(4)}`,
        public: true,
        redirect_uris: [`${registered}/callback`],
        allowed_origins: [registered],
        grant_types: ['authorization_code'],
        scopes: ['openid'],
      }),
    })
    expect(res.status, await res.clone().text()).toBe(201)
    const { client: created } = (await res.json()) as { client: { id: string; client_id: string } }
    client = { id: created.id, clientId: created.client_id }
  })

  afterAll(async () => {
    if (client) await provider.deleteClient(client)
  })

  // What an application reads before anyone has signed in says nothing about
  // a user, so a page on any origin may read it.
  it('lets a page on any origin read the documents that describe the deployment', async () => {
    for (const path of ['/.well-known/authway-config', '/api/v1/capabilities']) {
      const res = await fetch(`${api}${path}`, { headers: { Origin: stranger } })
      expect(res.status, path).toBe(200)
      expect(res.headers.get('access-control-allow-origin'), path).toBe('*')
      expect(res.headers.get('access-control-allow-credentials'), path).toBeNull()
    }
  })

  // A browser application exchanges its code at the token endpoint discovery
  // names (PKCE, no secret). The origin decides nothing there — the client
  // authenticates in the request — so a page on any origin reads the answer,
  // a refusal included.
  it('lets a page on any origin read what the token endpoint answers', async () => {
    const { token_endpoint } = await provider.discovery()
    const pre = await fetch(token_endpoint, {
      method: 'OPTIONS',
      headers: { Origin: stranger, 'Access-Control-Request-Method': 'POST', 'Access-Control-Request-Headers': 'content-type' },
    })
    expect(pre.status).toBeLessThan(300)
    expect([stranger, '*']).toContain(pre.headers.get('access-control-allow-origin'))

    const res = await fetch(token_endpoint, {
      method: 'POST',
      headers: { Origin: stranger, 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ grant_type: 'authorization_code', code: 'not-a-code', client_id: client.clientId, redirect_uri: `${registered}/callback`, code_verifier: randomToken(43) }),
    })
    expect(res.status).toBe(400)
    expect([stranger, '*']).toContain(res.headers.get('access-control-allow-origin'))
    expect(((await res.json()) as { error: string }).error).toBe('invalid_grant')
  })

  it("lets a registered origin call the routes a user calls with their own token", async () => {
    for (const [path, method] of [
      ['/api/v1/profile/me', 'GET'],
      ['/api/v1/profile/me', 'DELETE'],
      ['/api/v1/logout', 'POST'],
    ] as const) {
      const res = await preflight(path, registered, method)
      expect(res.headers.get('access-control-allow-origin'), `${method} ${path}`).toBe(registered)
      expect((res.headers.get('access-control-allow-headers') ?? '').toLowerCase(), `${method} ${path}`).toContain('authorization')
      // The token is the credential. Cookies are not sent from an application's origin.
      expect(res.headers.get('access-control-allow-credentials'), `${method} ${path}`).toBeNull()
    }

    // The answer carries the header as well, or the page could not read it —
    // here a refusal, since no token is presented.
    const res = await fetch(`${api}/api/v1/profile/me`, { headers: { Origin: registered } })
    expect(res.status).toBe(401)
    expect(res.headers.get('access-control-allow-origin')).toBe(registered)
  })

  it('does not admit an origin no client registered', async () => {
    for (const path of ['/api/v1/profile/me', '/api/v1/logout']) {
      const res = await preflight(path, stranger)
      expect(res.headers.get('access-control-allow-origin'), path).toBeNull()
    }
  })

  // Administration is done from the provider's own console or from a backend,
  // never from an application's pages.
  it('does not admit a registered origin to the admin API', async () => {
    for (const path of ['/api/v1/clients', '/api/v1/users', '/api/v1/webhooks']) {
      const res = await preflight(path, registered)
      expect(res.headers.get('access-control-allow-origin'), path).toBeNull()
    }
  })
})
