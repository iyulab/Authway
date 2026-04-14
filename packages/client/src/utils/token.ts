import { TokenClaims, User } from '../types'
import { TokenError } from '../types/errors'

/**
 * Decode JWT token
 */
export function decodeToken(token: string): TokenClaims {
  try {
    const parts = token.split('.')
    if (parts.length !== 3) {
      throw new TokenError('Invalid token format')
    }

    const payload = parts[1]
    const decoded = base64URLDecode(payload)
    return JSON.parse(decoded)
  } catch (error) {
    throw new TokenError(`Failed to decode token: ${error}`)
  }
}

/**
 * Base64 URL decode with UTF-8 support
 */
function base64URLDecode(str: string): string {
  // Replace URL-safe characters
  let base64 = str.replace(/-/g, '+').replace(/_/g, '/')

  // Pad with '=' to make length multiple of 4
  const pad = base64.length % 4
  if (pad) {
    if (pad === 1) {
      throw new Error('Invalid base64 string')
    }
    base64 += new Array(5 - pad).join('=')
  }

  // Decode base64 to binary string
  const binary = atob(base64)

  // Convert binary string to UTF-8
  // Handle multi-byte UTF-8 characters properly
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }

  // Decode UTF-8 bytes to string
  return new TextDecoder('utf-8').decode(bytes)
}

/**
 * Extract user from ID token
 */
export function extractUser(idToken: string): User {
  const claims = decodeToken(idToken)

  return {
    ...claims,
    sub: claims.sub,
    email: claims.email || '',
    email_verified: claims.email_verified || false,
    name: claims.name,
    given_name: claims.given_name,
    family_name: claims.family_name,
    picture: claims.picture,
    locale: claims.locale,
    zoneinfo: claims.zoneinfo,
    updated_at: claims.updated_at,
  }
}

/**
 * Check if token is expired
 */
export function isTokenExpired(token: string, leeway: number = 60): boolean {
  try {
    const claims = decodeToken(token)
    const now = Math.floor(Date.now() / 1000)
    return claims.exp < (now + leeway)
  } catch {
    return true
  }
}

/**
 * Get token expiration time
 */
export function getTokenExpiration(token: string): number {
  const claims = decodeToken(token)
  return claims.exp * 1000 // Convert to milliseconds
}

/**
 * Verify token audience
 */
export function verifyAudience(token: string, expectedAudience: string): boolean {
  try {
    const claims = decodeToken(token)
    const aud = Array.isArray(claims.aud) ? claims.aud : [claims.aud]
    return aud.includes(expectedAudience)
  } catch {
    return false
  }
}

/**
 * Verify token issuer
 */
export function verifyIssuer(token: string, expectedIssuer: string): boolean {
  try {
    const claims = decodeToken(token)
    return claims.iss === expectedIssuer
  } catch {
    return false
  }
}
