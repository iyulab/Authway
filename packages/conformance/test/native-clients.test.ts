import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { createInterface } from 'node:readline'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { Browser } from '../src/browser.js'
import { loadConfig } from '../src/config.js'
import { randomToken } from '../src/pkce.js'
import { Provider, type TestClient, type TestUser } from '../src/provider.js'

const config = loadConfig()
const provider = new Provider(config)

// The native sign-in clients this repository ships (`clients/`), driven end to end against the
// provider: each runs its sign-in example, the suite plays the user in the browser, and the
// example reports what it got. Name each example's command, as `name=command`, separated by `;`:
//   CONFORMANCE_NATIVE_CLIENTS="rust=clients/rust/target/debug/examples/sign_in.exe"
// The example is run as `<command> <issuer> <client id> --refresh-ahead <seconds>` and prints one
// `STEP value` line per step (see `clients/rust/examples/sign_in.rs`).
const clients = (process.env.CONFORMANCE_NATIVE_CLIENTS ?? '')
  .split(';')
  .map((entry) => entry.trim())
  .filter(Boolean)
  .map((entry) => {
    const at = entry.indexOf('=')
    return { name: entry.slice(0, at), command: entry.slice(at + 1) }
  })

const AUDIENCE = 'https://api.conformance.example'

/** Runs an example and hands out its output line by line, as `STEP value`. */
class Example {
  private readonly lines: string[] = []
  private readonly waiting: ((line: string) => void)[] = []
  readonly exited: Promise<number | null>

  constructor(private readonly child: ChildProcessWithoutNullStreams) {
    createInterface({ input: child.stdout }).on('line', (line) => {
      const waiter = this.waiting.shift()
      if (waiter) waiter(line)
      else this.lines.push(line)
    })
    this.exited = new Promise((resolve) => child.on('exit', resolve))
  }

  /** The value of the next line, which must be `step`. */
  async next(step: string): Promise<string> {
    const line = this.lines.shift() ?? (await new Promise<string>((resolve) => this.waiting.push(resolve)))
    const [got, ...rest] = line.split(' ')
    if (got !== step) throw new Error(`Expected ${step}, the example printed: ${line}`)
    return rest.join(' ')
  }

  kill(): void {
    this.child.kill()
  }
}

function run(command: string, args: string[]): Example {
  const [program, ...pre] = command.split(' ')
  return new Example(spawn(program!, [...pre, ...args], { stdio: ['pipe', 'pipe', 'pipe'] }))
}

/** Plays the user in the browser: signs in, and follows the provider back to the app's loopback port. */
async function signInAt(authorizeUrl: string, user: TestUser): Promise<Response> {
  const browser = new Browser()
  const screen = await provider.openLoginScreen(browser, authorizeUrl)
  const flow = screen.searchParams.get('flow')
  if (!flow) throw new Error(`The login screen opened without a flow id: ${screen}`)
  const login = await provider.submitPassword(browser, flow, user)
  const step = (await login.json()) as { next?: string; redirect_to?: string }
  if (step.next !== 'redirect' || !step.redirect_to) throw new Error(`Login did not continue: ${JSON.stringify(step)}`)
  // The client is registered to skip consent: the provider answers the app straight away.
  const back = await provider.afterLogin(browser, step.redirect_to)
  if (back.hostname !== '127.0.0.1' || !back.searchParams.has('code')) {
    throw new Error(`The provider did not send the browser back to the loopback port: ${back}`)
  }
  // The browser follows the redirect to the app — a real request to the port it listens on.
  return fetch(back)
}

function claimsOf(jwt: string): Record<string, unknown> {
  return JSON.parse(Buffer.from(jwt.split('.')[1]!, 'base64url').toString()) as Record<string, unknown>
}

describe.skipIf(clients.length === 0)('native sign-in clients', () => {
  let client: TestClient
  let user: TestUser
  let sub: string | undefined

  beforeAll(async () => {
    const res = await provider.admin('/api/v1/clients', {
      method: 'POST',
      body: JSON.stringify({
        tenant_id: await provider.tenantId(),
        name: `conformance-native-${randomToken(4)}`,
        public: true,
        skip_consent: true,
        redirect_uris: ['http://127.0.0.1/callback'],
        grant_types: ['authorization_code', 'refresh_token'],
        scopes: ['openid', 'profile', 'email', 'offline_access'],
        access_token_strategy: 'jwt',
        audience: [AUDIENCE],
      }),
    })
    expect(res.status, await res.clone().text()).toBe(201)
    const { client: created } = (await res.json()) as { client: { id: string; client_id: string } }
    client = { id: created.id, clientId: created.client_id }
    user = await provider.inviteAndAccept()
  })

  afterAll(async () => {
    if (client) await provider.deleteClient(client)
    if (sub) await provider.deleteUser(sub)
  })

  for (const { name, command } of clients) {
    it(`${name}: signs in through the loopback port, refreshes, and signs out`, { timeout: 120_000 }, async () => {
      // A refresh-ahead longer than the access token lives makes the first access_token() refresh.
      const example = run(command, [config.issuer, client.clientId, '--refresh-ahead', '7200'])
      try {
        const url = new URL(await example.next('URL'))
        expect(url.searchParams.get('code_challenge_method')).toBe('S256')
        expect(url.searchParams.get('scope')).toContain('offline_access')

        const page = await signInAt(url.toString(), user)
        expect(page.status).toBe(200)

        sub = await example.next('SIGNED_IN')
        expect(sub).toMatch(/^[0-9a-f-]{36}$/)
        const before = await example.next('REFRESH_BEFORE')
        const access = await example.next('ACCESS')
        const after = await example.next('REFRESH_AFTER')
        expect(before).toBeTruthy()
        // The access token was renewed with the refresh token, which the provider rotated.
        expect(after).toBeTruthy()
        expect(after).not.toBe(before)

        const claims = claimsOf(access)
        expect(claims.sub).toBe(sub)
        expect(claims.client_id).toBe(client.clientId)
        expect(claims.aud).toContain(AUDIENCE)

        // The stored session leaves the access token in the process, so it fits one Windows
        // Credential Manager entry (2,560 bytes) even with a JWT access token.
        const [bytes, held] = (await example.next('STORED')).split(' ')
        expect(held).toBe('access=false')
        expect(Number(bytes)).toBeLessThanOrEqual(2560)

        expect(await example.next('SIGNED_OUT')).toBe('revoked=true')
        expect(await example.exited).toBe(0)

        // Signed out: the provider no longer renews the session.
        const refused = await provider.refresh(client, after)
        expect(refused.status).toBeGreaterThanOrEqual(400)
        expect(refused.error).toBe('invalid_grant')
      } finally {
        example.kill()
      }
    })
  }
})
