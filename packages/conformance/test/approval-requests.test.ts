import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { conform } from '../src/contract.js'
import { loadConfig } from '../src/config.js'
import { generateKeyPair } from 'jose'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())
const api = provider.config.api
const APPROVAL_GRANT = 'urn:authway:params:oauth:grant-type:approval'

const claimsOf = (jwt: string): Record<string, unknown> =>
  JSON.parse(Buffer.from(jwt.split('.')[1], 'base64url').toString('utf8')) as Record<string, unknown>

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

// A client acting for a user (a delegation) asks the user to approve one
// action while they are away. The user answers on Authway's approval screen,
// signed in to the account application; the client polls the token endpoint
// and, once approved, receives one short-lived token bound to the approval.
// Polling waits out the interval (5 s) twice, so these take longer than the
// default test timeout.
describe('approval requests', { timeout: 90_000 }, () => {
  let offered = false
  let agent: TestClient
  let account: TestClient
  let user: TestUser
  let handle = ''
  let sub = ''

  const ask = async (fields: Record<string, string> = {}): Promise<{ status: number; body: Record<string, unknown> }> => {
    const res = await fetch(`${api}/oauth2/approval-requests`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        subject_token: handle,
        subject_token_type: 'urn:authway:params:oauth:token-type:delegation-grant',
        binding_message: 'Pay 10 EUR to Acme',
        authorization_details: JSON.stringify([{ type: 'payment', ref: 'order-42' }]),
        ...(await provider.clientAuth(agent)),
        ...fields,
      }),
    })
    return { status: res.status, body: (await res.json()) as Record<string, unknown> }
  }

  const poll = async (authReqId: string): Promise<{ status: number; body: Record<string, unknown> }> => {
    const { token_endpoint } = await provider.discovery()
    const res = await fetch(token_endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ grant_type: APPROVAL_GRANT, auth_req_id: authReqId, ...(await provider.clientAuth(agent)) }),
    })
    return { status: res.status, body: (await res.json()) as Record<string, unknown> }
  }

  const answer = async (token: string, id: string, decision: 'approve' | 'deny'): Promise<Response> => {
    const res = await fetch(`${api}/api/v1/users/approvals/${id}/${decision}`, { method: 'POST', headers: { Authorization: `Bearer ${token}` } })
    await conform('POST', `/api/v1/users/approvals/{id}/${decision}`, res.clone())
    return res
  }

  const accountToken = async (u: TestUser): Promise<string> => {
    const outcome = await provider.login(account, u, { scopes: ['openid'] })
    if (outcome.kind !== 'code') throw new Error(`signing in to the account application failed: ${JSON.stringify(outcome)}`)
    return outcome.tokens.access_token
  }

  const requestIdOf = (approveUrl: string): string => new URL(approveUrl).searchParams.get('request') ?? ''

  beforeAll(async () => {
    const caps = (await (await fetch(`${api}/api/v1/capabilities`)).json()) as { approval_requests?: boolean }
    offered = caps.approval_requests === true
    if (!offered) return
    agent = await provider.createDelegatingClient({ jwtAccessTokens: true })
    account = await provider.accountClient(agent)
    user = await provider.inviteAndAccept()
    const outcome = await provider.login(agent, user, { scopes: ['openid', 'profile', 'delegation'] })
    if (outcome.kind !== 'code') throw new Error(`granting the delegation failed: ${JSON.stringify(outcome)}`)
    handle = claimsOf(outcome.tokens.id_token ?? '').delegation_grant as string
    sub = outcome.sub
  })

  afterAll(async () => {
    if (agent) await provider.deleteClient(agent)
  })

  it('give a token bound to the approval once the user approves, and only once', async (ctx) => {
    if (!offered) ctx.skip()
    const asked = await ask({ scope: 'profile' })
    expect(asked.status, JSON.stringify(asked.body)).toBe(200)
    expect(asked.body.interval).toBe(5)
    expect(asked.body.expires_in).toBe(300)
    const authReqId = asked.body.auth_req_id as string
    const id = requestIdOf(asked.body.approve_url as string)

    const pending = await poll(authReqId)
    expect(pending.status).toBe(400)
    expect(pending.body.error).toBe('authorization_pending')
    const tooSoon = await poll(authReqId)
    expect(tooSoon.body.error).toBe('slow_down')

    const token = await accountToken(user)
    const shown = await fetch(`${api}/api/v1/users/approvals/${id}`, { headers: { Authorization: `Bearer ${token}` } })
    await conform('GET', '/api/v1/users/approvals/{id}', shown.clone())
    expect(shown.status).toBe(200)
    expect(await shown.json()).toMatchObject({
      client_id: agent.clientId,
      binding_message: 'Pay 10 EUR to Acme',
      scopes: ['profile'],
      status: 'pending',
      authorization_details: [{ type: 'payment', ref: 'order-42' }],
    })
    expect((await answer(token, id, 'approve')).status).toBe(204)
    expect((await answer(token, id, 'deny')).status).toBe(409)

    await sleep(5200)
    const issued = await poll(authReqId)
    expect(issued.status, JSON.stringify(issued.body)).toBe(200)
    expect(issued.body.refresh_token).toBeUndefined()
    const claims = claimsOf(issued.body.access_token as string)
    expect(claims.sub).toBe(sub)
    expect(claims.act).toEqual({ sub: agent.clientId })
    expect(claims.approval).toEqual({ id })
    expect(claims.authorization_details).toEqual([{ type: 'payment', ref: 'order-42' }])
    expect(claims.scope).toBe('profile')

    await sleep(5200)
    const again = await poll(authReqId)
    expect(again.body.error).toBe('invalid_grant')
  })

  it('end with access_denied when the user denies', async (ctx) => {
    if (!offered) ctx.skip()
    const asked = await ask()
    const id = requestIdOf(asked.body.approve_url as string)
    expect((await answer(await accountToken(user), id, 'deny')).status).toBe(204)
    const denied = await poll(asked.body.auth_req_id as string)
    expect(denied.body.error).toBe('access_denied')
  })

  it('are answered only by the user, on the approval screen', async (ctx) => {
    if (!offered) ctx.skip()
    const asked = await ask()
    const id = requestIdOf(asked.body.approve_url as string)

    // The client signs the user in to itself: that token cannot answer.
    const own = await provider.login(agent, user, { scopes: ['openid'] })
    if (own.kind !== 'code') throw new Error('sign-in failed')
    const byClient = await answer(own.tokens.access_token, id, 'approve')
    expect(byClient.status).toBe(403)
    expect(((await byClient.json()) as { code: string }).code).toBe('account_application_required')

    // Another user does not see it.
    const stranger = await accountToken(await provider.inviteAndAccept())
    const foreign = await fetch(`${api}/api/v1/users/approvals/${id}`, { headers: { Authorization: `Bearer ${stranger}` } })
    expect(foreign.status).toBe(404)
  })

  it('are refused for a handle the client does not hold, or without its secret', async (ctx) => {
    if (!offered) ctx.skip()
    expect((await ask({ subject_token: 'not-a-handle' })).body.error).toBe('invalid_grant')
    const noSecret = await ask({ client_secret: 'wrong' })
    expect(noSecret.status).toBe(401)
    expect(noSecret.body.error).toBe('invalid_client')
    expect((await ask({ scope: 'email' })).body.error).toBe('invalid_scope')
    expect((await ask({ binding_message: '' })).body.error).toBe('invalid_request')
    expect((await ask({ authorization_details: '{"not":"a list"}' })).body.error).toBe('invalid_authorization_details')
  })
})

