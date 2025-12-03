import React, { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'

// Validation schema - will use i18n messages dynamically
const createRegisterSchema = (t: (key: string) => string) => z.object({
  email: z.string().email(t('auth:validation.emailInvalid')),
  password: z.string().min(8, t('auth:validation.passwordMin8')),
  confirmPassword: z.string().min(8, t('auth:validation.confirmPasswordRequired')),
  firstName: z.string().min(1, t('auth:validation.firstNameRequired')),
  lastName: z.string().min(1, t('auth:validation.lastNameRequired')),
}).refine((data) => data.password === data.confirmPassword, {
  message: t('auth:validation.passwordMismatch'),
  path: ['confirmPassword'],
})

type RegisterFormData = z.infer<ReturnType<typeof createRegisterSchema>>

interface RegisterRequest {
  email: string
  password: string
  first_name: string
  last_name: string
}

interface RegisterResponse {
  id?: string
  email?: string
  name?: string
  error?: string
}

const RegisterPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  // Preserve login_challenge for OAuth flow
  const loginChallenge = searchParams.get('login_challenge')

  const registerSchema = createRegisterSchema(t)

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<RegisterFormData>({
    resolver: zodResolver(registerSchema),
  })

  // Error message mapping
  const getErrorMessage = (apiError: string): string => {
    const errorMessages: Record<string, string> = {
      'User with this email already exists': t('auth:errors.userAlreadyExists'),
      'Email and password are required': t('auth:errors.emailPasswordRequired'),
      'Invalid request body': t('auth:errors.invalidRequest'),
      'Failed to create user': t('auth:errors.createUserFailed'),
    }
    return errorMessages[apiError] || apiError
  }

  // Register mutation
  const registerMutation = useMutation({
    mutationFn: async (data: RegisterFormData): Promise<RegisterResponse> => {
      const response = await fetch(`${import.meta.env.VITE_API_URL}/register`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          email: data.email,
          password: data.password,
          first_name: data.firstName,
          last_name: data.lastName,
        } as RegisterRequest),
      })

      return response.json()
    },
    onSuccess: (data) => {
      if (data.error) {
        setError(getErrorMessage(data.error))
      } else {
        setSuccess(true)
        setError(null)
        // Redirect to login page after 3 seconds (preserve login_challenge)
        setTimeout(() => {
          const loginUrl = loginChallenge
            ? `/login?login_challenge=${loginChallenge}`
            : '/login'
          navigate(loginUrl)
        }, 3000)
      }
    },
    onError: (error) => {
      console.error('Register error:', error)
      setError(t('auth:errors.registerFailed'))
    },
  })

  const onSubmit = (data: RegisterFormData) => {
    setError(null)
    registerMutation.mutate(data)
  }

  if (success) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="max-w-md w-full space-y-8">
          <div className="text-center">
            <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-green-100">
              <svg
                className="h-6 w-6 text-green-600"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M5 13l4 4L19 7"
                />
              </svg>
            </div>
            <h2 className="mt-6 text-3xl font-extrabold text-gray-900">{t('auth:register.success.title')}</h2>
            <p className="mt-2 text-sm text-gray-600">
              {t('auth:register.success.message')}
            </p>
            <p className="mt-1 text-sm text-gray-500">
              {t('auth:register.success.redirect')}
            </p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8 relative">
      {/* Language Switcher - positioned at top right */}
      <div className="absolute top-4 right-4">
        <LanguageSwitcher variant="minimal" />
      </div>
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
                d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z"
              />
            </svg>
          </div>
          <h2 className="mt-6 text-center text-3xl font-extrabold text-gray-900">
            {t('auth:register.title')}
          </h2>
          <p className="mt-2 text-center text-sm text-gray-600">
            {t('auth:register.subtitle')}
          </p>
        </div>

        <form className="mt-8 space-y-6" onSubmit={handleSubmit(onSubmit)}>
          {error && (
            <div className="rounded-md bg-red-50 p-4">
              <div className="text-sm text-red-700">{error}</div>
            </div>
          )}

          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label htmlFor="firstName" className="block text-sm font-medium text-gray-700">
                  {t('auth:register.firstNameLabel')}
                </label>
                <input
                  {...register('firstName')}
                  type="text"
                  autoComplete="given-name"
                  className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
                  placeholder={t('auth:register.firstNamePlaceholder')}
                />
                {errors.firstName && (
                  <p className="mt-1 text-sm text-red-600">{errors.firstName.message}</p>
                )}
              </div>

              <div>
                <label htmlFor="lastName" className="block text-sm font-medium text-gray-700">
                  {t('auth:register.lastNameLabel')}
                </label>
                <input
                  {...register('lastName')}
                  type="text"
                  autoComplete="family-name"
                  className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
                  placeholder={t('auth:register.lastNamePlaceholder')}
                />
                {errors.lastName && (
                  <p className="mt-1 text-sm text-red-600">{errors.lastName.message}</p>
                )}
              </div>
            </div>

            <div>
              <label htmlFor="email" className="block text-sm font-medium text-gray-700">
                {t('auth:register.emailLabel')}
              </label>
              <input
                {...register('email')}
                type="email"
                autoComplete="email"
                className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
                placeholder={t('auth:register.emailPlaceholder')}
              />
              {errors.email && (
                <p className="mt-1 text-sm text-red-600">{errors.email.message}</p>
              )}
            </div>

            <div>
              <label htmlFor="password" className="block text-sm font-medium text-gray-700">
                {t('auth:register.passwordLabel')}
              </label>
              <input
                {...register('password')}
                type="password"
                autoComplete="new-password"
                className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
                placeholder={t('auth:register.passwordPlaceholder')}
              />
              {errors.password && (
                <p className="mt-1 text-sm text-red-600">{errors.password.message}</p>
              )}
            </div>

            <div>
              <label htmlFor="confirmPassword" className="block text-sm font-medium text-gray-700">
                {t('auth:register.confirmPasswordLabel')}
              </label>
              <input
                {...register('confirmPassword')}
                type="password"
                autoComplete="new-password"
                className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
                placeholder={t('auth:register.confirmPasswordPlaceholder')}
              />
              {errors.confirmPassword && (
                <p className="mt-1 text-sm text-red-600">{errors.confirmPassword.message}</p>
              )}
            </div>
          </div>

          <div>
            <button
              type="submit"
              disabled={isSubmitting || registerMutation.isPending}
              className="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isSubmitting || registerMutation.isPending ? (
                <div className="flex items-center">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                  {t('auth:register.submitting')}
                </div>
              ) : (
                t('auth:register.submitButton')
              )}
            </button>
          </div>

          <div className="text-center">
            <button
              type="button"
              onClick={() => navigate(`/login${loginChallenge ? `?login_challenge=${loginChallenge}` : ''}`)}
              className="text-sm text-indigo-600 hover:text-indigo-500"
            >
              {t('auth:register.hasAccount')}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default RegisterPage
