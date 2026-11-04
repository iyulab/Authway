import React, { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useAuthStore } from '../stores/auth'
import { apiErrorMessage, authApi, LoginRequest } from '../lib/api'
import LanguageSwitcher from '../components/LanguageSwitcher'
import { fetchConsoleEntry, signInWithAccount, type ConsoleEntry } from '../lib/consoleSignIn'

type LoginFormData = { password: string }

const LoginPage: React.FC = () => {
  const { t } = useTranslation('login')
  const navigate = useNavigate()
  // Validation schema — built per language, so its messages follow the switcher.
  const loginSchema = useMemo(
    () => z.object({ password: z.string().min(1, t('password.required')) }),
    [t],
  )
  const login = useAuthStore((state) => state.login)
  const [error, setError] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  // The console's own sign-in, when the deployment offers it.
  const [entry, setEntry] = useState<ConsoleEntry | null>(null)
  const [leaving, setLeaving] = useState(false)

  useEffect(() => {
    let current = true
    fetchConsoleEntry().then((found) => {
      if (current) setEntry(found)
    })
    return () => {
      current = false
    }
  }, [])

  const onAccount = async () => {
    if (!entry) return
    setLeaving(true)
    setError(null)
    try {
      await signInWithAccount(entry)
    } catch (err: any) {
      setLeaving(false)
      setError(err?.message || t('account.openFailed'))
    }
  }

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
  })

  const onSubmit = async (data: LoginFormData) => {
    setIsLoading(true)
    setError(null)

    try {
      const response = await authApi.login(data as LoginRequest)
      const { token, expires_at } = response.data

      // Store token
      login(token, expires_at)

      // Redirect to dashboard
      navigate('/dashboard')
    } catch (err: any) {
      console.error('Login error:', err)
      setError(apiErrorMessage(err, 'login:failed'))
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="relative min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <LanguageSwitcher className="absolute right-4 top-4" />
      <div className="max-w-md w-full space-y-8">
        <div>
          <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-indigo-100">
            <svg
              className="h-6 w-6 text-indigo-600"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
              />
            </svg>
          </div>
          <h2 className="mt-6 text-center text-3xl font-extrabold text-gray-900">
            {t('title')}
          </h2>
          <p className="mt-2 text-center text-sm text-gray-600">
            {entry ? t('subtitle.withAccount') : t('subtitle.passwordOnly')}
          </p>
        </div>

        {entry && (
          <div className="space-y-4">
            <button
              type="button"
              onClick={onAccount}
              disabled={leaving}
              className="w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {leaving ? t('account.leaving') : t('account.button')}
            </button>
            <div className="relative">
              <div className="absolute inset-0 flex items-center" aria-hidden="true">
                <div className="w-full border-t border-gray-300" />
              </div>
              <div className="relative flex justify-center text-sm">
                <span className="bg-gray-50 px-2 text-gray-500">{t('account.or')}</span>
              </div>
            </div>
          </div>
        )}

        <form className="mt-8 space-y-6" onSubmit={handleSubmit(onSubmit)}>
          {error && (
            <div className="rounded-md bg-red-50 p-4">
              <div className="text-sm text-red-700">{error}</div>
            </div>
          )}

          <div className="space-y-4">
            <div>
              <label htmlFor="password" className="block text-sm font-medium text-gray-700">
                {t('password.label')}
              </label>
              <input
                {...register('password')}
                id="password"
                type="password"
                autoComplete="current-password"
                className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-hidden focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
                placeholder={t('password.placeholder')}
              />
              {errors.password && (
                <p className="mt-1 text-sm text-red-600">{errors.password.message}</p>
              )}
            </div>
          </div>

          <div>
            <button
              type="submit"
              disabled={isLoading}
              className="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isLoading ? (
                <div className="flex items-center">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                  {t('submitting')}
                </div>
              ) : (
                t('submit')
              )}
            </button>
          </div>

        </form>
      </div>
    </div>
  )
}

export default LoginPage
