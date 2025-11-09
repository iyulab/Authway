import { PKCEChallenge } from '../types'

/**
 * Generate cryptographically random string
 */
function generateRandomString(length: number): string {
  const charset = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~'
  const randomValues = new Uint8Array(length)

  if (typeof window !== 'undefined' && window.crypto) {
    window.crypto.getRandomValues(randomValues)
  } else if (typeof global !== 'undefined' && global.crypto) {
    global.crypto.getRandomValues(randomValues)
  } else {
    throw new Error('No secure random number generator available')
  }

  return Array.from(randomValues)
    .map(x => charset[x % charset.length])
    .join('')
}

/**
 * SHA-256 hash
 */
async function sha256(plain: string): Promise<ArrayBuffer> {
  const encoder = new TextEncoder()
  const data = encoder.encode(plain)

  if (typeof window !== 'undefined' && window.crypto.subtle) {
    return window.crypto.subtle.digest('SHA-256', data)
  } else if (typeof global !== 'undefined' && global.crypto?.subtle) {
    return global.crypto.subtle.digest('SHA-256', data)
  }

  throw new Error('No crypto.subtle available')
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