// A client that holds a key rather than a secret asks and polls the way it
// authenticates at the token endpoint: a signed assertion, each one once.
describe('approval requests by a client that holds a key', { timeout: 60_000 }, () => {
  let offered = false
  let agent: TestClient
  let handle = ''

  const send = async (url: string, fields: Record<string, string>): Promise<{ status: number; body: Record<string, unknown> }> => {
    const res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body: new URLSearchParams(fields) })
    return { status: res.status, body: (await res.json()) as Record<string, unknown> }
  }
  const request = (auth: Record<string, string>) =>
    send(`${api}/oauth2/approval-requests`, {
      subject_token: handle,
      subject_token_type: 'urn:authway:params:oauth:token-type:delegation-grant',
      binding_message: 'K-7Q2',
      ...auth,
    })

  beforeAll(async () => {
    const caps = (await (await fetch(`${api}/api/v1/capabilities`)).json()) as { approval_requests?: boolean }
    offered = caps.approval_requests === true
    if (!offered) return
    agent = await provider.createDelegatingClient({ withKey: true })
    const user = await provider.inviteAndAccept()
    const outcome = await provider.login(agent, user, { scopes: ['openid', 'profile', 'delegation'] })
    if (outcome.kind !== 'code') throw new Error(`granting the delegation failed: ${JSON.stringify(outcome)}`)
    handle = claimsOf(outcome.tokens.id_token ?? '').delegation_grant as string
  })

  afterAll(async () => {
    if (agent) await provider.deleteClient(agent)
  })

  it('are taken with an assertion, and polled with another', async (ctx) => {
    if (!offered) ctx.skip()
    const asked = await request(await provider.clientAuth(agent))
    expect(asked.status, JSON.stringify(asked.body)).toBe(200)
    const { token_endpoint } = await provider.discovery()
    const pending = await send(token_endpoint, { grant_type: APPROVAL_GRANT, auth_req_id: asked.body.auth_req_id as string, ...(await provider.clientAuth(agent)) })
    expect(pending.body.error, JSON.stringify(pending.body)).toBe('authorization_pending')
  })

  it('refuse an assertion used before, or signed with another key', async (ctx) => {
    if (!offered) ctx.skip()
    const auth = await provider.clientAuth(agent)
    expect((await request(auth)).status).toBe(200)
    const replayed = await request(auth)
    expect(replayed.status).toBe(401)
    expect(replayed.body.error).toBe('invalid_client')

    const { privateKey } = await generateKeyPair('RS256')
    const forged = await request(await provider.clientAuth(agent, { privateKey, kid: agent.key!.kid }))
    expect(forged.status).toBe(401)
    expect(forged.body.error).toBe('invalid_client')
  })
})
