import React, { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { submitFlowStep } from '../utils/loginFlow'

interface LogoutErrorState {
  message: string
  fallbackRedirect: string | null
}

/** Origin of the page that sent the user here, to return them to on failure. */
function referrerOrigin(): string | null {
  try {
    return document.referrer ? new URL(document.referrer).origin : null
  } catch {
    return null
  }
}

/**
 * Completes a logout flow without asking: the backend ends the session and
 * says where to go; the authorization server has already checked that
 * destination against the client's registered post-logout URIs.
 */
const LogoutPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const [error, setError] = useState<LogoutErrorState | null>(null)
  const submittedRef = useRef<string | null>(null)

  const flow = searchParams.get('flow')

  // On failure, return the user to where they came from after a moment.
  useEffect(() => {
    if (!error?.fallbackRedirect) return
    const timer = window.setTimeout(() => {
      window.location.href = error.fallbackRedirect!
    }, 1000)
    return () => clearTimeout(timer)
  }, [error])

  useEffect(() => {
    if (!flow) {
      setError({ message: t('auth:logout.missingFlow', 'This sign-out link is not valid.'), fallbackRedirect: referrerOrigin() })
      return
    }
    // Strict Mode mounts twice; a logout flow can only be completed once.
    if (submittedRef.current === flow) return
    submittedRef.current = flow

    submitFlowStep('logout-flows', flow, '')
      .then((data) => {
        if (data.next === 'redirect' && data.redirect_to) {
          window.location.href = data.redirect_to
        } else {
          setError({ message: data.error || t('auth:logout.error'), fallbackRedirect: referrerOrigin() })
        }
      })
      .catch((err) => {
        console.error('[Authway Logout] Network or parsing error', err)
        setError({ message: t('auth:logout.networkError', 'Signing out failed because of a network error.'), fallbackRedirect: referrerOrigin() })
      })
  }, [flow, t])

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="text-center max-w-md px-4">
          <h1 className="text-2xl font-bold text-gray-900 mb-4">{t('auth:logout.error')}</h1>
          <p className="text-red-600 mb-4 text-sm">{error.message}</p>
          {error.fallbackRedirect ? (
            <p className="text-gray-600">{t('auth:logout.redirecting', 'Redirecting you back...')}</p>
          ) : (
            <p className="text-gray-600">{t('auth:logout.closeWindow', 'Please close this window or return to your application.')}</p>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="text-center">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mb-4"></div>
        <h1 className="text-2xl font-bold text-gray-900 mb-2">{t('auth:logout.loggingOut')}</h1>
        <p className="text-gray-600">{t('auth:logout.pleaseWait')}</p>
      </div>
    </div>
  )
}

export default LogoutPage
