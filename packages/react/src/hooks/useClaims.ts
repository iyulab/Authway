import { useState, useEffect, useCallback } from 'react'
import { useAuth } from './useAuth'
import { Claims } from '@authway/client'

/**
 * The signed-in user's claims, from the ID token. They are written by the
 * application's backend through the admin API, never from the browser, and
 * a change shows from the user's next sign-in on.
 */
export function useClaims() {
  const { client, isAuthenticated } = useAuth()
  const [claims, setClaims] = useState<Claims>({})
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<Error | null>(null)

  const loadClaims = useCallback(async () => {
    if (!isAuthenticated || !client) {
      setClaims({})
      return
    }

    setIsLoading(true)
    setError(null)

    try {
      setClaims(await client.getClaims())
    } catch (err) {
      setError(err as Error)
    } finally {
      setIsLoading(false)
    }
  }, [client, isAuthenticated])

  useEffect(() => {
    loadClaims()
  }, [loadClaims])

  return {
    claims,
    isLoading,
    error,
    refreshClaims: loadClaims
  }
}
