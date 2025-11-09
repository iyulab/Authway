import { HTMLAttributes } from 'react'
import { useUser } from '../hooks/useUser'

export interface UserProfileProps extends HTMLAttributes<HTMLDivElement> {
  /** Show user avatar */
  showAvatar?: boolean
  /** Show user email */
  showEmail?: boolean
  /** Custom className */
  className?: string
}

/**
 * Pre-built user profile component
 * Displays user information from the authenticated session
 *
 * @example
 * ```tsx
 * <UserProfile />
 *
 * <UserProfile
 *   showAvatar={true}
 *   showEmail={true}
 *   className="my-profile"
 * />
 * ```
 */
export function UserProfile({
  showAvatar = true,
  showEmail = true,
  className = '',
  ...divProps
}: UserProfileProps) {
  const { user, isLoading } = useUser()

  if (isLoading) {
    return (
      <div className={className} {...divProps}>
        <span>Loading...</span>
      </div>
    )
  }

  if (!user) {
    return null
  }

  return (
    <div className={className} {...divProps}>
      {showAvatar && user.picture && (
        <img
          src={user.picture}
          alt={user.name || 'User avatar'}
          style={{ width: 40, height: 40, borderRadius: '50%' }}
        />
      )}
      <div>
        {user.name && <div>{user.name}</div>}
        {showEmail && user.email && <div>{user.email}</div>}
      </div>
    </div>
  )
}
