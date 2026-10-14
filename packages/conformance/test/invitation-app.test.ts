import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { conform } from '../src/contract.js'
import { waitForLink } from '../src/mail.js'
import { randomToken } from '../src/pkce.js'
import { Provider, REDIRECT_URI, type TestClient } from '../src/provider.js'

const provider = new Provider(loadConfig())

// A person who accepts an emailed invitation arrives outside any sign-in an
// application started. An invitation can name the application it is for; the
// screen that accepts it then sends the person to where that application
// starts a sign-in (OpenID Connect Core 1.0 section 4), or to its home page.
describe('an invitation for an application', () => {
  let app: TestClient & { clientId: string }

  beforeAll(async () => {
    const res = await provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: `conformance-invite-app-${randomToken(4)}`,
        public: true,
        redirect_uris: [REDIRECT_URI],
        allowed_origins: ['http://localhost:9999'],
        grant_types: ['authorization_code'],
        scopes: ['openid', 'email'],
        initiate_login_uri: 'https://app.example.test/login',
        client_uri: 'https://app.example.test/',
      }),
    })
    await conform('POST', '/api/v1/clients', res.clone())
    expect(res.status, await res.clone().text()).toBe(201)
    const { client } = (await res.json()) as { client: { id: string; client_id: string; initiate_login_uri: string; client_uri: string } }
    expect(client.initiate_login_uri).toBe('https://app.example.test/login')
    expect(client.client_uri).toBe('https://app.example.test/')
    app = { id: client.id, clientId: client.client_id }
  })

  afterAll(async () => {
    if (app) await provider.deleteClient(app)
  })

  it('keeps where the application starts a sign-in and its home page, and changes or clears them', async () => {
    const changed = await provider.admin(`/api/v1/clients/${app.id}`, {
      method: 'PUT',
      body: JSON.stringify({ client_uri: '' , initiate_login_uri: 'https://app.example.test/start' }),
    })
    await conform('PUT', '/api/v1/clients/{id}', changed.clone())
    expect(changed.status, await changed.clone().text()).toBe(200)
    const read = await provider.admin(`/api/v1/clients/${app.id}`)
    await conform('GET', '/api/v1/clients/{id}', read.clone())
    const { client } = (await read.json()) as { client: { initiate_login_uri: string; client_uri: string } }
    expect(client).toMatchObject({ initiate_login_uri: 'https://app.example.test/start', client_uri: '' })
  })

  it('refuses an address that is not https', async () => {
    for (const field of ['initiate_login_uri', 'client_uri']) {
      const res = await provider.admin(`/api/v1/clients/${app.id}`, {
        method: 'PUT',
        body: JSON.stringify({ [field]: 'http://app.example.test/x' }),
      })
      await conform('PUT', '/api/v1/clients/{id}', res.clone())
      expect(res.status, field).toBe(400)
      expect(((await res.json()) as { code: string }).code).toBe(`invalid_${field}`)
    }
  })

  it('shows the application on the invitation, for the screen that accepts it', async () => {
    const email = `conformance-${randomToken(6)}@example.test`
    const created = await provider.admin('/api/v1/invitations', {
      method: 'POST',
      body: JSON.stringify({ email, client_id: app.clientId }),
    })
    await conform('POST', '/api/v1/invitations', created.clone())
    expect(created.status, await created.clone().text()).toBe(201)

    const link = await waitForLink(provider.config.mailApi, email, '/invitation/accept')
    const token = link.searchParams.get('token')!
    const shown = await fetch(`${provider.config.api}/api/v1/invitations/token/${encodeURIComponent(token)}`)
    await conform('GET', '/api/v1/invitations/token/{token}', shown.clone())
    expect(shown.status).toBe(200)
    const { invitation } = (await shown.json()) as { invitation: { client?: Record<string, string> } }
    expect(invitation.client).toMatchObject({ client_id: app.clientId, initiate_login_uri: 'https://app.example.test/start' })
    expect(invitation.client?.name).toBeTruthy()

    const id = ((await created.json()) as { invitation: { id: string } }).invitation.id
    await provider.admin(`/api/v1/invitations/${id}`, { method: 'DELETE' })
  })

  it('refuses an invitation for an application the tenant does not have', async () => {
    const res = await provider.admin('/api/v1/invitations', {
      method: 'POST',
      body: JSON.stringify({ email: `conformance-${randomToken(6)}@example.test`, client_id: `no-such-${randomToken(6)}` }),
    })
    await conform('POST', '/api/v1/invitations', res.clone())
    expect(res.status).toBe(400)
    expect(((await res.json()) as { code: string }).code).toBe('unknown_client')
  })
})
