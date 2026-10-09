import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'

const provider = new Provider(loadConfig())

describe('sign-in with an emailed link', () => {
  let client: TestClient
  let user: TestUser
  const subjects = new Set<string>()

  beforeAll(async () => {
    const caps = await (await fetch(`${provider.config.api}/api/v1/capabilities`)).json()
    if (!caps.magic_link) throw new Error('The provider does not offer sign-in links (capabilities.magic_link is false)')
    client = await provider.createPublicClient({ signInMethods: ['email', 'magic_link'] })
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    for (const sub of subjects) await provider.deleteUser(sub)
    if (client) await provider.deleteClient(client)
  })

  it('offers the link on the login screen of a client that enables it', async () => {
    const attempt = await provider.startLogin(client)
    const body = await (await fetch(provider.loginFlowUrl(attempt.flow))).json()
    expect(body.client.sign_in_methods).toContain('magic_link')
  })

  it('completes the authorization request when the link is redeemed in the same browser', async () => {
    const attempt = await provider.startLogin(client)
    const outcome = await provider.continueWithMagicLink(attempt, user.email)
    expect(outcome.kind, JSON.stringify(outcome)).toBe('code')
    if (outcome.kind === 'code') subjects.add(outcome.sub)
  })

  // A request that may send mail must not tell a caller whether the address
  // belongs to anyone: a registered address and an unknown one get the same
  // status and the same body.
  it('answers a registered address exactly like an unknown one', async () => {
    const nobody = `nobody-${randomToken(4)}@example.test`
    const post = (url: string, email: string) =>
      fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email }) })
    const answer = async (res: Response) => ({ status: res.status, body: await res.text() })

    for (const path of ['/api/email/send-verification', '/api/email/forgot-password']) {
      const known = await post(`${provider.config.api}${path}`, user.email)
      await conform('POST', path, known.clone())
      const unknown = await post(`${provider.config.api}${path}`, nobody)
      expect(await answer(known), path).toEqual(await answer(unknown))
    }

    const known = await post(provider.loginFlowUrl((await provider.startLogin(client)).flow, '/magic-link'), user.email)
    await conform('POST', '/api/v1/login-flows/{flow}/magic-link', known.clone())
    const unknown = await post(provider.loginFlowUrl((await provider.startLogin(client)).flow, '/magic-link'), nobody)
    expect(await answer(known), 'magic-link').toEqual(await answer(unknown))
  })

  it('refuses the link step for a client that does not enable it', async () => {
    const other = await provider.createPublicClient({ signInMethods: ['email'] })
    try {
      const attempt = await provider.startLogin(other)
      const res = await fetch(provider.loginFlowUrl(attempt.flow, '/magic-link'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: user.email }),
      })
      await conform('POST', '/api/v1/login-flows/{flow}/magic-link', res)
      expect(res.status).toBe(403)
    } finally {
      await provider.deleteClient(other)
    }
  })
})
