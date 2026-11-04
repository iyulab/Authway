import { ReactNode, ButtonHTMLAttributes, MouseEvent } from 'react'
import { useAuth } from '../hooks/useAuth'
import { message } from '../i18n'
import { RedirectLoginOptions } from '@authway/client'

export interface LoginButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** Custom children to render instead of the default text ("Log In", or "로그인" in Korean) */
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
  children,
  loginOptions,
  className = '',
  onClick,
  ...buttonProps
}: LoginButtonProps) {
  const { loginWithRedirect, isLoading, locale } = useAuth()

  // The application's own onClick runs first, and may call preventDefault()
  // to keep the button from signing the user in.
  const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
    onClick?.(event)
    if (event.defaultPrevented) return
    loginWithRedirect(loginOptions)
  }

  return (
    <button
      {...buttonProps}
      onClick={handleClick}
      disabled={isLoading || buttonProps.disabled}
      className={className}
    >
      {children ?? message(locale, 'logIn')}
    </button>
  )
}
