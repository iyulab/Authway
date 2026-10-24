import { randomUUID } from 'node:crypto'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { adminOperations, conform } from '../src/contract.js'
import { randomToken } from '../src/pkce.js'
import { Provider, REDIRECT_URI, type TestClient, type TestUser } from '../src/provider.js'

const CONSOLE_AUDIENCE = 'urn:authway:console'

const config = loadConfig()
const provider = new Provider(config)

const code = async (res: Response) => ((await res.json()) as { code: string }).code

/**
 * The administration routes that answer a console account about itself, not
 * about a tenant — they succeed whatever tenant the request names.
 */
const answersForItself = (method: string, path: string) =>
  (method === 'GET' && path === '/api/v1/console/me') || (method === 'GET' && path === '/api/v1/tenants')

/** An administration call with a console account's access token. */
const asAccount = (token: string, path: string, init: RequestInit & { tenant?: string } = {}) => {
  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${token}`)
  if (init.tenant) headers.set('X-Tenant-ID', init.tenant)
  if (init.body) headers.set('Content-Type', 'application/json')
  return fetch(`${config.api}${path}`, { ...init, headers })
}

/** An administration call with the admin key and no tenant named. */
const asOperatorKey = (path: string, init: RequestInit = {}) => {
  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${config.adminKey}`)
  if (init.body) headers.set('Content-Type', 'application/json')
  return fetch(`${config.api}${path}`, { ...init, headers })
}

