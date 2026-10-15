import React, { useState } from 'react'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'
import CodeInput from '../components/CodeInput'
import { followRedirect, submitFlowStep } from '../utils/loginFlow'

/** The answers the backend gives that this page explains itself. */
const KNOWN_ERRORS = ['user_code_invalid', 'user_code_required', 'invalid_flow', 'flow_expired', 'too_many_requests']

/**
 * Signing in a device (RFC 8628): a TV, a console or a command-line tool
 * shows a code and this address. The user types the code here — or finds it
 * filled in, when the device's link carried it — and the sign-in goes on like
 * any other: the login screen, then the consent screen, which shows which
 * application the device is.
 */
const DevicePage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const flow = searchParams.get('flow')
  const [code, setCode] = useState(searchParams.get('user_code') ?? '')
  const [error, setError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!code.trim()) {
      setError(t('auth:device.errors.user_code_required'))
      return
    }
    setIsSubmitting(true)
    setError(null)
    try {
      const step = await submitFlowStep('device-flows', flow ?? '', '', { user_code: code })
      if (step.next === 'redirect' && step.redirect_to) {
        followRedirect(step.redirect_to)
        return
      }
      setError(step.code && KNOWN_ERRORS.includes(step.code) ? t(`auth:device.errors.${step.code}`) : (step.detail ?? t('common:errorOccurred')))
    } catch {
      setError(t('common:errorOccurred'))
    } finally {
      setIsSubmitting(false)
    }
  }

  if (!flow) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
        <div className="max-w-md text-center">
          <h1 className="text-4xl font-bold text-gray-900">Authway</h1>
          <h2 className="mt-6 text-xl font-semibold text-gray-900">{t('auth:device.noFlowTitle')}</h2>
          <p className="mt-2 text-sm text-gray-600">{t('auth:device.noFlowMessage')}</p>
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
          <h2 className="mt-6 text-3xl font-extrabold text-gray-900">{t('auth:device.title')}</h2>
          <p className="mt-2 text-sm text-gray-600">{t('auth:device.subtitle')}</p>
        </div>
        <form onSubmit={handleSubmit} className="mt-8 space-y-6">
          {error && (
            <div className="rounded-md bg-red-50 p-4" role="alert">
              <div className="text-sm text-red-700">{error}</div>
            </div>
          )}
          <CodeInput id="user_code" label={t('auth:device.codeLabel')} value={code} onChange={setCode} autoFocus />
          <button
            type="submit"
            disabled={isSubmitting}
            className="w-full flex justify-center py-3 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isSubmitting ? t('auth:device.submitting') : t('auth:device.submit')}
          </button>
        </form>
      </div>
    </div>
  )
}

export default DevicePage
