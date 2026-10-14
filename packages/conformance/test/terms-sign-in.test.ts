import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Authenticator } from '../src/authenticator.js'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type LoginOutcome, type TestClient, type TestUser } from '../src/provider.js'

const base = new Provider(loadConfig())

type Owed = Extract<LoginOutcome, { kind: 'terms' }>

function expectTerms(outcome: LoginOutcome): Owed {
  if (outcome.kind !== 'terms') throw new Error(`The sign-in did not stop for the terms: ${JSON.stringify(outcome).slice(0, 300)}`)
  return outcome
}

function expectCode(outcome: LoginOutcome): Extract<LoginOutcome, { kind: 'code' }> {
  if (outcome.kind !== 'code') throw new Error(`The sign-in did not end with a code: ${JSON.stringify(outcome).slice(0, 300)}`)
  return outcome
}

const shown = (owed: Owed) => owed.terms.map((d) => `${d.kind}@${d.version}`)

// A user signs in to an application and owes the current version of every
// document the application's tenant asks for. These scenarios keep the
// documents in a tenant of their own: in a shared tenant every other
// scenario's sign-in would owe them.
describe('terms at sign-in', () => {
  let tenantId: string
  let provider: Provider
  let app: TestClient
  let user: TestUser
  let sub: string

  const publish = async (kind: string, version: string) => {
    const res = await provider.admin(`/api/v1/tenants/${tenantId}/terms/${kind}`, {
      method: 'PUT',
      body: JSON.stringify({ version, url: `https://example.test/${kind}/${version}`, title: kind === 'terms_of_service' ? 'Terms of Service' : undefined }),
    })
    expect(res.status, await res.clone().text()).toBeLessThan(300)
  }
  const acceptances = async () => {
    const res = await provider.admin(`/api/v1/users/${sub}/terms-acceptances`)
    expect(res.status).toBe(200)
    return ((await res.json()) as { acceptances: { kind: string; version: string; client_id?: string }[] }).acceptances
  }

  beforeAll(async () => {
    const caps = await (await fetch(`${base.config.api}/api/v1/capabilities`)).json()
    if (!caps.terms_acceptance) throw new Error('The provider does not offer terms (capabilities.terms_acceptance is false)')
    const slug = `conformance-terms-${randomToken(6).toLowerCase()}`
    const res = await base.admin('/api/v1/tenants', { method: 'POST', body: JSON.stringify({ name: 'Conformance terms sign-in', slug }) })
    expect(res.status, await res.clone().text()).toBe(201)
    tenantId = ((await res.json()) as { id: string }).id
    provider = new Provider({ ...base.config, tenantId })
    app = await provider.createPublicClient({ skipConsent: true })
    user = await provider.inviteAndAccept()

    // Signed in once before the tenant asked for anything.
    sub = expectCode(await provider.login(app, user)).sub
    await publish('terms_of_service', '1')
    await publish('privacy_policy', 'a')
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    if (app) await provider.deleteClient(app)
  })

  it('asks for the documents after the password, records the acceptance, and signs in', async () => {
    const owed = expectTerms(await provider.login(app, user))
    expect(shown(owed)).toEqual(['privacy_policy@a', 'terms_of_service@1'])
    expect(owed.terms.find((d) => d.kind === 'terms_of_service')?.title).toBe('Terms of Service')

    const done = expectCode(await provider.acceptTerms(owed))
    expect(done.sub).toBe(sub)

    const recorded = await acceptances()
    expect(recorded.map((a) => `${a.kind}@${a.version}`).sort()).toEqual(['privacy_policy@a', 'terms_of_service@1'])
    expect(recorded.every((a) => a.client_id === app.clientId)).toBe(true)
  })

  it('does not ask again for what was accepted', async () => {
    expectCode(await provider.login(app, user))
  })

  it('asks again for a new version only, and refuses an acceptance of what is no longer owed', async () => {
    await publish('terms_of_service', '2')
    const owed = expectTerms(await provider.login(app, user))
    expect(shown(owed)).toEqual(['terms_of_service@2'])

    // The screen still shows version 1, say: refused, with what is owed now,
    // and the same step stays good.
    const stale = await owed.attempt.browser.fetch(provider.loginFlowUrl(owed.flow, '/terms'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ terms_challenge: owed.challenge, accepted: [{ kind: 'terms_of_service', version: '1' }] }),
    })
    await conform('POST', '/api/v1/login-flows/{flow}/terms', stale.clone())
    expect(stale.status).toBe(400)
    const refused = (await stale.json()) as { code: string; terms: { kind: string; version: string }[] }
    expect(refused.code).toBe('terms_mismatch')
    expect(refused.terms.map((d) => `${d.kind}@${d.version}`)).toEqual(['terms_of_service@2'])

    expectCode(await provider.acceptTerms(owed))
  })

  it("asks a reused session for a newer version, and prompt=none gets interaction_required", async () => {
    const signedIn = expectCode(await provider.login(app, user))
    await publish('terms_of_service', '3')

    const silent = await provider.silentAuthorize(signedIn.browser, app)
    expect(silent).toEqual({ kind: 'error', error: 'interaction_required' })

    // With a prompt allowed, the session answers who the user is and the
    // screen asks only for the document.
    const attempt = await provider.startLogin(app, undefined, signedIn.browser)
    const step = await signedIn.browser.fetch(provider.loginFlowUrl(attempt.flow))
    await conform('GET', '/api/v1/login-flows/{flow}', step.clone())
    const owed = expectTerms(provider.termsOwed(attempt, await step.text()) ?? { kind: 'rejected', status: step.status, body: '' })
    expect(shown(owed)).toEqual(['terms_of_service@3'])
    expect(expectCode(await provider.acceptTerms(owed)).sub).toBe(sub)
  })

  it('tells the application access_denied when the user declines, and records nothing', async () => {
    await publish('privacy_policy', 'b')
    const before = (await acceptances()).length
    const owed = expectTerms(await provider.login(app, user))
    const declined = await provider.declineTerms(owed)
    expect(declined.kind).toBe('rejected')
    expect(declined.kind === 'rejected' && declined.body).toContain('error=access_denied')
    expect((await acceptances()).length).toBe(before)

    // The next sign-in asks again.
    expectCode(await provider.acceptTerms(expectTerms(await provider.login(app, user))))
  })

  it('refuses a terms step without its challenge', async () => {
    const attempt = await provider.startLogin(app)
    const res = await provider.owedTerms(attempt, attempt.flow, 'not-a-challenge')
    expect(res.status).toBe(400)
    expect(((await res.json()) as { code: string }).code).toBe('invalid_terms_challenge')

    const accept = await attempt.browser.fetch(provider.loginFlowUrl(attempt.flow, '/terms'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ terms_challenge: 'not-a-challenge', accepted: [] }),
    })
    await conform('POST', '/api/v1/login-flows/{flow}/terms', accept.clone())
    expect(accept.status).toBe(400)
  })

  it('asks for the documents after the second factor, not before', async () => {
    const signedIn = expectCode(await provider.login(app, user))
    const mfa = async (path: string, body?: unknown) =>
      fetch(`${base.config.api}/api/v1/users/mfa${path}`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${signedIn.tokens.access_token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) },
        body: body ? JSON.stringify(body) : undefined,
      })
    const setup = await mfa('/setup')
    expect(setup.status, await setup.clone().text()).toBe(200)
    const authenticator = new Authenticator(((await setup.json()) as { secret: string }).secret)
    expect((await mfa('/verify', { code: await authenticator.next() })).status).toBe(200)

    await publish('terms_of_service', '4')
    const first = await provider.login(app, user)
    if (first.kind !== 'second_factor') throw new Error(`The sign-in did not stop for the second factor: ${JSON.stringify(first).slice(0, 300)}`)
    const owed = expectTerms(await provider.continueWithSecondFactor(first, await authenticator.next()))
    expect(shown(owed)).toEqual(['terms_of_service@4'])
    const done = expectCode(await provider.acceptTerms(owed))
    const amr = JSON.parse(Buffer.from((done.tokens.id_token ?? '').split('.')[1], 'base64url').toString()).amr as string[]
    expect(amr).toContain('otp')
  })
})
