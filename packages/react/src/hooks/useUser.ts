import { User } from '@authway/client'
import { useAuth } from './useAuth'

export interface UseUserResult {
  user: User | null
  isLoading: boolean
  error: Error | null
}

/**
 * User profile hook
 *
 * @example
 * ```tsx
 * function UserProfile() {
 *   const { user, isLoading } = useUser()
 *
 *   if (isLoading) return <div>Loading...</div>
 *   if (!user) return null
 *
 *   return (
 *     <div>
 *       <img src={user.picture} alt={user.name} />
 *       <h2>{user.name}</h2>
 *       <p>{user.email}</p>
 *     </div>
 *   )
 * }
 * ```
 */
export function useUser(): UseUserResult {
  const { user, isLoading, error } = useAuth()

  return {
    user,
    isLoading,
    error
  }
}
