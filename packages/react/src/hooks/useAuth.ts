import { useContext } from 'react'
import { AuthwayContext, AuthContextValue } from '../context/AuthwayContext'

/**
 * Core authentication hook
 *
 * @example
 * ```tsx
 * function MyComponent() {
 *   const { isAuthenticated, user, loginWithRedirect, logout } = useAuth()
 *
 *   if (!isAuthenticated) {
 *     return <button onClick={() => loginWithRedirect()}>Log in</button>
 *   }
 *
 *   return (
 *     <div>
 *       <p>Welcome, {user.name}!</p>
 *       <button onClick={() => logout()}>Log out</button>
 *     </div>
 *   )
 * }
 * ```
 */
export function useAuth(): AuthContextValue {
  const context = useContext(AuthwayContext)

  if (!context) {
    throw new Error('useAuth must be used within AuthwayProvider')
  }

  return context
}
