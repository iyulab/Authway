/**
 * DPoP (Demonstrating Proof-of-Possession) Utilities
 * RFC 9449: https://datatracker.ietf.org/doc/html/rfc9449
 */

import { IStorage } from './storage'

const DPOP_KEY_STORAGE_KEY = 'authway_dpop_key'

/**
 * DPoP key pair stored as JWK
 */
interface DPoPKeyPair {
  publicKey: JsonWebKey
  privateKey: JsonWebKey
}

/**
 * Generate or retrieve DPoP key pair
 */
export async function getDPoPKeyPair(storage: IStorage): Promise<CryptoKeyPair> {
  // Try to load existing key pair
  const stored = storage.get(DPOP_KEY_STORAGE_KEY)

  if (stored) {
    try {
      const keyPair: DPoPKeyPair = JSON.parse(stored)

      // Import keys from JWK
      const publicKey = await crypto.subtle.importKey(
        'jwk',
        keyPair.publicKey,
        { name: 'ECDSA', namedCurve: 'P-256' },
        true,
        ['verify']
      )

      const privateKey = await crypto.subtle.importKey(
        'jwk',
        keyPair.privateKey,
        { name: 'ECDSA', namedCurve: 'P-256' },
        true,
        ['sign']
      )

      return { publicKey, privateKey }
    } catch (err) {
      // If import fails, generate new keys
      console.warn('Failed to import DPoP keys, generating new ones:', err)
    }
  }

  // Generate new key pair
  const keyPair = await crypto.subtle.generateKey(
    { name: 'ECDSA', namedCurve: 'P-256' },
    true,
    ['sign', 'verify']
  )

  // Export and store as JWK
  const publicJwk = await crypto.subtle.exportKey('jwk', keyPair.publicKey)
  const privateJwk = await crypto.subtle.exportKey('jwk', keyPair.privateKey)

  storage.set(DPOP_KEY_STORAGE_KEY, JSON.stringify({
    publicKey: publicJwk,
    privateKey: privateJwk
  }))

  return keyPair
}

/**
 * Create DPoP proof JWT
 */
export async function createDPoPProof(
  keyPair: CryptoKeyPair,
  htm: string, // HTTP method (GET, POST, etc.)
  htu: string, // HTTP URI (without query/fragment)
  accessToken?: string // Optional: include 'ath' claim when using access token
): Promise<string> {
  const publicJwk = await crypto.subtle.exportKey('jwk', keyPair.publicKey)

  // DPoP JWT header
  const header = {
    typ: 'dpop+jwt',
    alg: 'ES256',
    jwk: {
      kty: publicJwk.kty,
      crv: publicJwk.crv,
      x: publicJwk.x,
      y: publicJwk.y
    }
  }

  // DPoP JWT payload
  const payload: any = {
    jti: generateRandomString(16), // Unique ID
    htm,
    htu,
    iat: Math.floor(Date.now() / 1000)
  }

  // Add 'ath' (access token hash) if access token provided
  if (accessToken) {
    payload.ath = await hashAccessToken(accessToken)
  }

  // Encode header and payload
  const encodedHeader = base64UrlEncode(JSON.stringify(header))
  const encodedPayload = base64UrlEncode(JSON.stringify(payload))

  // Sign
  const message = `${encodedHeader}.${encodedPayload}`
  const encoder = new TextEncoder()
  const data = encoder.encode(message)

  const signature = await crypto.subtle.sign(
    { name: 'ECDSA', hash: 'SHA-256' },
    keyPair.privateKey,
    data
  )

  const encodedSignature = base64UrlEncode(new Uint8Array(signature))

  return `${message}.${encodedSignature}`
}

/**
 * Hash access token for 'ath' claim
 */
async function hashAccessToken(token: string): Promise<string> {
  const encoder = new TextEncoder()
  const data = encoder.encode(token)
  const hash = await crypto.subtle.digest('SHA-256', data)
  return base64UrlEncode(new Uint8Array(hash))
}

/**
 * Base64 URL encode
 */
function base64UrlEncode(input: Uint8Array | string): string {
  let base64: string

  if (typeof input === 'string') {
    base64 = btoa(input)
  } else {
    const binary = String.fromCharCode(...input)
    base64 = btoa(binary)
  }

  return base64
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '')
}

/**
 * Generate random string
 */
function generateRandomString(length: number): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
  const randomValues = new Uint8Array(length)
  crypto.getRandomValues(randomValues)

  let result = ''
  for (let i = 0; i < length; i++) {
    result += chars[randomValues[i] % chars.length]
  }

  return result
}

/**
 * Clear DPoP keys from storage
 */
export function clearDPoPKeys(storage: IStorage): void {
  storage.remove(DPOP_KEY_STORAGE_KEY)
}
