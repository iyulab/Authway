import { ApiError, TimeoutError } from '../types/errors'

export interface HttpOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'DELETE'
  headers?: Record<string, string>
  body?: any
  timeout?: number
}

/**
 * HTTP client with timeout support
 */
export async function http<T = any>(url: string, options: HttpOptions = {}): Promise<T> {
  const {
    method = 'GET',
    headers = {},
    body,
    timeout = 10000
  } = options

  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), timeout)

  try {
    const response = await fetch(url, {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...headers
      },
      body: body ? JSON.stringify(body) : undefined,
      signal: controller.signal
    })

    clearTimeout(timeoutId)

    if (!response.ok) {
      const errorData = await response.json().catch(() => ({}))
      throw new ApiError(
        errorData.message || `HTTP ${response.status}: ${response.statusText}`,
        response.status,
        errorData
      )
    }

    const contentType = response.headers.get('content-type')
    if (contentType?.includes('application/json')) {
      return await response.json()
    }

    // Detect HTML response (likely auth redirect after logout)
    if (contentType?.includes('text/html')) {
      console.warn(
        '⚠️ Authway: Received HTML response instead of JSON.\n' +
        'This usually happens when React Query refetches cached queries after logout.\n\n' +
        '💡 Solution: Clear React Query cache BEFORE calling logout():\n\n' +
        '  const handleLogout = () => {\n' +
        '    queryClient.cancelQueries()\n' +
        '    queryClient.clear()\n' +
        '    logout({ returnTo: window.location.origin })\n' +
        '  }\n\n' +
        '📚 See: @authway/react README > "React Query와 함께 사용"'
      )
    }

    return await response.text() as any
  } catch (error: any) {
    clearTimeout(timeoutId)

    if (error.name === 'AbortError') {
      throw new TimeoutError()
    }

    if (error instanceof ApiError) {
      throw error
    }

    // Detect JSON parse error on HTML content
    if (error.message?.includes('Unexpected token') && error.message?.includes('<')) {
      console.warn(
        '⚠️ Authway: JSON parse error on HTML response.\n' +
        'This usually happens when React Query refetches cached queries after logout.\n\n' +
        '💡 Solution: Clear React Query cache BEFORE calling logout():\n\n' +
        '  const handleLogout = () => {\n' +
        '    queryClient.cancelQueries()\n' +
        '    queryClient.clear()\n' +
        '    logout({ returnTo: window.location.origin })\n' +
        '  }\n\n' +
        '📚 See: @authway/react README > "React Query와 함께 사용"'
      )
    }

    throw new ApiError(error.message || 'Network request failed', 0)
  }
}

/**
 * GET request
 */
export function get<T = any>(url: string, headers?: Record<string, string>, timeout?: number): Promise<T> {
  return http<T>(url, { method: 'GET', headers, timeout })
}

/**
 * POST request
 */
export function post<T = any>(url: string, body?: any, headers?: Record<string, string>, timeout?: number): Promise<T> {
  return http<T>(url, { method: 'POST', body, headers, timeout })
}

/**
 * PATCH request
 */
export function patch<T = any>(url: string, body?: any, headers?: Record<string, string>, timeout?: number): Promise<T> {
  return http<T>(url, { method: 'PATCH', body, headers, timeout })
}

/**
 * POST form data (application/x-www-form-urlencoded)
 * Used for OAuth token endpoints
 */
export async function postForm<T = any>(url: string, data: Record<string, string>, headers?: Record<string, string>, timeout?: number): Promise<T> {
  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), timeout || 10000)

  try {
    const formBody = Object.keys(data)
      .map(key => encodeURIComponent(key) + '=' + encodeURIComponent(data[key]))
      .join('&')

    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        ...headers
      },
      body: formBody,
      signal: controller.signal
    })

    clearTimeout(timeoutId)

    if (!response.ok) {
      const errorData = await response.json().catch(() => ({}))
      throw new ApiError(
        errorData.error_description || errorData.message || `HTTP ${response.status}: ${response.statusText}`,
        response.status,
        errorData
      )
    }

    const contentType = response.headers.get('content-type')
    if (contentType?.includes('application/json')) {
      return await response.json()
    }

    // Detect HTML response (likely auth redirect after logout)
    if (contentType?.includes('text/html')) {
      console.warn(
        '⚠️ Authway: Received HTML response instead of JSON.\n' +
        'This usually happens when React Query refetches cached queries after logout.\n\n' +
        '💡 Solution: Clear React Query cache BEFORE calling logout():\n\n' +
        '  const handleLogout = () => {\n' +
        '    queryClient.cancelQueries()\n' +
        '    queryClient.clear()\n' +
        '    logout({ returnTo: window.location.origin })\n' +
        '  }\n\n' +
        '📚 See: @authway/react README > "React Query와 함께 사용"'
      )
    }

    return await response.text() as any
  } catch (error: any) {
    clearTimeout(timeoutId)

    if (error.name === 'AbortError') {
      throw new TimeoutError()
    }

    if (error instanceof ApiError) {
      throw error
    }

    // Detect JSON parse error on HTML content
    if (error.message?.includes('Unexpected token') && error.message?.includes('<')) {
      console.warn(
        '⚠️ Authway: JSON parse error on HTML response.\n' +
        'This usually happens when React Query refetches cached queries after logout.\n\n' +
        '💡 Solution: Clear React Query cache BEFORE calling logout():\n\n' +
        '  const handleLogout = () => {\n' +
        '    queryClient.cancelQueries()\n' +
        '    queryClient.clear()\n' +
        '    logout({ returnTo: window.location.origin })\n' +
        '  }\n\n' +
        '📚 See: @authway/react README > "React Query와 함께 사용"'
      )
    }

    throw new ApiError(error.message || 'Network request failed', 0)
  }
}
