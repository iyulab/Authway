import { ReactNode, ButtonHTMLAttributes, MouseEvent } from 'react'
import { useAuth } from '../hooks/useAuth'
import { message } from '../i18n'
import { LogoutOptions } from '@authway/client'

export interface LogoutButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** Custom children to render instead of the default text ("Log Out", or "로그아웃" in Korean) */
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
  children,
  logoutOptions,
  className = '',
  onClick,
  ...buttonProps
}: LogoutButtonProps) {
  const { logout, isLoading, locale } = useAuth()

  // The application's own onClick runs first, and may call preventDefault()
  // to keep the button from signing the user out.
  const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
    onClick?.(event)
    if (event.defaultPrevented) return
    void logout(logoutOptions)
  }

  return (
    <button
      {...buttonProps}
      onClick={handleClick}
      disabled={isLoading || buttonProps.disabled}
      className={className}
    >
      {children ?? message(locale, 'logOut')}
    </button>
  )
}
