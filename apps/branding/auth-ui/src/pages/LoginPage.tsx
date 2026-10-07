import React, { useState, useEffect, useRef } from 'react'
import { useSearchParams, useNavigate, Link } from 'react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation } from '@tanstack/react-query'
import { useTranslation, Trans } from 'react-i18next'
import { SocialLoginButton, SOCIAL_PROVIDERS } from '../components/SocialLoginButtons'
import LanguageSwitcher from '../components/LanguageSwitcher'
import { loginFlowUrl, startSocialSignIn, submitLoginStep, type FlowStep } from '../utils/loginFlow'

// Validation schema - will use i18n messages dynamically
const createLoginSchema = (t: (key: string) => string) => z.object({
  email: z.string().email(t('auth:validation.emailInvalid')),
  password: z.string().min(6, t('auth:validation.passwordMin6')),
  remember: z.boolean().optional(),
})

type LoginFormData = z.infer<ReturnType<typeof createLoginSchema>>

// Backend error codes that have a localized message on this screen.
const LOGIN_ERROR_KEYS: Record<string, string> = {
  invalid_credentials: 'auth:errors.invalidCredentials',
  invalid_flow: 'auth:errors.flowInvalid',
  sign_in_method_not_allowed: 'auth:errors.signInMethodNotAllowed',
  flow_expired: 'auth:errors.flowExpired',
}

interface ClientAuthConfig {
  client_id?: string
  enabled_auth_providers?: string[]
  allow_email_signup?: boolean
  allow_email_login?: boolean
  google_oauth_enabled?: boolean
  github_oauth_enabled?: boolean
  microsoft_oauth_enabled?: boolean
  apple_oauth_enabled?: boolean
}

interface LoginPageInfo {
  next: 'form'
  flow: string
  client_name: string
  requested_scope: string[]
  client?: ClientAuthConfig
}

const LoginPage: React.FC = () => {
  const { t } = useTranslation(['auth', 'common'])
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const [loginInfo, setLoginInfo] = useState<LoginPageInfo | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [autoGoogleLogin, setAutoGoogleLogin] = useState(false)

  // Ref to track which flow has been processed to prevent duplicates
  const processedFlowRef = useRef<string | null>(null)

  const flow = searchParams.get('flow')
  const connection = searchParams.get('connection')

  // Helper function to handle popup mode redirect
  const handlePopupRedirect = (redirectUrl: string) => {
    // Check both window.opener AND sessionStorage (survives cross-origin redirects)
    const hasWindowOpener = window.opener !== null && window.opener !== window
    const isSessionStoragePopup = sessionStorage.getItem('authway_popup_mode') === 'true'
    const isPopupMode = hasWindowOpener || isSessionStoragePopup

    // Check if response_mode is form_post (incompatible with popup iframe approach)
    try {
      const url = new URL(redirectUrl)
      const responseMode = url.searchParams.get('response_mode')
      if (responseMode === 'form_post') {
        console.log('[LoginPage] form_post response mode detected - popup approach not supported')
        console.log('[LoginPage] Clearing popup mode and using normal redirect')
        sessionStorage.removeItem('authway_popup_mode')
        return false // Use normal redirect
      }
    } catch {
      // URL parsing failed, continue with popup check
    }

    if (isPopupMode) {
      console.log('[LoginPage] Popup mode detected via', hasWindowOpener ? 'window.opener' : 'sessionStorage')

      // Use direct navigation instead of hidden iframe to avoid COOP issues with social logins (Google, etc.)
      // The callback.html in the popup will handle sending postMessage to the parent
      console.log('[LoginPage] Popup mode - navigating directly to:', redirectUrl)
      window.location.href = redirectUrl
      return true
    }

    return false // Not in popup mode
  }

  const loginSchema = createLoginSchema(t)

  const {
    register,
    handleSubmit,
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
          console.log('[LoginPage] redirect_to URL:', data.redirect_to)

          // Handle popup mode redirect
          console.log('[LoginPage] Calling handlePopupRedirect (SSO)...')
          const popupHandled = handlePopupRedirect(data.redirect_to)
          console.log('[LoginPage] handlePopupRedirect returned:', popupHandled)

          // If not in popup mode or popup handling failed, do normal redirect
          if (!popupHandled) {
            console.log('[LoginPage] Not popup mode, doing normal redirect')
            window.location.href = data.redirect_to
          }
          return
        }

        if (data.error) {
          const key = data.code ? LOGIN_ERROR_KEYS[data.code] : undefined
          setError(key ? t(key) : data.error)
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
  }, [flow, t])

  // Auto-start Google sign-in if connection=google
  useEffect(() => {
    if (!loginInfo || !flow || autoGoogleLogin) return
    if (connection !== 'google') return

    // Strict Mode mounts twice; leave for the provider only once per flow.
    if (processedFlowRef.current === flow) return
    processedFlowRef.current = flow
    setAutoGoogleLogin(true)
    startSocialSignIn(flow, 'google')
  }, [loginInfo, flow, connection, autoGoogleLogin])

  // Login mutation
  const loginMutation = useMutation({
    mutationFn: (data: LoginFormData): Promise<FlowStep> =>
      submitLoginStep(flow ?? '', '/password', {
        email: data.email,
        password: data.password,
        remember: data.remember,
      }),
    onSuccess: (data) => {
      if (data.next === 'mfa' && data.mfa_challenge && flow) {
        const params = new URLSearchParams({ flow, mfa_challenge: data.mfa_challenge })
        navigate(`/mfa/verify?${params}`)
      } else if (data.next === 'redirect' && data.redirect_to) {
        console.log('[LoginPage] Email/password login successful, redirecting...')
        console.log('[LoginPage] redirect_to URL:', data.redirect_to)

        // Handle popup mode redirect
        console.log('[LoginPage] Calling handlePopupRedirect (email/password)...')
        const popupHandled = handlePopupRedirect(data.redirect_to)
        console.log('[LoginPage] handlePopupRedirect returned:', popupHandled)

        // If not in popup mode or popup handling failed, do normal redirect
        if (!popupHandled) {
          console.log('[LoginPage] Not popup mode, doing normal redirect')
          window.location.href = data.redirect_to
        }
      } else if (data.error) {
        const key = data.code ? LOGIN_ERROR_KEYS[data.code] : undefined
        setError(key ? t(key) : data.error)
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

  // Helper functions for checking enabled auth providers
  const isProviderEnabled = (provider: string): boolean => {
    // Default providers if not set: email and google
    const enabledProviders = loginInfo?.client?.enabled_auth_providers || ['email', 'google']
    return enabledProviders.includes(provider)
  }

  const isEmailLoginEnabled = (): boolean => {
    // Default to true if not set
    return loginInfo?.client?.allow_email_login ?? true
  }

  // Check if any social provider is enabled
  const hasSocialProviders = (): boolean => {
    return isProviderEnabled('google') ||
           isProviderEnabled('github') ||
           isProviderEnabled('microsoft') ||
           isProviderEnabled('apple')
  }

  // No flow - direct access (should not happen in normal OAuth flow)
  if (!flow) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="text-center">
          <h1 className="text-4xl font-bold text-gray-900">Authway</h1>
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
          {isProviderEnabled('email') && isEmailLoginEnabled() && (
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

          {/* Social Login Providers */}
          {hasSocialProviders() && (
            <div className="mt-6">
              {/* Divider - only show if email login is also enabled */}
              {isProviderEnabled('email') && isEmailLoginEnabled() && (
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
