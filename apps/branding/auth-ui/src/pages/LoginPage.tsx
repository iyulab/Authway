import React, { useState, useEffect, useRef } from 'react'
import { useSearchParams, useNavigate, Link } from 'react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation } from '@tanstack/react-query'
import { useTranslation, Trans } from 'react-i18next'
import { SocialLoginButton, SOCIAL_PROVIDERS } from '../components/SocialLoginButtons'
import LanguageSwitcher from '../components/LanguageSwitcher'
import { followRedirect, loginFlowUrl, nextScreen, startSocialSignIn, submitLoginStep, type FlowStep, type SocialProvider } from '../utils/loginFlow'
import i18n from '../i18n'

// Validation schema - will use i18n messages dynamically
const createLoginSchema = (t: (key: string) => string) => z.object({
  email: z.string().email(t('auth:validation.emailInvalid')),
  // Signing in only needs a password to send; how long it must be is the
  // tenant's rule when it is set, and an account's password may predate it.
  password: z.string().min(1, t('auth:validation.passwordRequired')),
  remember: z.boolean().optional(),
})

type LoginFormData = z.infer<ReturnType<typeof createLoginSchema>>

// Backend error codes that have a localized message on this screen.
const LOGIN_ERROR_KEYS: Record<string, string> = {
  invalid_credentials: 'auth:errors.invalidCredentials',
  invalid_flow: 'auth:errors.flowInvalid',
  sign_in_method_not_allowed: 'auth:errors.signInMethodNotAllowed',
  too_many_requests: 'auth:errors.tooManyRequests',
  flow_expired: 'auth:errors.flowExpired',
}

interface ClientAuthConfig {
  client_id?: string
  /** What to offer, in order: "email" for the password form, then social providers. */
  sign_in_methods?: string[]
}

/**
 * What the application's authorization request asked of this screen. The
 * screen only has a flow id, so the request's own parameters arrive this way.
 */
interface RequestHints {
  /** The way of signing in to start with; one of client.sign_in_methods. */
  sign_in_method?: string
  /** The address the application expects to sign in. */
  login_hint?: string
  /** The languages the application prefers, most preferred first. */
  ui_locales?: string[]
}

interface LoginPageInfo {
  next: 'form'
  flow: string
  client_name: string
  requested_scope: string[]
  client?: ClientAuthConfig
  hints?: RequestHints
}

/** The first of the preferred languages this screen has, by its primary tag ("ko-KR" → "ko"). */
function supportedLanguage(preferred: string[] = []): string | undefined {
  return preferred.map((tag) => tag.split('-')[0].toLowerCase()).find((lng) => i18n.hasResourceBundle(lng, 'common'))
}

const LoginPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const [loginInfo, setLoginInfo] = useState<LoginPageInfo | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Ref to track which flow has been processed to prevent duplicates
  const processedFlowRef = useRef<string | null>(null)

  const flow = searchParams.get('flow')

  const loginSchema = createLoginSchema(t)

  const {
    register,
    handleSubmit,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      remember: false,
    },
  })

  // Fetch what the login flow needs
  useEffect(() => {
    const hasWindowOpener = window.opener !== null && window.opener !== window
    const hasSessionStorage = sessionStorage.getItem('authway_popup_mode') === 'true'

    console.log('[LoginPage] useEffect - checking popup mode:', {
      hasOpener: window.opener !== null,
      isSelfReference: window.opener === window,
      hasSessionStorage,
      isPopupMode: hasWindowOpener || hasSessionStorage,
      detectionMethod: hasWindowOpener ? 'window.opener' : (hasSessionStorage ? 'sessionStorage' : 'none')
    })

    if (!flow) {
      setIsLoading(false)
      return
    }

    fetch(loginFlowUrl(flow))
      .then(res => res.json())
      .then(data => {
        // SSO auto-login or a cleared session — either way no form is needed
        if (data.next === 'redirect' && data.redirect_to) {
          if (data.sso) {
            console.log('[LoginPage] SSO auto-login, redirecting...')
          } else if (data.session_cleared) {
            console.log('[LoginPage] Session cleared, redirecting...')
          }
          followRedirect(data.redirect_to)
          return
        }
        // A reused session whose user owes the tenant's documents.
        const owed = nextScreen(data, flow)
        if (owed) {
          navigate(owed)
          return
        }

        if (data.detail) {
          const key = data.code ? LOGIN_ERROR_KEYS[data.code] : undefined
          setError(key ? t(key) : data.detail)
        } else {
          setLoginInfo(data)
        }
      })
      .catch(err => {
        console.error('Login flow fetch error:', err)
        setError(t('auth:errors.loginInfoFailed'))
      })
      .finally(() => {
        setIsLoading(false)
      })
  }, [flow, t, navigate])

  // What the application asked of this screen. Applied once per flow: the
  // flow is asked about again whenever the language changes, and a user who
  // switches the language, or types another address, must not be put back.
  useEffect(() => {
    const hints = loginInfo?.hints
    if (!hints || !flow) return
    // Strict Mode mounts twice, too; leave for a provider only once per flow.
    if (processedFlowRef.current === flow) return
    processedFlowRef.current = flow

    // The language, unless the address names one itself (?lang=).
    const language = supportedLanguage(hints.ui_locales)
    if (language && !searchParams.get('lang') && language !== i18n.language) {
      void i18n.changeLanguage(language)
    }

    // Whom to expect: the address is filled in, and stays the user's to change.
    if (hints.login_hint) {
      setValue('email', hints.login_hint)
    }

    // Where to start: straight on to the provider the application named.
    const provider = hints.sign_in_method as SocialProvider | undefined
    if (provider && SOCIAL_PROVIDERS.includes(provider)) {
      startSocialSignIn(flow, provider)
    }
  }, [loginInfo, flow, searchParams, setValue])

  // Login mutation
  const loginMutation = useMutation({
    mutationFn: (data: LoginFormData): Promise<FlowStep> =>
      submitLoginStep(flow ?? '', '/password', {
        email: data.email,
        password: data.password,
        remember: data.remember,
      }),
    onSuccess: (data) => {
      const next = nextScreen(data, flow)
      if (next) {
        navigate(next)
      } else if (data.next === 'redirect' && data.redirect_to) {
        followRedirect(data.redirect_to)
      } else if (data.detail) {
        const key = data.code ? LOGIN_ERROR_KEYS[data.code] : undefined
        setError(key ? t(key) : data.detail)
      }
    },
    onError: (error) => {
      console.error('Login error:', error)
      setError(t('auth:errors.loginFailed'))
    },
  })

  const onSubmit = (data: LoginFormData) => {
    setError(null)
    loginMutation.mutate(data)
  }

  // The backend lists what this client can actually use here.
  const signInMethods = loginInfo?.client?.sign_in_methods ?? []
  const isProviderEnabled = (method: string): boolean => signInMethods.includes(method)
  const isEmailLoginEnabled = (): boolean => isProviderEnabled('email')
  const hasSocialProviders = (): boolean => SOCIAL_PROVIDERS.some(isProviderEnabled)

  // No flow: opened directly — a bookmark, a reload after the flow ended, a
  // link from outside an app. The sign-in this screen does belongs to an app's
  // request, so the way on is back through the app.
  if (!flow) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
        <div className="max-w-md text-center">
          <h1 className="text-4xl font-bold text-gray-900">Authway</h1>
          <h2 className="mt-6 text-xl font-semibold text-gray-900">{t('auth:login.noFlow.title')}</h2>
          <p className="mt-2 text-sm text-gray-600">{t('auth:login.noFlow.message')}</p>
        </div>
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600" data-testid="loading-spinner"></div>
      </div>
    )
  }

  if (error && !loginInfo) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="max-w-md w-full space-y-8">
          <div className="text-center">
            <h2 className="mt-6 text-3xl font-extrabold text-gray-900">{t('common:errorOccurred')}</h2>
            <p className="mt-2 text-sm text-red-600">{error}</p>
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
                d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
              />
            </svg>
          </div>
          <h2 className="mt-6 text-center text-3xl font-extrabold text-gray-900">
            {t('auth:login.title')}
          </h2>
          {loginInfo && (
            <div className="mt-2 text-center">
              <p className="text-sm text-gray-600">
                {loginInfo.client_name ? (
                  <Trans
                    i18nKey="auth:login.subtitle"
                    values={{ clientName: loginInfo.client_name }}
                    components={{ strong: <span className="font-medium" /> }}
                  />
                ) : (
                  t('auth:login.subtitleFallback')
                )}
              </p>
              {loginInfo.requested_scope?.length > 0 && (
                <p className="text-xs text-gray-500 mt-1">
                  {t('auth:login.requestedScopes', { scopes: loginInfo.requested_scope.join(', ') })}
                </p>
              )}
            </div>
          )}
        </div>

        {/* noValidate: let zod/react-hook-form own validation so custom i18n
            messages surface. Without it, the browser's native type="email"
            constraint blocks submit before zod runs. */}
        <form className="mt-8 space-y-6" noValidate onSubmit={handleSubmit(onSubmit)}>
          {error && (
            <div className="rounded-md bg-red-50 p-4">
              <div className="text-sm text-red-700">{error}</div>
            </div>
          )}

          {/* Email/Password Login - only show if enabled */}
          {isEmailLoginEnabled() && (
            <>
              <div className="space-y-4">
                <div>
                  <label htmlFor="email" className="block text-sm font-medium text-gray-700">
                    {t('auth:login.emailLabel')}
                  </label>
                  <input
                    id="email"
                    {...register('email')}
                    type="email"
                    autoComplete="email"
                    className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-hidden focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
                    placeholder={t('auth:login.emailPlaceholder')}
                  />
                  {errors.email && (
                    <p className="mt-1 text-sm text-red-600">{errors.email.message}</p>
                  )}
                </div>

                <div>
                  <label htmlFor="password" className="block text-sm font-medium text-gray-700">
                    {t('auth:login.passwordLabel')}
                  </label>
                  <input
                    id="password"
                    {...register('password')}
                    type="password"
                    autoComplete="current-password"
                    className="mt-1 appearance-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-md focus:outline-hidden focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
                    placeholder={t('auth:login.passwordPlaceholder')}
                  />
                  {errors.password && (
                    <p className="mt-1 text-sm text-red-600">{errors.password.message}</p>
                  )}
                </div>

                <div className="flex items-center justify-between">
                  <div className="flex items-center">
                    <input
                      id="remember"
                      {...register('remember')}
                      type="checkbox"
                      className="h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-300 rounded"
                    />
                    <label htmlFor="remember" className="ml-2 block text-sm text-gray-900">
                      {t('auth:login.rememberMe')}
                    </label>
                  </div>
                  <Link
                    to={loginInfo?.client?.client_id ? `/forgot-password?client_id=${encodeURIComponent(loginInfo.client.client_id)}` : '/forgot-password'}
                    className="text-sm text-indigo-600 hover:underline"
                  >
                    {t('auth:login.forgotPassword')}
                  </Link>
                </div>
              </div>

              <div>
                <button
                  type="submit"
                  disabled={isSubmitting || loginMutation.isPending}
                  className="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {isSubmitting || loginMutation.isPending ? (
                    <div className="flex items-center">
                      <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                      {t('auth:login.submitting')}
                    </div>
                  ) : (
                    t('auth:login.submitButton')
                  )}
                </button>
              </div>
            </>
          )}

          {/* Other sign-in methods: an emailed link, social providers */}
          {(hasSocialProviders() || isProviderEnabled('magic_link')) && (
            <div className="mt-6">
              {/* Divider - only show if email login is also enabled */}
              {isEmailLoginEnabled() && (
                <div className="relative mb-6">
                  <div className="absolute inset-0 flex items-center">
                    <div className="w-full border-t border-gray-300" />
                  </div>
                  <div className="relative flex justify-center text-sm">
                    <span className="px-2 bg-gray-50 text-gray-500">{t('common:or')}</span>
                  </div>
                </div>
              )}

              <div className="space-y-3">
                {isProviderEnabled('magic_link') && (
                  <Link
                    to={`/magic-link?flow=${encodeURIComponent(flow)}`}
                    className="relative w-full flex justify-center items-center px-4 py-3 border border-gray-300 rounded-lg text-sm font-medium text-gray-700 bg-white hover:bg-gray-50"
                  >
                    {t('auth:magicLink.useLink', 'Email me a sign-in link')}
                  </Link>
                )}
                {SOCIAL_PROVIDERS.filter(isProviderEnabled).map((provider) => (
                  <SocialLoginButton
                    key={provider}
                    provider={provider}
                    flow={flow}
                    disabled={isSubmitting || loginMutation.isPending}
                  />
                ))}
              </div>
            </div>
          )}

          {/* Onboarding is invitation-only: no public self-registration CTA. */}
        </form>
      </div>
    </div>
  )
}

export default LoginPage
