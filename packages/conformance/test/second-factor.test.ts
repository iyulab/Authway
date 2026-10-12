import { createHmac } from 'node:crypto'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { loadConfig } from '../src/config.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

/** The current code of an authenticator holding `secret` (RFC 6238: SHA-1, 30 s, 6 digits). */
function authenticatorCode(secret: string, at = Date.now()): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const ch of secret.replace(/=+$/, '').toUpperCase()) bits += alphabet.indexOf(ch).toString(2).padStart(5, '0')
  const key = Buffer.from((bits.match(/.{8}/g) ?? []).map((b) => parseInt(b, 2)))
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 30000)))
  const mac = createHmac('sha1', key).update(counter).digest()
  const offset = mac[mac.length - 1] & 0xf
  return ((mac.readUInt32BE(offset) & 0x7fffffff) % 1_000_000).toString().padStart(6, '0')
}

// A user's access token says who they are. Once they have a second factor,
// that token alone must not be able to replace the factor or switch it off —
// otherwise a stolen token takes the whole account.
describe('the second factor is not changed by a token alone', () => {
  let app: TestClient
  let user: TestUser
  let sub: string
  let accessToken: string
  let secret: string

  const mfa = (method: string, path: string, body?: unknown): Promise<Response> =>
    fetch(`${config.api}/api/v1/users/mfa${path}`, {
      method,
      headers: { Authorization: `Bearer ${accessToken}`, ...(body ? { 'Content-Type': 'application/json' } : {}) },
      body: body ? JSON.stringify(body) : undefined,
    })
  const isOn = async (): Promise<boolean> => ((await (await mfa('GET', '/status')).json()) as { enabled: boolean }).enabled

  beforeAll(async () => {
    app = await provider.createPublicClient()
    user = await provider.inviteAndAccept()
    const outcome = await provider.login(app, user)
    if (outcome.kind !== 'code') throw new Error(`Signing in failed: ${JSON.stringify(outcome)}`)
    sub = outcome.sub
    accessToken = outcome.tokens.access_token

    const setup = await mfa('POST', '/setup')
    expect(setup.status, await setup.clone().text()).toBe(200)
    secret = ((await setup.json()) as { secret: string }).secret
    const enabled = await mfa('POST', '/verify', { code: authenticatorCode(secret) })
    expect(enabled.status, await enabled.clone().text()).toBe(200)
  })

  afterAll(async () => {
    if (sub) await provider.deleteUser(sub)
    if (app) await provider.deleteClient(app)
  })

  it('refuses to set up another authenticator while one is on', async () => {
    const res = await mfa('POST', '/setup')
    expect(res.status).toBe(409)
    expect(((await res.json()) as { code: string }).code).toBe('mfa_already_enabled')
    // The authenticator the user has still works: turning the factor off with
    // its code is accepted further down.
    expect(await isOn()).toBe(true)
  })

  it('refuses to switch it off or to issue new recovery codes without a code', async () => {
    for (const [method, path] of [
      ['DELETE', ''],
      ['POST', '/recovery/regenerate'],
    ] as const) {
      for (const body of [undefined, { code: '000000' }]) {
        const res = await mfa(method, path, body)
        expect(res.status, `${method} ${path} ${JSON.stringify(body)}`).toBe(403)
        expect(((await res.json()) as { code: string }).code).toBe('mfa_code_required')
      }
    }
    expect(await isOn()).toBe(true)
  })

  it("switches it off with a code from the user's authenticator", async () => {
    const res = await mfa('DELETE', '', { code: authenticatorCode(secret) })
    expect(res.status, await res.clone().text()).toBe(200)
    expect(await isOn()).toBe(false)
  })
})
