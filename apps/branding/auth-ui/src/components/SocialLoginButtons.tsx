import React, { useState } from 'react'
import { useTranslation } from 'react-i18next'

interface SocialLoginButtonProps {
  onError?: (error: string) => void
  disabled?: boolean
  clientId?: string
}

// GitHub Login Button
export const GitHubLoginButton: React.FC<SocialLoginButtonProps> = ({
  onError,
  disabled = false,
  clientId,
}) => {
  const { t } = useTranslation(['common'])
  const [isLoading, setIsLoading] = useState(false)

  const handleLogin = async () => {
    if (disabled || isLoading) return

    setIsLoading(true)
    console.log('[GitHubLogin] Button clicked - starting OAuth flow')

    try {
      const urlParams = new URLSearchParams(window.location.search)
      const loginChallenge = urlParams.get('login_challenge')

      if (!loginChallenge) {
        throw new Error('Missing login_challenge parameter')
      }

      const authBackendUrl = import.meta.env.VITE_AUTH_BACKEND_URL || import.meta.env.VITE_API_URL
      const response = await fetch(`${authBackendUrl}/auth/github/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          login_challenge: loginChallenge,
          client_id: clientId || '',
        }),
        credentials: 'include',
      })

      if (!response.ok) {
        const error = await response.json()
        throw new Error(error.error || 'GitHub login failed')
      }

      const data = await response.json()
      if (data.redirect_url) {
        const isPopupMode = window.opener !== null && window.opener !== window
        if (isPopupMode) {
          sessionStorage.setItem('authway_popup_mode', 'true')
        } else {
          sessionStorage.removeItem('authway_popup_mode')
        }
        window.location.href = data.redirect_url
      } else {
        throw new Error('No redirect URL in response')
      }
    } catch (error) {
      console.error('GitHub login error:', error)
      setIsLoading(false)
      if (onError) {
        onError(error instanceof Error ? error.message : 'GitHub login failed')
      }
    }
  }

  return (
    <button
      type="button"
      onClick={handleLogin}
      disabled={disabled || isLoading}
      className="relative w-full flex justify-center items-center px-4 py-3 border border-gray-300 rounded-lg text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-gray-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors duration-200"
    >
      {isLoading ? (
        <div className="flex items-center">
          <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-gray-600 mr-2"></div>
          {t('common:connecting')}
        </div>
      ) : (
        <div className="flex items-center">
          <svg className="w-5 h-5 mr-3" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
          </svg>
          {t('common:githubLogin', 'Sign in with GitHub')}
        </div>
      )}
    </button>
  )
}

// Microsoft Login Button
export const MicrosoftLoginButton: React.FC<SocialLoginButtonProps> = ({
  onError,
  disabled = false,
  clientId,
}) => {
  const { t } = useTranslation(['common'])
  const [isLoading, setIsLoading] = useState(false)

  const handleLogin = async () => {
    if (disabled || isLoading) return

    setIsLoading(true)
    console.log('[MicrosoftLogin] Button clicked - starting OAuth flow')

    try {
      const urlParams = new URLSearchParams(window.location.search)
      const loginChallenge = urlParams.get('login_challenge')

      if (!loginChallenge) {
        throw new Error('Missing login_challenge parameter')
      }

      const authBackendUrl = import.meta.env.VITE_AUTH_BACKEND_URL || import.meta.env.VITE_API_URL
      const response = await fetch(`${authBackendUrl}/auth/microsoft/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          login_challenge: loginChallenge,
          client_id: clientId || '',
        }),
        credentials: 'include',
      })

      if (!response.ok) {
        const error = await response.json()
        throw new Error(error.error || 'Microsoft login failed')
      }

      const data = await response.json()
      if (data.redirect_url) {
        const isPopupMode = window.opener !== null && window.opener !== window
        if (isPopupMode) {
          sessionStorage.setItem('authway_popup_mode', 'true')
        } else {
          sessionStorage.removeItem('authway_popup_mode')
        }
        window.location.href = data.redirect_url
      } else {
        throw new Error('No redirect URL in response')
      }
    } catch (error) {
      console.error('Microsoft login error:', error)
      setIsLoading(false)
      if (onError) {
        onError(error instanceof Error ? error.message : 'Microsoft login failed')
      }
    }
  }

  return (
    <button
      type="button"
      onClick={handleLogin}
      disabled={disabled || isLoading}
      className="relative w-full flex justify-center items-center px-4 py-3 border border-gray-300 rounded-lg text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors duration-200"
    >
      {isLoading ? (
        <div className="flex items-center">
          <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-gray-600 mr-2"></div>
          {t('common:connecting')}
        </div>
      ) : (
        <div className="flex items-center">
          <svg className="w-5 h-5 mr-3" viewBox="0 0 23 23">
            <path fill="#f3f3f3" d="M0 0h23v23H0z"/>
            <path fill="#f35325" d="M1 1h10v10H1z"/>
            <path fill="#81bc06" d="M12 1h10v10H12z"/>
            <path fill="#05a6f0" d="M1 12h10v10H1z"/>
            <path fill="#ffba08" d="M12 12h10v10H12z"/>
          </svg>
          {t('common:microsoftLogin', 'Sign in with Microsoft')}
        </div>
      )}
    </button>
  )
}

