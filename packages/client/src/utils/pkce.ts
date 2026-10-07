import { PKCEChallenge } from '../types'

// 64 unreserved characters (RFC 7636 §4.1): 256 is a multiple of 64, so
// mapping a random byte onto the set with `% 64` introduces no bias.
const CHARSET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'

function webCrypto(): Crypto {
  const crypto = globalThis.crypto
  if (!crypto?.getRandomValues || !crypto.subtle) {
    throw new Error('Web Crypto is not available in this environment')
  }
  return crypto
}

/**
 * Generate cryptographically random string
 */
function generateRandomString(length: number): string {
  const randomValues = new Uint8Array(length)
  webCrypto().getRandomValues(randomValues)
  return Array.from(randomValues, x => CHARSET[x % CHARSET.length]).join('')
}

/**
 * SHA-256 hash
 */
async function sha256(plain: string): Promise<ArrayBuffer> {
  return webCrypto().subtle.digest('SHA-256', new TextEncoder().encode(plain))
}

/**
 * Base64 URL encode
 */
function base64URLEncode(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let binary = ''

  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i])
  }

  return btoa(binary)
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '')
}

/**
 * Generate PKCE challenge
 */
export async function generatePKCEChallenge(): Promise<PKCEChallenge> {
  const codeVerifier = generateRandomString(128)
  const hashed = await sha256(codeVerifier)
  const codeChallenge = base64URLEncode(hashed)
  const state = generateRandomString(32)
  const nonce = generateRandomString(32)

  return {
    codeVerifier,
    codeChallenge,
    state,
    nonce
  }
}

/**
 * Verify state parameter
 */
export function verifyState(receivedState: string, expectedState: string): boolean {
  return receivedState === expectedState
}
