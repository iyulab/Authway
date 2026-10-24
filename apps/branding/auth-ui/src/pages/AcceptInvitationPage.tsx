import React, { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useQuery, useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import LanguageSwitcher from '../components/LanguageSwitcher'
import { getConfig } from '../config'

/** What every tenant asks of a password; a tenant may ask for more. */
const LEAST_PASSWORD_LENGTH = 8

// Validation schema - will use i18n messages dynamically
const createAcceptSchema = (t: (key: string, options?: Record<string, unknown>) => string, passwordLength: number) => z.object({
  password: z.string().min(passwordLength, t('auth:validation.passwordMin', { length: passwordLength })),
  confirmPassword: z.string().min(1, t('auth:validation.confirmPasswordRequired')),
  firstName: z.string().min(1, t('auth:validation.firstNameRequired')),
  lastName: z.string().min(1, t('auth:validation.lastNameRequired')),
}).refine((data) => data.password === data.confirmPassword, {
  message: t('auth:validation.passwordMismatch'),
  path: ['confirmPassword'],
})

type AcceptFormData = z.infer<ReturnType<typeof createAcceptSchema>>

/** The application an invitation is for, when it names one. */
interface InvitationApp {
  client_id: string
  name: string
  initiate_login_uri?: string
  client_uri?: string
}

/** The inviter name the API gives an invitation no person sent (the admin key, the console password). */
const SYSTEM_INVITER = 'system'

/**
 * What a person who accepted an invitation is told when Authway has nowhere to
 * send them: the application by name when the invitation names one, else who
 * invited them, so they know whom to ask for the address.
 */
function nowhereToGo(invitation: InvitationDetails | undefined, t: TFunction): string {
  const tenant = invitation?.tenant_name ?? ''
  if (invitation?.client) return t('auth:invitation.success.signInFromNamedApp', { tenant, name: invitation.client.name })
  const inviter = invitation?.inviter_name
  if (inviter && inviter !== SYSTEM_INVITER) return t('auth:invitation.success.signInFromAppInviter', { tenant, inviter })
  return t('auth:invitation.success.signInFromApp', { tenant })
}

interface InvitationDetails {
  email: string
  tenant_name: string
  inviter_name: string
  role: string
  client?: InvitationApp
}

/**
 * Where a person who accepted an invitation for an application goes next:
 * the address where the application starts a sign-in, told the issuer and
 * whom to expect (OpenID Connect Core 1.0 section 4) — or, without one, the
 * application's home page. Only an https address is followed.
 */
export function continueTo(app: InvitationApp, issuer: string | undefined, email: string): string | undefined {
  const https = (raw?: string) => {
    try {
      const u = raw ? new URL(raw) : undefined
      return u && u.protocol === 'https:' ? u : undefined
    } catch {
      return undefined
    }
  }
  const start = https(app.initiate_login_uri)
  if (start && issuer) {
    start.searchParams.set('iss', issuer)
    start.searchParams.set('login_hint', email)
    return start.toString()
  }
  return https(app.client_uri)?.toString()
}

/** The invitation, and the length the password it asks for has to have. */
interface ShownInvitation {
  invitation: InvitationDetails
  passwordLength: number
}

const inputClass =
  'mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-hidden focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm'

const AcceptInvitationPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  const token = searchParams.get('token') ?? ''
  const flow = searchParams.get('flow')
  const apiUrl = getConfig().apiUrl

  // Validate the invitation token up front. The subject email is fixed by the
  // invitation and never chosen by the user.
  const {
    data: shown,
    isLoading,
    isError,
  } = useQuery<ShownInvitation>({
    queryKey: ['invitation', token],
    enabled: token !== '',
    retry: false,
    queryFn: async () => {
      const res = await fetch(`${apiUrl}/api/v1/invitations/token/${encodeURIComponent(token)}`)
      if (!res.ok) {
        throw new Error('invitation not acceptable')
      }
      const body = await res.json()
      return {
        invitation: body.invitation as InvitationDetails,
        passwordLength: Number(body.password_min_length) || LEAST_PASSWORD_LENGTH,
      }
    },
  })
  const invitation = shown?.invitation

  // The issuer, for sending the person on to the application the invitation
  // is for (OpenID Connect Core 1.0 section 4).
  const { data: issuer } = useQuery<string>({
    queryKey: ['bootstrap-issuer'],
    enabled: Boolean(invitation?.client?.initiate_login_uri),
    retry: false,
    queryFn: async () => {
      const res = await fetch(`${apiUrl}/.well-known/authway-config`)
      if (!res.ok) throw new Error('bootstrap document unavailable')
      return ((await res.json()) as { issuer: string }).issuer
    },
  })
  const next = invitation?.client ? continueTo(invitation.client, issuer, invitation.email) : undefined

  const acceptSchema = createAcceptSchema(t, shown?.passwordLength ?? LEAST_PASSWORD_LENGTH)
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<AcceptFormData>({ resolver: zodResolver(acceptSchema) })

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
        // The lookup's length can be behind the tenant's by the time the
        // form is sent; the refusal carries the one in force.
        if (body.code === 'password_too_short') {
          throw new Error(t('auth:validation.passwordMin', { length: body.min_length ?? LEAST_PASSWORD_LENGTH }))
        }
        throw new Error(body.detail || 'accept failed')
      }
      return body
    },
    onSuccess: () => {
      setSuccess(true)
      setError(null)
      // Only a sign-in an app started has somewhere to go on to. An invitation
      // opened from its email — the usual way — has none: the sign-in screen
      // means nothing without the app's flow, so the person stays here and is
      // told to sign in from the app.
      if (flow) {
        setTimeout(() => {
          navigate(`/login?flow=${encodeURIComponent(flow)}`)
        }, 3000)
      }
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
        {flow && (
          <button
            type="button"
            onClick={() => navigate(`/login?flow=${encodeURIComponent(flow)}`)}
            className="mt-6 text-sm text-indigo-600 hover:text-indigo-500"
          >
            {t('auth:invitation.backToLogin')}
          </button>
        )}
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
        {!flow && next && invitation?.client ? (
          <a
            href={next}
            className="mt-6 inline-flex justify-center py-2 px-4 rounded-md text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700"
          >
            {t('auth:invitation.success.continueTo', { name: invitation.client.name })}
          </a>
        ) : (
          <p className="mt-1 text-sm text-gray-500">
            {flow ? t('auth:invitation.success.redirect') : nowhereToGo(invitation, t)}
          </p>
        )}
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
            className="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
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