// Apple Login Button
export const AppleLoginButton: React.FC<SocialLoginButtonProps> = ({
  onError,
  disabled = false,
  clientId,
}) => {
  const { t } = useTranslation(['common'])
  const [isLoading, setIsLoading] = useState(false)

  const handleLogin = async () => {
    if (disabled || isLoading) return

    setIsLoading(true)
    console.log('[AppleLogin] Button clicked - starting OAuth flow')

    try {
      const urlParams = new URLSearchParams(window.location.search)
      const loginChallenge = urlParams.get('login_challenge')

      if (!loginChallenge) {
        throw new Error('Missing login_challenge parameter')
      }

      const authBackendUrl = import.meta.env.VITE_AUTH_BACKEND_URL || import.meta.env.VITE_API_URL
      const response = await fetch(`${authBackendUrl}/auth/apple/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          login_challenge: loginChallenge,
          client_id: clientId || '',
        }),
        credentials: 'include',
      })

      if (!response.ok) {
        const error = await response.json()
        throw new Error(error.error || 'Apple login failed')
      }

      const data = await response.json()
      if (data.redirect_url) {
        const isPopupMode = window.opener !== null && window.opener !== window
        if (isPopupMode) {
          sessionStorage.setItem('authway_popup_mode', 'true')
        } else {
          sessionStorage.removeItem('authway_popup_mode')
        }
        window.location.href = data.redirect_url
      } else {
        throw new Error('No redirect URL in response')
      }
    } catch (error) {
      console.error('Apple login error:', error)
      setIsLoading(false)
      if (onError) {
        onError(error instanceof Error ? error.message : 'Apple login failed')
      }
    }
  }

  return (
    <button
      type="button"
      onClick={handleLogin}
      disabled={disabled || isLoading}
      className="relative w-full flex justify-center items-center px-4 py-3 border border-gray-300 rounded-lg text-sm font-medium text-white bg-black hover:bg-gray-800 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-gray-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors duration-200"
    >
      {isLoading ? (
        <div className="flex items-center">
          <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
          {t('common:connecting')}
        </div>
      ) : (
        <div className="flex items-center">
          <svg className="w-5 h-5 mr-3" viewBox="0 0 24 24" fill="currentColor">
            <path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"/>
          </svg>
          {t('common:appleLogin', 'Sign in with Apple')}
        </div>
      )}
    </button>
  )
}

// Combined Social Login Buttons component
interface SocialLoginButtonsProps {
  onError?: (error: string) => void
  disabled?: boolean
  clientId?: string
  enabledProviders?: ('google' | 'github' | 'microsoft' | 'apple')[]
}

export const SocialLoginButtons: React.FC<SocialLoginButtonsProps> = ({
  onError,
  disabled = false,
  clientId,
  enabledProviders = ['google', 'github', 'microsoft', 'apple'],
}) => {
  return (
    <div className="space-y-3">
      {enabledProviders.includes('github') && (
        <GitHubLoginButton onError={onError} disabled={disabled} clientId={clientId} />
      )}
      {enabledProviders.includes('microsoft') && (
        <MicrosoftLoginButton onError={onError} disabled={disabled} clientId={clientId} />
      )}
      {enabledProviders.includes('apple') && (
        <AppleLoginButton onError={onError} disabled={disabled} clientId={clientId} />
      )}
    </div>
  )
}

export default SocialLoginButtons
