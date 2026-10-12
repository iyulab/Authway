import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import { randomToken } from './pkce.js'

/** Who the stand-in identity provider says has signed in. */
export interface StandInIdentity {
  /** The provider's own stable id for the account. */
  subject: string
  email: string
  /** Whether the provider vouches that the address belongs to the account. */
  emailVerified: boolean
  name?: string
}

/**
 * An identity provider the suite plays itself, so that sign-in through
 * another provider can be driven without a real one: the provider under test
 * is configured to exchange codes and read the user's profile here.
 *
 * The suite also plays the user: it never opens the stand-in's sign-in page,
 * it hands the browser the code the page would have sent it back with
 * (`codeFor`). What the provider under test then asks — the code exchange and
 * the profile — is answered from the identity that code stands for.
 *
 * The profile carries the field names of both generations of Google's API
 * (`id`/`sub`, `verified_email`/`email_verified`), since that is the provider
 * the stand-in is put in the place of.
 */
export class StandInProvider {
  private server?: Server
  private readonly codes = new Map<string, StandInIdentity>()
  private readonly tokens = new Map<string, StandInIdentity>()

  constructor(readonly url: URL) {}

  get origin(): string {
    return this.url.origin
  }

  async start(): Promise<void> {
    const server = createServer((req, res) => void this.answer(req, res))
    await new Promise<void>((resolve, reject) => {
      server.once('error', reject)
      server.listen(Number(this.url.port), resolve)
    })
    this.server = server
  }

  async stop(): Promise<void> {
    const server = this.server
    if (!server) return
    await new Promise<void>((resolve) => server.close(() => resolve()))
  }

  /** A one-time code that signs `identity` in. */
  codeFor(identity: StandInIdentity): string {
    const code = randomToken(16)
    this.codes.set(code, identity)
    return code
  }

  private async answer(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const path = new URL(req.url ?? '/', this.url).pathname
    const json = (status: number, body: unknown): void => {
      res.writeHead(status, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify(body))
    }

    if (req.method === 'POST' && path.endsWith('/token')) {
      let raw = ''
      for await (const chunk of req) raw += chunk
      const code = new URLSearchParams(raw).get('code') ?? ''
      const identity = this.codes.get(code)
      if (!identity) return json(400, { error: 'invalid_grant' })
      this.codes.delete(code)
      const accessToken = randomToken(16)
      this.tokens.set(accessToken, identity)
      return json(200, { access_token: accessToken, token_type: 'Bearer', expires_in: 60 })
    }

    if (req.method === 'GET' && path.endsWith('/userinfo')) {
      const identity = this.tokens.get((req.headers.authorization ?? '').replace(/^Bearer /, ''))
      if (!identity) return json(401, { error: 'invalid_token' })
      const [given, ...family] = (identity.name ?? 'Stand-in User').split(' ')
      return json(200, {
        id: identity.subject,
        sub: identity.subject,
        email: identity.email,
        verified_email: identity.emailVerified,
        email_verified: identity.emailVerified,
        name: identity.name ?? 'Stand-in User',
        given_name: given,
        family_name: family.join(' '),
      })
    }

    json(404, { error: 'not_found' })
  }
}
