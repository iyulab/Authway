import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const provider = new Provider(loadConfig())

// An address that differs only in case is the same address: an account
// invited under one spelling signs in under any other, and the address cannot
// be invited again by writing it differently.
describe('the case of an address', () => {
  let client: TestClient
  let user: TestUser
  const subjects = new Set<string>()
  const shouted = (address: string) => address.toUpperCase()

  beforeAll(async () => {
    client = await provider.createPublicClient()
    user = await provider.inviteAndAccept(shouted)
  })

  afterAll(async () => {
    for (const sub of subjects) await provider.deleteUser(sub)
    if (client) await provider.deleteClient(client)
  })

  it('signs in whichever way the address is written', async () => {
    const plain = await provider.login(client, user)
    expect(plain.kind, JSON.stringify(plain)).toBe('code')
    if (plain.kind !== 'code') return
    subjects.add(plain.sub)

    const loud = await provider.login(client, { ...user, email: shouted(user.email) })
    expect(loud.kind, JSON.stringify(loud)).toBe('code')
    if (loud.kind !== 'code') return
    expect(loud.sub).toBe(plain.sub)
  })

  it('does not invite an address that is already a member by writing it differently', async () => {
    const res = await provider.admin('/api/v1/invitations', {
      method: 'POST',
      body: JSON.stringify({ email: user.email }),
    })
    expect(res.status).toBe(409)
    const again = await provider.admin('/api/v1/invitations', {
      method: 'POST',
      body: JSON.stringify({ email: shouted(user.email) }),
    })
    expect(again.status).toBe(409)
  })
})
