import React, { useState, useEffect } from 'react'
import { useSearchParams, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'

interface MFASetupResponse {
  secret: string
  qr_code: string
  recovery_codes?: string[]
}

const MFASetupPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()

  const [step, setStep] = useState<'setup' | 'verify' | 'recovery'>('setup')
  const [setupData, setSetupData] = useState<MFASetupResponse | null>(null)
  const [verificationCode, setVerificationCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const challenge = searchParams.get('mfa_challenge')
  const redirectUrl = searchParams.get('redirect_to')

  // Fetch MFA setup data
  useEffect(() => {
    if (!challenge) {
      setError('Missing MFA challenge parameter')
      setIsLoading(false)
      return
    }

    const fetchSetupData = async () => {
      try {
        const authBackendUrl = import.meta.env.VITE_AUTH_BACKEND_URL || import.meta.env.VITE_API_URL
        const response = await fetch(`${authBackendUrl}/auth/mfa/setup`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ challenge }),
          credentials: 'include',
        })

        if (!response.ok) {
          const data = await response.json()
          throw new Error(data.error || 'Failed to setup MFA')
        }

        const data = await response.json()
        setSetupData(data)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to setup MFA')
      } finally {
        setIsLoading(false)
      }
    }

    fetchSetupData()
  }, [challenge])

  const handleVerify = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!verificationCode || verificationCode.length !== 6) {
      setError('Please enter a 6-digit code')
      return
    }

    setIsSubmitting(true)
    setError(null)

    try {
      const authBackendUrl = import.meta.env.VITE_AUTH_BACKEND_URL || import.meta.env.VITE_API_URL
      const response = await fetch(`${authBackendUrl}/auth/mfa/verify`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          challenge,
          code: verificationCode,
        }),
        credentials: 'include',
      })

      if (!response.ok) {
        const data = await response.json()
        throw new Error(data.error || 'Invalid verification code')
      }

      const data = await response.json()

      if (data.recovery_codes) {
        setRecoveryCodes(data.recovery_codes)
        setStep('recovery')
      } else if (redirectUrl) {
        window.location.href = redirectUrl
      } else {
        navigate('/')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Verification failed')
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleComplete = () => {
    if (redirectUrl) {
      window.location.href = redirectUrl
    } else {
      navigate('/')
    }
  }

  const copyRecoveryCodes = () => {
    navigator.clipboard.writeText(recoveryCodes.join('\n'))
  }

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600"></div>
      </div>
    )
  }

  if (error && !setupData) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="max-w-md w-full space-y-8 px-4">
          <div className="text-center">
            <h2 className="mt-6 text-3xl font-extrabold text-gray-900">
              {t('common:errorOccurred', 'Error')}
            </h2>
            <p className="mt-2 text-sm text-red-600">{error}</p>
          </div>
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
        {/* Setup Step */}
        {step === 'setup' && setupData && (
          <>
            <div className="text-center">
              <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-indigo-100">
                <svg className="h-6 w-6 text-indigo-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                </svg>
              </div>
              <h2 className="mt-6 text-3xl font-extrabold text-gray-900">
                {t('auth:mfa.setupTitle', 'Set up Two-Factor Authentication')}
              </h2>
              <p className="mt-2 text-sm text-gray-600">
                {t('auth:mfa.setupDescription', 'Scan the QR code with your authenticator app')}
              </p>
            </div>

            <div className="mt-8 space-y-6">
              {/* QR Code */}
              <div className="flex justify-center">
                <div className="p-4 bg-white rounded-lg shadow-md">
                  <img
                    src={setupData.qr_code}
                    alt="MFA QR Code"
                    className="w-48 h-48"
                  />
                </div>
              </div>

              {/* Manual Entry */}
              <div className="text-center">
                <p className="text-sm text-gray-500 mb-2">
                  {t('auth:mfa.manualEntry', 'Or enter this code manually:')}
                </p>
                <code className="px-4 py-2 bg-gray-100 rounded text-sm font-mono break-all">
                  {setupData.secret}
                </code>
              </div>

              <button
                onClick={() => setStep('verify')}
                className="w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
              >
                {t('common:continue', 'Continue')}
              </button>
            </div>
          </>
        )}

        {/* Verify Step */}
        {step === 'verify' && (
          <>
            <div className="text-center">
              <h2 className="mt-6 text-3xl font-extrabold text-gray-900">
                {t('auth:mfa.verifyTitle', 'Verify Setup')}
              </h2>
              <p className="mt-2 text-sm text-gray-600">
                {t('auth:mfa.verifyDescription', 'Enter the 6-digit code from your authenticator app')}
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
                  {t('auth:mfa.codeLabel', 'Verification Code')}
                </label>
                <input
                  id="code"
                  type="text"
                  inputMode="numeric"
                  pattern="[0-9]*"
                  maxLength={6}
                  value={verificationCode}
                  onChange={(e) => setVerificationCode(e.target.value.replace(/\D/g, ''))}
                  className="appearance-none relative block w-full px-3 py-4 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 text-center text-2xl tracking-widest font-mono"
                  placeholder="000000"
                  autoComplete="one-time-code"
                  autoFocus
                />
              </div>

              <div className="flex space-x-4">
                <button
                  type="button"
                  onClick={() => setStep('setup')}
                  className="flex-1 py-2 px-4 border border-gray-300 text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
                >
                  {t('common:back', 'Back')}
                </button>
                <button
                  type="submit"
                  disabled={isSubmitting || verificationCode.length !== 6}
                  className="flex-1 py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {isSubmitting ? (
                    <span className="flex items-center justify-center">
                      <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                      {t('common:verifying', 'Verifying...')}
                    </span>
                  ) : (
                    t('common:verify', 'Verify')
                  )}
                </button>
              </div>
            </form>
          </>
        )}

        {/* Recovery Codes Step */}
        {step === 'recovery' && recoveryCodes.length > 0 && (
          <>
            <div className="text-center">
              <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-green-100">
                <svg className="h-6 w-6 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <h2 className="mt-6 text-3xl font-extrabold text-gray-900">
                {t('auth:mfa.successTitle', 'MFA Enabled!')}
              </h2>
              <p className="mt-2 text-sm text-gray-600">
                {t('auth:mfa.recoveryDescription', 'Save these recovery codes in a safe place. You can use them to access your account if you lose your authenticator device.')}
              </p>
            </div>

            <div className="mt-8 space-y-6">
              <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
                <p className="text-sm font-medium text-yellow-800 mb-3">
                  {t('auth:mfa.recoveryWarning', 'Warning: These codes will only be shown once!')}
                </p>
                <div className="grid grid-cols-2 gap-2">
                  {recoveryCodes.map((code, index) => (
                    <code key={index} className="px-2 py-1 bg-white rounded text-sm font-mono text-center border">
                      {code}
                    </code>
                  ))}
                </div>
              </div>

              <button
                type="button"
                onClick={copyRecoveryCodes}
                className="w-full py-2 px-4 border border-gray-300 text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
              >
                {t('common:copy', 'Copy to Clipboard')}
              </button>

              <button
                onClick={handleComplete}
                className="w-full py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
              >
                {t('common:done', 'Done')}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

export default MFASetupPage
