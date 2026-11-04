import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())

// A person who cannot sign in to a service that is for invited people only has
// no way to learn it from "the email or the password is wrong". The sign-in
// screen is told how the tenant takes people in, and says so: a fact about the
// tenant, so the same whoever asks and whatever address they typed.
describe('telling who can sign in', () => {
  let client: TestClient
  let user: TestUser

  beforeAll(async () => {
    client = await provider.createPublicClient()
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    if (client) await provider.deleteClient(client)
  })

  it('has the sign-in flow carry how the tenant takes people in — invite-only for a tenant that never chose', async () => {
    const attempt = await provider.startLogin(client)
    const res = await fetch(provider.loginFlowUrl(attempt.flow))
    await conform('GET', '/api/v1/login-flows/{flow}', res.clone())
    const form = (await res.json()) as { next: string; signup_mode?: string }
    expect(form.next).toBe('form')
    expect(form.signup_mode).toBe('invite_only')
  })

  it('answers a password reset that names the application the same for an address with an account and one without, with the same fact', async () => {
    const ask = async (email: string, clientId?: string) => {
      const res = await fetch(`${provider.config.api}/api/v1/email/forgot-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(clientId ? { email, client_id: clientId } : { email }),
      })
      await conform('POST', '/api/v1/email/forgot-password', res.clone())
      expect(res.status).toBe(200)
      return (await res.json()) as { message: string; signup_mode?: string }
    }

    const known = await ask(user.email, client.clientId)
    const unknown = await ask(`conformance-${randomToken(6)}@example.test`, client.clientId)
    expect(unknown).toEqual(known)
    expect(known.signup_mode).toBe('invite_only')

    // With no application named there is no tenant to speak of.
    const anonymous = await ask(`conformance-${randomToken(6)}@example.test`)
    expect(anonymous).not.toHaveProperty('signup_mode')
  })

  it('says the same when a new verification link is asked for', async () => {
    const ask = async (email: string, clientId?: string) => {
      const res = await fetch(`${provider.config.api}/api/v1/email/send-verification`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(clientId ? { email, client_id: clientId } : { email }),
      })
      await conform('POST', '/api/v1/email/send-verification', res.clone())
      expect(res.status).toBe(200)
      return (await res.json()) as { message: string; signup_mode?: string }
    }

    const known = await ask(user.email, client.clientId)
    const unknown = await ask(`conformance-${randomToken(6)}@example.test`, client.clientId)
    expect(unknown).toEqual(known)
    expect(known.signup_mode).toBe('invite_only')
    expect(await ask(user.email)).not.toHaveProperty('signup_mode')
  })
})

describe.skipIf(!(await provider.offers('magic_link')))('telling who can sign in — an emailed sign-in link', () => {
  let client: TestClient

  beforeAll(async () => {
    client = await provider.createPublicClient({ signInMethods: ['email', 'magic_link'] })
  })

  afterAll(async () => {
    if (client) await provider.deleteClient(client)
  })

  it('answers a request for a link with how the tenant takes people in, whichever address asked', async () => {
    const ask = async (email: string) => {
      const attempt = await provider.startLogin(client)
      const res = await fetch(`${provider.loginFlowUrl(attempt.flow)}/magic-link`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email }),
      })
      await conform('POST', '/api/v1/login-flows/{flow}/magic-link', res.clone())
      expect(res.status).toBe(200)
      return (await res.json()) as { next: string; signup_mode?: string }
    }

    const first = await ask(`conformance-${randomToken(6)}@example.test`)
    const second = await ask(`conformance-${randomToken(6)}@example.test`)
    expect(first).toEqual({ next: 'email_sent', signup_mode: 'invite_only' })
    expect(second).toEqual(first)
  })
})
