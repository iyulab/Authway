import { createHash, randomBytes } from 'node:crypto'

export interface Pkce {
  verifier: string
  challenge: string
}

/** RFC 7636 S256 code verifier and challenge. */
export function createPkce(): Pkce {
  const verifier = randomBytes(32).toString('base64url')
  const challenge = createHash('sha256').update(verifier).digest('base64url')
  return { verifier, challenge }
}

export function randomToken(bytes = 16): string {
  return randomBytes(bytes).toString('hex')
}
