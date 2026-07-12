import React, { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useQuery, useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'

// Validation schema - will use i18n messages dynamically
const createAcceptSchema = (t: (key: string) => string) => z.object({
  password: z.string().min(8, t('auth:validation.passwordMin8')),
  confirmPassword: z.string().min(8, t('auth:validation.confirmPasswordRequired')),
  firstName: z.string().min(1, t('auth:validation.firstNameRequired')),
  lastName: z.string().min(1, t('auth:validation.lastNameRequired')),
}).refine((data) => data.password === data.confirmPassword, {
  message: t('auth:validation.passwordMismatch'),
  path: ['confirmPassword'],
})

type AcceptFormData = z.infer<ReturnType<typeof createAcceptSchema>>

interface InvitationDetails {
  email: string
  tenant_name: string
  inviter_name: string
  role: string
}

const inputClass =
  'mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm'

const AcceptInvitationPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  const token = searchParams.get('token') ?? ''
  const loginChallenge = searchParams.get('login_challenge')
  const apiUrl = import.meta.env.VITE_API_URL

  const acceptSchema = createAcceptSchema(t)
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<AcceptFormData>({ resolver: zodResolver(acceptSchema) })

  // Validate the invitation token up front. The subject email is fixed by the
  // invitation and never chosen by the user.
  const {
    data: invitation,
    isLoading,
    isError,
  } = useQuery<InvitationDetails>({
    queryKey: ['invitation', token],
    enabled: token !== '',
    retry: false,
    queryFn: async () => {
      const res = await fetch(`${apiUrl}/api/v1/invitations/token/${encodeURIComponent(token)}`)
      if (!res.ok) {
        throw new Error('invitation not acceptable')
      }
      const body = await res.json()
      return body.invitation as InvitationDetails
    },
  })

  const acceptMutation = useMutation({
    mutationFn: async (data: AcceptFormData) => {
      const res = await fetch(`${apiUrl}/api/v1/invitations/accept`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          token,
          name: `${data.firstName} ${data.lastName}`.trim(),
          password: data.password,
        }),
      })
      const body = await res.json()
      if (!res.ok) {
        throw new Error(body.error || 'accept failed')
      }
      return body
    },
    onSuccess: () => {
      setSuccess(true)
      setError(null)
      setTimeout(() => {
        navigate(loginChallenge ? `/login?login_challenge=${loginChallenge}` : '/login')
      }, 3000)
    },
    onError: (err: Error) => {
      setError(err.message)
    },
  })

  const onSubmit = (data: AcceptFormData) => {
    setError(null)
    acceptMutation.mutate(data)
  }

  const shell = (children: React.ReactNode) => (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8 relative">
      <div className="absolute top-4 right-4">
        <LanguageSwitcher variant="minimal" />
      </div>
      <div className="max-w-md w-full space-y-8">{children}</div>
    </div>
  )

  // Missing/invalid/expired invitation, or no token at all: invitation-only, so
  // there is no public self-registration fallback.
  if (token === '' || isError) {
    return shell(
      <div className="text-center">
        <h2 className="mt-6 text-3xl font-extrabold text-gray-900">{t('auth:invitation.invalidTitle')}</h2>
        <p className="mt-2 text-sm text-gray-600">{t('auth:invitation.invalidMessage')}</p>
        <button
          type="button"
          onClick={() => navigate(loginChallenge ? `/login?login_challenge=${loginChallenge}` : '/login')}
          className="mt-6 text-sm text-indigo-600 hover:text-indigo-500"
        >
          {t('auth:invitation.backToLogin')}
        </button>
      </div>
    )
  }

  if (isLoading || !invitation) {
    return shell(
      <div className="text-center">
        <div className="mx-auto animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600"></div>
        <p className="mt-4 text-sm text-gray-600">{t('auth:invitation.loading')}</p>
      </div>
    )
  }

  if (success) {
    return shell(
      <div className="text-center">
        <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-green-100">
          <svg className="h-6 w-6 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
          </svg>
        </div>
        <h2 className="mt-6 text-3xl font-extrabold text-gray-900">{t('auth:invitation.success.title')}</h2>
        <p className="mt-2 text-sm text-gray-600">{t('auth:invitation.success.message')}</p>
        <p className="mt-1 text-sm text-gray-500">{t('auth:invitation.success.redirect')}</p>
      </div>
    )
  }

  return shell(
    <>
      <div>
        <div className="mx-auto h-12 w-12 flex items-center justify-center rounded-full bg-indigo-100">
          <svg className="h-6 w-6 text-indigo-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
          </svg>
        </div>
        <h2 className="mt-6 text-center text-3xl font-extrabold text-gray-900">{t('auth:invitation.title')}</h2>
        <p className="mt-2 text-center text-sm text-gray-600">
          {t('auth:invitation.subtitle', { tenant: invitation.tenant_name })}
        </p>
      </div>

      <form className="mt-8 space-y-6" onSubmit={handleSubmit(onSubmit)}>
        {error && (
          <div className="rounded-md bg-red-50 p-4">
            <div className="text-sm text-red-700">{error}</div>
          </div>
        )}

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">{t('auth:invitation.emailLabel')}</label>
            <input type="email" value={invitation.email} readOnly disabled className={`${inputClass} bg-gray-100`} />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label htmlFor="firstName" className="block text-sm font-medium text-gray-700">
                {t('auth:register.firstNameLabel')}
              </label>
              <input {...register('firstName')} type="text" autoComplete="given-name" className={inputClass} placeholder={t('auth:register.firstNamePlaceholder')} />
              {errors.firstName && <p className="mt-1 text-sm text-red-600">{errors.firstName.message}</p>}
            </div>
            <div>
              <label htmlFor="lastName" className="block text-sm font-medium text-gray-700">
                {t('auth:register.lastNameLabel')}
              </label>
              <input {...register('lastName')} type="text" autoComplete="family-name" className={inputClass} placeholder={t('auth:register.lastNamePlaceholder')} />
              {errors.lastName && <p className="mt-1 text-sm text-red-600">{errors.lastName.message}</p>}
            </div>
          </div>

          <div>
            <label htmlFor="password" className="block text-sm font-medium text-gray-700">
              {t('auth:register.passwordLabel')}
            </label>
            <input {...register('password')} type="password" autoComplete="new-password" className={inputClass} placeholder={t('auth:register.passwordPlaceholder')} />
            {errors.password && <p className="mt-1 text-sm text-red-600">{errors.password.message}</p>}
          </div>

          <div>
            <label htmlFor="confirmPassword" className="block text-sm font-medium text-gray-700">
              {t('auth:register.confirmPasswordLabel')}
            </label>
            <input {...register('confirmPassword')} type="password" autoComplete="new-password" className={inputClass} placeholder={t('auth:register.confirmPasswordPlaceholder')} />
            {errors.confirmPassword && <p className="mt-1 text-sm text-red-600">{errors.confirmPassword.message}</p>}
          </div>
        </div>

        <div>
          <button
            type="submit"
            disabled={isSubmitting || acceptMutation.isPending}
            className="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isSubmitting || acceptMutation.isPending ? (
              <div className="flex items-center">
                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                {t('auth:invitation.submitting')}
              </div>
            ) : (
              t('auth:invitation.submitButton')
            )}
          </button>
        </div>
      </form>
    </>
  )
}

export default AcceptInvitationPage
