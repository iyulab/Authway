import { createHash, generateKeyPairSync, randomBytes, sign, type KeyObject } from 'node:crypto'

/**
 * A passkey device for the suite: it makes credentials and signs challenges the
 * way an authenticator does, so a provider's passkey endpoints can be run
 * without a browser. The suite takes no dependency for it — the only CBOR an
 * attestation needs is a few maps, integers and byte strings.
 */

/** CBOR (RFC 8949) for what a WebAuthn "none" attestation holds. */
export function cbor(value: unknown): Buffer {
  const head = (major: number, n: number): Buffer => {
    if (n < 24) return Buffer.from([(major << 5) | n])
    if (n < 0x100) return Buffer.from([(major << 5) | 24, n])
    if (n < 0x10000) return Buffer.from([(major << 5) | 25, n >> 8, n & 0xff])
    const b = Buffer.alloc(5)
    b[0] = (major << 5) | 26
    b.writeUInt32BE(n, 1)
    return b
  }
  if (typeof value === 'number') return value >= 0 ? head(0, value) : head(1, -1 - value)
  if (typeof value === 'string') {
    const text = Buffer.from(value, 'utf8')
    return Buffer.concat([head(3, text.length), text])
  }
  if (Buffer.isBuffer(value) || value instanceof Uint8Array) {
    const bytes = Buffer.from(value)
    return Buffer.concat([head(2, bytes.length), bytes])
  }
  if (value instanceof Map) {
    return Buffer.concat([head(5, value.size), ...[...value].flatMap(([k, v]) => [cbor(k), cbor(v)])])
  }
  if (value && typeof value === 'object') return cbor(new Map(Object.entries(value)))
  throw new Error(`cbor: cannot encode ${typeof value}`)
}

/** Authenticator-data flags. */
export const Flag = { present: 0x01, verified: 0x04, backupEligible: 0x08, backedUp: 0x10, attested: 0x40 } as const

const b64u = (b: Buffer | Uint8Array): string => Buffer.from(b).toString('base64url')

/** What a device and browser put into an answer; a scenario bends one value at a time. */
export interface Ceremony {
  origin: string
  rpId: string
  challenge: string
  flags: number
  /** The client data type: "webauthn.create" or "webauthn.get". */
  type: string
  /** The user handle a sign-in answers; the one the credential was made for when absent. */
  userHandle?: Buffer | null
}

export class PasskeyDevice {
  readonly credentialId = randomBytes(32)
  /** The user.id of the options the credential was made for. */
  userHandle?: Buffer
  /** Whether the device reports a signature counter, as a hardware key does. */
  counts = false
  private counter = 0
  private readonly privateKey: KeyObject
  private readonly x: Buffer
  private readonly y: Buffer

  /** `origin` is the login screen's origin, the page a browser would ask from. */
  constructor(readonly origin: string) {
    const { privateKey, publicKey } = generateKeyPairSync('ec', { namedCurve: 'P-256' })
    this.privateKey = privateKey
    const jwk = publicKey.export({ format: 'jwk' })
    this.x = Buffer.from(jwk.x!, 'base64url')
    this.y = Buffer.from(jwk.y!, 'base64url')
  }

  /** The answer a browser sends for creation options (the server's `options`). */
  register(options: { challenge: string; rp: { id: string }; user: { id: string } }, bend: Partial<Ceremony> = {}): object {
    this.userHandle = Buffer.from(options.user.id, 'base64url')
    const c = this.ceremony(options.challenge, options.rp.id, 'webauthn.create', Flag.present | Flag.verified | Flag.backupEligible | Flag.backedUp | Flag.attested, bend)
    const cose = cbor(new Map<number, unknown>([[1, 2], [3, -7], [-1, 1], [-2, this.x], [-3, this.y]]))
    const id = Buffer.alloc(2)
    id.writeUInt16BE(this.credentialId.length)
    const authData = Buffer.concat([this.authenticatorData(c, 0), Buffer.alloc(16), id, this.credentialId, cose])
    const attestation = cbor({ fmt: 'none', attStmt: new Map(), authData })
    return {
      id: b64u(this.credentialId),
      rawId: b64u(this.credentialId),
      type: 'public-key',
      response: { clientDataJSON: b64u(this.clientData(c)), attestationObject: b64u(attestation), transports: ['internal'] },
      authenticatorAttachment: 'platform',
      clientExtensionResults: {},
    }
  }

  /** The answer a browser sends for request options. */
  signIn(options: { challenge: string; rpId: string }, bend: Partial<Ceremony> = {}): object {
    if (this.counts) this.counter++
    const c = this.ceremony(options.challenge, options.rpId, 'webauthn.get', Flag.present | Flag.verified | Flag.backupEligible | Flag.backedUp, bend)
    const authData = this.authenticatorData(c, this.counter)
    const signature = sign('sha256', Buffer.concat([authData, createHash('sha256').update(this.clientData(c)).digest()]), this.privateKey)
    const response: Record<string, string> = {
      clientDataJSON: b64u(this.clientData(c)),
      authenticatorData: b64u(authData),
      signature: b64u(signature),
    }
    if (c.userHandle !== null && c.userHandle !== undefined) response.userHandle = b64u(c.userHandle)
    return {
      id: b64u(this.credentialId),
      rawId: b64u(this.credentialId),
      type: 'public-key',
      response,
      authenticatorAttachment: 'platform',
      clientExtensionResults: {},
    }
  }

  /** Makes the next answer report `counter + 1`, to play a copy of the device. */
  rewind(counter: number): void {
    this.counter = counter
  }

  private ceremony(challenge: string, rpId: string, type: string, flags: number, bend: Partial<Ceremony>): Ceremony {
    return { origin: this.origin, rpId, challenge, flags, type, userHandle: this.userHandle, ...bend }
  }

  private clientData(c: Ceremony): Buffer {
    return Buffer.from(JSON.stringify({ type: c.type, challenge: c.challenge, origin: c.origin, crossOrigin: false }))
  }

  private authenticatorData(c: Ceremony, counter: number): Buffer {
    const count = Buffer.alloc(4)
    count.writeUInt32BE(counter)
    return Buffer.concat([createHash('sha256').update(c.rpId).digest(), Buffer.from([c.flags]), count])
  }
}
