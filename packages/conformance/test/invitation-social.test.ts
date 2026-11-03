import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Browser } from '../src/browser.js'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { waitForLink } from '../src/mail.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestClient } from '../src/provider.js'
import { StandInProvider, type StandInIdentity } from '../src/standin.js'

const config = loadConfig()
const provider = new Provider(config)

// A person who opens an emailed invitation has no application's sign-in
// behind them. When the invitation's application lets users in through
// another provider, the accept screen offers it — and accepts the
// invitation only for an account that provider has verified as the address
// the invitation was sent to. Any other account changes nothing.
describe('accepting an invitation with another identity provider', () => {
  let offered: string[] = []
  let standIn: StandInProvider
  let app: TestClient
  const users: string[] = []

  const invite = async (email: string): Promise<string> => {
    const created = await provider.admin('/api/v1/invitations', { method: 'POST', body: JSON.stringify({ email, client_id: app.clientId }) })
    expect(created.status, await created.clone().text()).toBe(201)
    const link = await waitForLink(config.mailApi, email, '/invitation/accept')
    return link.searchParams.get('token')!
  }

  const lookUp = (token: string) => fetch(`${config.api}/api/v1/invitations/token/${encodeURIComponent(token)}`)

  /** Presses "continue with <provider>" and has the stand-in say who signed in; returns where the browser ends. */
  const acceptWith = async (token: string, identity: StandInIdentity): Promise<URL> => {
    const browser = new Browser()
    const toProvider = await browser.redirectFrom(`${config.api}/api/v1/invitations/token/${encodeURIComponent(token)}/social/${config.idpStandInAs}`)
    expect(toProvider.origin).toBe(standIn.origin)
    const back = new URL(toProvider.searchParams.get('redirect_uri')!)
    back.searchParams.set('code', standIn.codeFor(identity))
    back.searchParams.set('state', toProvider.searchParams.get('state')!)
    const res = await browser.fetch(back.toString())
    expect([302, 303], `the callback answered ${res.status}`).toContain(res.status)
    return new URL(res.headers.get('location')!, back)
  }

  const userByEmail = async (email: string) => {
    const res = await provider.admin('/api/v1/users?limit=100')
    return ((await res.json()) as { users: { id: string; email: string; email_verified: boolean }[] }).users.find((u) => u.email === email)
  }

  beforeAll(async () => {
    offered = ((await (await fetch(`${config.api}/api/v1/capabilities`)).json()) as { providers: string[] }).providers
    if (offered.length === 0) return // this deployment signs no one in through a provider
    if (!config.idpStandIn || !offered.includes(config.idpStandInAs)) {
      throw new Error(`The provider offers ${offered.join(', ')}, but the suite has no stand-in for "${config.idpStandInAs}". Set CONFORMANCE_IDP_STANDIN.`)
    }
    standIn = new StandInProvider(new URL(config.idpStandIn))
    await standIn.start()
    app = await provider.createPublicClient({ signInMethods: ['email', config.idpStandInAs] })
  })

  afterAll(async () => {
    for (const id of users) await provider.deleteUser(id)
    if (app) await provider.deleteClient(app)
    if (standIn) await standIn.stop()
  })

  it('tells the screen which providers the invitation can be accepted with', async ({ skip }) => {
    if (offered.length === 0) skip()
    const token = await invite(`conformance-${randomToken(6)}@example.test`)
    const res = await lookUp(token)
    await conform('GET', '/api/v1/invitations/token/{token}', res.clone())
    const { invitation } = (await res.json()) as { invitation: { sign_in_with?: string[] } }
    expect(invitation.sign_in_with).toContain(config.idpStandInAs)
  })

  it("accepts it for the account the provider has verified as the invited address, without a password, and shows the usual end screen", async ({ skip }) => {
    if (offered.length === 0) skip()
    const email = `conformance-${randomToken(6)}@example.test`
    const token = await invite(email)

    const end = await acceptWith(token, { subject: `standin-${randomToken(8)}`, email, emailVerified: true })

    expect(end.pathname).toBe('/invitation/accept')
    const receiptId = end.searchParams.get('receipt')
    expect(receiptId, end.toString()).toBeTruthy()
    const receipt = await fetch(`${config.api}/api/v1/invitations/receipts/${encodeURIComponent(receiptId!)}`)
    await conform('GET', '/api/v1/invitations/receipts/{id}', receipt.clone())
    expect(receipt.status).toBe(200)
    expect(((await receipt.json()) as { invitation: { email: string } }).invitation.email).toBe(email)

    const account = await userByEmail(email)
    expect(account?.email_verified).toBe(true)
    if (account) users.push(account.id)
    // Used: the invitation can be accepted once.
    expect((await lookUp(token)).status).toBe(400)
  })

  it('refuses any other account — nothing is created and the invitation stays', async ({ skip }) => {
    if (offered.length === 0) skip()
    const email = `conformance-${randomToken(6)}@example.test`
    const token = await invite(email)

    const end = await acceptWith(token, { subject: `standin-${randomToken(8)}`, email: `other-${randomToken(6)}@example.test`, emailVerified: true })

    expect(end.pathname).toBe('/invitation/accept')
    expect(end.searchParams.get('error')).toBe('address_mismatch')
    expect(end.searchParams.get('receipt')).toBeNull()
    expect(await userByEmail(email)).toBeUndefined()
    expect((await lookUp(token)).status).toBe(200)
  })

  it('refuses the invited address when the provider has not verified it', async ({ skip }) => {
    if (offered.length === 0) skip()
    const email = `conformance-${randomToken(6)}@example.test`
    const token = await invite(email)

    const end = await acceptWith(token, { subject: `standin-${randomToken(8)}`, email, emailVerified: false })

    expect(end.searchParams.get('error')).toBe('address_mismatch')
    expect(await userByEmail(email)).toBeUndefined()
    expect((await lookUp(token)).status).toBe(200)
  })

  it('does not start for a provider the invitation cannot be accepted with', async ({ skip }) => {
    if (offered.length === 0) skip()
    const token = await invite(`conformance-${randomToken(6)}@example.test`)
    const res = await new Browser().fetch(`${config.api}/api/v1/invitations/token/${encodeURIComponent(token)}/social/not-a-provider`)
    expect(res.status).toBe(302)
    expect(new URL(res.headers.get('location')!, config.api).searchParams.get('error')).toBe('provider_unavailable')
  })

  it('answers a receipt that does not exist as not found', async () => {
    const res = await fetch(`${config.api}/api/v1/invitations/receipts/${randomToken(24)}`)
    await conform('GET', '/api/v1/invitations/receipts/{id}', res.clone())
    expect(res.status).toBe(404)
  })
})
