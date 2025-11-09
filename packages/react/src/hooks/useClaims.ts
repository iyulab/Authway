import { useState, useEffect, useCallback } from 'react'
import { useAuth } from './useAuth'
import { Claims } from '@authway/client'

/**
 * Hook for managing user claims
 */
export function useClaims() {
  const { client, isAuthenticated } = useAuth()
  const [claims, setClaims] = useState<Claims>({})
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<Error | null>(null)

  // Load claims (backend returns both system and user claims)
  const loadClaims = useCallback(async () => {
    if (!isAuthenticated || !client) {
      setClaims({})
      return
    }

    setIsLoading(true)
    setError(null)

    try {
      // Get ALL claims from backend (system + user)
      const allClaims = await client.getClaims()
      setClaims(allClaims)
    } catch (err) {
      setError(err as Error)
    } finally {
      setIsLoading(false)
    }
  }, [client, isAuthenticated])

  // Update claims
  const updateClaims = useCallback(
    async (newClaims: Partial<Claims>) => {
      if (!client) {
        throw new Error('Not authenticated')
      }

      setIsLoading(true)
      setError(null)

      try {
        await client.updateClaims(newClaims)
        await loadClaims() // Reload claims after update
      } catch (err) {
        setError(err as Error)
        throw err
      } finally {
        setIsLoading(false)
      }
    },
    [client, loadClaims]
  )

  // Load claims on mount and when auth state changes
  useEffect(() => {
    loadClaims()
  }, [loadClaims])

  return {
    claims,
    isLoading,
    error,
    updateClaims,
    refreshClaims: loadClaims
  }
}
