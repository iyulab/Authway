/**
 * Build URL with query parameters
 */
export function buildUrl(base: string, params: Record<string, any>): string {
  const url = new URL(base)

  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null) {
      if (Array.isArray(value)) {
        value.forEach(v => url.searchParams.append(key, String(v)))
      } else {
        url.searchParams.append(key, String(value))
      }
    }
  })

  return url.toString()
}

/**
 * Parse query string
 */
export function parseQueryString(query: string): Record<string, string> {
  const params = new URLSearchParams(query)
  const result: Record<string, string> = {}

  params.forEach((value, key) => {
    result[key] = value
  })

  return result
}

/**
 * Get current URL without query/hash
 */
export function getBaseUrl(): string {
  if (typeof window === 'undefined') return ''
  return `${window.location.origin}${window.location.pathname}`
}

/**
 * Parse callback URL
 */
export function parseCallbackUrl(url?: string): { code?: string; state?: string; error?: string; error_description?: string } {
  const urlToParse = url || (typeof window !== 'undefined' ? window.location.href : '')
  const urlObj = new URL(urlToParse)

  return {
    code: urlObj.searchParams.get('code') || undefined,
    state: urlObj.searchParams.get('state') || undefined,
    error: urlObj.searchParams.get('error') || undefined,
    error_description: urlObj.searchParams.get('error_description') || undefined
  }
}
