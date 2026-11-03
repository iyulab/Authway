import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestClient } from '../src/provider.js'
import { hashOf, RangeStandIn } from '../src/standin.js'

const config = loadConfig()
const provider = new Provider(config)

// A provider that checks the passwords being set against known data breaches
// refuses one that is listed (400 password_breached), sends only five
// characters of its hash to the range service, lets a password through when the
// service cannot answer, and does not look at a password again once it is set.
// The suite plays the range service (`CONFORMANCE_BREACHED_PASSWORD_RANGE`);
// the provider under test is configured to ask it. Skipped where the provider
// does not offer the check.
describe('breached password check', () => {
  let offered = false
  let range: RangeStandIn
  let app: TestClient | undefined
  const created: string[] = []

  /** A password nobody has set before, so that no earlier answer is cached for its hash prefix. */
  const fresh = (): string => `Cf-${randomToken(14)}`

  beforeAll(async () => {
    const caps = (await (await fetch(`${config.api}/api/v1/capabilities`)).json()) as { breached_password_check?: boolean }
    offered = caps.breached_password_check === true
    if (!offered) return
    // Fail loudly instead of skipping: a provider that offers the check with no
    // stand-in to ask would leave the scenarios unverified.
    if (!config.breachedPasswordRange) {
      throw new Error(
        'The provider checks passwords against breaches (capabilities.breached_password_check) but CONFORMANCE_BREACHED_PASSWORD_RANGE is not set. See packages/conformance/README.md.',
      )
    }
    range = new RangeStandIn(new URL(config.breachedPasswordRange))
    await range.start()
  })

  afterAll(async () => {
    for (const sub of created) await provider.deleteUser(sub)
    if (app) await provider.deleteClient(app)
    if (range) await range.stop()
  })

  it('refuses a listed password with password_breached, and the invitation can still be accepted', async (ctx) => {
    if (!offered) ctx.skip()
    const listed = fresh()
    range.list(listed)
    const { token } = await provider.invite()

    const refused = await provider.acceptInvitation(token, listed)
    expect(refused.status, await refused.clone().text()).toBe(400)
    const body = (await refused.json()) as { code: string; detail: string }
    expect(body.code).toBe('password_breached')
    expect(body.detail).not.toContain(listed)

    // The refusal did not spend the invitation: a password that is not listed is accepted.
    const accepted = await provider.acceptInvitation(token, fresh())
    expect(accepted.status, await accepted.clone().text()).toBe(200)
  })

  it('sends the range service the first five characters of the hash and nothing else', async (ctx) => {
    if (!offered) ctx.skip()
    const listed = fresh()
    range.list(listed)
    const before = range.requests.length
    const { token } = await provider.invite()
    await provider.acceptInvitation(token, listed)

    const sent = range.requests.slice(before)
    expect(sent.length).toBeGreaterThan(0)
    const { prefix, suffix } = hashOf(listed)
    for (const request of sent) {
      expect(request.path).toMatch(/^\/range\/[0-9A-F]{5}$/i)
      expect(request.path.toUpperCase()).not.toContain(suffix.slice(0, 10))
      expect(request.path).not.toContain(listed)
      // Asks the service to pad its answers, so their size does not say how many are real.
      expect(request.padding).toBe('true')
    }
    expect(sent.map((r) => r.path.toUpperCase())).toContain(`/RANGE/${prefix}`)
  })

  it('accepts a password when the range service cannot answer', async (ctx) => {
    if (!offered) ctx.skip()
    range.mode = 'down'
    try {
      const { token } = await provider.invite()
      const accepted = await provider.acceptInvitation(token, fresh())
      expect(accepted.status, await accepted.clone().text()).toBe(200)
    } finally {
      range.mode = 'up'
    }
  })

  it('does not look at a password again once it is set: signing in with it asks nobody', async (ctx) => {
    if (!offered) ctx.skip()
    app = await provider.createPublicClient()
    const user = await provider.inviteAndAccept()
    // The password is in a breach now, but it was not when it was set.
    range.list(user.password)
    const before = range.requests.length

    const outcome = await provider.login(app, user)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    created.push(outcome.sub)
    expect(range.requests.length).toBe(before)
  })
})
