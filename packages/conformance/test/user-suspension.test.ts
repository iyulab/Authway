import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, REDIRECT_URI, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

const WITH_REFRESH = ['openid', 'profile', 'email', 'offline_access']

// An administrator suspends an account to stop it signing in without deleting
// it: its sessions end, and signing in is refused until the suspension is
// lifted. The account is kept as it was.
describe('suspending a user', () => {
  let client: TestClient
  let user: TestUser
  let sub: string | undefined

  beforeAll(async () => {
    client = await provider.createPublicClient({ scopes: WITH_REFRESH })
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    if (client) await provider.deleteClient(client)
  })

  const setActive = async (active: boolean): Promise<{ active: boolean }> => {
    const res = await provider.admin(`/api/v1/users/${sub}`, { method: 'PUT', body: JSON.stringify({ active }) })
    await conform('PUT', '/api/v1/users/{id}', res)
    expect(res.status).toBe(200)
    return ((await res.json()) as { user: { active: boolean } }).user
  }

  it('ends its sessions, refuses its sign-in, and lets it back in when lifted', async () => {
    const before = await provider.login(client, user, { scopes: WITH_REFRESH })
    if (before.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(before)}`)
    sub = before.sub
    expect(before.tokens.refresh_token).toBeTruthy()

    expect((await setActive(false)).active).toBe(false)

    // The session it had is over: its refresh token no longer works.
    const renewed = await provider.refresh(client, before.tokens.refresh_token!)
    expect(renewed.status).toBe(400)

    // A wrong password says nothing about the account; the right one is refused for it.
    const attempt = await provider.startLogin(client, WITH_REFRESH)
    const wrong = await provider.submitPassword(attempt.browser, attempt.flow, { ...user, password: `${user.password}-wrong` })
    expect(wrong.status).toBe(401)
    const refused = await provider.submitPassword(attempt.browser, attempt.flow, user)
    expect(refused.status).toBe(403)
    expect(((await refused.json()) as { code: string }).code).toBe('account_suspended')

    // Recorded, and sent to webhooks, as a suspension.
    let recorded = 0
    for (let i = 0; i < 40 && recorded === 0; i++) {
      const res = await provider.admin(`/api/v1/audit/logs?action=user.suspended&resource_id=${sub}&limit=5`)
      recorded = ((await res.json()) as { logs: unknown[] }).logs.length
      if (recorded === 0) await new Promise((r) => setTimeout(r, 250))
    }
    expect(recorded).toBe(1)

    expect((await setActive(true)).active).toBe(true)
    const after = await provider.login(client, user, { scopes: WITH_REFRESH })
    expect(after.kind).toBe('code')
  })
})

// An access token issued before the suspension is refused by everything that
// asks the provider about it — Authway's own endpoints, userinfo and
// introspection — whichever form it was issued in. Only a resource server
// that verifies a JWT itself keeps accepting it until it expires.
describe("a suspended user's access tokens", () => {
  const cleanup: Array<() => Promise<void>> = []
  let resourceServer: TestClient & { secret: string }

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
    cleanup.push(() => provider.deleteClient(resourceServer))
  })

  afterAll(async () => {
    for (const done of cleanup.reverse()) await done()
  })

  const introspect = async (token: string): Promise<boolean> => {
    const res = await fetch(`${config.api}/api/v1/oauth2/introspect`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        Authorization: `Basic ${Buffer.from(`${encodeURIComponent(resourceServer.clientId)}:${encodeURIComponent(resourceServer.secret)}`).toString('base64')}`,
      },
      body: new URLSearchParams({ token }),
    })
    expect(res.status).toBe(200)
    return ((await res.json()) as { active: boolean }).active
  }

  for (const strategy of ['opaque', 'jwt'] as const) {
    it(`are refused at once (${strategy} access tokens)`, async () => {
      const res = await provider.admin('/api/v1/clients', {
        method: 'POST',
        body: JSON.stringify({
          tenant_id: await provider.tenantId(),
          name: `conformance-suspended-${strategy}-${randomToken(4)}`,
          public: true,
          redirect_uris: [REDIRECT_URI],
          allowed_origins: ['http://localhost:9999'],
          grant_types: ['authorization_code', 'refresh_token'],
          scopes: ['openid', 'profile', 'email'],
          access_token_strategy: strategy,
        }),
      })
      expect(res.status, await res.clone().text()).toBe(201)
      const { client: created } = (await res.json()) as { client: { id: string; client_id: string } }
      const client: TestClient = { id: created.id, clientId: created.client_id }
      cleanup.push(() => provider.deleteClient(client))

      const user = await provider.inviteAndAccept()
      const outcome = await provider.login(client, user)
      if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
      cleanup.push(() => provider.deleteUser(outcome.sub))
      const token = outcome.tokens.access_token
      const profile = (): Promise<Response> =>
        fetch(`${config.api}/api/v1/profile/me`, { headers: { Authorization: `Bearer ${token}` } })

      expect((await profile()).status).toBe(200)
      expect((await provider.userinfo(token)).status).toBe(200)
      expect(await introspect(token)).toBe(true)

      const suspended = await provider.admin(`/api/v1/users/${outcome.sub}`, { method: 'PUT', body: JSON.stringify({ active: false }) })
      expect(suspended.status).toBe(200)

      expect((await profile()).status).toBe(401)
      expect((await provider.userinfo(token)).status).toBe(401)
      expect(await introspect(token)).toBe(false)
    })
  }
})

// A suspended account is sent no password reset and no sign-in link: it could
// not use either. The answers are the ones any address gets, so they say
// nothing about the account.
describe('mail for a suspended user', () => {
  let client: TestClient
  let user: TestUser
  let sub: string | undefined

  beforeAll(async () => {
    client = await provider.createPublicClient({ signInMethods: ['email', 'magic_link'] })
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    if (client) await provider.deleteClient(client)
  })

  const mailTo = async (address: string): Promise<number> => {
    const res = await fetch(`${config.mailApi}/api/v2/search?kind=to&query=${encodeURIComponent(address)}`)
    return ((await res.json()) as { total: number }).total
  }

  it('sends it neither a password reset nor a sign-in link', async () => {
    const signedIn = await provider.login(client, user)
    if (signedIn.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(signedIn)}`)
    sub = signedIn.sub
    const suspended = await provider.admin(`/api/v1/users/${sub}`, { method: 'PUT', body: JSON.stringify({ active: false }) })
    expect(suspended.status).toBe(200)
    const before = await mailTo(user.email)

    const reset = await fetch(`${config.api}/api/v1/email/forgot-password`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: user.email }),
    })
    const resetForNobody = await fetch(`${config.api}/api/v1/email/forgot-password`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: `nobody-${randomToken(6)}@example.test` }),
    })
    expect(reset.status).toBe(200)
    expect(await reset.text()).toBe(await resetForNobody.text())

    const attempt = await provider.startLogin(client)
    const link = await attempt.browser.fetch(provider.loginFlowUrl(attempt.flow, '/magic-link'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: user.email }),
    })
    expect(link.status).toBe(200)
    expect(((await link.json()) as { next: string }).next).toBe('email_sent')

    // Mail is handed over asynchronously; give it the time a sent one takes.
    await new Promise((r) => setTimeout(r, 3000))
    expect(await mailTo(user.email)).toBe(before)
  })
})
