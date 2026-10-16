import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Browser } from '../src/browser.js'
import { loadConfig } from '../src/config.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())

// Signing in opens a session at the provider: the browser is known there until
// the session ends. The user's next application does not ask them to sign in
// again, an application can ask whether the user is still signed in without
// showing them anything (`prompt=none`), and a logout has something to end.
// None of that depends on a box the user may leave unticked.
describe('the session a sign-in opens', () => {
  let app: TestClient
  let ownApp: TestClient
  let user: TestUser
  const subjects = new Set<string>()

  beforeAll(async () => {
    app = await provider.createPublicClient()
    ownApp = await provider.createPublicClient({ skipConsent: true })
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    for (const sub of subjects) await provider.deleteUser(sub)
    for (const client of [app, ownApp]) if (client) await provider.deleteClient(client)
  })

  const signIn = async (client: TestClient, options: { rememberConsent?: boolean } = {}) => {
    const outcome = await provider.login(client, user, options)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    subjects.add(outcome.sub)
    return outcome
  }

  it('is reused by the next authorization in the same browser, without "keep me signed in"', async () => {
    const first = await signIn(app)
    // The login screen is told to go on; no password is asked for.
    const again = await provider.reauthorize(first.browser, app)
    expect(again.access_token).toBeTruthy()
  })

  it('answers an application that asks without a prompt, for a user who let their consent be remembered', async () => {
    const first = await signIn(app, { rememberConsent: true })
    expect(first.consent).toBe('asked')

    const silent = await provider.silentAuthorize(first.browser, app)
    expect(silent.kind, JSON.stringify(silent)).toBe('code')
    if (silent.kind === 'code') expect(silent.sub).toBe(first.sub)
  })

  it('says consent is required, without a prompt, when the user was asked and did not have the answer remembered', async () => {
    // An application this user has given no remembered answer to.
    const other = await provider.createPublicClient()
    try {
      const first = await signIn(other)
      expect(first.consent).toBe('asked')

      const silent = await provider.silentAuthorize(first.browser, other)
      expect(silent).toEqual({ kind: 'error', error: 'consent_required' })
    } finally {
      await provider.deleteClient(other)
    }
  })

  it('says sign-in is required, without a prompt, in a browser that has no session', async () => {
    const silent = await provider.silentAuthorize(new Browser(), app)
    expect(silent).toEqual({ kind: 'error', error: 'login_required' })
  })

  it('does not ask the users of an application registered to skip consent — on the first sign-in or a silent one', async () => {
    const first = await signIn(ownApp)
    expect(first.consent).not.toBe('asked')

    const silent = await provider.silentAuthorize(first.browser, ownApp)
    expect(silent.kind, JSON.stringify(silent)).toBe('code')
  })

  it('asks again once an application stops skipping consent: the answers remembered for it are withdrawn', async () => {
    const own = await provider.createPublicClient({ skipConsent: true })
    try {
      const first = await signIn(own)
      expect((await provider.silentAuthorize(first.browser, own)).kind).toBe('code')

      const turned = await provider.admin(`/api/v1/clients/${own.id}`, { method: 'PUT', body: JSON.stringify({ skip_consent: false }) })
      expect(turned.status, await turned.clone().text()).toBe(200)

      expect(await provider.silentAuthorize(first.browser, own)).toEqual({ kind: 'error', error: 'consent_required' })
    } finally {
      await provider.deleteClient(own)
    }
  })

  it('still asks for consent when the user arrives at another application on an existing session', async () => {
    const first = await signIn(ownApp)
    // Signed in already, so no password — but this application has not been
    // allowed anything yet, and it is not one that skips the question.
    const other = await provider.createPublicClient()
    try {
      const reached = await provider.pastTheLoginStep(first.browser, other)
      expect(reached, 'the user was asked to sign in again').toBeDefined()
      const stop = reached!.stop
      expect(stop.searchParams.has('code'), `the application was answered without the user being asked: ${stop.origin}${stop.pathname}`).toBe(false)
      const consent = (await (await first.browser.fetch(provider.flowUrl('consent-flows', provider.consentFlowFrom(stop)))).json()) as { next: string }
      expect(consent.next).toBe('form')
    } finally {
      await provider.deleteClient(other)
    }
  })

  it('is reused without the login UI being opened: nothing is asked, so no screen is loaded', async () => {
    const { auth_ui: loginUi } = (await (await fetch(`${provider.config.api}/.well-known/authway-config`)).json()) as { auth_ui: string }
    const first = await signIn(ownApp)
    const screensOpened = (from: number) => first.browser.sentTo.slice(from).filter((u) => u.origin === new URL(loginUi).origin)

    // A request without a prompt, and an ordinary one: both end at the
    // application without a page of the login UI in between.
    let mark = first.browser.sentTo.length
    const silent = await provider.silentAuthorize(first.browser, ownApp)
    expect(silent.kind, JSON.stringify(silent)).toBe('code')
    expect(screensOpened(mark).map(String)).toEqual([])

    mark = first.browser.sentTo.length
    await provider.reauthorize(first.browser, ownApp)
    expect(screensOpened(mark).map(String)).toEqual([])
  })

  it('ends with a logout: the next request without a prompt is told sign-in is required', async () => {
    const first = await signIn(ownApp)
    if (!first.tokens.id_token) throw new Error('no id_token to hint the logout with')
    await provider.rpInitiatedLogout(first.browser, first.tokens.id_token)

    const silent = await provider.silentAuthorize(first.browser, ownApp)
    expect(silent).toEqual({ kind: 'error', error: 'login_required' })
  })
})
