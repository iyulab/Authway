import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Authenticator } from '../src/authenticator.js'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { PasskeyDevice } from '../src/passkey.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type LoginOutcome, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

// A passkey is a key pair a user's device keeps; the provider keeps the public
// half and the user signs in with the device. These scenarios run the device
// (no browser) against the provider's passkey endpoints: registering is the
// account screen's, signing in completes a login flow, and a passkey belongs to
// the tenant it was registered in.
describe('passkeys', () => {
  let offered = false
  let origin: string
  let app: TestClient
  let account: TestClient
  let user: TestUser
  let sub: string
  let accountToken: string
  let device: PasskeyDevice
  let passkeyId: string

  const idClaims = (outcome: Extract<LoginOutcome, { kind: 'code' }>): Record<string, unknown> =>
    JSON.parse(Buffer.from((outcome.tokens.id_token ?? '').split('.')[1], 'base64url').toString())

  /** A call to the user's own passkey endpoints with a token. */
  const call = async (token: string, method: string, path: string, body?: unknown): Promise<Response> => {
    const res = await fetch(`${config.api}/api/v1/users/passkeys${path}`, {
      method,
      headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) },
      body: body ? JSON.stringify(body) : undefined,
    })
    const template = path.startsWith('/') && !path.startsWith('/register') ? '/{id}' : path
    await conform(method, `/api/v1/users/passkeys${template}`, res.clone())
    return res
  }

  /** Registers `who` on the account screen's token and returns the passkey. */
  const register = async (who: PasskeyDevice, token = accountToken, name = 'Suite device'): Promise<{ id: string; name: string }> => {
    const begun = await call(token, 'POST', '/register/begin')
    expect(begun.status, await begun.clone().text()).toBe(200)
    const { ceremony_id, options } = (await begun.json()) as { ceremony_id: string; options: { challenge: string; rp: { id: string }; user: { id: string } } }
    const finished = await call(token, 'POST', '/register/finish', { ceremony_id, name, credential: who.register(options) })
    expect(finished.status, await finished.clone().text()).toBe(201)
    return ((await finished.json()) as { passkey: { id: string; name: string } }).passkey
  }

  beforeAll(async () => {
    const caps = (await (await fetch(`${config.api}/api/v1/capabilities`)).json()) as { passkeys?: boolean }
    offered = caps.passkeys === true
    if (!offered) return
    origin = new URL(((await (await fetch(`${config.api}/.well-known/authway-config`)).json()) as { auth_ui: string }).auth_ui).origin
    app = await provider.createPublicClient()
    account = await provider.accountClient(app)
    user = await provider.inviteAndAccept()
    const outcome = await provider.login(account, user)
    if (outcome.kind !== 'code') throw new Error(`Signing in to the account screen failed: ${JSON.stringify(outcome)}`)
    sub = outcome.sub
    accountToken = outcome.tokens.access_token
    device = new PasskeyDevice(origin)
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    if (app) await provider.deleteClient(app)
  })

  it('is managed with the account screen’s token and no other application’s', async (ctx) => {
    if (!offered) ctx.skip()
    const outcome = await provider.login(app, user)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    const elsewhere = outcome.tokens.access_token
    for (const [method, path, body] of [
      ['GET', ''],
      ['POST', '/register/begin'],
      ['POST', '/register/finish', { ceremony_id: 'x', credential: {} }],
      ['DELETE', `/${crypto.randomUUID()}`],
    ] as const) {
      const res = await call(elsewhere, method, path, body)
      const text = await res.text()
      expect(res.status, `${method} ${path}: ${text}`).toBe(403)
      expect(JSON.parse(text).code, `${method} ${path}`).toBe('account_application_required')
    }
    // Without a token there is no one to act on.
    const anonymous = await fetch(`${config.api}/api/v1/users/passkeys`)
    await conform('GET', '/api/v1/users/passkeys', anonymous.clone())
    expect(anonymous.status).toBe(401)
  })

  it('registers a discoverable credential, lists it without the credential, and names it so a second one is not offered', async (ctx) => {
    if (!offered) ctx.skip()
    const begun = await call(accountToken, 'POST', '/register/begin')
    const body = (await begun.json()) as { ceremony_id: string; options: Record<string, unknown> & { challenge: string; rp: { id: string }; user: { id: string }; authenticatorSelection: Record<string, string>; attestation: string } }
    expect(begun.status).toBe(200)
    expect(body.options.authenticatorSelection.residentKey).toBe('required')
    expect(body.options.authenticatorSelection.userVerification).toBe('required')
    expect(body.options.attestation).toBe('none')
    expect(body.options.excludeCredentials ?? []).toEqual([])
    const finished = await call(accountToken, 'POST', '/register/finish', { ceremony_id: body.ceremony_id, name: '  Suite device ', credential: device.register(body.options) })
    expect(finished.status, await finished.clone().text()).toBe(201)
    const passkey = ((await finished.json()) as { passkey: { id: string; name: string; last_used_at: string | null } }).passkey
    passkeyId = passkey.id
    expect(passkey.name).toBe('Suite device')
    expect(passkey.last_used_at).toBeNull()

    const listed = await call(accountToken, 'GET', '')
    const text = await listed.text()
    expect(listed.status, text).toBe(200)
    expect((JSON.parse(text) as { passkeys: unknown[] }).passkeys).toHaveLength(1)
    // What the screen sees does not include the credential.
    expect(text).not.toContain(device.credentialId.toString('base64url'))

    // A new ceremony names it, so the browser does not offer this device another.
    const again = await call(accountToken, 'POST', '/register/begin')
    const excluded = ((await again.json()) as { options: { excludeCredentials?: { id: string }[] } }).options.excludeCredentials ?? []
    expect(excluded.map((c) => c.id)).toEqual([device.credentialId.toString('base64url')])
  })

  it('refuses a registration answer that is not for its ceremony, and keeps the ceremony from being used twice', async (ctx) => {
    if (!offered) ctx.skip()
    const stranger = new PasskeyDevice(origin)
    const begin = async () => {
      const res = await call(accountToken, 'POST', '/register/begin')
      return (await res.json()) as { ceremony_id: string; options: { challenge: string; rp: { id: string }; user: { id: string } } }
    }
    const finish = (ceremony_id: string, credential: object) => call(accountToken, 'POST', '/register/finish', { ceremony_id, credential })

    for (const bend of [{ origin: 'https://evil.example' }, { rpId: 'evil.example' }, { challenge: Buffer.alloc(32, 1).toString('base64url') }, { flags: 0x41 /* present, not verified */ }]) {
      const b = await begin()
      const res = await finish(b.ceremony_id, stranger.register(b.options, bend))
      const text = await res.text()
      expect(res.status, `${JSON.stringify(bend)}: ${text}`).toBe(400)
      expect(JSON.parse(text).code).toBe('passkey_rejected')
    }

    const b = await begin()
    const credential = stranger.register(b.options)
    expect((await finish(b.ceremony_id, credential)).status).toBe(201)
    const replay = await finish(b.ceremony_id, credential)
    expect(replay.status).toBe(400)
    expect(((await replay.json()) as { code: string }).code).toBe('ceremony_expired')
    // Take the extra passkey away again.
    const listed = (await (await call(accountToken, 'GET', '')).json()) as { passkeys: { id: string }[] }
    for (const p of listed.passkeys.filter((p) => p.id !== passkeyId)) expect((await call(accountToken, 'DELETE', `/${p.id}`)).status).toBe(200)
  })

  it('signs a user in to a login flow with no one naming themselves, with amr hwk and mfa', async (ctx) => {
    if (!offered) ctx.skip()
    const attempt = await provider.startLogin(app)
    const begun = await provider.beginPasskeySignIn(attempt)
    const { options } = (await begun.json()) as { options: { allowCredentials?: unknown[]; userVerification: string; rpId: string } }
    // Nobody has said who is signing in: no credential is named.
    expect(options.allowCredentials ?? []).toEqual([])
    expect(options.userVerification).toBe('required')

    const outcome = await provider.continueWithPasskey(await provider.startLogin(app), device)
    expect(outcome.kind, JSON.stringify(outcome)).toBe('code')
    if (outcome.kind !== 'code') return
    expect(outcome.sub).toBe(sub)
    expect(idClaims(outcome).amr).toEqual(['hwk', 'mfa'])
    const info = await provider.userinfo(outcome.tokens.access_token)
    expect((info.body as { email: string }).email).toBe(user.email)

    const listed = (await (await call(accountToken, 'GET', '')).json()) as { passkeys: { last_used_at: string | null }[] }
    expect(listed.passkeys[0].last_used_at).not.toBeNull()
  })

  it('refuses every answer that is not the registered device’s for this flow, with one answer that gives no reason', async (ctx) => {
    if (!offered) ctx.skip()
    // Refused answers count toward the provider's limit on failed sign-ins (a few
    // a minute, until one succeeds): four here, and the sign-in at the end of
    // the scenario succeeds. The other reasons - another relying party, a
    // missing user handle - are the provider's own tests.
    const attempts: [string, (a: Awaited<ReturnType<Provider['startLogin']>>) => Promise<LoginOutcome>][] = [
      ['a device that was never registered', async (a) => { const d = new PasskeyDevice(origin); d.userHandle = device.userHandle; return provider.continueWithPasskey(a, d) }],
      ['another origin', (a) => provider.continueWithPasskey(a, device, { origin: 'https://evil.example' })],
      ['a user who was not verified', (a) => provider.continueWithPasskey(a, device, { flags: 0x19 /* present, backup bits, no verification */ })],
      ['another account’s user handle', (a) => provider.continueWithPasskey(a, device, { userHandle: Buffer.alloc(16, 7) })],
    ]
    const bodies = new Set<string>()
    for (const [what, run] of attempts) {
      const outcome = await run(await provider.startLogin(app))
      expect(outcome.kind, what).toBe('rejected')
      if (outcome.kind !== 'rejected') continue
      expect(outcome.status, what).toBe(401)
      expect(JSON.parse(outcome.body).code, what).toBe('passkey_rejected')
      bodies.add(JSON.parse(outcome.body).detail)
    }
    // Whatever the reason, the answer is the same.
    expect(bodies.size).toBe(1)
    // The right device is still let in - and that clears the count of failures.
    expect((await provider.continueWithPasskey(await provider.startLogin(app), device)).kind).toBe('code')
  })

  it('takes a ceremony once and for its own flow only', async (ctx) => {
    if (!offered) ctx.skip()
    const one = await provider.startLogin(app)
    const other = await provider.startLogin(app)
    const begun = (await (await provider.beginPasskeySignIn(one)).json()) as { ceremony_id: string; options: { challenge: string; rpId: string } }
    const answer = device.signIn(begun.options)

    // Another flow of the same tenant cannot finish it.
    const stolen = await provider.finishPasskeySignIn(other, begun.ceremony_id, answer)
    expect(stolen.status).toBe(400)
    expect(((await stolen.json()) as { code: string }).code).toBe('ceremony_expired')
    // And trying spent it.
    const spent = await provider.finishPasskeySignIn(one, begun.ceremony_id, answer)
    expect(spent.status).toBe(400)
    expect((await provider.continueWithPasskey(await provider.startLogin(app), device)).kind).toBe('code')
  })

  it('is offered where the password form is, and not to an application that offers only other ways', async (ctx) => {
    if (!offered) ctx.skip()
    const attempt = await provider.startLogin(app)
    const flow = (await (await fetch(provider.loginFlowUrl(attempt.flow))).json()) as { client: { sign_in_methods: string[] } }
    expect(flow.client.sign_in_methods).toContain('passkey')

    const sso = await provider.createPublicClient({ signInMethods: ['google'] })
    try {
      const closed = await provider.startLogin(sso)
      const res = await provider.beginPasskeySignIn(closed)
      expect(res.status).toBe(403)
      expect(((await res.json()) as { code: string }).code).toBe('sign_in_method_not_allowed')
      const listed = (await (await fetch(provider.loginFlowUrl(closed.flow))).json()) as { client: { sign_in_methods: string[] } }
      expect(listed.client.sign_in_methods).not.toContain('passkey')
    } finally {
      await provider.deleteClient(sso)
    }
  })

  it('opens only the flows of the tenant it was registered in', async (ctx) => {
    if (!offered) ctx.skip()
    const created = await provider.admin('/api/v1/tenants', {
      method: 'POST',
      body: JSON.stringify({ name: 'Conformance passkeys', slug: `conformance-passkeys-${randomToken(4).toLowerCase()}` }),
    })
    expect(created.status, await created.clone().text()).toBe(201)
    const tenantId = ((await created.json()) as { id: string }).id
    const other = new Provider({ ...config, tenantId })
    const otherApp = await other.createPublicClient()
    try {
      // The login screen is one relying party for every tenant, so this device
      // would be offered on the other tenant's screen; the provider refuses it.
      const outcome = await other.continueWithPasskey(await other.startLogin(otherApp), device)
      expect(outcome.kind).toBe('rejected')
      if (outcome.kind === 'rejected') expect(outcome.status).toBe(401)
      // And it still opens this tenant's.
      const here = await provider.continueWithPasskey(await provider.startLogin(app), device)
      expect(here.kind).toBe('code')
    } finally {
      await other.deleteClient(otherApp)
      await provider.admin(`/api/v1/tenants/${tenantId}`, { method: 'DELETE' })
    }
  })

  it('does not skip a second factor the user has', async (ctx) => {
    if (!offered) ctx.skip()
    const owner = await provider.inviteAndAccept()
    const signedIn = await provider.login(account, owner)
    if (signedIn.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(signedIn)}`)
    try {
      const mine = new PasskeyDevice(origin)
      await register(mine, signedIn.tokens.access_token, 'With a second factor')

      const mfa = async (path: string, body?: unknown) =>
        fetch(`${config.api}/api/v1/users/mfa${path}`, {
          method: 'POST',
          headers: { Authorization: `Bearer ${signedIn.tokens.access_token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) },
          body: body ? JSON.stringify(body) : undefined,
        })
      const setup = await mfa('/setup')
      expect(setup.status, await setup.clone().text()).toBe(200)
      const authenticator = new Authenticator(((await setup.json()) as { secret: string }).secret)
      expect((await mfa('/verify', { code: await authenticator.next() })).status).toBe(200)

      const outcome = await provider.continueWithPasskey(await provider.startLogin(app), mine)
      expect(outcome.kind, JSON.stringify(outcome)).toBe('second_factor')
      if (outcome.kind !== 'second_factor') return
      const done = await provider.continueWithSecondFactor(outcome, await authenticator.next())
      expect(done.kind, JSON.stringify(done)).toBe('code')
      if (done.kind === 'code') expect(idClaims(done).amr).toEqual(['hwk', 'mfa', 'otp'])
    } finally {
      await provider.deleteUser(signedIn.sub)
    }
  })

  it('refuses a signature counter that goes back, as a sign of a copied credential', async (ctx) => {
    if (!offered) ctx.skip()
    const hardware = new PasskeyDevice(origin)
    hardware.counts = true
    const registered = await register(hardware, accountToken, 'A key that counts')
    try {
      for (let i = 0; i < 2; i++) {
        const ok = await provider.continueWithPasskey(await provider.startLogin(app), hardware)
        expect(ok.kind, `sign-in ${i + 1}: ${JSON.stringify(ok)}`).toBe('code')
      }
      hardware.rewind(0) // a copy that has not seen the last two uses
      const copied = await provider.continueWithPasskey(await provider.startLogin(app), hardware)
      expect(copied.kind).toBe('rejected')
      if (copied.kind === 'rejected') expect(copied.status).toBe(401)
    } finally {
      await call(accountToken, 'DELETE', `/${registered.id}`)
    }
  })

  it('stops signing in once removed — and keeps the account’s last way to sign in', async (ctx) => {
    if (!offered) ctx.skip()
    const removed = await call(accountToken, 'DELETE', `/${passkeyId}`)
    expect(removed.status, await removed.clone().text()).toBe(200)

    const outcome = await provider.continueWithPasskey(await provider.startLogin(app), device)
    expect(outcome.kind).toBe('rejected')
    const listed = (await (await call(accountToken, 'GET', '')).json()) as { passkeys: unknown[] }
    expect(listed.passkeys).toHaveLength(0)

    const gone = await call(accountToken, 'DELETE', `/${passkeyId}`)
    expect(gone.status).toBe(404)
  })
})
