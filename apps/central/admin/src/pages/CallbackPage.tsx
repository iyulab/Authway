import React, { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '../stores/auth'
import { useTenantStore } from '../stores/tenant'
import { completeSignIn } from '../lib/consoleSignIn'
import LanguageSwitcher from '../components/LanguageSwitcher'

/**
 * Where Authway's sign-in comes back to: takes the console account's token
 * and goes on to the tenant picker.
 */
const CallbackPage: React.FC = () => {
  const { t } = useTranslation('login')
  const navigate = useNavigate()
  const login = useAuthStore((state) => state.login)
  const [error, setError] = useState<string | null>(null)
  // React runs effects twice in development; a code is redeemed once.
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true
    completeSignIn()
      .then(({ token, expiresAt }) => {
        // A tenant picked in an earlier session may not be one this account administers.
        useTenantStore.getState().clearSelectedTenant()
        login(token, expiresAt, 'account')
        navigate('/', { replace: true })
      })
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)))
  }, [login, navigate])

  return (
    <div className="relative min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <LanguageSwitcher className="absolute right-4 top-4" />
      {error ? (
        <div className="max-w-md w-full space-y-4 text-center">
          <div className="rounded-md bg-red-50 p-4 text-sm text-red-700" role="alert">
            {t('callback.failed', { error })}
          </div>
          <Link to="/login" className="text-sm text-indigo-600 hover:text-indigo-500">
            {t('callback.back')}
          </Link>
        </div>
      ) : (
        <div className="flex items-center text-sm text-gray-600">
          <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-indigo-600 mr-2"></div>
          {t('callback.signingIn')}
        </div>
      )}
    </div>
  )
}

export default CallbackPage
