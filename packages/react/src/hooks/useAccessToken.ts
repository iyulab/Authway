import { useState, useEffect } from 'react'
import { useAuth } from './useAuth'

export interface UseAccessTokenResult {
  token: string | null
  isLoading: boolean
  error: Error | null
  getToken: () => Promise<string>
}

/**
 * Access token hook with auto-refresh
 *
 * @example
 * ```tsx
 * function ApiRequest() {
 *   const { token, getToken } = useAccessToken()
 *
 *   const fetchData = async () => {
 *     const accessToken = await getToken()
 *     const response = await fetch('/api/data', {
 *       headers: {
 *         'Authorization': `Bearer ${accessToken}`
 *       }
 *     })
 *     return response.json()
 *   }
 *
 *   return <button onClick={fetchData}>Fetch Data</button>
 * }
 * ```
 */
export function useAccessToken(): UseAccessTokenResult {
  const { client, isAuthenticated } = useAuth()
  const [token, setToken] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<Error | null>(null)

  useEffect(() => {
    if (isAuthenticated) {
      getToken()
    }
  }, [isAuthenticated])

  const getToken = async (): Promise<string> => {
    try {
      setIsLoading(true)
      setError(null)
      const accessToken = await client.getAccessToken()
      setToken(accessToken)
      return accessToken
    } catch (err) {
      setError(err as Error)
      throw err
    } finally {
      setIsLoading(false)
    }
  }

  return {
    token,
    isLoading,
    error,
    getToken
  }
}
