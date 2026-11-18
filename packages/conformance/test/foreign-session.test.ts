import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const admin = new Provider(config)

// A browser keeps one session, and it is someone's. A person who comes to
// sign in on a browser that holds another person's session — in the same
// tenant, or in another one — must get in on their first answer. An
// authorization server accepts a login that reuses a session only for the
// session's own user, so a sign-in shown on such a request is lost and asked for
// a second time: the right password is accepted and the person is back at the
// form.
describe('signing in on a browser that holds another person’s session', () => {
  let app: TestClient
  let first: TestUser
  let second: TestUser
  const subs = new Set<string>()

  beforeAll(async () => {
    app = await admin.createPublicClient()
    first = await admin.inviteAndAccept()
    second = await admin.inviteAndAccept()
  })

  afterAll(async () => {
    for (const sub of subs) await admin.deleteUser(sub)
    if (app) await admin.deleteClient(app)
  })

  it('signs in the next person, when the application asks to sign in anew, on the first answer', async () => {
    const earlier = await admin.login(app, first)
    expect(earlier.kind, JSON.stringify(earlier)).toBe('code')
    if (earlier.kind !== 'code') return
    subs.add(earlier.sub)

    const attempt = await admin.startLogin(app, undefined, earlier.browser, { prompt: 'login' })
    const outcome = await admin.continueLogin(attempt, second)
    expect(outcome.kind, JSON.stringify(outcome)).toBe('code')
    if (outcome.kind !== 'code') return
    subs.add(outcome.sub)
    expect(outcome.sub).not.toBe(earlier.sub)
  })
})

describe.skipIf(!(await admin.offers('multi_tenant')))('signing in to a tenant on a browser that holds a session of another', () => {
  let tenantId: string
  let other: Provider
  let appHere: TestClient
  let appThere: TestClient
  let here: TestUser
  let there: TestUser
  const subsHere = new Set<string>()
  const subsThere = new Set<string>()

  beforeAll(async () => {
    const res = await admin.admin('/api/v1/tenants', {
      method: 'POST',
      body: JSON.stringify({ name: 'Conformance other tenant', slug: `conformance-other-${randomToken(4).toLowerCase()}` }),
    })
    expect(res.status, await res.clone().text()).toBe(201)
    tenantId = ((await res.json()) as { id: string }).id
    other = new Provider({ ...config, tenantId })
    appHere = await admin.createPublicClient()
    appThere = await other.createPublicClient()
    here = await admin.inviteAndAccept()
    there = await other.inviteAndAccept()
  })

  afterAll(async () => {
    for (const sub of subsHere) await admin.deleteUser(sub)
    for (const sub of subsThere) await other.deleteUser(sub)
    if (appHere) await admin.deleteClient(appHere)
    if (appThere) await other.deleteClient(appThere)
    if (tenantId) await admin.admin(`/api/v1/tenants/${tenantId}`, { method: 'DELETE' })
  })

  it('signs the person in on the first answer', async () => {
    const earlier = await admin.login(appHere, here)
    expect(earlier.kind, JSON.stringify(earlier)).toBe('code')
    if (earlier.kind !== 'code') return
    subsHere.add(earlier.sub)

    // The session is of no use to this application: it asks the person to sign in.
    const attempt = await other.startLogin(appThere, undefined, earlier.browser)
    const outcome = await other.continueLogin(attempt, there)
    expect(outcome.kind, JSON.stringify(outcome)).toBe('code')
    if (outcome.kind !== 'code') return
    subsThere.add(outcome.sub)
    expect(outcome.sub).not.toBe(earlier.sub)
  })

  it('tells an application that asked for no prompt that sign-in is required', async () => {
    const earlier = await admin.login(appHere, here)
    expect(earlier.kind, JSON.stringify(earlier)).toBe('code')
    if (earlier.kind !== 'code') return
    subsHere.add(earlier.sub)

    const silent = await other.silentAuthorize(earlier.browser, appThere)
    expect(silent).toEqual({ kind: 'error', error: 'login_required' })
  })
})
