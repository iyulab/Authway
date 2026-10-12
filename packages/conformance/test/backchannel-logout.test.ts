import http from 'node:http'
import type { AddressInfo } from 'node:net'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Browser } from '../src/browser.js'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, POST_LOGOUT_REDIRECT_URI, REDIRECT_URI, type TestClient, type TestUser, type Tokens } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

const claimsOf = (jwt: string): Record<string, unknown> =>
  JSON.parse(Buffer.from(jwt.split('.')[1], 'base64url').toString('utf8')) as Record<string, unknown>

const LOGOUT_EVENT = 'http://schemas.openid.net/event/backchannel-logout'

// An application that keeps its own session on a server is not in the browser
// when the user signs out somewhere else. It registers an address, and the
// provider tells it there — with a logout token naming the session that ended
// (OpenID Connect Back-Channel Logout 1.0).
describe('back-channel logout', () => {
  const received: { app: string; claims: Record<string, unknown> }[] = []
  let receiver: http.Server
  let apps: Record<'a' | 'b', TestClient>
  let user: TestUser
  let sub: string

  /** Waits until both applications have been told, or gives up. */
  const bothTold = async (sid: string): Promise<typeof received> => {
    for (let i = 0; i < 50; i++) {
      const told = received.filter((r) => r.claims.sid === sid)
      if (new Set(told.map((r) => r.app)).size === 2) return told
      await new Promise((r) => setTimeout(r, 200))
    }
    return received.filter((r) => r.claims.sid === sid)
  }

  /** Signs the user in to both applications in one browser session. */
  const signInToBoth = async (): Promise<{ browser: Browser; a: Tokens; b: Tokens; sid: string }> => {
    const browser = new Browser()
    const attempt = await provider.startLogin(apps.a, ['openid'], browser)
    // An ordinary sign-in: the second application is signed in on its session.
    const outcome = await provider.continueLogin(attempt, user)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    sub = outcome.sub
    const b = await provider.reauthorize(browser, apps.b, ['openid'])
    return { browser, a: outcome.tokens, b, sid: claimsOf(outcome.tokens.id_token ?? '').sid as string }
  }

  beforeAll(async () => {
    receiver = http.createServer((req, res) => {
      let body = ''
      req.on('data', (chunk) => (body += chunk))
      req.on('end', () => {
        const token = new URLSearchParams(body).get('logout_token')
        if (req.method === 'POST' && token) received.push({ app: (req.url ?? '').slice(-1), claims: claimsOf(token) })
        res.writeHead(200).end()
      })
    })
    await new Promise<void>((resolve) => receiver.listen(0, '0.0.0.0', resolve))
    const base = `http://${config.callbackHost}:${(receiver.address() as AddressInfo).port}`

    const register = async (app: string): Promise<TestClient> => {
      const res = await provider.admin('/api/v1/clients', {
        method: 'POST',
        body: JSON.stringify({
          tenant_id: await provider.tenantId(),
          name: `conformance-bcl-${app}-${randomToken(4)}`,
          public: true,
          redirect_uris: [REDIRECT_URI],
          post_logout_redirect_uris: [POST_LOGOUT_REDIRECT_URI],
          allowed_origins: ['http://localhost:9999'],
          grant_types: ['authorization_code'],
          scopes: ['openid'],
          backchannel_logout_uri: `${base}/backchannel-logout/${app}`,
        }),
      })
      await conform('POST', '/api/v1/clients', res.clone())
      expect(res.status, await res.clone().text()).toBe(201)
      const { client } = (await res.json()) as { client: { id: string; client_id: string; backchannel_logout_uri: string } }
      expect(client.backchannel_logout_uri).toBe(`${base}/backchannel-logout/${app}`)
      return { id: client.id, clientId: client.client_id }
    }
    apps = { a: await register('a'), b: await register('b') }
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    for (const app of Object.values(apps ?? {})) await provider.deleteClient(app)
    await new Promise((resolve) => receiver?.close(resolve))
  })

  // The provider calls the address from inside its own network. However far
  // a deployment lets that reach — this one reaches the suite's receiver at a
  // private address — an address that is no one's server is never one of
  // them: a link-local address is where cloud platforms answer for the
  // machine itself.
  it('refuses to be pointed at an address that is no one\'s server', async () => {
    for (const uri of ['http://169.254.169.254/latest/meta-data/', 'https://169.254.169.254/backchannel-logout']) {
      const res = await provider.admin('/api/v1/clients', {
        method: 'POST',
        body: JSON.stringify({
          tenant_id: await provider.tenantId(),
          name: `conformance-bcl-refused-${randomToken(4)}`,
          public: true,
          redirect_uris: [REDIRECT_URI],
          allowed_origins: ['http://localhost:9999'],
          grant_types: ['authorization_code'],
          scopes: ['openid'],
          backchannel_logout_uri: uri,
        }),
      })
      await conform('POST', '/api/v1/clients', res.clone())
      const body = (await res.json()) as { code?: string; client?: { id: string; client_id: string } }
      // Nothing may be left registered if the provider wrongly took it.
      if (body.client) await provider.deleteClient({ id: body.client.id, clientId: body.client.client_id })
      expect(res.status, `${uri}: ${JSON.stringify(body)}`).toBe(400)
      expect(body.code).toBe('invalid_backchannel_logout_uri')
    }

    // The same on a change to a client that exists.
    const res = await provider.admin(`/api/v1/clients/${apps.a.id}`, {
      method: 'PUT',
      body: JSON.stringify({ backchannel_logout_uri: 'http://169.254.169.254/latest/meta-data/' }),
    })
    await conform('PUT', '/api/v1/clients/{id}', res.clone())
    expect(res.status, await res.clone().text()).toBe(400)
    expect(((await res.json()) as { code?: string }).code).toBe('invalid_backchannel_logout_uri')
  })

  it('says it tells applications about a logout', async () => {
    const res = await fetch(`${config.api}/api/v1/capabilities`)
    await conform('GET', '/api/v1/capabilities', res.clone())
    expect(((await res.json()) as { backchannel_logout: boolean }).backchannel_logout).toBe(true)
  })

  it('names the session in the ID token, the same for every application signed in on it', async () => {
    const { a, b, sid } = await signInToBoth()
    expect(typeof sid).toBe('string')
    expect(sid).toBeTruthy()
    expect(claimsOf(b.id_token ?? '').sid).toBe(sid)
    expect(claimsOf(a.id_token ?? '').sid).toBe(sid)
  })

  it('tells every application signed in on the session when the user signs out of one', async () => {
    const { browser, a, sid } = await signInToBoth()
    await provider.rpInitiatedLogout(browser, a.id_token ?? '')

    const told = await bothTold(sid)
    expect(told.map((t) => t.app).sort()).toEqual(['a', 'b'])
    for (const { app, claims } of told) {
      expect(claims.iss, app).toBe(config.issuer)
      expect(claims.aud, app).toContain(apps[app as 'a' | 'b'].clientId)
      expect(claims.events, app).toHaveProperty([LOGOUT_EVENT])
      expect(claims.sid, app).toBe(sid)
      // A logout token must never be usable as an ID token.
      expect(claims.nonce, app).toBeUndefined()
    }
  })

  // Ending the user's sessions from the API — signing out everywhere — is a
  // logout like any other to the applications.
  it('tells them when the user signs out everywhere', async () => {
    const { a, sid } = await signInToBoth()
    const res = await provider.logout(a.access_token)
    expect(res.status, await res.clone().text()).toBe(200)

    const told = await bothTold(sid)
    expect(told.map((t) => t.app).sort()).toEqual(['a', 'b'])
  })
})
