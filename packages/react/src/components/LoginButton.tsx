import { ReactNode, ButtonHTMLAttributes } from 'react'
import { useAuth } from '../hooks/useAuth'
import { RedirectLoginOptions } from '@authway/client'

export interface LoginButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** Custom children to render instead of default "Log In" text */
  children?: ReactNode
  /** Options to pass to loginWithRedirect */
  loginOptions?: RedirectLoginOptions
  /** Custom className */
  className?: string
}

/**
 * Pre-built login button component
 *
 * @example
 * ```tsx
 * <LoginButton>Sign In</LoginButton>
 *
 * <LoginButton
 *   loginOptions={{ appState: { returnTo: '/dashboard' } }}
 *   className="my-custom-class"
 * >
 *   Login with Authway
 * </LoginButton>
 * ```
 */
export function LoginButton({
  children = 'Log In',
  loginOptions,
  className = '',
  ...buttonProps
}: LoginButtonProps) {
  const { loginWithRedirect, isLoading } = useAuth()

  const handleClick = () => {
    loginWithRedirect(loginOptions)
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