describe('console accounts: a system tenant user administers what its memberships say', () => {
  let systemTenant: string
  let administered: string
  let system: Provider
  let consoleClient: TestClient
  let otherApp: TestClient
  let token: string
  let otherAppToken: string
  let sub: string
  let user: TestUser
  const memberships: string[] = []

  const registerClient = async (audience?: string[]): Promise<TestClient> => {
    const res = await system.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: systemTenant,
        name: `conformance-console-${randomToken(4)}`,
        public: true,
        redirect_uris: [REDIRECT_URI],
        allowed_origins: ['http://localhost:9999'],
        grant_types: ['authorization_code'],
        scopes: ['openid', 'profile', 'email'],
        access_token_strategy: 'jwt',
        ...(audience ? { audience } : {}),
      }),
    })
    await conform('POST', '/api/v1/clients', res.clone())
    expect(res.status, await res.clone().text()).toBe(201)
    const { client } = (await res.json()) as { client: { id: string; client_id: string } }
    return { id: client.id, clientId: client.client_id }
  }

  const put = async (body: Record<string, string>) => {
    const res = await asOperatorKey('/api/v1/console/memberships', { method: 'PUT', body: JSON.stringify(body) })
    await conform('PUT', '/api/v1/console/memberships', res.clone())
    expect([200, 201]).toContain(res.status)
    const { membership } = (await res.json()) as { membership: { id: string } }
    if (!memberships.includes(membership.id)) memberships.push(membership.id)
    return membership.id
  }

  beforeAll(async () => {
    administered = await provider.tenantId()
    const created = await asOperatorKey('/api/v1/tenants', {
      method: 'POST',
      body: JSON.stringify({ name: `Conformance console ${randomToken(4)}`, slug: `conformance-console-${randomToken(6).toLowerCase()}` }),
    })
    expect(created.status).toBe(201)
    systemTenant = ((await created.json()) as { id: string }).id

    const chosen = await asOperatorKey('/api/v1/console/system-tenant', { method: 'PUT', body: JSON.stringify({ tenant_id: systemTenant }) })
    await conform('PUT', '/api/v1/console/system-tenant', chosen.clone())
    expect(chosen.status, await chosen.clone().text()).toBe(200)

    system = new Provider({ ...config, tenantId: systemTenant })
    user = await system.inviteAndAccept()
    consoleClient = await registerClient([CONSOLE_AUDIENCE])
    otherApp = await registerClient()

    const signedIn = await system.login(consoleClient, user)
    if (signedIn.kind !== 'code') throw new Error(`console sign-in stopped at ${signedIn.kind}`)
    token = signedIn.tokens.access_token
    sub = signedIn.sub
    const elsewhere = await system.login(otherApp, user)
    if (elsewhere.kind !== 'code') throw new Error(`sign-in to the other application stopped at ${elsewhere.kind}`)
    otherAppToken = elsewhere.tokens.access_token
  })

  afterAll(async () => {
    for (const id of memberships) await asOperatorKey(`/api/v1/console/memberships/${id}`, { method: 'DELETE' })
    if (sub) await system.deleteUser(sub)
    for (const c of [consoleClient, otherApp]) if (c) await system.deleteClient(c)
  })

  it('refuses an account that holds no membership', async () => {
    const res = await asAccount(token, '/api/v1/console/me')
    await conform('GET', '/api/v1/console/me', res.clone())
    expect(res.status).toBe(403)
    expect(await code(res)).toBe('no_console_access')
  })

  it('does not take a token the account holds for another application of the system tenant', async () => {
    await put({ user_id: sub, tenant_id: administered, role: 'admin' })
    const res = await asAccount(otherAppToken, '/api/v1/console/me')
    expect(res.status).toBe(401)
  })

  it('lets an admin administer its tenant, and nothing else', async () => {
    await put({ user_id: sub, tenant_id: administered, role: 'admin' })

    const me = await asAccount(token, '/api/v1/console/me')
    await conform('GET', '/api/v1/console/me', me.clone())
    expect(me.status).toBe(200)
    const body = (await me.json()) as { actor_type: string; operator: boolean; memberships: { tenant_id: string; role: string }[] }
    expect(body).toMatchObject({ actor_type: 'console_user', operator: false })
    expect(body.memberships).toEqual([expect.objectContaining({ tenant_id: administered, role: 'admin' })])

    // Its one tenant is the one a call is about, named or not.
    for (const tenant of [undefined, administered]) {
      const clients = await asAccount(token, '/api/v1/clients?limit=1', { tenant })
      await conform('GET', '/api/v1/clients', clients.clone())
      expect(clients.status, `clients, tenant ${tenant}`).toBe(200)
    }
    const tenants = await asAccount(token, '/api/v1/tenants')
    await conform('GET', '/api/v1/tenants', tenants.clone())
    expect(((await tenants.json()) as { id: string }[]).map((t) => t.id)).toEqual([administered])

    // Another tenant is answered as one that does not exist.
    for (const [path, template] of [
      ['/api/v1/clients', '/api/v1/clients'],
      ['/api/v1/users', '/api/v1/users'],
      ['/api/v1/webhooks', '/api/v1/webhooks'],
    ] as const) {
      const res = await asAccount(token, path, { tenant: systemTenant })
      await conform('GET', template, res.clone())
      expect(res.status, path).toBe(404)
    }
    const otherTenant = await asAccount(token, `/api/v1/tenants/${systemTenant}`)
    expect(otherTenant.status).toBe(404)

    // The platform's operations and the owner's are refused.
    const create = await asAccount(token, '/api/v1/tenants', { method: 'POST', body: JSON.stringify({ name: 'Not allowed', slug: `not-allowed-${randomToken(6).toLowerCase()}` }) })
    await conform('POST', '/api/v1/tenants', create.clone())
    expect(create.status).toBe(403)
    expect(await code(create)).toBe('insufficient_scope')
    const credentials = await asAccount(token, `/api/v1/tenants/${administered}/service-clients`)
    expect(credentials.status).toBe(403)
    const rename = await asAccount(token, `/api/v1/tenants/${administered}`, { method: 'PUT', body: '{}' })
    expect(rename.status).toBe(403)
    const giveOperator = await asAccount(token, '/api/v1/console/memberships', { method: 'PUT', body: JSON.stringify({ user_id: sub, role: 'operator' }) })
    expect(giveOperator.status).toBe(403)
  })

  it('names the console account that invites, in its tenant only', async () => {
    await put({ user_id: sub, tenant_id: administered, role: 'admin' })
    const email = `conformance-invited-${randomToken(6).toLowerCase()}@example.test`
    const res = await asAccount(token, '/api/v1/invitations', { method: 'POST', body: JSON.stringify({ email }) })
    await conform('POST', '/api/v1/invitations', res.clone())
    expect(res.status, await res.clone().text()).toBe(201)
    const { invitation } = (await res.json()) as { invitation: { id: string; inviter_id: string | null; inviter_name: string } }
    expect(invitation.inviter_id).toBe(sub)
    expect(invitation.inviter_name).not.toBe('system')
    await asAccount(token, `/api/v1/invitations/${invitation.id}`, { method: 'DELETE' })

    const elsewhere = await asAccount(token, '/api/v1/invitations', { method: 'POST', tenant: systemTenant, body: JSON.stringify({ email }) })
    expect(elsewhere.status).toBe(404)
  })

  it('signs console accounts in with the console’s own client', async () => {
    const entry = await fetch(`${config.api}/api/v1/console/entry`)
    await conform('GET', '/api/v1/console/entry', entry.clone())
    if (!config.consoleUrl) {
      expect(entry.status).toBe(404)
      expect(await code(entry)).toBe('console_sign_in_unavailable')
      return
    }
    expect(entry.status, await entry.clone().text()).toBe(200)
    const given = (await entry.json()) as { client_id: string; redirect_uri: string; scope: string; audience: string }
    expect(given).toMatchObject({ redirect_uri: `${config.consoleUrl}/callback`, audience: CONSOLE_AUDIENCE })
    const again = (await (await fetch(`${config.api}/api/v1/console/entry`)).json()) as { client_id: string }
    expect(again.client_id).toBe(given.client_id)

    await put({ user_id: sub, tenant_id: administered, role: 'admin' })
    const signedIn = await system.login({ id: '', clientId: given.client_id, redirectUri: given.redirect_uri }, user, { scopes: given.scope.split(' ') })
    if (signedIn.kind !== 'code') throw new Error(`console sign-in stopped at ${signedIn.kind}`)
    // No consent screen, and no refresh token: the console signs in again silently.
    expect(signedIn.consent).not.toBe('asked')
    expect(signedIn.tokens.refresh_token).toBeUndefined()
    const me = await asAccount(signedIn.tokens.access_token, '/api/v1/console/me')
    await conform('GET', '/api/v1/console/me', me.clone())
    expect(me.status, await me.clone().text()).toBe(200)
    expect(await me.json()).toMatchObject({ actor_type: 'console_user', user_id: sub })

    // It is Authway's own: the administration API does not change or delete it.
    const listed = (await (await system.admin('/api/v1/clients?limit=100')).json()) as { clients: { id: string; client_id: string; system: boolean }[] }
    const own = listed.clients.find((c) => c.client_id === given.client_id)
    expect(own?.system).toBe(true)
    const removed = await system.admin(`/api/v1/clients/${own!.id}`, { method: 'DELETE' })
    await conform('DELETE', '/api/v1/clients/{id}', removed.clone())
    expect(removed.status).toBe(409)
    expect(await code(removed)).toBe('system_client')
  })

  it('holds the routes a service credential may also call to the same scope', async () => {
    await put({ user_id: sub, tenant_id: administered, role: 'admin' })
    const member = await provider.inviteAndAccept()
    const users = (await (await provider.admin('/api/v1/users?limit=100')).json()) as { users: { id: string; email: string }[] }
    const memberId = users.users.find((u) => u.email === member.email)?.id
    if (!memberId) throw new Error('the invited user is not listed')
    const issued = await provider.admin(`/api/v1/tenants/${administered}/service-clients`, {
      method: 'POST',
      body: JSON.stringify({ name: `conformance-console-${randomToken(4)}`, scopes: ['admin.claims:write'] }),
    })
    expect(issued.status, await issued.clone().text()).toBe(201)
    const { service_client: svc, credentials } = (await issued.json()) as {
      service_client: { id: string }
      credentials: { client_id: string; client_secret: string }
    }
    try {
      const claims = JSON.stringify({ claims: { conformance_console: 'yes' } })

      // A console account: its tenant's user, not the system tenant's.
      const own = await asAccount(token, `/api/v1/users/${memberId}/claims`, { method: 'PATCH', body: claims })
      await conform('PATCH', '/api/v1/users/{id}/claims', own.clone())
      expect(own.status, await own.clone().text()).toBe(200)
      const outside = await asAccount(token, `/api/v1/users/${sub}/claims`, { method: 'PATCH', body: claims })
      await conform('PATCH', '/api/v1/users/{id}/claims', outside.clone())
      expect(outside.status).toBe(404)

      // A service credential of the tenant, naming another tenant.
      const { token_endpoint } = await provider.discovery()
      const granted = await fetch(token_endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({ grant_type: 'client_credentials', client_id: credentials.client_id, client_secret: credentials.client_secret }),
      })
      expect(granted.status).toBe(200)
      const serviceToken = ((await granted.json()) as { access_token: string }).access_token
      const elsewhere = await asAccount(serviceToken, `/api/v1/users/${sub}/claims`, { method: 'PATCH', body: claims, tenant: systemTenant })
      await conform('PATCH', '/api/v1/users/{id}/claims', elsewhere.clone())
      expect(elsewhere.status).toBe(404)
      const mine = await asAccount(serviceToken, `/api/v1/users/${memberId}/claims`, { method: 'PATCH', body: claims })
      expect(mine.status).toBe(200)
    } finally {
      await provider.admin(`/api/v1/tenants/${administered}/service-clients/${svc.id}`, { method: 'DELETE' })
      await provider.deleteUser(memberId)
    }
  })

  // Every administration route the contract has, called by an account that
  // administers one tenant and naming another: none of them answers it.
  it('keeps every administration route to what the account administers', async () => {
    await put({ user_id: sub, tenant_id: administered, role: 'admin' })
    const leaks: string[] = []
    for (const { method, path } of adminOperations()) {
      const concrete = path.replace(/\{([^}]+)\}/g, (_, name: string) =>
        name === 'id' && path.startsWith('/api/v1/tenants/{id}') ? systemTenant : randomUUID(),
      )
      const res = await asAccount(token, concrete, {
        method,
        tenant: systemTenant,
        ...(method === 'GET' || method === 'DELETE' ? {} : { body: '{}' }),
      })
      if (res.ok && !answersForItself(method, path)) leaks.push(`${method} ${path} → ${res.status}`)
    }
    expect(leaks).toEqual([])
  })

  it('tells the admin key it is an operator with no memberships', async () => {
    const me = await asOperatorKey('/api/v1/console/me')
    await conform('GET', '/api/v1/console/me', me.clone())
    expect(me.status).toBe(200)
    expect(await me.json()).toMatchObject({ actor_type: 'api_key', operator: true, memberships: [] })
  })

  it('lets an owner change its tenant but not turn it off', async () => {
    await put({ user_id: sub, tenant_id: administered, role: 'owner' })
    const current = (await (await asAccount(token, `/api/v1/tenants/${administered}`)).json()) as { name: string; active: boolean }

    const same = await asAccount(token, `/api/v1/tenants/${administered}`, { method: 'PUT', body: JSON.stringify({ name: current.name }) })
    await conform('PUT', '/api/v1/tenants/{id}', same.clone())
    expect(same.status, await same.clone().text()).toBe(200)

    const off = await asAccount(token, `/api/v1/tenants/${administered}`, { method: 'PUT', body: JSON.stringify({ active: !current.active }) })
    await conform('PUT', '/api/v1/tenants/{id}', off.clone())
    expect(off.status).toBe(403)
    expect(await code(off)).toBe('insufficient_scope')

    const listed = await asAccount(token, '/api/v1/console/memberships')
    await conform('GET', '/api/v1/console/memberships', listed.clone())
    expect(listed.status).toBe(200)
  })

  it("lets an operator take the platform's actions on every tenant", async () => {
    await put({ user_id: sub, role: 'operator' })
    const tenants = await asAccount(token, '/api/v1/tenants')
    const ids = ((await tenants.json()) as { id: string }[]).map((t) => t.id)
    expect(ids).toEqual(expect.arrayContaining([administered, systemTenant]))
    const read = await asAccount(token, '/api/v1/console/system-tenant')
    await conform('GET', '/api/v1/console/system-tenant', read.clone())
    expect(((await read.json()) as { tenant: { id: string; is_system: boolean } }).tenant).toMatchObject({ id: systemTenant, is_system: true })
    const record = await asAccount(token, `/api/v1/tenants/${administered}`)
    await conform('GET', '/api/v1/tenants/{id}', record.clone())
    expect(record.status).toBe(200)
  })

  // An operator reaches into a tenant's contents only through support access
  // the tenant's audit log records — not through a role it also holds there
  // (the account is still the earlier test's owner of `administered`).
  it('lets an operator into a tenant only through support access, on the record', async () => {
    await put({ user_id: sub, role: 'operator' })
    const users = () => asAccount(token, '/api/v1/users', { tenant: administered })

    const shut = await users()
    await conform('GET', '/api/v1/users', shut.clone())
    expect(shut.status).toBe(403)
    expect(await code(shut)).toBe('support_access_required')

    const start = (body: Record<string, string>) =>
      asAccount(token, '/api/v1/console/support-access', { method: 'POST', body: JSON.stringify(body) })
    const noReason = await start({ tenant_id: administered, reason: '  ' })
    await conform('POST', '/api/v1/console/support-access', noReason.clone())
    expect(noReason.status).toBe(400)
    expect(((await noReason.json()) as { field?: string }).field).toBe('reason')
    const byKey = await asOperatorKey('/api/v1/console/support-access', {
      method: 'POST',
      body: JSON.stringify({ tenant_id: administered, reason: 'conformance' }),
    })
    await conform('POST', '/api/v1/console/support-access', byKey.clone())
    expect(byKey.status).toBe(403)
    expect(await code(byKey)).toBe('console_account_required')

    const reason = `conformance ${randomToken(6)}`
    const opened = await start({ tenant_id: administered, reason })
    await conform('POST', '/api/v1/console/support-access', opened.clone())
    expect(opened.status, await opened.clone().text()).toBe(201)
    const access = ((await opened.json()) as { support_access: { id: string; tenant_id: string; started_at: string; expires_at: string } }).support_access
    expect(access.tenant_id).toBe(administered)
    expect(Date.parse(access.expires_at) - Date.parse(access.started_at)).toBe(60 * 60 * 1000)

    const inside = await users()
    await conform('GET', '/api/v1/users', inside.clone())
    expect(inside.status, await inside.clone().text()).toBe(200)
    // Another tenant stays shut.
    const elsewhere = await asAccount(token, '/api/v1/users', { tenant: systemTenant })
    expect(elsewhere.status).toBe(403)

    const listed = await asAccount(token, '/api/v1/console/support-access')
    await conform('GET', '/api/v1/console/support-access', listed.clone())
    expect(((await listed.json()) as { support_access: { id: string }[] }).support_access.map((a) => a.id)).toContain(access.id)

    const recorded = async (action: string) => {
      const deadline = Date.now() + 20_000
      for (;;) {
        const res = await provider.admin(`/api/v1/audit/logs?action=${action}&resource_id=${access.id}&limit=5`)
        expect(res.status).toBe(200)
        const logs = ((await res.json()) as { logs: { actor_id: string | null; details?: Record<string, unknown> }[] }).logs
        if (logs.length > 0 || Date.now() > deadline) return logs
        await new Promise((r) => setTimeout(r, 250))
      }
    }
    const started = await recorded('support_access.started')
    expect(started).toHaveLength(1)
    expect(started[0].actor_id).toBe(sub)

    const end = await asAccount(token, `/api/v1/console/support-access/${access.id}`, { method: 'DELETE' })
    await conform('DELETE', '/api/v1/console/support-access/{id}', end.clone())
    expect(end.status).toBe(204)
    const again = await asAccount(token, `/api/v1/console/support-access/${access.id}`, { method: 'DELETE' })
    expect(again.status).toBe(404)
    expect((await users()).status).toBe(403)
    expect(await recorded('support_access.ended')).toHaveLength(1)
  })

  it('keeps the system tenant on and in place', async () => {
    const del = await asOperatorKey(`/api/v1/tenants/${systemTenant}`, { method: 'DELETE' })
    await conform('DELETE', '/api/v1/tenants/{id}', del.clone())
    expect(del.status).toBe(409)
    const replace = await asOperatorKey('/api/v1/console/system-tenant', { method: 'PUT', body: JSON.stringify({ tenant_id: administered }) })
    await conform('PUT', '/api/v1/console/system-tenant', replace.clone())
    expect(replace.status).toBe(409)
    expect(await code(replace)).toBe('system_tenant_in_use')
    const unknown = await asOperatorKey('/api/v1/console/memberships', { method: 'PUT', body: JSON.stringify({ user_id: randomUUID(), tenant_id: administered, role: 'admin' }) })
    expect(unknown.status).toBe(400)
    // The system tenant's users are the console accounts: only operators
    // administer them, or an owner of it could take an operator's account.
    const ownSystem = await asOperatorKey('/api/v1/console/memberships', { method: 'PUT', body: JSON.stringify({ user_id: sub, tenant_id: systemTenant, role: 'owner' }) })
    await conform('PUT', '/api/v1/console/memberships', ownSystem.clone())
    expect(ownSystem.status).toBe(400)
    expect(((await ownSystem.json()) as { field?: string }).field).toBe('tenant_id')
  })
})
