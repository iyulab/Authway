import { createHmac } from 'node:crypto'

const PERIOD_MS = 30_000

/** The code an authenticator holding `secret` shows at `at` (RFC 6238: SHA-1, 30 s, 6 digits). */
export function codeAt(secret: string, at: number): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const ch of secret.replace(/=+$/, '').toUpperCase()) bits += alphabet.indexOf(ch).toString(2).padStart(5, '0')
  const key = Buffer.from((bits.match(/.{8}/g) ?? []).map((b) => parseInt(b, 2)))
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(at / PERIOD_MS)))
  const mac = createHmac('sha1', key).update(counter).digest()
  const offset = mac[mac.length - 1] & 0xf
  return ((mac.readUInt32BE(offset) & 0x7fffffff) % 1_000_000).toString().padStart(6, '0')
}

/**
 * A user's authenticator app. A provider accepts each code once, so a
 * scenario that gives the second factor several times needs a new code each
 * time, as a user would have to wait for one.
 *
 * `next()` gives the code of the earliest time step not yet used. A provider
 * accepts a code one step ahead of its own clock, so the step after the
 * current one is used without waiting; beyond that it waits for the clock.
 */
export class Authenticator {
  private lastStep = 0

  constructor(readonly secret: string) {}

  async next(): Promise<string> {
    const step = Math.max(Math.floor(Date.now() / PERIOD_MS), this.lastStep + 1)
    const earliest = (step - 1) * PERIOD_MS // when the provider's clock is one step behind this code
    const wait = earliest - Date.now()
    if (wait > 0) await new Promise((resolve) => setTimeout(resolve, wait + 250))
    this.lastStep = step
    return codeAt(this.secret, step * PERIOD_MS)
  }

  /** The code handed out last — by now used up. */
  last(): string {
    return codeAt(this.secret, this.lastStep * PERIOD_MS)
  }
}
