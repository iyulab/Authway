import { ReactNode, ButtonHTMLAttributes } from 'react'
import { useAuth } from '../hooks/useAuth'
import { LogoutOptions } from '@authway/client'

export interface LogoutButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** Custom children to render instead of default "Log Out" text */
  children?: ReactNode
  /** Options to pass to logout */
  logoutOptions?: LogoutOptions
  /** Custom className */
  className?: string
}

/**
 * Pre-built logout button component
 *
 * @example
 * ```tsx
 * <LogoutButton>Sign Out</LogoutButton>
 *
 * <LogoutButton
 *   logoutOptions={{ returnTo: window.location.origin }}
 *   className="my-custom-class"
 * >
 *   Logout
 * </LogoutButton>
 * ```
 */
export function LogoutButton({
  children = 'Log Out',
  logoutOptions,
  className = '',
  ...buttonProps
}: LogoutButtonProps) {
  const { logout, isLoading } = useAuth()

  const handleClick = () => {
    logout(logoutOptions)
  }

  return (
    <button
      onClick={handleClick}
      disabled={isLoading || buttonProps.disabled}
      className={className}
      {...buttonProps}
    >
      {children}
    </button>
  )
}
