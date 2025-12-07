import React, { useState, useEffect } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'

const MagicLinkPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()

  const [email, setEmail] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [isSent, setIsSent] = useState(false)
  const [isVerifying, setIsVerifying] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const challenge = searchParams.get('login_challenge')
  const token = searchParams.get('token')

  // Handle magic link verification
  useEffect(() => {
    if (!token) return

    const verifyToken = async () => {
      setIsVerifying(true)
      setError(null)

      try {
        const authBackendUrl = import.meta.env.VITE_AUTH_BACKEND_URL || import.meta.env.VITE_API_URL
        const response = await fetch(`${authBackendUrl}/auth/magic-link/verify`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ token }),
          credentials: 'include',
        })

        if (!response.ok) {
          const data = await response.json()
          throw new Error(data.error || 'Verification failed')
        }

        const data = await response.json()

        if (data.redirect_to) {
          // Handle popup mode
          const isPopupMode = window.opener !== null && window.opener !== window ||
            sessionStorage.getItem('authway_popup_mode') === 'true'

          if (isPopupMode) {
            sessionStorage.removeItem('authway_popup_mode')
          }

          window.location.href = data.redirect_to
        } else {
          navigate('/')
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Verification failed')
      } finally {
        setIsVerifying(false)
      }
    }

    verifyToken()
  }, [token, navigate])

  const handleRequestMagicLink = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!email) {
      setError(t('auth:validation.emailRequired', 'Email is required'))
      return
    }

    setIsSubmitting(true)
    setError(null)

    try {
      const authBackendUrl = import.meta.env.VITE_AUTH_BACKEND_URL || import.meta.env.VITE_API_URL
      const response = await fetch(`${authBackendUrl}/auth/magic-link/request`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email,
          login_challenge: challenge,
        }),
        credentials: 'include',
      })

      if (!response.ok) {
        const data = await response.json()
        throw new Error(data.error || 'Failed to send magic link')
      }

      setIsSent(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to send magic link')
    } finally {
      setIsSubmitting(false)
    }
  }

  // Verifying token
  if (token) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="max-w-md w-full space-y-8 px-4">
          {isVerifying ? (
            <div className="text-center">
              <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto"></div>
              <p className="mt-4 text-gray-600">
                {t('auth:magicLink.verifying', 'Verifying your magic link...')}
              </p>
            </div>
          ) : error ? (
            <div className="text-center">
              <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-red-100">
                <svg className="h-6 w-6 text-red-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </div>
              <h2 className="mt-6 text-2xl font-bold text-gray-900">
                {t('auth:magicLink.invalidLink', 'Invalid Magic Link')}
              </h2>
              <p className="mt-2 text-sm text-red-600">{error}</p>
              <button
                onClick={() => navigate('/login')}
                className="mt-4 text-indigo-600 hover:text-indigo-500"
              >
                {t('auth:magicLink.tryAgain', 'Try logging in again')}
              </button>
            </div>
          ) : null}
        </div>
      </div>
    )
  }

  // Email sent success
  if (isSent) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8 relative">
        <div className="absolute top-4 right-4">
          <LanguageSwitcher variant="minimal" />
        </div>

        <div className="max-w-md w-full space-y-8">
          <div className="text-center">
            <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-green-100">
              <svg className="h-6 w-6 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
              </svg>
            </div>
            <h2 className="mt-6 text-3xl font-extrabold text-gray-900">
              {t('auth:magicLink.checkEmail', 'Check your email')}
            </h2>
            <p className="mt-2 text-sm text-gray-600">
              {t('auth:magicLink.sentTo', 'We sent a magic link to')}
            </p>
            <p className="font-medium text-gray-900">{email}</p>
            <p className="mt-4 text-sm text-gray-500">
              {t('auth:magicLink.clickLink', 'Click the link in the email to sign in. The link expires in 15 minutes.')}
            </p>
          </div>

          <div className="mt-6 text-center">
            <button
              onClick={() => {
                setIsSent(false)
                setEmail('')
              }}
              className="text-sm text-indigo-600 hover:text-indigo-500"
            >
              {t('auth:magicLink.useDifferentEmail', 'Use a different email')}
            </button>
          </div>
        </div>
      </div>
    )
  }

  // Request magic link form
  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8 relative">
      <div className="absolute top-4 right-4">
        <LanguageSwitcher variant="minimal" />
      </div>

      <div className="max-w-md w-full space-y-8">
        <div className="text-center">
          <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-indigo-100">
            <svg className="h-6 w-6 text-indigo-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
            </svg>
          </div>
          <h2 className="mt-6 text-3xl font-extrabold text-gray-900">
            {t('auth:magicLink.title', 'Sign in with Magic Link')}
          </h2>
          <p className="mt-2 text-sm text-gray-600">
            {t('auth:magicLink.description', 'We\'ll send you a secure link to sign in without a password')}
          </p>
        </div>

        <form onSubmit={handleRequestMagicLink} className="mt-8 space-y-6">
          {error && (
            <div className="rounded-md bg-red-50 p-4">
              <div className="text-sm text-red-700">{error}</div>
            </div>
          )}

          <div>
            <label htmlFor="email" className="block text-sm font-medium text-gray-700">
              {t('auth:login.emailLabel', 'Email address')}
            </label>
            <input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
              placeholder={t('auth:login.emailPlaceholder', 'you@example.com')}
              autoComplete="email"
              autoFocus
            />
          </div>

          <div>
            <button
              type="submit"
              disabled={isSubmitting || !email}
              className="w-full flex justify-center py-3 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isSubmitting ? (
                <span className="flex items-center">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                  {t('common:sending', 'Sending...')}
                </span>
              ) : (
                t('auth:magicLink.sendLink', 'Send Magic Link')
              )}
            </button>
          </div>

          <div className="text-center">
            <button
              type="button"
              onClick={() => navigate(`/login${challenge ? `?login_challenge=${challenge}` : ''}`)}
              className="text-sm text-indigo-600 hover:text-indigo-500"
            >
              {t('auth:magicLink.backToLogin', 'Back to login')}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default MagicLinkPage
