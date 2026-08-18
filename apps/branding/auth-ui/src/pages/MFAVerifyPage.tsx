import React, { useState } from 'react'
import { useSearchParams, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'

const MFAVerifyPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()

  const [code, setCode] = useState('')
  const [useRecovery, setUseRecovery] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const challenge = searchParams.get('mfa_challenge')

  const handleVerify = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!code) {
      setError(t('auth:mfa.codeRequired', 'Please enter a code'))
      return
    }

    setIsSubmitting(true)
    setError(null)

    try {
      const authBackendUrl = import.meta.env.VITE_AUTH_BACKEND_URL || import.meta.env.VITE_API_URL
      const endpoint = useRecovery ? '/auth/mfa/recovery' : '/auth/mfa/verify'

      const response = await fetch(`${authBackendUrl}${endpoint}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          challenge,
          code: code.replace(/\s/g, ''),
        }),
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
          window.location.href = data.redirect_to
        } else {
          window.location.href = data.redirect_to
        }
      } else {
        navigate('/')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Verification failed')
    } finally {
      setIsSubmitting(false)
    }
  }

  if (!challenge) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="text-center">
          <h2 className="text-2xl font-bold text-gray-900">
            {t('auth:mfa.invalidChallenge', 'Invalid MFA Challenge')}
          </h2>
          <p className="mt-2 text-gray-600">
            {t('auth:mfa.challengeExpired', 'The MFA challenge has expired or is invalid.')}
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8 relative">
      <div className="absolute top-4 right-4">
        <LanguageSwitcher variant="minimal" />
      </div>

      <div className="max-w-md w-full space-y-8">
        <div className="text-center">
          <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-indigo-100">
            <svg className="h-6 w-6 text-indigo-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
          </div>
          <h2 className="mt-6 text-3xl font-extrabold text-gray-900">
            {t('auth:mfa.verifyTitle', 'Two-Factor Authentication')}
          </h2>
          <p className="mt-2 text-sm text-gray-600">
            {useRecovery
              ? t('auth:mfa.enterRecoveryCode', 'Enter one of your recovery codes')
              : t('auth:mfa.enterCode', 'Enter the 6-digit code from your authenticator app')}
          </p>
        </div>

        <form onSubmit={handleVerify} className="mt-8 space-y-6">
          {error && (
            <div className="rounded-md bg-red-50 p-4">
              <div className="text-sm text-red-700">{error}</div>
            </div>
          )}

          <div>
            <label htmlFor="code" className="sr-only">
              {useRecovery
                ? t('auth:mfa.recoveryCodeLabel', 'Recovery Code')
                : t('auth:mfa.codeLabel', 'Verification Code')}
            </label>
            <input
              id="code"
              type="text"
              inputMode={useRecovery ? 'text' : 'numeric'}
              pattern={useRecovery ? undefined : '[0-9]*'}
              maxLength={useRecovery ? 16 : 6}
              value={code}
              onChange={(e) => setCode(useRecovery ? e.target.value : e.target.value.replace(/\D/g, ''))}
              className={`appearance-none relative block w-full px-3 py-4 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 text-center ${
                useRecovery ? 'text-lg' : 'text-2xl tracking-widest'
              } font-mono`}
              placeholder={useRecovery ? 'XXXX-XXXX-XXXX' : '000000'}
              autoComplete="one-time-code"
              autoFocus
            />
          </div>

          <div>
            <button
              type="submit"
              disabled={isSubmitting || !code}
              className="w-full flex justify-center py-3 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isSubmitting ? (
                <span className="flex items-center">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                  {t('common:verifying', 'Verifying...')}
                </span>
              ) : (
                t('common:verify', 'Verify')
              )}
            </button>
          </div>

          <div className="text-center">
            <button
              type="button"
              onClick={() => {
                setUseRecovery(!useRecovery)
                setCode('')
                setError(null)
              }}
              className="text-sm text-indigo-600 hover:text-indigo-500"
            >
              {useRecovery
                ? t('auth:mfa.useAuthenticator', 'Use authenticator app instead')
                : t('auth:mfa.useRecoveryCode', 'Use a recovery code instead')}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default MFAVerifyPage
