import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

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

  it('refuses the link step for a client that does not enable it', async () => {
    const other = await provider.createPublicClient({ signInMethods: ['email'] })
    try {
      const attempt = await provider.startLogin(other)
      const res = await fetch(provider.loginFlowUrl(attempt.flow, '/magic-link'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: user.email }),
      })
      expect(res.status).toBe(403)
    } finally {
      await provider.deleteClient(other)
    }
  })
})
